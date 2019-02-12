package service

/*
The `NewProtocol` method is used to define the protocol and to register
the handlers that will be called if a certain type of message is received.
The handlers will be treated according to their signature.

The protocol-file defines the actions that the protocol needs to do in each
step. The root-node will call the `Start`-method of the protocol. Each
node will only use the `Handle`-methods, and not call `Start` again.
*/

import (
	"errors"
	"sync"
	"time"

	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
)

type ParentsFn func() []*onet.TreeNode
type ServiceFn func() *Service

func init() {
	network.RegisterMessage(Announce{})
	network.RegisterMessage(Reply{})
}

// VerificationFn is called on every node. Where msg is the message that is
// co-signed and the data is additional data for verification.

// TemplateProtocol holds the state of a given protocol.
//
// For this example, it defines a channel that will receive the number
// of children. Only the root-node will write to the channel.

type LocalityPresProtocol struct {
	*onet.TreeNodeInstance
	GetService  ServiceFn
	MsgReceived bool
	Count       int
	TotalCount  chan int
	index       int
	wg          sync.WaitGroup
}

// Check that *TemplateProtocol implements onet.ProtocolInstance
var _ onet.ProtocolInstance = (*LocalityPresProtocol)(nil)

// NewProtocol initialises the structure for use in one roun
func NewLocalityProtocol(n *onet.TreeNodeInstance, sf ServiceFn) (onet.ProtocolInstance, error) {

	t := &LocalityPresProtocol{
		TreeNodeInstance: n,
		GetService:       sf,
		MsgReceived:      false,
		Count:            1,
		TotalCount:       make(chan int, 1),
		index:            0,
	}

	for _, handler := range []interface{}{t.HandleAnnounce, t.HandleReply} {
		if err := t.RegisterHandler(handler); err != nil {
			return nil, errors.New("couldn't register handler: " + err.Error())
		}
	}
	return t, nil
}

var i int

// Start sends the Announce-message to all children
func (p *LocalityPresProtocol) Start() error {
	i = 1
	p.index = p.GetService().getIndex(p.Tree())

	p.MsgReceived = true
	Announce := StructAnnounce{
		p.TreeNode(),
		Announce{p.index},
	}
	GraphTreeNode := p.GetService().FromBinaryNodeToGraphNode(p.TreeNode(), p.index)

	if len(GraphTreeNode.Children) == 0 {
		p.TotalCount <- p.Count
		p.Done()
	}

	log.LLvl1("index is", p.index)

	for _, n := range GraphTreeNode.Children {

		log.LLvl1(p.GetService().Nodes.GetServerIdentityToName(p.ServerIdentity()), "sends to child", p.GetService().Nodes.GetServerIdentityToName(n.ServerIdentity))

		p.DelayedSend(n, &Announce.Announce)

	}

	return nil

}

// HandleAnnounce is the first message and is used to send an ID that
// is stored in all nodes.
func (p *LocalityPresProtocol) HandleAnnounce(msg StructAnnounce) error {
	if p.MsgReceived == true {

		return nil

	} else {
		p.index = msg.Announce.Message
		Reply := StructReply{
			p.TreeNode(),
			Reply{1},
		}

		p.MsgReceived = true

		if !p.GetService().alive {
			treeNode := p.GetService().FromBinaryNodeToGraphNode(p.Root(), p.index)
			p.DelayedSend(treeNode, &Reply.Reply)
			return nil

		}
		i++
		log.LLvl1("Received and alive:", i)

		GraphTreeNode := p.GetService().FromBinaryNodeToGraphNode(p.TreeNode(), p.index)

		for _, n := range GraphTreeNode.Children {

			p.DelayedSend(n, &msg.Announce)
		}

		treeNode := p.GetService().FromBinaryNodeToGraphNode(p.Root(), p.index)
		p.DelayedSend(treeNode, &Reply.Reply)

		p.wg.Wait()

		p.Done()

		return nil
	}
}

// HandleReply is the message going up the tree and holding a counter
// to verify the number of nodes.
func (p *LocalityPresProtocol) HandleReply(Reply StructReply) error {

	if p.IsRoot() {

		p.Count++

		if p.Count == len(p.Tree().Roster.List) {
			p.TotalCount <- p.Count
			p.wg.Wait()
			p.Done()
		}

		return nil
	}
	TreeParent := p.GetService().FromBinaryNodeToGraphNode(p.TreeNode(), p.index).Parent
	p.DelayedSend(TreeParent, &Reply.Reply)

	return nil

}

func (p *LocalityPresProtocol) DelayedSend(to *onet.TreeNode, msg interface{}) {

	go func() {
		p.wg.Add(1)

		ReceiverLocalityNode := p.GetService().Nodes.GetByName(p.GetService().Nodes.GetServerIdentityToName(to.ServerIdentity))
		SenderLocalityNode := p.GetService().Nodes.GetByName(p.GetService().Nodes.GetServerIdentityToName(p.ServerIdentity()))
		time.Sleep(time.Duration(p.GetService().distances[SenderLocalityNode][ReceiverLocalityNode]) * time.Millisecond)
		p.SendTo(p.GetService().FromGraphNodeToBinaryTreeNode(to, p.index), msg)

		p.wg.Done()
	}()

}
