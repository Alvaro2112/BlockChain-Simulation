// Package bftcosi is a PBFT-like protocol but uses collective signing
// (DEPRECATED).
//
// BFTCoSi is a byzantine-fault-tolerant protocol to sign a message given a
// verification-function. It uses two rounds of signing - the first round
// indicates the willingness of the rounds to sign the message, and the second
// round is only started if at least a 'threshold' number of nodes signed off
// in the first round. Please see
// https://gopkg.in/dedis/cothority.v2/blob/master/bftcosi/README.md for
// details.
//
// DEPRECATED: this package is kept here for historical and research purposes.
// It should not be used in other services as it has been deprecated by the
// byzcoinx package.
//
package service

import (
	//"crypto/sha512"
	"errors"
	"sync"
	"time"

	"go.dedis.ch/cothority/v3/cosi/crypto"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"

	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"runtime"
	"strconv"

	"go.dedis.ch/protobuf"
)

// Make this variable so we can set it to 100ms in the tests.
var defaultTimeout = 100 * time.Second

// VerificationFunction can be passes to each protocol node. It will be called
// (in a go routine) during the (start/handle) challenge prepare phase of the
// protocol. The passed message is the same as sent in the challenge phase.
// The `Data`-part is only to help the VerificationFunction do it's job. In
// the case of the services, this part should be replaced by the correct
// passing of the service-configuration-data, which is not done yet.
type VerificationFunction func(Msg []byte, Data []byte) bool

type ServiceFn func() *Service

// ProtocolBFTCoSi is the main struct for running the protocol
type ProtocolBFTCoSi struct {
	ReceivedMessages map[[32]byte]bool

	Parent *onet.TreeNode
	// the node we are represented-in
	*onet.TreeNodeInstance
	// all data we need during the signature-rounds
	collectStructs

	// The message that will be signed by the BFTCosi
	Msg []byte
	// Data going along the msg to the verification
	Data []byte
	// Timeout is how long to wait while gathering commits.
	Timeout time.Duration
	// last block computed
	lastBlock string
	// refusal to sign for the commit phase or not. This flag is set during the
	// Challenge of the commit phase and will be used during the response of the
	// commit phase to put an exception or to sign.
	signRefusal bool
	// allowedExceptions for how much exception is allowed. If more than allowedExceptions number
	// of conodes refuse to sign, no signature will be created.
	allowedExceptions int
	// our index in the Roster list
	rosterIndex int

	// onet-channels used to communicate the protocol
	// channel for announcement
	announceChanPrepare chan announceChan

	// channel for commit announcement
	announceChanCommit chan announceChanCommit

	// channel for commitment
	commitChan chan commitChan
	// Two channels for the challenge through the 2 rounds: difference is that
	// during the commit round, we need the previous signature of the "prepare"
	// round.
	// channel for challenge during the prepare phase
	challengePrepareChan chan challengePrepareChan
	// channel for challenge during the commit phase
	challengeCommitChan chan challengeCommitChan
	// channel for response
	responseChan chan responseChan

	// Internal communication channels
	// channel used to wait for the verification of the block
	verifyChan chan bool

	// handler-functions
	// onDone is the callback that will be called at the end of the
	// protocol when all nodes have finished. Either at the end of the response
	// phase of the commit round or at the end of a view change.
	onDone func()
	// onSignatureDone is the callback that will be called when a signature has
	// been generated ( at the end of the response phase of the commit round)
	onSignatureDone func(*BFTSignature)
	// VerificationFunction will be called
	// during the (start/handle) challenge prepare phase of the protocol
	VerificationFunction VerificationFunction
	// closing is true if the node is being shut down
	closing bool
	// mutex for closing down properly
	closingMutex sync.Mutex

	loopMutex sync.Mutex

	nrCommitPrep sync.Mutex

	nrClosingAnnounceChan int
	closingAnnounceChan   sync.Mutex

	CommitPrepareFinished chan bool

	dynamicChildren    []*onet.TreeNode
	dynamicChildrenAux []*onet.TreeNode

	GetService  ServiceFn
	MsgReceived bool
	Count       int
	TotalCount  chan int
	index       int
	wg          sync.WaitGroup
}

// collectStructs holds the variables that are used during the protocol to hold
// messages
type collectStructs struct {
	// prepare-round cosi
	prepare *crypto.CoSi
	// commit-round cosi
	commit *crypto.CoSi

	// prepareSignature is the signature generated during the prepare phase
	// This signature is adapted according to the exceptions that occured during
	// the prepare phase.
	prepareSignature []byte

	// mutex for all temporary structures
	tmpMutex sync.Mutex
	// exceptions given during the rounds that is used in the signature
	tempExceptions []Exception
	// temporary buffer of "prepare" commitments
	tempPrepareCommit []kyber.Point
	// temporary buffer of "commit" commitments
	tempCommitCommit []kyber.Point
	// temporary buffer of "prepare" responses
	tempPrepareResponse []kyber.Scalar
	// temporary buffer of the public keys for nodes that responded
	tempPrepareResponsePublics []kyber.Point
	// temporary buffer of "commit" responses
	tempCommitResponse []kyber.Scalar
	//
	nrPrepareCommit      int
	startedCommitPrepare bool
}

// NewBFTCoSiProtocol returns a new bftcosi struct
func NewBFTCoSiProtocol(n *onet.TreeNodeInstance, verify VerificationFunction, sf ServiceFn) (*ProtocolBFTCoSi, error) {

	// initialize the bftcosi node/protocol-instance

	nodes := len(n.Tree().List())
	bft := &ProtocolBFTCoSi{
		ReceivedMessages: make(map[[32]byte]bool),
		TreeNodeInstance: n,
		collectStructs: collectStructs{
			prepare: crypto.NewCosi(n.Suite(), n.Private(), n.Roster().Publics()),
			commit:  crypto.NewCosi(n.Suite(), n.Private(), n.Roster().Publics()),
		},
		verifyChan:           make(chan bool),
		VerificationFunction: verify,
		allowedExceptions:    nodes - (nodes+1)*2/3,
		Msg:                  make([]byte, 0),
		Data:                 make([]byte, 0),
		Timeout:              defaultTimeout,

		// here comes
		dynamicChildren:       make([]*onet.TreeNode, 0),
		dynamicChildrenAux:    make([]*onet.TreeNode, 0),
		GetService:            sf,
		MsgReceived:           false,
		Count:                 1,
		CommitPrepareFinished: make(chan bool, 1),
		TotalCount:            make(chan int, 1),
		index:                 0,
	}

	idx, _ := n.Roster().Search(bft.ServerIdentity().ID)
	bft.rosterIndex = idx

	// Registering channels.
	err := bft.RegisterChannels(&bft.announceChanPrepare, &bft.announceChanCommit,
		&bft.challengePrepareChan, &bft.challengeCommitChan,
		&bft.commitChan, &bft.responseChan)
	if err != nil {
		return nil, err
	}

	bft.index = bft.GetService().getIndex(bft.Tree(), bft.GetService().Nodes.GetServerIdentityToName(bft.Root().ServerIdentity))

	n.OnDoneCallback(bft.nodeDone)

	//log.LLvlf1("node %s starts %p", n.ServerIdentity(), bft)

	return bft, nil
}

// Start will start both rounds "prepare" and "commit" at same time. The
// "commit" round will wait till the end of the "prepare" round during its
// challenge phase.
func (bft *ProtocolBFTCoSi) Start() error {

	log.Lvl2(bft.ProtocolName(), bft.index, "==================================================", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "starts in", bft.ProtocolName())

	//log.Lvlf1("================================================== node starting %p", n.TreeNode())
	log.Lvlf2("================================================== nogrep de starting %p", bft)

	log.Lvl2(bft.ProtocolName(), bft.index, bft.ServerIdentity(), "starting")

	if err := bft.startAnnouncement(RoundPrepare); err != nil {
		return err
	}

	go func() {
		bft.startAnnouncement(RoundCommit)
	}()

	return nil
}

// Dispatch makes sure that the order of the messages is correct by waiting
// on each channel in the correct order.
// By closing the channels for the leafs we can avoid having
// `if !bft.IsLeaf` in the code.
func (bft *ProtocolBFTCoSi) Dispatch() error {

	defer bft.Done()

	/*
		bft.closingMutex.Lock()
		if bft.closing {
			return nil
		}
		// Close unused channels for the leaf nodes, so they won't listen
		// and block on those messages which they will only send but never
		// receive.
		// Unfortunately this is not possible for the announce- and
		// challenge-channels, so the root-node has to send the message
		// to the channel instead of using a simple `SendToChildren`.

		/*


	*/

	//bft.closingMutex.Unlock()

	// Start prepare round

	if err := bft.handleAnnouncement(bft.announceChanPrepare); err != nil {
		return err
	}

	//if bft.hasOneChild() && !bft.IsRoot() {
	//	close(bft.commitChan)
	//}

	if err := bft.handleCommitmentPrepare(bft.commitChan); err != nil {
		return err
	}

	//log.Lvl1(bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "done")

	// wait for parents and dynamic children to be set
	// it's busy waiting, but in a go-routine, so does not block main execution.
	// should probably use a channel instead
	<-bft.CommitPrepareFinished

	//bft.Done()

	/*
		if bft.isAnnounceChanClosed {

		}
	*/

	// Start commit round

	var err error
	//go func() {

	if err = bft.handleAnnouncementCommit(bft.announceChanCommit); err != nil {
		return err
	}

	if !bft.isLocalityTreeLeaf() {
		if err = bft.handleCommitmentCommit(bft.commitChan); err != nil {
			return err
		}
	}

	//}()
	if err != nil {
		return err
	}

	if bft.isLocalityTreeLeaf() {
		close(bft.responseChan)
	}

	// Finish the prepare round
	if err := bft.handleChallengePrepare(<-bft.challengePrepareChan); err != nil {
		return err
	}
	if !bft.isLocalityTreeLeaf() {

		for len(bft.tempCommitCommit) != len(bft.tempPrepareCommit) || len(bft.tempPrepareCommit) == 0 {

		}

		if err := bft.handleResponsePrepare(bft.responseChan); err != nil {
			return err
		}
	}

	//if bft.isAnnounceChanClosed {
	//	bft.Done()
	//}

	// Finish the commit round
	if err := bft.handleChallengeCommit(<-bft.challengeCommitChan); err != nil {
		return err
	}
	if !bft.isLocalityTreeLeaf() {
		if err := bft.handleResponseCommit(bft.responseChan); err != nil {
			return err
		}
	}

	log.Lvl2("!!!!!!!!!!!!!!!!!!", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "finishing the instance", bft.ProtocolName(), "ring", bft.index)

	return nil
}

// Signature will generate the final signature, the output of the BFTCoSi
// protocol.
// The signature contains the commit round signature, with the message.
// If the prepare phase failed, the signature will be nil and the Exceptions
// will contain the exception from the prepare phase. It can be useful to see
// which cosigners refused to sign (each exceptions contains the index of a
// refusing-to-sign signer).
// Expect this function to have an undefined behavior when called from a
// non-root Node.
func (bft *ProtocolBFTCoSi) Signature() *BFTSignature {
	bftSig := &BFTSignature{
		Sig:        bft.commit.Signature(),
		Msg:        bft.Msg,
		Exceptions: nil,
	}
	if bft.signRefusal {
		bftSig.Sig = nil
		bftSig.Exceptions = bft.tempExceptions
	}

	// This is a hack to include exceptions which are the result of offline
	// nodes rather than nodes that refused to sign.
	if bftSig.Exceptions == nil {
		for _, ex := range bft.tempExceptions {
			if ex.Commitment.Equal(bft.Suite().Point().Null()) {
				bftSig.Exceptions = append(bftSig.Exceptions, ex)
			}
		}
	}

	return bftSig
}

// RegisterOnDone registers a callback to call when the bftcosi protocols has
// really finished
func (bft *ProtocolBFTCoSi) RegisterOnDone(fn func()) {
	bft.onDone = fn
}

// RegisterOnSignatureDone register a callback to call when the bftcosi
// protocol reached a signature on the block
func (bft *ProtocolBFTCoSi) RegisterOnSignatureDone(fn func(*BFTSignature)) {
	bft.onSignatureDone = fn
}

// Shutdown closes all channels in case we're done
func (bft *ProtocolBFTCoSi) Shutdown() error {
	defer func() {
		// In case the channels were already closed
		recover()
	}()
	bft.setClosing()
	close(bft.announceChanPrepare)
	close(bft.challengePrepareChan)
	close(bft.challengeCommitChan)
	if !bft.isLocalityTreeLeaf() {
		close(bft.commitChan)
		close(bft.responseChan)
	}
	return nil
}

func HashMsg(msg Announce) [32]byte {
	encodedStruct, err := protobuf.Encode(&msg)
	if err != nil {
		log.Error(err)
	}
	hashedMsg := sha256.Sum256(encodedStruct)

	return hashedMsg
}

func (bft *ProtocolBFTCoSi) isLoopAndSet(msg announceChan) bool {
	//return false

	bft.loopMutex.Lock()
	defer bft.loopMutex.Unlock()

	hashedMsg := HashMsg(msg.Announce)

	if bft.ReceivedMessages[hashedMsg] {
		log.Lvl2(bft.ProtocolName(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), " has seen this msg before")
		return true

	}

	if bft.Parent != nil {
		log.Error("Overwriting an already-set parent!")
	}
	// set the parent and remember the message
	bft.Parent = msg.TreeNode
	bft.ReceivedMessages[hashedMsg] = true

	return false
}

// handleAnnouncement passes the announcement to the right CoSi struct.
func (bft *ProtocolBFTCoSi) handleAnnouncement(c chan announceChan) error {

	ann := <-c

	if err := bft.readAnnouncement(ann); err != nil {
		return err

	}

	// check that message has a sender and is not internal
	if ann.TreeNode != nil {
		//bft.updateChanState(c)
	}

	//!!!
	/*

		var err error
		for {

			log.Lvl2(bft.ProtocolName(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "looping in handle announcement")

			select {
			case msg, ok := <-c:
				if !ok {
					log.Lvl2(bft.ProtocolName(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "announcement Channel closed")
					break
				}

				go func(msg_read announceChan) {
					if err = bft.readAnnouncement(msg_read); err != nil {
						return

					}
				}(msg)

				if err != nil {
					break
				}
			//bft.updateChanState(c)

			}

		}
	*/

	var err error
	go func() {
		for {

			log.Lvl2(bft.ProtocolName(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "looping in handle announcement")

			select {
			case msg, ok := <-c:
				if !ok {
					log.Lvl2(bft.ProtocolName(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "announcement Channel closed")
					return
				}

				if err := bft.readAnnouncement(msg); err != nil {
					return

				}
				//bft.updateChanState(c)

			}

		}

	}()

	if err != nil {
		return err
	}

	return err
}

// handleAnnouncement passes the announcement to the right CoSi struct.
func (bft *ProtocolBFTCoSi) handleAnnouncementCommit(c chan announceChanCommit) error {

	var err error
	go func() {
		//for {

		log.Lvl2(bft.ProtocolName(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "looping in handle announcement commit!!!!!!!!!")

		select {
		case msg, ok := <-c:
			if !ok {
				log.Lvl3(bft.ProtocolName(), bft.index, "Channel commit  closed")
				return
			}

			if err := bft.readAnnouncementCommit(msg); err != nil {
				return

			}
		}

		//}
	}()

	return err
}

func (bft *ProtocolBFTCoSi) isLeafInRound() bool {

	log.Lvl3(bft.ProtocolName(), bft.index, "index is", bft.index)

	GraphTreeNode := bft.GetService().FromBinaryNodeToGraphNode(bft.TreeNode(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.Root().ServerIdentity))
	nrChildren := len(GraphTreeNode.Children)

	if nrChildren == 1 && bft.Parent != nil {
		log.Lvl2(bft.ProtocolName(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "starting commit round <------------------")
		return true
	}

	return false
}

func (bft *ProtocolBFTCoSi) updateChanState(c chan announceChan) {

	bft.closingAnnounceChan.Lock()
	defer bft.closingAnnounceChan.Unlock()
	bft.nrClosingAnnounceChan++

	GraphTreeNode := bft.GetService().FromBinaryNodeToGraphNode(bft.TreeNode(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.Root().ServerIdentity))
	nrNeighbors := len(GraphTreeNode.Children)

	parentFound := false
	for _, n := range GraphTreeNode.Children {

		// see if parent is among children, otherwise we need to send to him as well
		if GraphTreeNode.Parent != nil && n.ServerIdentity.String() == GraphTreeNode.Parent.ServerIdentity.String() {
			parentFound = true
		}

	}

	if GraphTreeNode.Parent != nil && !parentFound {
		nrNeighbors += 1
	}

	if bft.IsRoot() {
		close(bft.announceChanPrepare)
	} else {

		if bft.nrClosingAnnounceChan == nrNeighbors {
			log.Lvl2(bft.ProtocolName(), bft.index, "||||||||||||||| ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "closing channel")

			close(bft.announceChanPrepare)

			//bft.isAnnounceChanClosed = true
		}
	}
}

// handleAnnouncement passes the announcement to the right CoSi struct.
func (bft *ProtocolBFTCoSi) readAnnouncement(msg announceChan) error {

	//log.Lvl1("+++++++++++++++++++++++++++ first time announcement gid is", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), getGID())

	ann := msg.Announce
	if bft.isClosing() {
		log.Lvl2(bft.ProtocolName(), bft.index, "Closing", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()))
		return nil
	}

	bft.index = msg.TreeIndex
	if bft.index == 999 {
		panic(bft.ProtocolName() + " " + strconv.Itoa(bft.index) + "!!!!!!!!!")
	}

	if msg.TreeNode != nil {
		log.Lvl2(bft.ProtocolName(), bft.index, "ANNOUNCEMENT received: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "<------", bft.GetService().Nodes.GetServerIdentityToName(msg.ServerIdentity))
	} else {
		log.Lvl2(bft.ProtocolName(), bft.index, "ANNOUNCEMENT received: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()))
	}

	// check if the node has seen the message before

	//bft.loopMutex.Lock()
	//defer bft.loopMutex.Unlock()
	if bft.isLoopAndSet(msg) {
		log.Lvl2(bft.ProtocolName(), bft.index, "LOOP BREAK send:", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "-------->", bft.GetService().Nodes.GetServerIdentityToName(msg.TreeNode.ServerIdentity))

		// send a loop break reply to sender node and stop
		SenderTreeNode := bft.GetService().FromBinaryNodeToGraphNode(msg.TreeNode, bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.Root().ServerIdentity))
		bft.DelayedSend(SenderTreeNode, &Commitment{TYPE: LoopBreak})

		return nil

	} else {
		if bft.isLeafInRound() {
			close(bft.commitChan)
			return nil
		}

		//err := bft.sendToNeighbors(&ann)
		//if err != nil {
		//	return err
		//}

		//return nil
		//	return bft.startCommitment(RoundPrepare)
		//}
	}

	return bft.sendToNeighborsExcParent(&ann)
}

// handleAnnouncement passes the announcement to the right CoSi struct.
func (bft *ProtocolBFTCoSi) readAnnouncementCommit(msg announceChanCommit) error {

	//log.Lvl1("+++++++++++++++++++++++++++ first time announcement gid is", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), getGID())

	if msg.TreeNode != nil {
		log.Lvl2(bft.ProtocolName(), bft.index, "ANNOUNCEMENT COMMIT received: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "<------", bft.GetService().Nodes.GetServerIdentityToName(msg.ServerIdentity))
	} else {
		log.Lvl2(bft.ProtocolName(), bft.index, "ANNOUNCEMENT COMMIT received: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()))
	}

	ann := msg.AnnounceCommit
	if bft.isClosing() {
		log.Lvl3(bft.ProtocolName(), bft.index, "Closing")
		return nil
	}

	if !bft.isLocalityTreeLeaf() {
		log.Lvl2(bft.ProtocolName(), bft.index, "ANNOUNCEMENT COMMIT send to children: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()))
		return bft.sendToDynamicChildren(&ann)
	}

	return bft.startCommitment(RoundCommit)
}

func (bft *ProtocolBFTCoSi) hasOneChild() bool {
	log.Lvl2(bft.index)
	GraphTreeNode := bft.GetService().FromBinaryNodeToGraphNode(bft.TreeNode(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.Root().ServerIdentity))
	nrChildren := len(GraphTreeNode.Children)

	return nrChildren == 1
}

// handleCommitmentPrepare handles incoming commit messages in the prepare phase
// and then computes the aggregate commit when enough messages arrive.
// The aggregate is sent to the parent if the node is not a root otherwise it
// starts the challenge.
func (bft *ProtocolBFTCoSi) handleCommitmentPrepare(c chan commitChan) error {

	//log.Lvl1(bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "handling commit prepare ======================")

	// TODO change that, this might be the src of deadlock!
	bft.tmpMutex.Lock()
	defer bft.tmpMutex.Unlock() // NOTE potentially locked for the whole timeout

	// wait until we have enough RoundPrepare commitments or timeout
	// should do nothing if `c` is closed
	if err := bft.readCommitChan(c, RoundPrepare); err != nil {
		return err
	}

	bft.CommitPrepareFinished <- true

	// TODO this will not always work for non-star graphs
	/*
		if len(bft.tempPrepareCommit) < len(bft.Children())-bft.allowedExceptions {
			bft.signRefusal = true
			log.Error("not enough prepare commitment messages")
		}
	*/

	commitment := bft.prepare.Commit(bft.Suite().RandomStream(), bft.tempPrepareCommit)
	if bft.IsRoot() {
		log.Lvl2(bft.ProtocolName(), bft.index, "Root starts challenge round prepare")
		return bft.startChallenge(RoundPrepare)
	}

	log.Lvl2(bft.ProtocolName(), bft.index, "COMMIT PREPARE send:", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "--------->", bft.GetService().Nodes.GetServerIdentityToName(bft.Parent.ServerIdentity))

	return bft.DelayedSendToParent(&Commitment{
		TYPE:       RoundPrepare,
		Commitment: commitment,
	})

	/*
		return bft.SendToParent(&Commitment{
			TYPE:       RoundPrepare,
			Commitment: commitment,
		})
	*/
}

// handleCommitmentCommit is similar to handleCommitmentPrepare except it is for
// the commit phase.
func (bft *ProtocolBFTCoSi) handleCommitmentCommit(c chan commitChan) error {

	bft.tmpMutex.Lock()
	defer bft.tmpMutex.Unlock() // NOTE potentially locked for the whole timeout

	// wait until we have enough RoundCommit commitments or timeout
	// should do nothing if `c` is closed
	bft.readCommitChan(c, RoundCommit)

	// TODO this will not always work for non-star graphs
	/*
		if len(bft.tempCommitCommit) < len(bft.Children())-bft.allowedExceptions {
			bft.signRefusal = true
			log.Error("not enough commit commitment messages")
		}
	*/

	//close(bft.commitChan)
	commitment := bft.commit.Commit(bft.Suite().RandomStream(), bft.tempCommitCommit)
	if bft.IsRoot() {
		// do nothing:
		// stop the processing of the round, wait the end of
		// the "prepare" round: calls startChallengeCommit
		return nil
	}
	return bft.DelayedSendToParent(&Commitment{
		TYPE:       RoundCommit,
		Commitment: commitment,
	})

	/*
		return bft.SendToParent(&Commitment{
			TYPE:       RoundCommit,
			Commitment: commitment,
		})
	*/
}

// handleChallengePrepare collects the challenge-messages
func (bft *ProtocolBFTCoSi) handleChallengePrepare(msg challengePrepareChan) error {

	if msg.TreeNode != nil {
		log.Lvl2(bft.ProtocolName(), bft.index, "CHAL PREPARE received: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "<------", bft.GetService().Nodes.GetServerIdentityToName(msg.ServerIdentity))
	} else {
		log.Lvl2(bft.ProtocolName(), bft.index, "CHAL PREPARE received: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "<------")
	}

	if bft.isClosing() {
		return nil
	}
	ch := msg.ChallengePrepare
	if !bft.IsRoot() {
		bft.Msg = ch.Msg
		bft.Data = ch.Data
		// start the verification of the message
		// acknowledge the challenge and send it down
		bft.prepare.Challenge(ch.Challenge)
	}
	go func() {
		bft.verifyChan <- bft.VerificationFunction(bft.Msg, bft.Data)
	}()

	if bft.isLocalityTreeLeaf() {
		log.Lvl2(bft.ProtocolName(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "is locality tree leaf")
		return bft.startResponse(RoundPrepare)
	}
	return bft.sendToDynamicChildren(&ch)
}

func (bft *ProtocolBFTCoSi) isLocalityTreeLeaf() bool {
	return len(bft.dynamicChildren) == 0
}

// handleChallengeCommit verifies the signature and checks if not more than
// the threshold of participants refused to sign
func (bft *ProtocolBFTCoSi) handleChallengeCommit(msg challengeCommitChan) error {

	if msg.TreeNode != nil {
		log.Lvl2(bft.ProtocolName(), bft.index, "CHAL COMMIT received: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "<------", bft.GetService().Nodes.GetServerIdentityToName(msg.ServerIdentity))
	} else {
		log.Lvl2(bft.ProtocolName(), bft.index, "CHAL COMMIT received: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()))
	}

	if bft.isClosing() {
		return nil
	}
	ch := msg.ChallengeCommit
	if !bft.IsRoot() {
		bft.commit.Challenge(ch.Challenge)
	}

	// verify if the signature is correct
	data := sha512.Sum512(ch.Signature.Msg)
	bftPrepareSig := &BFTSignature{
		Sig:        ch.Signature.Sig,
		Msg:        data[:],
		Exceptions: ch.Signature.Exceptions,
	}
	if err := bftPrepareSig.Verify(bft.Suite(), bft.Roster().Publics()); err != nil {
		log.Error(bft.Name(), "Verification of the signature failed:", err)
		bft.signRefusal = true
	}

	// check if we have no more than threshold failed nodes
	if len(ch.Signature.Exceptions) > int(bft.allowedExceptions) {
		log.Errorf("%s: More than threshold (%d/%d) refused to sign - aborting.",
			bft.Roster(), len(ch.Signature.Exceptions), len(bft.Roster().List))
		bft.signRefusal = true
	}

	// store the exceptions for later usage
	bft.tempExceptions = ch.Signature.Exceptions

	if bft.isLocalityTreeLeaf() {
		// bft.responseChan should be closed
		return bft.handleResponseCommit(bft.responseChan)
	}
	return bft.sendToDynamicChildren(&ch)
}

// handleResponsePrepare handles response messages in the prepare phase.
// If the node is not the root, it'll aggregate the response and forward to
// the parent. Otherwise it verifies the response.
func (bft *ProtocolBFTCoSi) handleResponsePrepare(c chan responseChan) error {

	log.Lvl2(bft.ProtocolName(), bft.index, "HANDLE resp prepare ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()))

	bft.tmpMutex.Lock()
	defer bft.tmpMutex.Unlock() // NOTE potentially locked for the whole timeout

	// wait until we have enough RoundPrepare responses or timeout
	// does nothing if channel is closed
	if err := bft.readResponseChan(c, RoundPrepare); err != nil {
		return err
	}

	// TODO this will only work for star-graphs
	// check if we have enough messages
	/*
		if len(bft.tempPrepareResponse) < len(bft.Children())-bft.allowedExceptions {
			log.Error("not enough prepare response messages")
			bft.signRefusal = true
		}
	*/

	// wait for verification
	bzrReturn, ok := bft.waitResponseVerification()
	// append response
	if !ok {
		log.LLvl3(bft.Roster(), "Refused to sign")
	}

	// Return if we're not root
	if !bft.IsRoot() {
		return bft.SendTo(bft.Parent, bzrReturn)
	}

	// Since cosi does not support exceptions yet, we have to remove
	// the responses that are not supposed to be there,i.e. exceptions.
	cosiSig := bft.prepare.Signature()
	correctResponseBuff, err := bzrReturn.Response.MarshalBinary()
	if err != nil {
		return err
	}

	// signature is aggregate commit || aggregate response || mask
	// replace the old aggregate response with the corrected one
	pointLen := bft.Suite().PointLen()
	sigLen := pointLen + bft.Suite().ScalarLen()
	copy(cosiSig[pointLen:sigLen], correctResponseBuff)
	bft.prepareSignature = cosiSig

	// Verify the signature is correct
	data := sha512.Sum512(bft.Msg)
	sig := &BFTSignature{
		Msg:        data[:],
		Sig:        cosiSig,
		Exceptions: bft.tempExceptions,
	}

	log.Lvl3(sig.Msg)

	if err := sig.Verify(bft.Suite(), bft.Roster().Publics()); err != nil {

		log.LLvl3("exceptions", bft.tempExceptions)

		log.LLvl3("Root:", bft.GetService().Nodes.GetServerIdentityToName(bft.Root().ServerIdentity), "Node:", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()),
			"temp prep response len", bft.tempPrepareResponse, len(bft.tempPrepareResponse), "temp prep commit len", bft.tempPrepareCommit, len(bft.tempPrepareCommit))

		log.LLvl3("dynamic children aux")
		for _, n := range bft.dynamicChildrenAux {
			log.LLvl3(bft.GetService().Nodes.GetServerIdentityToName(n.ServerIdentity))
		}

		log.LLvl3("dynamic children ")

		for _, n := range bft.dynamicChildren {
			log.LLvl3(bft.GetService().Nodes.GetServerIdentityToName(n.ServerIdentity))
		}

		/*
			log.LLvl3("temp prepare response publics ")
			for _, n := range bft.tempPrepareResponsePublics {
				log.LLvl3(bft.GetService().Nodes.GetServerIdentityToName(n))
			}
		*/

		log.Error(bft.Name(), "Verification of the signature failed:", err)
		bft.signRefusal = true
		return err
	}
	log.Lvl3(bft.Name(), "Verification of signature successful")

	// Start the challenge of the 'commit'-round

	if err := bft.startChallenge(RoundCommit); err != nil {
		log.Error(bft.Name(), err)
		return err
	}

	return nil
}

func getGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

// handleResponseCommit is similar to `handleResponsePrepare` except it is for
// the commit phase. A key distinction is that the protocol ends at the end of
// this function and final signature is generated if it is called by the root.
func (bft *ProtocolBFTCoSi) handleResponseCommit(c chan responseChan) error {

	bft.tmpMutex.Lock()
	defer bft.tmpMutex.Unlock()

	// wait until we have enough RoundCommit responses or timeout
	// does nothing if channel is closed
	bft.readResponseChan(c, RoundCommit)

	// TODO this will only work for star-graphs
	// check if we have enough messages
	/*
		if len(bft.tempCommitResponse) < len(bft.Children())-bft.allowedExceptions {
			log.Error("not enough commit response messages")
			bft.signRefusal = true
		}
	*/

	r := &Response{
		TYPE:     RoundCommit,
		Response: bft.Suite().Scalar().Zero(),
	}

	var err error
	if bft.isLocalityTreeLeaf() {
		r.Response, err = bft.commit.CreateResponse()
	} else {
		r.Response, err = bft.commit.Response(bft.tempCommitResponse)
	}
	if err != nil {
		return err
	}

	if bft.signRefusal {
		r.Exceptions = append(r.Exceptions, Exception{
			Index:      bft.rosterIndex,
			Commitment: bft.commit.GetCommitment(),
		})
		// don't include our own!
		r.Response.Sub(r.Response, bft.commit.GetResponse())
	}

	// notify we have finished to participate in this signature
	log.Lvl3(bft.Name(), "refusal=", bft.signRefusal)
	// if root we have finished
	if bft.IsRoot() {
		sig := bft.Signature()
		if bft.onSignatureDone != nil {
			bft.onSignatureDone(sig)
		}
		return nil
	}

	err = bft.DelayedSendToParent(r)
	//err = bft.SendToParent(r)
	return err
}

func (bft *ProtocolBFTCoSi) updateCommitPrepAndCompare(neighborsToWaitForCorrection int, com *commitChan) bool {
	bft.nrCommitPrep.Lock()
	defer bft.nrCommitPrep.Unlock()
	bft.nrPrepareCommit++
	if com != nil {
		bft.tempPrepareCommit = append(bft.tempPrepareCommit, com.Commitment.Commitment)
		log.Lvl2(bft.ProtocolName(), bft.index, "!!!!!!!!!!!!!!!ADD: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "<------", bft.GetService().Nodes.GetServerIdentityToName(com.ServerIdentity))

		bft.dynamicChildren = append(bft.dynamicChildren, com.TreeNode)
	}
	GraphTreeNode := bft.GetService().FromBinaryNodeToGraphNode(bft.TreeNode(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.Root().ServerIdentity))
	nrNeighbors := len(GraphTreeNode.Children)

	parentFound := false
	for _, n := range GraphTreeNode.Children {

		// see if parent is among children, otherwise we need to send to him as well
		if GraphTreeNode.Parent != nil && n.ServerIdentity.String() == GraphTreeNode.Parent.ServerIdentity.String() {
			parentFound = true
		}

	}

	if GraphTreeNode.Parent != nil && !parentFound {
		nrNeighbors += 1
	}

	return bft.nrPrepareCommit == nrNeighbors+neighborsToWaitForCorrection
}

// readCommitChan reads until all commit messages are received or a timeout for message type `t`
func (bft *ProtocolBFTCoSi) readCommitChan(c chan commitChan, t RoundType) error {
	timeout := time.After(bft.Timeout)

	//log.Lvl1("+++++++++++++++++++++++++++ read commit chan gid is", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), getGID())

	ready := make(chan bool, 1)
	for {

		log.Lvl2(bft.ProtocolName(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "in read commit chan")

		if bft.isClosing() {
			log.Lvl3("Closing")
			return errors.New("Closing")
		}

		//log.LLvl1("here")

		select {

		case <-ready:
			return nil
		case msg, ok := <-c:
			if !ok {
				log.Lvl3("Channel closed")
				return nil
			}

			go func(msgRead commitChan, r chan bool) {

				comm := msgRead.Commitment
				// store the message and return when we have enough
				switch comm.TYPE {

				case LoopBreak:
					{

						log.Lvl2(bft.ProtocolName(), bft.index, "LOOP BREAK received:", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "<----------", bft.GetService().Nodes.GetServerIdentityToName(msg.ServerIdentity))

						if !bft.IsRoot() {
							if bft.updateCommitPrepAndCompare(-1, nil) {
								log.Lvl2(bft.ProtocolName(), bft.index, "COMMIT PREPARE DONE", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "exiting")
								r <- true
								return
							}
						} else {
							if bft.updateCommitPrepAndCompare(0, nil) {
								log.Lvl2(bft.ProtocolName(), bft.index, "COMMIT PREPARE DONE root exiting")
								r <- true
								return
							}
						}

					}

				case RoundPrepare:

					log.Lvl2(bft.ProtocolName(), bft.index, "COMMIT PREPARE received:", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "<-----------", bft.GetService().Nodes.GetServerIdentityToName(msg.ServerIdentity))

					if !bft.IsRoot() {
						if bft.updateCommitPrepAndCompare(-1, &msgRead) {
							log.Lvl2(bft.ProtocolName(), bft.index, "COMMIT PREPARE DONE", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "exiting")
							r <- true
							return
						}
					} else {
						if bft.updateCommitPrepAndCompare(0, &msgRead) {
							log.Lvl2(bft.ProtocolName(), bft.index, "COMMIT PREPARE DONE root exiting")
							r <- true
							return
						}
					}

				case RoundCommit:

					log.Lvl2(bft.ProtocolName(), bft.index, "COMMIT COMMIT received:", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "<----------", bft.GetService().Nodes.GetServerIdentityToName(msg.ServerIdentity))

					bft.tempCommitCommit = append(bft.tempCommitCommit, comm.Commitment)
					// In case the prepare round had some exceptions, we
					// will not wait for more commits from the commit
					// round. The possibility of having a different set
					// of nodes failing in both cases is inferiour to the
					// speedup in case of one node failing in both rounds.

					if t == RoundCommit && len(bft.tempCommitCommit) == len(bft.tempPrepareCommit) {
						log.Lvl2(bft.ProtocolName(), bft.index, "COMMIT COMMIT DONE root exiting")
						r <- true
						return
					}
				}
			}(msg, ready)

		case <-timeout:
			// in some cases this might be ok because we accept a certain number of faults
			// the caller is responsible for checking if enough messages are received
			panic(bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()) + " timeout while trying to read commit message")
			return nil
		}
	}
}

// should do nothing if the channel is closed
func (bft *ProtocolBFTCoSi) readResponseChan(c chan responseChan, t RoundType) error {
	timeout := time.After(bft.Timeout)
	for {
		if bft.isClosing() {
			return errors.New("Closing")
		}

		select {
		case msg, ok := <-c:
			if !ok {
				log.Lvl3("Channel closed")
				return nil
			}
			from := msg.ServerIdentity.Public
			r := msg.Response

			switch msg.Response.TYPE {
			case RoundPrepare:

				log.Lvl2(bft.ProtocolName(), bft.index, "READ PREPARE RESPONSE chan: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "<------", bft.GetService().Nodes.GetServerIdentityToName(msg.ServerIdentity))

				bft.tempPrepareResponse = append(bft.tempPrepareResponse, r.Response)
				bft.dynamicChildrenAux = append(bft.dynamicChildrenAux, msg.TreeNode)

				//bft.tempExceptions = append(bft.tempExceptions, r.Exceptions...)
				bft.tempPrepareResponsePublics = append(bft.tempPrepareResponsePublics, from)

				if from.String() != msg.TreeNode.ServerIdentity.Public.String() {
					panic("!!!!!!!")
				}

				// There is no need to have more responses than we have
				// commits. We _should_ check here if we get the same
				// responses from the same nodes. But as this is deprecated
				// and replaced by ByzCoinX, we'll leave it like that.

				if t == RoundPrepare && len(bft.tempPrepareResponse) == len(bft.tempPrepareCommit) {
					//log.LLvl3("Root:", bft.GetService().Nodes.GetServerIdentityToName(bft.Root().ServerIdentity), "Node:", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()),
					//	"temp prep response len", len(bft.tempPrepareResponse), "temp prep commit len", len(bft.tempPrepareCommit))

					return nil
				}
			case RoundCommit:

				log.Lvl2(bft.ProtocolName(), bft.index, "READ COMMIT RESPONSE chan: ", bft.GetService().Nodes.GetServerIdentityToName(bft.TreeNodeInstance.ServerIdentity()), "<------", bft.GetService().Nodes.GetServerIdentityToName(msg.ServerIdentity))

				bft.tempCommitResponse = append(bft.tempCommitResponse, r.Response)
				// Same reasoning as in RoundPrepare.
				if t == RoundCommit && len(bft.tempCommitResponse) == len(bft.tempCommitCommit) {
					//log.Lvl1("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!", bft.ProtocolName() + " " + strconv.Itoa(bft.index) + " FINAL")
					log.Lvl2("#######################################", "node",
						bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), bft.ProtocolName()+" "+strconv.Itoa(bft.index)+" FINAL exit")
					return nil
				} else {

				}
			}
		case <-timeout:
			panic(bft.ProtocolName() + " " + strconv.Itoa(bft.index) + " timeout while trying to read response messages")
			return nil
		}
	}
}

// startAnnouncementPrepare create its announcement for the prepare round and
// sends it down the tree.
func (bft *ProtocolBFTCoSi) startAnnouncement(t RoundType) error {

	switch t {
	case RoundPrepare:
		msg := announceChan{Announce: Announce{TYPE: t, Timeout: bft.Timeout, TreeIndex: bft.index}}
		bft.announceChanPrepare <- msg
	case RoundCommit:
		log.Lvl2(bft.ProtocolName(), bft.index, "START ANNOUNCEMENT COMMIT received:", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "!!!!!!!!!!!!!!!!")
		msg := announceChanCommit{AnnounceCommit: AnnounceCommit{TYPE: t, Timeout: bft.Timeout}}
		bft.announceChanCommit <- msg
	}

	return nil
}

// startCommitment sends the first commitment to the parent node
func (bft *ProtocolBFTCoSi) startCommitment(t RoundType) error {

	cm := bft.getCosi(t).CreateCommitment(bft.Suite().RandomStream())
	//return bft.SendToParent(&Commitment{TYPE: t, Commitment: cm})
	return bft.DelayedSendToParent(&Commitment{TYPE: t, Commitment: cm})
}

// startChallenge creates the challenge and sends it to its children
func (bft *ProtocolBFTCoSi) startChallenge(t RoundType) error {
	switch t {
	case RoundPrepare:
		// need to hash the message before so challenge in both phases are not
		// the same
		data := sha512.Sum512(bft.Msg)
		ch, err := bft.prepare.CreateChallenge(data[:])
		if err != nil {
			return err
		}
		bftChal := ChallengePrepare{
			Challenge: ch,
			Msg:       bft.Msg,
			Data:      bft.Data,
		}
		bft.challengePrepareChan <- challengePrepareChan{ChallengePrepare: bftChal}
	case RoundCommit:
		// commit phase
		ch, err := bft.commit.CreateChallenge(bft.Msg)
		if err != nil {
			return err
		}

		// send challenge + signature
		cc := ChallengeCommit{
			Challenge: ch,
			Signature: &BFTSignature{
				Msg:        bft.Msg,
				Sig:        bft.prepareSignature,
				Exceptions: bft.tempExceptions,
			},
		}
		bft.challengeCommitChan <- challengeCommitChan{ChallengeCommit: cc}
	}
	return nil
}

// startResponse dispatches the response to the correct round-type
func (bft *ProtocolBFTCoSi) startResponse(t RoundType) error {
	if !bft.isLocalityTreeLeaf() {
		panic("Only leaf can call startResponse")
	}

	switch t {
	case RoundPrepare:

		return bft.handleResponsePrepare(bft.responseChan)
	case RoundCommit:
		return bft.handleResponseCommit(bft.responseChan)
	}
	return nil
}

// waitResponseVerification waits till the end of the verification and returns
// the BFTCoSiResponse along with the flag:
// true => no exception, the verification is correct
// false => exception, the verification failed
func (bft *ProtocolBFTCoSi) waitResponseVerification() (*Response, bool) {
	log.Lvl2(bft.ProtocolName(), bft.index, "Waiting for response verification:")
	// wait the verification
	verified := <-bft.verifyChan

	// sanity check
	if bft.isLocalityTreeLeaf() && len(bft.tempPrepareResponse) != 0 {
		panic("bft.tempPrepareResponse is not 0 on leaf node")
	}

	resp, err := bft.prepare.Response(bft.tempPrepareResponse)
	if err != nil {
		return nil, false
	}

	if !verified {
		// Add our exception
		bft.tempExceptions = append(bft.tempExceptions, Exception{
			Index:      bft.rosterIndex,
			Commitment: bft.prepare.GetCommitment(),
		})

		panic("Exceptions 2")

		// Don't include our response!
		resp = bft.Suite().Scalar().Set(resp).Sub(resp, bft.prepare.GetResponse())
		log.Lvl2(bft.ProtocolName(), bft.index, "Response verification: failed")
	}

	// if we didn't get all the responses, add them to the exception
	// 1, find children that are not in tempPrepareResponsePublics
	// 2, for the missing ones, find the global index and then add it to the exception

	log.Lvl2(bft.ProtocolName(), bft.index, bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()), "publics len", len(bft.tempPrepareResponsePublics))

	//publicsMap := make(map[kyber.Point]bool)
	publicsMap := make(map[string]bool)
	for _, p := range bft.tempPrepareResponsePublics {
		publicsMap[p.String()] = true
	}
	for _, tn := range bft.dynamicChildren {
		if !publicsMap[tn.ServerIdentity.Public.String()] {
			// We assume the server was also not available for the commitment
			// so no need to subtract the commitment.
			// Conversely, we cannot handle nodes which fail right
			// after making a commitment at the moment.

			log.LLvl2("Root", bft.GetService().Nodes.GetServerIdentityToName(bft.Root().ServerIdentity), "Node:", bft.GetService().Nodes.GetServerIdentityToName(bft.ServerIdentity()),
				"publics len", len(bft.tempPrepareResponsePublics))

			log.LLvl2(bft.GetService().Nodes.GetServerIdentityToName(tn.ServerIdentity))

			log.LLvl2("dynamic children")
			for _, n := range bft.dynamicChildren {
				log.LLvl2(bft.GetService().Nodes.GetServerIdentityToName(n.ServerIdentity), "public", n.ServerIdentity.Public.String())
			}

			for k, n := range publicsMap {
				if n {
					log.LLvl2("public is", k)
				}

				log.LLvl2(tn.ServerIdentity.Public.String() == k)
			}

			panic("Exceptions 1")

			bft.tempExceptions = append(bft.tempExceptions, Exception{
				Index:      tn.RosterIndex,
				Commitment: bft.Suite().Point().Null(),
			})
		}
	}

	r := &Response{
		TYPE:       RoundPrepare,
		Exceptions: bft.tempExceptions,
		Response:   resp,
	}

	log.Lvl2(bft.ProtocolName(), bft.index, "Response verification:", verified)
	return r, verified
}

// nodeDone is either called by the end of EndProtocol or by the end of the
// response phase of the commit round.
func (bft *ProtocolBFTCoSi) nodeDone() bool {
	if bft.onDone != nil {
		// only true for the root
		bft.onDone()
	}
	return true
}

func (bft *ProtocolBFTCoSi) getCosi(t RoundType) *crypto.CoSi {
	if t == RoundPrepare {
		return bft.prepare
	}
	return bft.commit
}

func (bft *ProtocolBFTCoSi) isClosing() bool {
	bft.closingMutex.Lock()
	defer bft.closingMutex.Unlock()
	return bft.closing
}

func (bft *ProtocolBFTCoSi) setClosing() {
	bft.closingMutex.Lock()
	bft.closing = true
	bft.closingMutex.Unlock()
}

func (bft *ProtocolBFTCoSi) sendToDynamicChildren(msg interface{}) error {
	// TODO send to only nodes that did reply

	bft.DelayedSendToChildren(msg)
	return nil
}

func (bft *ProtocolBFTCoSi) sendToNeighbors(msg interface{}) error {
	// TODO send to only nodes that did reply

	bft.DelayedSendToNeighbors(msg)
	return nil
}

func (bft *ProtocolBFTCoSi) sendToNeighborsExcParent(msg interface{}) error {
	// TODO send to only nodes that did reply

	bft.DelayedSendToNeighborsExcParent(msg)
	return nil
}

func (p *ProtocolBFTCoSi) DelayedSend(to *onet.TreeNode, msg interface{}) {

	//	Node := p.GetService().FromGraphNodeToBinaryTreeNode(to, p.index)
	go func() {
		p.wg.Add(1)

		/*
			log.Lvl1(p.GetService())
			log.Lvl1(p.ServerIdentity().String())
			log.Lvl1(to.ServerIdentity)
			log.Lvl1(p.GetService().Nodes)
			log.Lvl1(p.GetService().Nodes.GetServerIdentityToName(to.ServerIdentity))
			log.Lvl1(p.GetService().Nodes.GetByName(p.GetService().Nodes.GetServerIdentityToName(to.ServerIdentity)))
		*/

		//	log.LLvl1(p.GetService().distances[SenderLocalityNode][ReceiverLocalityNode])

		ReceiverLocalityNode := p.GetService().Nodes.GetByName(p.GetService().Nodes.GetServerIdentityToName(to.ServerIdentity))
		SenderLocalityNode := p.GetService().Nodes.GetByName(p.GetService().Nodes.GetServerIdentityToName(p.ServerIdentity()))
		time.Sleep(time.Duration(p.GetService().Distances[SenderLocalityNode][ReceiverLocalityNode]) * time.Millisecond)

		// TODO uncomment line below and comment the 3 lines above for thorough testing with random delays between messages
		//time.Sleep(time.Duration(rand.Intn(2000)) * time.Millisecond)

		err := p.SendTo(p.GetService().FromGraphNodeToBinaryTreeNode(to, p.index, p.GetService().Nodes.GetServerIdentityToName(p.Root().ServerIdentity)), msg)
		if err != nil {
			log.Error(err)
		}

		p.wg.Done()
	}()

}

func (p *ProtocolBFTCoSi) DelayedSendToChildren(msg interface{}) error {

	log.Lvl2(p.ProtocolName(), p.index, "<----------------", p.GetService().Nodes.GetServerIdentityToName(p.TreeNodeInstance.ServerIdentity()), "sending to children")

	for _, n := range p.dynamicChildren {

		log.Lvl2("child", p.GetService().Nodes.GetServerIdentityToName(n.ServerIdentity))
		p.DelayedSend(n, msg)
	}

	return nil

}

func (p *ProtocolBFTCoSi) DelayedSendToNeighbors(msg interface{}) error {
	log.Lvl2(p.ProtocolName(), p.index, "<----------------", p.GetService().Nodes.GetServerIdentityToName(p.TreeNodeInstance.ServerIdentity()), "sending to NEIGHBORD")

	GraphTreeNode := p.GetService().FromBinaryNodeToGraphNode(p.TreeNode(), p.index, p.GetService().Nodes.GetServerIdentityToName(p.Root().ServerIdentity))

	parentFound := false
	for _, n := range GraphTreeNode.Children {

		// see if parent is among children, otherwise we need to send to him as well
		if GraphTreeNode.Parent != nil && n.ServerIdentity.String() == GraphTreeNode.Parent.ServerIdentity.String() {
			parentFound = true
		}

		log.Lvl2(p.ProtocolName(), p.index, p.GetService().Nodes.GetServerIdentityToName(p.ServerIdentity()), "neighbor", p.GetService().Nodes.GetServerIdentityToName(n.ServerIdentity))
		p.DelayedSend(n, msg)
	}

	if GraphTreeNode.Parent != nil && !parentFound {
		log.Lvl2(p.ProtocolName(), p.index, p.GetService().Nodes.GetServerIdentityToName(p.ServerIdentity()), "neighbor", p.GetService().Nodes.GetServerIdentityToName(GraphTreeNode.Parent.ServerIdentity))
		p.DelayedSend(GraphTreeNode.Parent, msg)
	}

	return nil

}

func (p *ProtocolBFTCoSi) DelayedSendToNeighborsExcParent(msg interface{}) error {
	log.Lvl2(p.ProtocolName(), p.index, "<----------------", p.GetService().Nodes.GetServerIdentityToName(p.TreeNodeInstance.ServerIdentity()), "sending to NEIGHBORD")

	GraphTreeNode := p.GetService().FromBinaryNodeToGraphNode(p.TreeNode(), p.index, p.GetService().Nodes.GetServerIdentityToName(p.Root().ServerIdentity))

	for _, n := range GraphTreeNode.Children {

		// see if parent is among children, otherwise we need to send to him as well
		if p.Parent != nil && n.ServerIdentity.String() == p.Parent.ServerIdentity.String() {
			log.Lvl2(p.ProtocolName(), p.index, " except parent", p.GetService().Nodes.GetServerIdentityToName(n.ServerIdentity))
			continue
		}
		log.Lvl2(p.ProtocolName(), p.index, p.GetService().Nodes.GetServerIdentityToName(p.ServerIdentity()), "real neighbor exc parent", p.GetService().Nodes.GetServerIdentityToName(n.ServerIdentity))

		p.DelayedSend(n, msg)
	}

	return nil

}

func (p *ProtocolBFTCoSi) DelayedSendToParent(msg interface{}) error {
	//GraphTreeNode := p.GetService().FromBinaryNodeToGraphNode(p.TreeNode(), p.index)

	if p.Parent == nil {
		log.Error(p.ProtocolName(), p.index, "no parent!!!!!!!!!", p.GetService().Nodes.GetServerIdentityToName(p.ServerIdentity()))
	}

	log.Lvl2(p.ProtocolName(), p.index, "up up up up up up up up up", p.GetService().Nodes.GetServerIdentityToName(p.ServerIdentity()), "sends to parent", p.GetService().Nodes.GetServerIdentityToName(p.Parent.ServerIdentity), msg)

	p.DelayedSend(p.Parent, msg)

	return nil

}
