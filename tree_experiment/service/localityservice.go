// Package service implements a ftcosi service for which clients can connect to
// and then sign messages.
package service

import (
	"errors"
	"os"

	gentree "github.com/dedis/paper_nyle/gentree"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"math"
)

// This file contains all the code to run a CoSi service. It is used to reply to
// client request for signing something using CoSi.
// As a prototype, it just signs and returns. It would be very easy to write an
// updated version that chains all signatures for example.

// ServiceName is the name to refer to the CoSi service
const ServiceName = "LocalityService"

var ID onet.ServiceID

func init() {
	ID, _ = onet.RegisterNewService(ServiceName, newLocalityService)
	network.RegisterMessage(&MulticastRequest{})
	network.RegisterMessage(&BooleanResponse{})
	network.RegisterMessage(&InitRequest{})
}

// Service is the service that handles collective signing operations
type Service struct {
	*onet.ServiceProcessor

	file         *os.File
	Nodes        gentree.LocalityNodes
	LocalityTree *onet.Tree
	Parents      []*onet.TreeNode
	GraphTree    []GraphTree
	BinaryTree   []*onet.Tree
	alive        bool
	distances    map[*gentree.LocalityNode]map[*gentree.LocalityNode]float64
}

// SignatureRequest is what the Cosi service is expected to receive from clients.
type MulticastRequest struct {
	Message []byte
	Roster  *onet.Roster
}

type InitRequest struct {
	Nodes                []*gentree.LocalityNode
	ServerIdentityToName map[*network.ServerIdentity]string
}

//Represents The actual graph that will be linked to the Binary Tree of the Protocol
type GraphTree struct {
	Tree        *onet.Tree
	ListOfNodes []*onet.TreeNode
	Parents     map[*onet.TreeNode][]*onet.TreeNode
}

// SignatureResponse is what the Cosi service will reply to clients.
type BooleanResponse struct {
	Success bool
}

// SignatureRequest treats external request to this service.
func (s *Service) MulticastRequest(req *MulticastRequest) (*BooleanResponse, error) {

	pi, err := s.CreateProtocol(LocalityProtocolName, s.LocalityTree)
	checkErr(err)
	pi.Start()

	return nil, nil
}

func (s *Service) InitRequest(req *InitRequest) (*BooleanResponse, error) {
	log.Lvl1("here", s.ServerIdentity().String())
	s.Setup(req)

	return &BooleanResponse{true}, nil
}

//Returns the Current SERVICE
func (s *Service) GetService() *Service {

	return s
}

//Initializes the Graph And Creates The Binary Tree Based On the Graph
func (s *Service) Setup(req *InitRequest) {

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
	// order nodes in s.Nodes in the order of index
	nodes := make([]*gentree.LocalityNode, len(s.Nodes.All))
	for _, n := range s.Nodes.All {
		nodes[gentree.NodeNameToInt(n.Name)] = n
	}
	s.Nodes.All = nodes

	s.Nodes.ClusterBunchDistances = make(map[*gentree.LocalityNode]map[*gentree.LocalityNode]float64)
	s.Nodes.Links = make(map[*gentree.LocalityNode]map[*gentree.LocalityNode]map[*gentree.LocalityNode]bool)

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

	log.LLvl1("called init service on", s.Nodes.GetServerIdentityToName(s.ServerIdentity()))

}

// TODO
func (s *Service) FTtest(RandomCoordsLevels bool, Levels int, Optimized bool, OmtimisationLevel int, OptType int) ([]GraphTree, []*onet.Tree, map[network.ServerIdentityID]string, map[*gentree.LocalityNode]map[*gentree.LocalityNode]float64) {
	// 2nd argument is if the coordinates should be random or not
	// 3rd argument is if the levels should be random or not
	// 4th arguments is if the levels are random how many of them should there be
	// Imputs are independent of previous computation
	gentree.CreateLocalityGraph(s.Nodes, RandomCoordsLevels, RandomCoordsLevels, Levels)

	// NOTE maybe better if these functions return values, it's uncertain
	// whether it's modifying `myNode` from the caller.
	gentree.GenerateRings(s.Nodes)
	myname := s.Nodes.GetServerIdentityToName(s.ServerIdentity())

	//Careful, the bound can not be less than 2
	if Optimized {

		//Omtimizes Graph
		gentree.OptimizeGraph(s.Nodes, myname, OmtimisationLevel, OptType)

	}

	tree, NodesList, Parents, Distances := gentree.CreateOnetLPTree(s.Nodes, myname, OmtimisationLevel)

	s.distances = Distances

	for i, n := range tree {
		s.GraphTree = append(s.GraphTree, GraphTree{
			n,
			NodesList[i],
			Parents[i],
		})
	}
	for _, n := range s.GraphTree {

		s.BinaryTree = append(s.BinaryTree, s.CreateBinaryTreeFromGraphTree(n))
	}
	return s.GraphTree, s.BinaryTree, s.Nodes.ServerIdentityToName, s.distances

}

// TODO
func (s *Service) MultipleRadiusesTreesTest(RandomCoordsLevels bool, Levels int, Optimized bool, OptimisationLevel int, OptType int) ([]GraphTree, []*onet.Tree, map[network.ServerIdentityID]string, map[*gentree.LocalityNode]map[*gentree.LocalityNode]float64) {

	// 2nd argument is if the coordinates should be random or not
	// 3rd argument is if the levels should be random or not
	// 4th arguments is if the levels are random how many of them should there be
	// Imputs are independent of previous computation
	gentree.CreateLocalityGraph(s.Nodes, RandomCoordsLevels, RandomCoordsLevels, Levels)



	// NOTE maybe better if these functions return values, it's uncertain
	// whether it's modifying `myNode` from the caller.
	gentree.GenerateRings(s.Nodes)
	myname := s.Nodes.GetServerIdentityToName(s.ServerIdentity())

	//Creates the tree and optimizes it
	//3rd arguments is if the tree will be optimised or not
	//4rth arguments is if the graph is optimised what will be the lower bound of the bunch of each bunch
	//Careful, the bound can not be less than 2

	distsOrig := gentree.AproximateDistanceOracle(s.Nodes)

	if Optimized {

		//Omtimizes Graph
		log.LLvl1("optimizing")
		gentree.OptimizeGraph(s.Nodes, myname, OptimisationLevel, OptType)
		log.LLvl1("DONE")

	}

	dist2 := gentree.AproximateDistanceOracle(s.Nodes)

	tree, NodesList, Parents := gentree.CreateOnetRings(s.Nodes, myname, dist2)
	//gentree.CreateOnetLPTree(s.Nodes, myname, OptimisationLevel)


	// test how good the optimization is


	tooLargeDist := 0
	for n1, dists := range dist2 {
		for n2, dist := range dists {
			if dist > 5 * gentree.ComputeDist(n1, n2) {
				tooLargeDist++
				ratio := dist / gentree.ComputeDist(n1, n2)
				log.LLvl1(n1.Name, n2.Name, gentree.ComputeDist(n1, n2), distsOrig[n1][n2], dist, ratio)
			}

		}
	}

	log.LLvl1("--------->", tooLargeDist, "<----------")

	s.distances = dist2
	for i, n := range tree {
		s.GraphTree = append(s.GraphTree, GraphTree{
			n,
			NodesList[i],
			Parents[i],
		})
	}
	for _, n := range s.GraphTree {

		s.BinaryTree = append(s.BinaryTree, s.CreateBinaryTreeFromGraphTree(n))
	}

	// run the ping protocol on each of these trees
	// TODO how are results returned?
	return s.GraphTree, s.BinaryTree, s.Nodes.ServerIdentityToName, s.distances
}

//Runs The Protocol
func (s *Service) Run() bool {

	// TODO call eitherFTtest() or MultipleRadiusesTreesTest()

	proto := make([]*LocalityPresProtocol, 0)
	for _, n := range s.BinaryTree {
		protoo, _ := s.CreateProtocol(LocalityProtocolName, n)
		protocoll := protoo.(*LocalityPresProtocol)
		proto = append(proto, protocoll)
	}

	log.LLvl1("Startint Protocol...")

	log.LLvl1("Starting protocol on:", s.Nodes.GetServerIdentityToName(s.ServerIdentity()), s.Nodes.GetByName(s.Nodes.GetServerIdentityToName(s.ServerIdentity())).Level)
	//When running it will print how many of the alive nodes have received the message
	for _, n := range proto {
		n.Start()

		nodes := <-n.TotalCount
		log.LLvl1("Finished Protocol Successfully whith", nodes, "Nodes.")

	}
	return true
}

//Takes A Node from The Binary Tree And Returns its Corresponding Node Of The Graph
func (s *Service) FromBinaryNodeToGraphNode(Node *onet.TreeNode, index int) *onet.TreeNode {

	var GraphTreeNode *onet.TreeNode

	for _, n := range s.GraphTree[index].ListOfNodes {

		if n.ServerIdentity.String() == Node.ServerIdentity.String() {

			GraphTreeNode = n
			break
		}
	}

	return GraphTreeNode
}

func (s *Service) FromGraphNodeToBinaryTreeNode(Node *onet.TreeNode, index int) *onet.TreeNode {

	var BinaryTreeNode *onet.TreeNode

	for _, n := range s.BinaryTree[index].List() {

		if n.ServerIdentity.String() == Node.ServerIdentity.String() {
			BinaryTreeNode = n
			break
		}
	}
	return BinaryTreeNode
}

func (s *Service) getIndex(tree *onet.Tree) int {

	for i, n := range s.BinaryTree {

		if tree == n {
			return i
		}
	}

	// TODO what is 999
	return 999
}

// NewProtocol is called on all nodes of a Tree (except the root, since it is
// the one starting the protocol) so it's the Service that will be called to
// generate the PI on all others node.
func (s *Service) NewProtocol(tn *onet.TreeNodeInstance, cnf *onet.GenericConfig) (onet.ProtocolInstance, error) {
	log.Lvl3("Cosi Service received New Protocol event")
	if tn.ProtocolName() == LocalityProtocolName {
		return NewLocalityProtocol(tn, s.GetService)
	}

	return nil, errors.New("no such protocol " + tn.ProtocolName())
}

func newLocalityService(c *onet.Context) (onet.Service, error) {

	log.Lvl1("$$$$$$$$$$$$")

	s := &Service{
		ServiceProcessor: onet.NewServiceProcessor(c),
	}
	if err := s.RegisterHandler(s.MulticastRequest); err != nil {
		log.Error("couldn't register message:", err)
		return nil, err
	}

	if err := s.RegisterHandler(s.InitRequest); err != nil {
		log.Error("couldn't register message:", err)
		return nil, err
	}

	s.ProtocolRegister(LocalityProtocolName, func(n *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {

		return NewLocalityProtocol(n, s.GetService)
	})

	return s, nil
}
