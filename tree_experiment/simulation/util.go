package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dedis/paper_nyle/gentree"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"

	//"gopkg.in/dedis/crypto.v0/math"
	"go.dedis.ch/onet/v3/network"
)

func (s *SimulationProtocol) ReadNodeInfo(isLocalTest bool) {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	log.Lvl1(dir)
	if isLocalTest {
		s.ReadNodesFromFile(NODEPATHTEST)
	} else {
		s.ReadNodesFromFile(NODEPATH)
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

func (s *SimulationProtocol) ReadNodesFromFile(filename string) {
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

func (s *SimulationProtocol) InitializeMaps(config *onet.SimulationConfig, isLocalTest bool) map[*network.ServerIdentity]string {

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
