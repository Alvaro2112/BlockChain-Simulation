package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dedis/paper_nyle/gentree"
	"github.com/dedis/paper_nyle/nyle"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
	"go.dedis.ch/onet/v3/app"
	"go.dedis.ch/onet/v3/simul/monitor"
	nyleSvc "github.com/dedis/paper_nyle/nyle/cosi_service"
	//"time"
	//"sync"
	"sync"
	"github.com/pkg/profile"
)

const NODEPATHREMOTE = "nodes_local_20.txt"
const NODEPATHLOCAL = "../nodes_local_20.txt"
const simulationName = "nyleSimul"

func init() {
	onet.SimulationRegister(simulationName, newNyleSimulation)
}

// NyleSimulation is the s
type NyleSimulation struct {
	onet.SimulationBFTree

	Nodes   gentree.LocalityNodes
	Parents map[string][]string
}

func newNyleSimulation(config string) (onet.Simulation, error) {
	es := &NyleSimulation{}
	_, err := toml.Decode(config, es)
	if err != nil {
		return nil, err
	}
	return es, nil
}

// Setup implements onet.Simulation.
func (s *NyleSimulation) Setup(dir string, hosts []string) (*onet.SimulationConfig, error) {

	app.Copy(dir, "nodes_local_20.txt") ///!!!

	sc := &onet.SimulationConfig{}
	s.CreateRoster(sc, hosts, 2000)
	err := s.CreateTree(sc)
	if err != nil {
		return nil, err
	}
	return sc, nil
}

// Node can be used to initialize each node before it will be run
// by the server. Here we call the 'Node'-method of the
// SimulationBFTree structure which will load the roster- and the
// tree-structure to speed up the first round.
func (s *NyleSimulation) Node(config *onet.SimulationConfig) error {
	index, _ := config.Roster.Search(config.Server.ServerIdentity.ID)
	if index < 0 {
		log.Fatal("Didn't find this node in roster")
	}
	log.Lvl3("Initializing node-index", index)


	//config.Overlay.RegisterTree()

	s.ReadNodeInfo(false)

	mymap := s.InitializeMaps(config, true)

	myService := config.GetService(nyleSvc.Name).(*nyleSvc.Service)

	serviceReq := &nyleSvc.InitRequest{
		Nodes:                s.Nodes.All,
		ServerIdentityToName: mymap,
	}

	if s.Nodes.GetServerIdentityToName(config.Server.ServerIdentity) == "node_19" {
		myService.InitRequest(serviceReq)

		for _, trees := range myService.BinaryTree {
			for _, tree := range trees {
				config.Overlay.RegisterTree(tree)
			}
		}
	}

	/*
	for rootName, trees := range myService.BinaryTree {

		h := sha256.New()
		for _, tree := range trees {

			h.Write(tree.ID[:])
		}
		log.LLvl1("rootName", rootName, h.Sum(nil))
	}
	log.LLvl1(myService.BinaryTree)
	*/

	return s.SimulationBFTree.Node(config)

}

// Run implements onet.Simulation.
func (s *NyleSimulation) Run(config *onet.SimulationConfig) error {

	profile.Start().Stop()
	defer log.Lvl1("deferring run")

	/*
		client := onet.NewClient(cothority.Suite, service.Name)
		req := service.BFTCoSiSimul{
			Roster: *config.Roster,
		}
		var resp service.BFTCoSiSimulResp
		err := client.SendProtobuf(config.Roster.List[0], &req, &resp)
		if err != nil {
			return err
		}
	*/

	//s.ReadNodeInfo(true)
	s.ReadNodeInfo(false)



	// create maps of nodes to server identities and vice versa

	s.InitializeMaps(config, true)


	//client := nyle.NewClient()


	// send init request to all nodes


	/*
	for _, dst := range config.Roster.List {
		client.InitRequest(dst, s.Nodes.All, mymap)
	}
	*/

	//s.RunVersatileSim(config)


	//time.Sleep(60 * time.Second)

	return nil
}

func (s *NyleSimulation) RunVersatileSim(config *onet.SimulationConfig) {


	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {

		wg.Add(1)
		dstName := "node_" + strconv.Itoa(i)
		dst := s.Nodes.NameToServerIdentity(dstName)

		var err error
		go func(dest *network.ServerIdentity, reqId int) {

			client1 := nyle.NewClient()

			log.Lvl1("Sim sending req to", dstName)
			root_consensus_time := monitor.NewTimeMeasure("consensus_time|" + dstName + "|" + strconv.Itoa(reqId) + "|")

			log.Lvl1("before loop")
			_, err = client1.StartCoSi(dest, config.Roster, reqId)
			log.Lvl1("after loop")

			if err != nil {
				panic(err)
			} else {
				log.LLvl1("--------------OK")
			}

			root_consensus_time.Record()
			wg.Done()
		}(dst, i)

		/*
		wg.Add(1)
		go func(dest *network.ServerIdentity, reqId int) {
			client2 := nyle.NewClient()
			log.Lvl1("Sim sending req to", dstName)
			_, err = client2.StartCoSi(dest, config.Roster, reqId)

			if err != nil {
				log.Error(err)
			} else {
				log.Info("OK")
			}

			wg.Done()
		}(dst, i + 1)
		*/

		//if _, err := client.StartCoSi(dst, config.Roster); err != nil {

	}

	wg.Wait()

	//time.Sleep(60 * time.Second)

	//return nil
}

func (s *NyleSimulation) ReadNodeInfo(isLocalTest bool) {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	log.Lvl1(dir)
	if isLocalTest {
		s.ReadNodesFromFile(NODEPATHLOCAL)
	} else {
		s.ReadNodesFromFile(NODEPATHREMOTE)
	}

	for _, nodeRef := range s.Nodes.All {
		node := *nodeRef
		log.Lvl1(node.Name, node.X, node.Y, node.IP)
	}
}

func ReadFileLineByLine(configFilePath string) func() string {
	wd, err := os.Getwd()
	log.Lvl1(wd)
	f, err := os.Open(configFilePath)
	//defer close(f)
	checkErr(err)
	reader := bufio.NewReader(f)
	//defer close(reader)
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

func checkErr(e error) {
	if e != nil && e != io.EOF {
		fmt.Print(e)
		panic(e)
	}
}

func (s *NyleSimulation) ReadNodesFromFile(filename string) {
	s.Nodes.All = make([]*gentree.LocalityNode, 0)

	readLine := ReadFileLineByLine(filename)

	for true {
		line := readLine()
		//fmt.Println(line)
		if line == "" {
			//fmt.Println("end")
			break
		}

		if strings.HasPrefix(line, "#") {
			continue
		}

		tokens := strings.Split(line, " ")
		coords := strings.Split(tokens[1], ",")
		name, x_str, y_str, IP, level_str := tokens[0], coords[0], coords[1], tokens[2], tokens[3]

		x, _ := strconv.ParseFloat(x_str, 64)
		y, _ := strconv.ParseFloat(y_str, 64)
		level, err := strconv.Atoi(level_str)

		if err != nil {
			log.Lvl1("Error", err)

		}

		//	log.Lvl1("reqd node level", name, level_str, "lvl", level)

		myNode := CreateNode(name, x, y, IP, level)
		s.Nodes.All = append(s.Nodes.All, myNode)
	}
}

func (s *NyleSimulation) InitializeMaps(config *onet.SimulationConfig, isLocalTest bool) map[*network.ServerIdentity]string {

	s.Nodes.ServerIdentityToName = make(map[network.ServerIdentityID]string)
	ServerIdentityToName := make(map[*network.ServerIdentity]string)

	if isLocalTest {
		for i := range s.Nodes.All {
			treeNode := config.Tree.List()[i]
			s.Nodes.All[i].ServerIdentity = treeNode.ServerIdentity
			s.Nodes.ServerIdentityToName[treeNode.ServerIdentity.ID] = s.Nodes.All[i].Name
			ServerIdentityToName[treeNode.ServerIdentity] = s.Nodes.All[i].Name
			log.Lvl1("associating", treeNode.ServerIdentity.String(), "to", s.Nodes.All[i].Name)
		}
	} else {
		for _, treeNode := range config.Tree.List() {
			serverIP := treeNode.ServerIdentity.Address.Host()
			node := s.Nodes.GetByIP(serverIP)
			node.ServerIdentity = treeNode.ServerIdentity
			s.Nodes.ServerIdentityToName[treeNode.ServerIdentity.ID] = node.Name
			ServerIdentityToName[treeNode.ServerIdentity] = node.Name
			log.Lvl1("associating", treeNode.ServerIdentity.String(), "to", node.Name)
		}
	}

	return ServerIdentityToName
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
	myNode.Cluster = make(map[string]bool)
	myNode.Bunch = make(map[string]bool)
	myNode.Rings = make([]string, 0)
	return &myNode
}
