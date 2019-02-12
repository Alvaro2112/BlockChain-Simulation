package service

/*
The test-file should at the very least run the protocol for a varying number
of nodes. It is even better practice to test the different methods of the
protocol, as in Test Driven Development.
*/

import (
	"bufio"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/dedis/paper_nyle/gentree"
	"go.dedis.ch/kyber/v3/suites"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
)

var tSuite = suites.MustFind("Ed25519")

func TestMain(m *testing.M) {
	log.MainTest(m, 5)
}

func CreateNode(Name string, x float64, y float64, IP string, level int) *gentree.LocalityNode {
	var myNode gentree.LocalityNode

	myNode.X = x
	myNode.Y = y
	myNode.Name = Name
	myNode.IP = IP
	myNode.Level = level
	myNode.ADist = make([]float64, 0)
	myNode.PDist = make([]string, 0)
	myNode.OptimalCluster = make(map[string]bool)
	myNode.OptimalBunch = make(map[string]bool)
	myNode.Cluster = make(map[string]bool)
	myNode.Bunch = make(map[string]bool)

	return &myNode
}

func ReadFileLineByLine(configFilePath string) func() string {
	wd, err := os.Getwd()
	log.Lvl1(wd)
	f, err := os.Open(configFilePath)

	checkErr(err)
	reader := bufio.NewReader(f)

	var line string
	return func() string {
		if err == io.EOF {
			return ""
		}
		line, err = reader.ReadString('\n')
		checkErr(err)
		line = strings.Split(line, "\n")[0]
		return line
	}
}

func ReadNodesFromFile(random bool, filename string, NumberOfNodes int) ([]*gentree.LocalityNode, map[string]*gentree.LocalityNode, map[string]*gentree.LocalityNode) {
	LinesRead := 0

	Nodes := make([]*gentree.LocalityNode, 0)
	NameToNode := make(map[string]*gentree.LocalityNode)
	IPToNode := make(map[string]*gentree.LocalityNode)

	if random {

		for i := 0; i < NumberOfNodes; i++ {

			myNode := CreateNode("node_"+strconv.Itoa(i), 0, 0, "127.0.0.1", 0)
			Nodes = append(Nodes, myNode)

		}
		return Nodes, nil, nil

	}

	readLine := ReadFileLineByLine(filename)

	for LinesRead < NumberOfNodes {

		line := readLine()

		if line == "" {
			break
		}

		if strings.HasPrefix(line, "#") {
			continue
		}

		LinesRead++
		tokens := strings.Split(line, " ")
		coords := strings.Split(tokens[1], ",")
		name, x_str, y_str, IP, level_str := tokens[0], coords[0], coords[1], tokens[2], tokens[3]

		x, _ := strconv.ParseFloat(x_str, 64)
		y, _ := strconv.ParseFloat(y_str, 64)
		level, err := strconv.Atoi(level_str)

		if err != nil {
			log.LLvl1("Error", err)

		}

		myNode := CreateNode(name, x, y, IP, level)
		Nodes = append(Nodes, myNode)
		NameToNode[myNode.Name] = myNode
		IPToNode[myNode.IP] = myNode

	}
	return Nodes, NameToNode, IPToNode
}

func SetServerIdentities(nodes gentree.LocalityNodes, Identities []*network.ServerIdentity, isLocalTest bool) (map[string]*network.ServerIdentity, map[*network.ServerIdentity]string) {

	NameToServerIdentity := make(map[string]*network.ServerIdentity)
	ServerIdentityToName := make(map[*network.ServerIdentity]string)

	if isLocalTest {

		for i := range nodes.All {
			nodes.All[i].ServerIdentity = Identities[i]
			ServerIdentityToName[Identities[i]] = nodes.All[i].Name
			//	log.LLvl1("associating", Identities[i].String(), "to", nodes.All[i].Name)
		}
	} else {
		for _, ID := range Identities {
			serverIP := ID.Address.Host()
			node := nodes.GetByIP(serverIP)
			node.ServerIdentity = ID
			ServerIdentityToName[ID] = node.Name
			//	log.LLvl1("associating", ID.String(), "to", node.Name)
		}
	}
	return NameToServerIdentity, ServerIdentityToName

}

func selectDeadNodes(numberOfDeadNodes int, numNodes int) map[string]bool {

	numbers := make(map[int]bool)
	for i := 0; i < numberOfDeadNodes; i++ {
		new := rand.Intn(numNodes)
		for {
			if numbers[new] || new == 0 {
				new = rand.Intn(numNodes)
				continue
			}
			break
		}
		numbers[new] = true

	}
	nodes := make(map[string]bool)
	for k, _ := range numbers {

		nodes["node_"+strconv.Itoa(k)] = true

	}

	return nodes
}

const NR_NODES = 20
const RND_NODES = false
const NR_LEVELS = 3
const OPTIMIZED = true
const OPTTYPE = 1
const MIN_BUNCH_SIZE = 12
const NR_DEAD_NODES = 0
const ROOT_NB = 7

// Tests a 2, 5 and 13-node system. It is good practice to test different
// sizes of trees to make sure your protocol is stable .
func TestNode(t *testing.T) {

	local := onet.NewLocalTest(tSuite)
	defer local.CloseAll()

	sid := make([]*network.ServerIdentity, 0)
	servers := make([]onet.Service, 0)

	//Get Services
	for _, sv := range local.GetServices(local.GenServers(NR_NODES), ID) {

		service := sv.(*Service)

		service.alive = true

		servers = append(servers, sv)
		sid = append(sid, service.ServerIdentity())
	}
	//Initializes Services
	//first argument represent  if the nodes will be random or not (Read from a file)
	//the second argument is the file from wich they will be read if the first argument is set to false
	File, _, _ := ReadNodesFromFile(RND_NODES, "nodes_read.txt", NR_NODES)

	for _, sv := range servers {

		service := sv.(*Service)

		service.Nodes.All = File

	}
	//sets the root as the first service
	root := servers[ROOT_NB].(*Service)

	_, sitoname := SetServerIdentities(root.Nodes, sid, true)

	//Sets the number of dead nodes, the first argument is the number of dead nodes
	//the second arguments is the number of nodes
	NodesToBeKilled := selectDeadNodes(NR_DEAD_NODES, NR_NODES)

	for _, s := range servers {
		server := s.(*Service)

		if NodesToBeKilled[sitoname[server.ServerIdentity()]] {
			server.alive = false
		}

	}

	init := InitRequest{
		Nodes:                root.Nodes.All,
		ServerIdentityToName: sitoname,
	}

	//Gets the information of the first service (root)
	root.Setup(&init)

	// 1st argument is if the coordinates and levels should be random or not
	// 2nd arguments isthe number of levels (used only if 1st argument is true)
	// 3rd arguments is if the graph will be optimised or not
	// 4th arguments is what the upperbound of each node's bunch will be  (used only if 3rd argument is true)
	// Inputs are independent of previous computation
	log.LLvl1("here!")

	//gentree.CreateLocalityGraph(root.Nodes, RND_NODES, RND_NODES, NR_LEVELS)


	GraphTree, BinaryTree, serverIdentity, distances := root.MultipleRadiusesTreesTest(RND_NODES, NR_LEVELS, OPTIMIZED, MIN_BUNCH_SIZE, OPTTYPE)

	//sets the information of the root to the other services
	for _, sv := range servers {
		service := sv.(*Service)
		service.GraphTree, service.BinaryTree, service.Nodes.ServerIdentityToName, service.distances = GraphTree, BinaryTree, serverIdentity, distances

	}

	root.Run()

}
