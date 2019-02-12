package service

import (
	"errors"
	"time"

	"bufio"
	"math"
	"os"
	"strconv"
	"sync"

	"github.com/dedis/paper_nyle/gentree"
	"github.com/dedis/paper_nyle/nyle/simplechain"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"go.dedis.ch/onet/v3/simul/monitor"
)

var startCosiMsgID network.MessageTypeID
var replyCosiMsgID network.MessageTypeID

func init() {
	onet.RegisterNewService(Name, newService)

	startCosiMsgID = network.RegisterMessage(&StartBFTCosiReq{})
	replyCosiMsgID = network.RegisterMessage(&ReplyBFTCosiReq{})
}

const multicastTx = "multicastTx"

const multicastVote = "multicastVote"

const nyleBftCosi = "nyleBftCosi"

const OWN_ONLY = false

// Name is the name of the service.
var Name = "Nyle"

// Service holds the state of the service.
type Service struct {
	*onet.ServiceProcessor
	// configures a set of rings that we are a part of
	// rings mapset.Set
	Nodes        gentree.LocalityNodes
	LocalityTree *onet.Tree
	Parents      []*onet.TreeNode
	GraphTree    map[string][]GraphTree
	BinaryTree   map[string][]*onet.Tree
	alive        bool
	Distances    map[*gentree.LocalityNode]map[*gentree.LocalityNode]float64
	CosiWg       map[int]*sync.WaitGroup
	W            *bufio.Writer
	File         *os.File
	metrics      map[string]*monitor.TimeMeasure
	metricsMutex sync.Mutex

	BandwidthRx uint64
	BandwidthTx uint64
	NrMsgRx     uint64
	NrMsgTx     uint64

	NrProtocolsStarted uint64
}

func (s *Service) InitRequest(req *InitRequest) (*InitResponse, error) {
	log.Lvl1("here", s.ServerIdentity().String())
	s.Setup(req)

	return &InitResponse{}, nil
}

const RND_NODES = false
const NR_LEVELS = 3
const OPTIMIZED = false
const OPTTYPE = 1
const MIN_BUNCH_SIZE = 12

func (s *Service) Setup(req *InitRequest) /*([]GraphTree, []*onet.Tree, map[network.ServerIdentityID]string, map[*gentree.LocalityNode]map[*gentree.LocalityNode]float64)*/ {

	s.Nodes.All = req.Nodes
	s.Nodes.ServerIdentityToName = make(map[network.ServerIdentityID]string)
	for k, v := range req.ServerIdentityToName {
		s.Nodes.ServerIdentityToName[k.ID] = v
	}
	for _, myNode := range s.Nodes.All {

		myNode.ADist = make([]float64, 0)
		myNode.PDist = make([]string, 0)
		myNode.OptimalCluster = make(map[string]bool)
		myNode.OptimalBunch = make(map[string]bool)
		myNode.Cluster = make(map[string]bool)
		myNode.Bunch = make(map[string]bool)
		myNode.Rings = make([]string, 0)

	}
	// order nodesin s.Nodes in the order of index
	nodes := make([]*gentree.LocalityNode, len(s.Nodes.All))
	for _, n := range s.Nodes.All {
		nodes[gentree.NodeNameToInt(n.Name)] = n
	}
	s.Nodes.All = nodes
	s.Nodes.ClusterBunchDistances = make(map[*gentree.LocalityNode]map[*gentree.LocalityNode]float64)
	s.Nodes.Links = make(map[*gentree.LocalityNode]map[*gentree.LocalityNode]map[*gentree.LocalityNode]bool)
	s.GraphTree = make(map[string][]GraphTree)
	s.BinaryTree = make(map[string][]*onet.Tree)

	// allocate distances
	for _, node := range s.Nodes.All {
		s.Nodes.ClusterBunchDistances[node] = make(map[*gentree.LocalityNode]float64)
		s.Nodes.Links[node] = make(map[*gentree.LocalityNode]map[*gentree.LocalityNode]bool)
		for _, node2 := range s.Nodes.All {
			s.Nodes.ClusterBunchDistances[node][node2] = math.MaxFloat64
			s.Nodes.Links[node][node2] = make(map[*gentree.LocalityNode]bool)

			if node == node2 {
				s.Nodes.ClusterBunchDistances[node][node2] = 0
			}

			//log.LLvl1("init map", node.Name, node2.Name)
		}
	}

	s.CosiWg = make(map[int]*sync.WaitGroup)
	s.metrics = make(map[string]*monitor.TimeMeasure)

	//ROOT_NAME := s.Nodes.GetServerIdentityToName(s.ServerIdentity())
	//filename := "fullcall-" + ROOT_NAME + ".txt"

	/*
		var file *os.File
		if _, err := os.Stat(filename); !os.IsNotExist(err) {

			//file, err = os.OpenFile(filename, os.O_APPEND | os.O_WRONLY, 0600)
			file, err = os.OpenFile(filename, os.O_WRONLY, 0600)
			if err != nil {
				panic(err)
			}
		} else {
			file, err = os.Create(filename)
		}

		s.File = file
		s.W = bufio.NewWriter(file)
	*/

	/*
		for _, node := range s.Nodes.All {
			log.Lvl1(node.Name)
		}
	*/
	//log.Lvl1(s.Nodes.ServerIdentityToName)

	log.LLvl1("called init service on", s.Nodes.GetServerIdentityToName(s.ServerIdentity()), s.ServerIdentity())

	s.genTrees(RND_NODES, NR_LEVELS, OPTIMIZED, MIN_BUNCH_SIZE, OPTTYPE)

	// create all rings you're part of

	//log.LLvl1(len(s.Nodes.All))

}

//Coumputes A Binary Tree Based On A Graph
func (s *Service) CreateBinaryTreeFromGraphTree(GraphTree GraphTree) *onet.Tree {

	BinaryTreeRoster := GraphTree.Tree.Roster
	Tree := BinaryTreeRoster.GenerateBinaryTree()

	return Tree
}

//Takes A Node from The Binary Tree And Returns its Corresponding Node Of The Graph
func (s *Service) FromBinaryNodeToGraphNode(Node *onet.TreeNode, index int, rootName string) *onet.TreeNode {

	var GraphTreeNode *onet.TreeNode

	for _, n := range s.GraphTree[rootName][index].ListOfNodes {

		if n.ServerIdentity.String() == Node.ServerIdentity.String() {

			GraphTreeNode = n
			break
		}
	}

	return GraphTreeNode
}

func (s *Service) FromGraphNodeToBinaryTreeNode(Node *onet.TreeNode, index int, rootName string) *onet.TreeNode {

	var BinaryTreeNode *onet.TreeNode

	for _, n := range s.BinaryTree[rootName][index].List() {

		if n.ServerIdentity.String() == Node.ServerIdentity.String() {
			BinaryTreeNode = n
			break
		}
	}
	return BinaryTreeNode
}

func (s *Service) getIndex(tree *onet.Tree, rootName string) int {

	for i, n := range s.BinaryTree[rootName] {

		if tree == n {
			return i
		}
	}
	return 999
}

//Returns the Current SERVICE
func (s *Service) GetService() *Service {

	return s
}

/*
func (s *Service)SendGraphTrees() {

	log.Lvl1(s.Nodes.GetServerIdentityToName(s.ServerIdentity()), "sends graph trees")

	ringName := s.Nodes.GetServerIdentityToName(s.ServerIdentity())
	for i, gt := range s.GraphTree {
		for _, participantNode := range gt.ListOfNodes {
			if participantNode.ServerIdentity.String() != s.ServerIdentity().String() {

				log.Lvl1("sending to", s.Nodes.GetServerIdentityToName(participantNode.ServerIdentity))

				sentRingName := ringName + " _" + strconv.Itoa(i)
				req := StoreGraphReq{Ring: sentRingName, Children: participantNode.Children}
				err := s.SendRaw(participantNode.ServerIdentity, &req)

				log.Lvl1("after send")

				if err != nil {
					log.Lvl1(s.ServerIdentity(), "could not send graph to", participantNode.ServerIdentity, err)
				}
			}
		}
	}
}
*/

func (s *Service) genTrees(RandomCoordsLevels bool, Levels int, Optimized bool, OptimisationLevel int, OptType int) {

	// genTrees placeholder code, ideally we'll generate trees from small to large

	gentree.CreateLocalityGraph(s.Nodes, RandomCoordsLevels, RandomCoordsLevels, Levels)

	// we are rooting trees here
	myname := s.Nodes.GetServerIdentityToName(s.ServerIdentity())

	if Optimized {
		gentree.OptimizeGraph(s.Nodes, myname, OptimisationLevel, OptType)
	}

	//tree, NodesList, Parents, Distances := gentree.CreateOnetLPTree(s.Nodes, myname, OptimisationLevel)

	// route request to the roots of all rings i'm part of, using the distance oracles thingie

	// then everyone runs consensus in their trees

	// if you're not a highest level node, then find the first highest level node, build

	dist2 := gentree.AproximateDistanceOracle(s.Nodes)

	// TODO we generate trees for all nodes
	for _, crtRoot := range s.Nodes.All {
		crtRootName := crtRoot.Name

		trees, radiuses := gentree.CreateOnetRings(s.Nodes, crtRootName, dist2, Levels)

		log.Lvl1("+++++++ ROOt", crtRootName, "++++++++")

		// update distances only if i'm the root
		if crtRootName == myname {
			s.Distances = dist2
		}

		for i, treeArray := range trees {
			if len(treeArray) > 0 {
				log.LLvl1("Trees with radius", radiuses[i])
				for _, t := range treeArray {
					if t != nil {
						for i, node := range t.List() {
							if i == 0 {
								log.LLvl1("rootName", s.Nodes.GetServerIdentityToName(node.ServerIdentity))
							} else {
								log.LLvl1(s.Nodes.GetServerIdentityToName(node.Parent.ServerIdentity), "->", s.Nodes.GetServerIdentityToName(node.ServerIdentity))
							}
						}
						//log.LLvl1("rootName", s.Nodes.GetServerIdentityToName(t.Root.ServerIdentity), "creates binary with roster", t.Roster.List)
						//log.LLvl1(t.Dump())
					}
					//s.BinaryTree[rootName] = append(s.BinaryTree[rootName], s.CreateBinaryTreeFromGraphTree(n))
				}
			}
		}

		log.Lvl1("done")

		/*
			for i, n := range trees {


				s.GraphTree[crtRootName] = append(s.GraphTree[crtRootName], GraphTree{
					n,
					NodesList[i],
					Parents[i],
				})
			}
		*/

	}

	//panic("")
	log.Lvl1("done")

	// send the graph trees to all nodes part of them

	//s.SendGraphTrees()

	// print the trees

	log.Lvl1("done")

}

func getBunchMetric(src string, bunchNode string) string {
	return "bunch|" + src + "|" + bunchNode + "|"
}

func getOwnMetric(src string) string {
	return "own|" + src + "|"
}

func getOwnRxMetric(src string) string {
	return "ownRx|" + src + "|"
}

func getOwnTxMetric(src string) string {
	return "ownTx|" + src + "|"
}

func getOwnNrRxMetric(src string) string {
	return "ownRxNr|" + src + "|"
}

func getOwnNrTxMetric(src string) string {
	return "ownTxNr|" + src + "|"
}

func (s *Service) sendReqToBunch(reqId int) {
	myName := s.Nodes.GetServerIdentityToName(s.ServerIdentity())

	for bunchNodeName, exists := range s.Nodes.GetByName(myName).Bunch {
		if exists {
			req := StartBFTCosiReq{SenderName: myName, ReqId: reqId}

			go func(nodeName string) {
				bunchNode := s.Nodes.GetByName(nodeName)

				metric := getBunchMetric(s.Nodes.GetServerIdentityToName(s.ServerIdentity()), nodeName)
				s.metricsMutex.Lock()
				s.metrics[metric] = monitor.NewTimeMeasure(metric)
				s.metricsMutex.Unlock()

				err := s.SendRaw(bunchNode.ServerIdentity, &req)
				if err != nil {
					panic(err)
				}
				log.LLvl1("<---------------BBBBBBBBB Request", myName, "send cosi req to bunch to node", s.Nodes.GetServerIdentityToName(bunchNode.ServerIdentity))
			}(bunchNodeName)
		}
	}
}

func (s *Service) recvProtoRespFromBunchNode(env *network.Envelope) error {

	// call done on cosiwg
	req, ok := env.Msg.(*ReplyBFTCosiReq)

	if !ok {
		log.Error(s.ServerIdentity(), "failed to cast to ReplyBFTCosiReq")
		return errors.New("failed to cast to ReplyBFTCosiReq")
	}

	senderNodeName := s.Nodes.GetServerIdentityToName(env.ServerIdentity)
	receiverNodeName := s.Nodes.GetServerIdentityToName(s.ServerIdentity())
	log.LLvl2("---------------BBBBBBB Response", receiverNodeName, "received REPLY from", senderNodeName, "on request identity", req)

	metric := getBunchMetric(receiverNodeName, senderNodeName)
	s.metricsMutex.Lock()
	s.metrics[metric].Record()
	s.metricsMutex.Unlock()

	log.LLvl1(s.Nodes.GetServerIdentityToName(s.ServerIdentity()), "decreases count once for bunch from node", req.ReqId)
	s.CosiWg[req.ReqId].Done()

	return nil
}

func (s *Service) recvCosiReqFromNode(env *network.Envelope) error {
	// Parse message
	req, ok := env.Msg.(*StartBFTCosiReq)

	if !ok {
		log.Error(s.ServerIdentity(), "failed to cast to StartBFTCosiReq")
		return errors.New("failed to cast to StartBFTCosiReq")
	}

	senderNodeName := s.Nodes.GetServerIdentityToName(env.ServerIdentity)
	receiverNodeName := s.Nodes.GetServerIdentityToName(s.ServerIdentity())
	reqId := req.ReqId

	log.Lvl2("--------------->BBBBBB Incoming", receiverNodeName, "received REQ from", senderNodeName, req)

	ROOT_NAME := s.Nodes.GetServerIdentityToName(s.ServerIdentity())

	go s.RunCosiOnOwnTreesWithParticipant(ROOT_NAME, senderNodeName, reqId)
	return nil
}

func (s *Service) runCosiOnOwnTreesWithRestriction(rootName string, minTreeIndex int, reqId int) error {

	//filename := "../fullcall-" + rootName + "-from" + ".txt"

	/*
		var file *os.File
		if _, err := os.Stat(filename); !os.IsNotExist(err) {

			//file, err = os.OpenFile(filename, os.O_APPEND | os.O_WRONLY, 0600)
			file, err = os.OpenFile(filename, os.O_WRONLY, 0600)
			if err != nil {
				panic(err)
			}
		} else {
			file, err = os.Create(filename)
		}
		defer file.Close()
		w := bufio.NewWriter(file)
	*/

	//log.Lvl1("done", len(s.BinaryTree))

	//log.Lvl1("called on", s.Nodes.GetServerIdentityToName(s.ServerIdentity()))

	trees := s.BinaryTree[rootName]
	doneChan := make(chan int, len(trees)-minTreeIndex)

	//start := time.Now()

	//w := s.W

	//w.WriteString("---------------------   Root " + s.Nodes.GetServerIdentityToName(s.ServerIdentity()) + " ---------------------\n")

	for i, t := range trees {
		if i < minTreeIndex {
			continue
		}

		//log.Lvl2("-------------------------", t.Root.ServerIdentity.String(), s.GraphTree[rootName][i].Tree.Root.ServerIdentity.String())

		namesOfRingNodes := ""
		for _, node := range s.GraphTree[rootName][i].ListOfNodes {
			namesOfRingNodes += s.Nodes.GetServerIdentityToName(node.ServerIdentity) + " "
		}

		//w.WriteString("Ring " + strconv.Itoa(i) + " number of nodes " + strconv.Itoa(len(s.GraphTree[rootName][i].ListOfNodes)) + " " + namesOfRingNodes + "\n")

		//log.Lvl2(" number of nodes", len(s.GraphTree[rootName][i].ListOfNodes))

		//if len(s.GraphTree[i].ListOfNodes) == 1 || i != TREE_ID {
		if len(s.GraphTree[rootName][i].ListOfNodes) == 1 {
			//if i != TREE_ID {
			doneChan <- i
			continue
		}

		go func(crtTree *onet.Tree, name string, req int, ringNr int) {

			c, err := s.startBftCosi(crtTree, name, req, ringNr)

			<-c
			doneChan <- ringNr

			if err != nil {
				panic("proto returned an error " + err.Error())
			}

		}(t, rootName, reqId, i)

		//log.Lvl2("&&&&&&&", t.Root.ServerIdentity, s.ServerIdentity())
	}

	for i := range trees {
		if i < minTreeIndex {
			continue
		}

		select {
		case ringNr := <-doneChan:

			protoName := nyleBftCosi + "-" + rootName + "-" + strconv.Itoa(reqId)
			//radiusStr := strconv.FormatFloat(20 * math.Pow(math.Sqrt(2), float64(i)), 'f', 6, 64)
			log.Lvl2("----------- END", s.Nodes.GetServerIdentityToName(s.ServerIdentity()), "Protocol", protoName, "DONE! on ring", ringNr, "with radius", 20*math.Pow(math.Sqrt(2), float64(i)))
		//dur := time.Now().Sub(start)
		//log.Lvl1(dur)
		//w.WriteString("Protocol " + protoName + " DONE! on ring " + strconv.Itoa(i) + " with radius " + radiusStr + " in time " + dur.String() + "\n")

		case <-time.After(10 * time.Minute):
			return errors.New("protocols did not finish in time")
		}

		//w.Flush()
	}

	//w.Flush()

	log.Lvl2("done", s.Nodes.GetServerIdentityToName(s.ServerIdentity()))
	return nil
}

func (s *Service) RunCosiOnOwnTrees(rootName string, reqId int) error {
	//filename := "../fullcall-" + rootName + ".txt"

	log.Lvl2("-------------------- RRRRRRRRRRR own trees", s.Nodes.GetServerIdentityToName(s.ServerIdentity()))

	return s.runCosiOnOwnTreesWithRestriction(rootName, 0, reqId)
}

func (s *Service) RunCosiOnOwnTreesWithParticipant(rootName string, participantName string, reqId int) error {

	trees := s.BinaryTree[rootName]
	for i := range trees {

		for _, node := range s.GraphTree[rootName][i].ListOfNodes {
			if participantName == s.Nodes.GetServerIdentityToName(node.ServerIdentity) {
				// found participant, return i

				//filename := "../fullcall-" + rootName + "-from-" + participantName + ".txt"

				log.Lvl2("---------------RRRRRRRRRRRRun ", s.Nodes.GetServerIdentityToName(s.ServerIdentity()), "starts bunch call from", participantName, "with req id", reqId)

				err := s.runCosiOnOwnTreesWithRestriction(rootName, i, reqId)
				if err != nil {
					panic(err.Error())
				}

				log.Lvl2("<---------------BBBBBBB Response", "SENDING", s.Nodes.GetServerIdentityToName(s.ServerIdentity()))

				req := ReplyBFTCosiReq{ReqId: reqId}
				err = s.SendRaw(s.Nodes.GetByName(participantName).ServerIdentity, &req)
				if err != nil {
					panic(err.Error())
				}

				//found = true
				//break
				return nil
			}
		}
	}
	log.Error("Participant", participantName, "not found by", rootName)
	return nil
}

// BFTCoSiSimul starts the bftcosi simulation.
func (s *Service) BFTCoSiSimul(req *BFTCoSiSimul) (*BFTCoSiSimulResp, error) {

	ROOT_NAME := s.Nodes.GetServerIdentityToName(s.ServerIdentity())

	log.Lvl1("---------------$$$$$$$$$ main fct called on", s.Nodes.GetServerIdentityToName(s.ServerIdentity()), "with", req.ReqId, "bunch", s.Nodes.GetByName(ROOT_NAME).Bunch)

	defer s.File.Close()

	//defer s.W.

	/*
		filename := "../fullcall-" + ROOT_NAME + ".txt"

		var file *os.File
		if _, err := os.Stat(filename); !os.IsNotExist(err) {

			file, err = os.OpenFile(filename, os.O_APPEND | os.O_WRONLY, 0600)
			if err != nil {
				panic(err)
			}
		} else {
			file, err = os.Create(filename)
		}
		defer file.Close()
		w := bufio.NewWriter(file)
	*/

	// TODO send to bunch nodes so that they start it on their own trees

	start := time.Now()

	//log.Lvl1("done", len(s.BinaryTree))

	var err error

	reqId := req.ReqId

	// wait group
	var wg sync.WaitGroup
	counter := 0

	s.CosiWg[reqId] = &wg
	s.CosiWg[reqId].Add(1)
	counter++

	if !OWN_ONLY {
		for _, exists := range s.Nodes.GetByName(ROOT_NAME).Bunch {
			if exists {
				s.CosiWg[reqId].Add(1)
				counter++
			}
		}
	}

	log.LLvl1(s.Nodes.GetServerIdentityToName(s.ServerIdentity()), "counter", counter)

	go func(reqId int) {
		metric := getOwnMetric(s.Nodes.GetServerIdentityToName(s.ServerIdentity()))
		s.metricsMutex.Lock()
		s.metrics[metric] = monitor.NewTimeMeasure(metric)
		s.metricsMutex.Unlock()

		err = s.RunCosiOnOwnTrees(ROOT_NAME, reqId)
		log.LLvl1(s.Nodes.GetServerIdentityToName(s.ServerIdentity()), "decreases count once for own node with reqid", reqId)
		s.metricsMutex.Lock()
		s.metrics[metric].Record()
		s.metricsMutex.Unlock()
		s.CosiWg[reqId].Done()
	}(reqId)

	if !OWN_ONLY {
		go s.sendReqToBunch(reqId)
	}

	s.CosiWg[reqId].Wait()

	metricRx := getOwnRxMetric(s.Nodes.GetServerIdentityToName(s.ServerIdentity()))
	metricRxNr := getOwnNrRxMetric(s.Nodes.GetServerIdentityToName(s.ServerIdentity()))
	metricTx := getOwnTxMetric(s.Nodes.GetServerIdentityToName(s.ServerIdentity()))
	metricTxNr := getOwnNrTxMetric(s.Nodes.GetServerIdentityToName(s.ServerIdentity()))
	monitor.RecordSingleMeasure(metricRx, float64(s.BandwidthRx))
	monitor.RecordSingleMeasure(metricRxNr, float64(s.NrMsgRx))
	monitor.RecordSingleMeasure(metricTx, float64(s.BandwidthTx))
	monitor.RecordSingleMeasure(metricTxNr, float64(s.NrMsgTx))

	log.LLvl1("!!!!!!!!!!!!!!", s.Nodes.GetServerIdentityToName(s.ServerIdentity()), "finished the CALL", s.NrProtocolsStarted)

	//w.Flush()

	//time.Sleep(180 * time.Second)

	dur := time.Now().Sub(start)
	return &BFTCoSiSimulResp{
		Duration: dur,
	}, err
}

func (s *Service) startBftCosi(tree *onet.Tree, rootName string, reqId int, ringNr int) (chan bool, error) {

	c := make(chan bool, 1)

	//protoName := nyleBftCosi + "-" + rootName + "-" + strconv.Itoa(reqId)
	//protoName := nyleBftCosi + "-" + rootName

	protoName := nyleBftCosi

	// TODO log.Lvl1("------------- START", s.Nodes.GetServerIdentityToName(s.ServerIdentity()), "would start the protocol", protoName, "on ring", ringNr)

	//pi, err := s.CreateProtocol(nyleBftCosi, tree)
	pi, err := s.CreateProtocol(protoName, tree)
	if err != nil {
		return c, err
	}
	root := pi.(*ProtocolBFTCoSi)
	root.Msg = []byte("test")
	root.Timeout = 60 * time.Second
	root.RegisterOnDone(func() {
		c <- true
		s.metricsMutex.Lock()
		s.BandwidthRx += root.Rx()
		s.BandwidthTx += root.Tx()
		s.NrMsgRx += 0 // root.NrRx()
		s.NrMsgTx += 0 // root.NrTx()
		s.metricsMutex.Unlock()

	})

	s.NrProtocolsStarted++
	if err := root.Start(); err != nil {
		return c, err
	}

	//time.Sleep(time.Duration(rand.Intn(20000)) * time.Millisecond)

	//c <- true
	return c, nil
}

// AddTx checks and forwards the client transaction to the Nyle network.
func (s *Service) AddTx(tx *AddTxReq) (*AddTxResp, error) {
	// TODO check that the spending ring is correct
	// TODO check that sender is actually allowed to spend this tx (need to look at the source ring)
	// TODO check that the signature is correct
	// TODO initialise the protocol
	if err := s.startMulticastTx(&tx.Tx); err != nil {
		return nil, err
	}
	return &AddTxResp{}, nil
}

func (s *Service) startMulticastTx(tx *simplechain.Tx) error {
	// TODO generate/get the roster
	// TODO s.CreateProtocol()
	return nil
}

// newTxCallback is called whenever we are notified about a new transaction.
func (s *Service) newTxCallback(sid *network.ServerIdentity) {
	// TODO verify the tx and then add it to database etc.
	// TODO we need to give some information back to the protocol to say
	// whether we continue to propagate the message and what kind of
	// message, we we have to return something but now sure exactly what it
	// should be yet.
}

// newVoteCallback is called whenever we are notified about a new vote.
func (s *Service) newVoteCallback(sid *network.ServerIdentity) {
}

func dummyVerify(m []byte, d []byte) bool {
	return true
}

func newService(ctx *onet.Context) (onet.Service, error) {

	log.LLvl1("new service")

	s := &Service{
		ServiceProcessor: onet.NewServiceProcessor(ctx),
	}
	log.ErrFatal(s.RegisterHandlers(s.AddTx, s.BFTCoSiSimul, s.InitRequest))
	if _, err := s.ProtocolRegister(multicastTx, NewMulticastProtocol(s.newTxCallback)); err != nil {
		return nil, err
	}

	s.RegisterProcessorFunc(startCosiMsgID, s.recvCosiReqFromNode)
	s.RegisterProcessorFunc(replyCosiMsgID, s.recvProtoRespFromBunchNode)

	if _, err := s.ProtocolRegister(multicastVote, NewMulticastProtocol(s.newVoteCallback)); err != nil {
		return nil, err
	}

	/*
		for i := 0; i < 20; i++ {
			for j := 0; j < 20; j++ {
				protoName := nyleBftCosi + "-node_" + strconv.Itoa(i) + "-" + strconv.Itoa(j)
				_, err := s.ProtocolRegister(protoName, func(n *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
					return NewBFTCoSiProtocol(n, dummyVerify, s.GetService)
				})
				if err != nil {
					return nil, err
				}
			}
		}
		*


		/*
		for i := 0; i < 20; i++ {
			protoName := nyleBftCosi + "-node_" + strconv.Itoa(i)
			_, err := s.ProtocolRegister(protoName, func(n *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
				return NewBFTCoSiProtocol(n, dummyVerify, s.GetService)
			})
			if err != nil {
				return nil, err
			}
		}
	*/

	_, err := s.ProtocolRegister(nyleBftCosi, func(n *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
		return NewBFTCoSiProtocol(n, dummyVerify, s.GetService)
	})
	if err != nil {
		return nil, err
	}
	//for i := range s.Nodes

	return s, nil
}
