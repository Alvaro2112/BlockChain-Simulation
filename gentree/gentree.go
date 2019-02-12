package gentree

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"bufio"
	"io"
	"sort"

	uuid "github.com/satori/go.uuid"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
)

// TreeConverter is a structure for converting between a recursive tree (graph)
// and a binary tree.
type TreeConverter struct {
	BinaryTree    *onet.Tree
	RecursiveTree *onet.Tree
}

// ToRecursiveTreeNode finds the equivalent tree node in the recursive tree.
func (t *TreeConverter) ToRecursiveTreeNode(target *onet.TreeNode) (*onet.TreeNode, error) {
	return findTreeNode(t.RecursiveTree, target)
}

// ToBinaryTreeNode finds the equivalent tree node in the binary tree.
func (t *TreeConverter) ToBinaryTreeNode(target *onet.TreeNode) (*onet.TreeNode, error) {
	return findTreeNode(t.BinaryTree, target)
}

// LocalityNode represents a locality preserving node.
type LocalityNode struct {
	Name           string
	IP             string
	X              float64
	Y              float64
	Level          int
	ADist          []float64 // ADist[l] - minimum distance to level l
	PDist          []string  // pDist[l] - the node at level l whose distance from the crt Node isADist[l]
	Cluster        map[string]bool
	Bunch          map[string]bool
	OptimalCluster map[string]bool
	OptimalBunch   map[string]bool
	Rings          []string
	NrOwnRings     int
	ServerIdentity *network.ServerIdentity
}

// LocalityNodes is a list of LocalityNode
type LocalityNodes struct {
	All                   []*LocalityNode
	ServerIdentityToName  map[network.ServerIdentityID]string
	ClusterBunchDistances map[*LocalityNode]map[*LocalityNode]float64
	Links                 map[*LocalityNode]map[*LocalityNode]map[*LocalityNode]bool
}

// GetByIP gets the node by IP.
func (ns LocalityNodes) GetByIP(ip string) *LocalityNode {

	for _, n := range ns.All {
		if n.IP == ip {
			return n
		}
	}
	return nil
}

// GetByName gets the node by name.
func (ns LocalityNodes) GetByName(name string) *LocalityNode {
	nodeIdx := NodeNameToInt(name)
	if len(ns.All) < nodeIdx {
		return nil
	}
	return ns.All[nodeIdx]
}

// NameToServerIdentity gets the server identity by name.
func (ns LocalityNodes) NameToServerIdentity(name string) *network.ServerIdentity {
	node := ns.GetByName(name)
	if node != nil {
		return node.ServerIdentity
	}
	return nil
}

// ServerIdentityToName gets the name by server identity.
func (ns LocalityNodes) GetServerIdentityToName(sid *network.ServerIdentity) string {
	return ns.ServerIdentityToName[sid.ID]
}

func findTreeNode(tree *onet.Tree, target *onet.TreeNode) (*onet.TreeNode, error) {

	for _, node := range tree.List() {
		if node.ID.Equal(target.ID) {
			return node, nil
		}
	}
	return nil, errors.New("not found")
}

//First argument is all the nodes
//Second argument is if the coordinates are random
//Third arguments is if the levels are random
//Fourth argument is how many levels there should be if they are random
//Second and Third arguments should always be the same
func CreateLocalityGraph(all LocalityNodes, randomCoords, randomLevels bool, levels int) {

	nodes := all.All

	randSrc := rand.New(rand.NewSource(time.Now().UnixNano()))

	if randomCoords {
		//Computes random coordinates
		for _, n := range nodes {
			n.X = randSrc.Float64() * 500
			n.Y = randSrc.Float64() * 500
		}
	}

	if randomLevels {
		//Computes random levels
		probability := 1.0 / math.Pow(float64(len(nodes)), 1.0/float64(levels))
		for i := 1; i < levels; i++ {
			for _, n := range nodes {
				if n.Level == i-1 && randSrc.Float64() < probability {
					n.Level = i
				}
			}
		}
	}

	for i := 0; i < levels; i++ {

		for _, v1 := range nodes {

			var node LocalityNode
			distance := math.MaxFloat64
			min := math.MaxFloat64

			for _, v2 := range nodes {

				if v2.Level >= i {

					distance = ComputeDist(v1, v2)

					if distance < min {
						min = distance
						node = *v2
					}
				}
			}

			v1.PDist = append(v1.PDist, node.Name)
			v1.ADist = append(v1.ADist, min)
		}
	}

	for _, v1 := range nodes {

		for i := 0; i < levels-1; i++ {

			for _, v2 := range nodes {

				d := ComputeDist(v1, v2)

				//log.LLvl1("assigning", v1.Name, v2.Name)

				if v2.Level >= i {

					if checkDistance(d, i, levels, v1.ADist) && v2 != v1 {

						v1.Bunch[v2.Name] = true
						v1.OptimalBunch[v2.Name] = true
						all.ClusterBunchDistances[v1][v2] = d
						all.ClusterBunchDistances[v2][v1] = d

					}

				}

				if v2.Level == levels-1 && v2 != v1 {

					v1.Bunch[v2.Name] = true
					v1.OptimalBunch[v2.Name] = true
					all.ClusterBunchDistances[v1][v2] = d
					all.ClusterBunchDistances[v2][v1] = d
				}
			}
		}
	}

	for _, v1 := range nodes {

		for _, v2 := range nodes {

			if v2.Bunch[v1.Name] && v2.Name != v1.Name {

				v1.OptimalCluster[v2.Name] = true
				v1.Cluster[v2.Name] = true
				all.ClusterBunchDistances[v1][v2] = ComputeDist(v1, v2)
				all.ClusterBunchDistances[v2][v1] = ComputeDist(v1, v2)
			}
		}
	}

	// write to file
	file, _ := os.Create("../outTrees/original.txt")
	w := bufio.NewWriter(file)
	w.WriteString(strconv.Itoa(len(all.All)) + "\n")
	for _, node := range all.All {
		w.WriteString(fmt.Sprint(node.X) + " " + fmt.Sprint(node.Y) + "\n")

	}

	for _, node := range all.All {
		for clusterNodeName, exists := range node.Cluster {
			if exists {
				name1 := strings.Split(node.Name, "_")[1]
				name2 := strings.Split(clusterNodeName, "_")[1]

				w.WriteString(name1 + " " + name2 + "\n")
			}
		}

		for clusterNodeName, exists := range node.Bunch {
			if exists {
				name1 := strings.Split(node.Name, "_")[1]
				name2 := strings.Split(clusterNodeName, "_")[1]

				w.WriteString(name1 + " " + name2 + "\n")
			}
		}
	}

	w.Flush()
	file.Close()

	file, _ = os.Create("nodes_read.txt")
	w = bufio.NewWriter(file)
	for _, node := range all.All {
		w.WriteString(node.Name + " " + fmt.Sprint(node.X) + "," + fmt.Sprint(node.Y) + " 127.0.0.1 " + strconv.Itoa(node.Level) + "\n")

	}

	w.Flush()
	file.Close()

}

//Checks if a Node is suitable to be another Node's bunch depending on its distance to it
func checkDistance(distance float64, lvl int, lvls int, Adist []float64) bool {

	for i := lvl + 1; i < lvls; i++ {

		if distance <= Adist[i] {
			// do nothing
		} else {
			return false
		}
	}
	return true
}

// GenerateRings TODO documentation
func GenerateRings(allNodes LocalityNodes) {

	for _, node := range allNodes.All {
		GenerateClusterRings(node, allNodes)
		GenerateBunchRings(node, allNodes)
	}
}

func GenerateClusterRings(node *LocalityNode, allNodes LocalityNodes) {

	maxDist := 0.0
	for clusterNodeName := range node.Cluster {
		crtDist := ComputeDist(node, allNodes.GetByName(clusterNodeName))
		if crtDist > maxDist {
			maxDist = crtDist
		}
	}
	log.Lvl1("node maxdist", node.Name, maxDist)

	radiuses := generateRadius(maxDist)

	for i := range radiuses {
		node.Rings = append(node.Rings, generateRingID(node.Name, i))
	}

	node.NrOwnRings = len(node.Rings)
	log.Lvl1(node.Name, "rings", node.NrOwnRings)
}

func GenerateBunchRings(node *LocalityNode, allNodes LocalityNodes) {

	for bunchNodeName := range node.Bunch {
		crtDist := ComputeDist(node, allNodes.GetByName(bunchNodeName))
		ringID := getRingIDFromDistance(crtDist)
		log.Lvl1(node.Name, bunchNodeName, ringID, allNodes.GetByName(bunchNodeName).NrOwnRings)
		// join ringID and all biggerw rings of node bunchNode
		for i := ringID; i < allNodes.GetByName(bunchNodeName).NrOwnRings; i++ {
			log.Lvl1("adding ring", generateRingID(bunchNodeName, i))
			node.Rings = append(node.Rings, generateRingID(bunchNodeName, i))
		}
	}
}

//Computes the Euclidian distance between two nodes
func ComputeDist(v1 *LocalityNode, v2 *LocalityNode) float64 {
	dist := math.Sqrt(math.Pow(v1.X-v2.X, 2) + math.Pow(v1.Y-v2.Y, 2))
	return dist
}

func generateRadius(maxDist float64) []float64 {
	multiplier := 20.0
	base := math.Sqrt(3)
	radiuses := make([]float64, 0)
	for i := 0; ; i++ {
		crtMaxRadius := multiplier * math.Pow(base, float64(i))
		prevRadius := 0.0
		if i != 0 {
			prevRadius = multiplier * math.Pow(base, float64(i-1))
		}

		if crtMaxRadius > maxDist && prevRadius > maxDist {
			break
		}
		radiuses = append(radiuses, crtMaxRadius)
	}

	radiuses = []float64{20.0, 30.0, 50.0, 80.0, 100.0, 130.0, 150.0, 200.0, 300.0, 500.0, 1000.0, 10000.0}
	////radiuses = []float64{20.0, 50.0, 100.0, 150.0, 200.0, 300.0, 500.0, 10000.0}
	log.LLvl1(radiuses)

	return radiuses
}

func generateRingID(node string, ringNumber int) string {
	return node + "_" + strconv.Itoa(ringNumber)
}

func getRingIDFromDistance(distance float64) int {
	radiuses := generateRadius(distance)
	return len(radiuses) - 1
}

// Called on a node, It will add all the coresponding children depending on the optimisation stated previously
// AllowedNodes are the nodes that remain in the tree after the optimisation and the filter by radius (Rings)
// treeNode is the node that we are about to set the parents/childrens of
func CreateAndSetChildren(Rings bool, AllowedNodes map[string]bool, all LocalityNodes, treeNode *onet.TreeNode, NodeList map[string]*onet.TreeNode, parents map[*onet.TreeNode][]*onet.TreeNode) map[*onet.TreeNode][]*onet.TreeNode {

	//Ranges through the nodes Cluster
	for clusterNodeName, exists := range all.All[treeNode.RosterIndex].OptimalCluster {
		//for clusterNodeName, exists := range all.All[treeNode.RosterIndex].Cluster {

		//Continues if the node is not in the range of the radius if we have activated the Ring mode
		if Rings && !AllowedNodes[clusterNodeName] {
			continue
		}
		//Continues if the node doesnt exist in the tree
		if !exists {

			continue

		}

		var clusterTreeNode *onet.TreeNode

		// check if clusternode is not already a tree node and creates it if doesn't exist yet
		if clusterAuxTreeNode, ok := NodeList[all.NameToServerIdentity(clusterNodeName).String()]; !ok {

			ClusterNodeID := all.NameToServerIdentity(clusterNodeName)
			clusterTreeNode = onet.NewTreeNode(NodeNameToInt(clusterNodeName), ClusterNodeID)
			clusterTreeNode.Parent = treeNode
			NodeList[clusterTreeNode.ServerIdentity.String()] = clusterTreeNode

		} else {

			clusterTreeNode = clusterAuxTreeNode
		}

		childExists := false

		//Checks if the node that is about to be added as a children is already a children
		for _, child := range treeNode.Children {

			if child.RosterIndex == clusterTreeNode.RosterIndex {

				childExists = true
				break
			}
		}

		//Adds node as a children
		if !childExists {

			//nodeName := all.GetServerIdentityToName(treeNode.ServerIdentity)

			//clusterTreeNodeName := all.GetServerIdentityToName(clusterTreeNode.ServerIdentity)

			//--> w.WriteString(strconv.Itoa(NodeNameToInt(nodeName)) + " " + strconv.Itoa(NodeNameToInt(clusterTreeNodeName)) + "\n")

			//_, err := fmt.Fprintf(file, strconv.Itoa(NodeNameToInt(nodeName)) + " " + strconv.Itoa(NodeNameToInt(clusterTreeNodeName)) + "\n")

			treeNode.Children = append(treeNode.Children, clusterTreeNode)

		}

	}

	//Does the same as the first loop but with the Bunch
	for bunchNodeName, exists := range all.All[treeNode.RosterIndex].OptimalBunch {
		//for bunchNodeName, exists := range all.All[treeNode.RosterIndex].Bunch {

		if Rings && !AllowedNodes[bunchNodeName] {
			continue
		}

		if !exists {

			continue

		}

		var bunchTreeNode *onet.TreeNode

		// check if clusternode is not already a tree node
		if bunchAuxTreeNode, ok := NodeList[all.NameToServerIdentity(bunchNodeName).String()]; !ok {
			BunchNodeID := all.NameToServerIdentity(bunchNodeName)
			bunchTreeNode = onet.NewTreeNode(NodeNameToInt(bunchNodeName), BunchNodeID)
			bunchTreeNode.Parent = treeNode
			NodeList[bunchTreeNode.ServerIdentity.String()] = bunchTreeNode
			parents[treeNode] = append(parents[treeNode], NodeList[all.NameToServerIdentity(bunchNodeName).String()])

		} else {
			bunchTreeNode = bunchAuxTreeNode
			parents[treeNode] = append(parents[treeNode], NodeList[all.NameToServerIdentity(bunchNodeName).String()])
		}

		childExists := false
		for _, child := range treeNode.Children {
			if child.RosterIndex == bunchTreeNode.RosterIndex {
				childExists = true
				break
			}
		}

		if !childExists {

			//nodeName := all.GetServerIdentityToName(treeNode.ServerIdentity)

			//bunchTreeNodeName := all.GetServerIdentityToName(bunchTreeNode.ServerIdentity)

			treeNode.Children = append(treeNode.Children, bunchTreeNode)
			//--->w.WriteString(strconv.Itoa(NodeNameToInt(nodeName)) + " " + strconv.Itoa(NodeNameToInt(bunchTreeNodeName)) + "\n")

		}
	}

	return parents
}

//Converts a Node to it's index
func NodeNameToInt(nodeName string) int {
	separation := strings.Split(nodeName, "_")
	if len(separation) != 2 {
		log.LLvl1(separation)
	}
	idx, err := strconv.Atoi(separation[1])
	if err != nil {
		panic(err.Error())
	}
	return idx
}

//Filters Nodes depending on their distance to the root given as an argument
//distances is the distance between two nodes in the current graph
func Filter(all LocalityNodes, root *LocalityNode, radius float64, distances map[*LocalityNode]map[*LocalityNode]float64) []*LocalityNode {

	//Childrend That are in the radius
	ChildrenInRange := make(map[*LocalityNode]bool)

	StartPointEndPoint := make(map[*LocalityNode][]*LocalityNode)

	//Node used to go from node A to node B if needed
	MiddlePoint := make(map[*LocalityNode]map[*LocalityNode]*LocalityNode)

	MiddlePoint[root] = make(map[*LocalityNode]*LocalityNode)

	for _, n := range all.All {

		MiddlePoint[n] = make(map[*LocalityNode]*LocalityNode)
	}

	for n, _ := range root.Cluster {

		//checks if node is inside the radius
		if distances[root][all.GetByName(n)] <= radius {
			//Adds it to the final nodes if it is inside of the radius
			ChildrenInRange[all.GetByName(n)] = true

			//ranges through the links that connect the root to node n if there are any
			for k, _ := range all.Links[root][all.GetByName(n)] {

				//Adds them to the final nodes
				ChildrenInRange[k] = true
				//Adds n as present indirect connection from the root to n
				StartPointEndPoint[root] = append(StartPointEndPoint[root], all.GetByName(n))
				//Adds the link as the middle point between the root and node n
				MiddlePoint[root][all.GetByName(n)] = k

			}

		}

	}

	for {

		CopyStartPointEndPoint := make(map[*LocalityNode][]*LocalityNode)
		CopyMiddlePoint := make(map[*LocalityNode]map[*LocalityNode]*LocalityNode)
		CopyMiddlePoint[root] = make(map[*LocalityNode]*LocalityNode)

		i := 0
		j := 0

		//Ranges through all the nodes that are currently used in a path and that are not directly connected to the root to check for possible links
		for Root, EndPoints := range StartPointEndPoint {

			//Ranges throught the nodes that are not directly connected to the root
			for _, EndPoint := range EndPoints {

				j++

				if len(all.Links[Root][MiddlePoint[root][EndPoint]]) == 0 {
					i++
				}

				//Ranges through the links that connect the root to the node that is currently the furthest from the node in the cluster that is not directly connected to the root
				//In other words you can imagine this as two nodes wich are not directly connecte, the root and another node, and we try to reconstruct the path starting by the node that is not the root
				//So endpoint would represent the second that is the furthest starting from the node that is note the root, and middlepoint of that node and the root would be the node that is the
				// furthes from the node that is not the root, the last node of the path that is being built towards the root
				//In this loop we range through that last node and the root links to see what nodes we can use to connect them if there is any
				for a, _ := range all.Links[Root][MiddlePoint[root][EndPoint]] {
					ChildrenInRange[a] = true
					CopyStartPointEndPoint[Root] = append(CopyStartPointEndPoint[Root], MiddlePoint[root][EndPoint])
					MiddlePoint[Root][MiddlePoint[root][EndPoint]] = a

				}
				if len(all.Links[MiddlePoint[root][EndPoint]][EndPoint]) == 0 {
					i++
				}
				//In this loop we do the same as the previous loop but instead of starting from the node that is not the root we start from the root
				for a, _ := range all.Links[MiddlePoint[root][EndPoint]][EndPoint] {

					ChildrenInRange[a] = true
					CopyStartPointEndPoint[MiddlePoint[root][EndPoint]] = append(CopyStartPointEndPoint[MiddlePoint[root][EndPoint]], EndPoint)
					MiddlePoint[MiddlePoint[root][EndPoint]][EndPoint] = a

				}

			}

		}
		//It breaks when every time links is empty, meaning the path is completed
		if i == j*2 {
			break
		}

		StartPointEndPoint = CopyStartPointEndPoint
		MiddlePoint = CopyMiddlePoint
	}

	FinalNodes := ChildrenInRange

	FinalNodes[root] = true

	Nodes := make([]*LocalityNode, 0)

	for k, _ := range FinalNodes {

		Nodes = append(Nodes, k)
	}

	return Nodes
}

//Root is the root of the graph
//Optimization is the upperBound set on the bunch of each node of the graph
func OptimizeGraph(all LocalityNodes, rootName string, Optimization int, OptType int) {
	RemoveLinks(all, all.GetByName(rootName), Optimization, OptType)
}

// CreateOnetLPTree TODO add documentation
// Will Build The Tree calling different functions for different purposes
// It's the main function, all functions created are called directly or indirectly through this one
func CreateOnetLPTree(all LocalityNodes, rootName string, BunchLowerBound int) ([]*onet.Tree, [][]*onet.TreeNode, []map[*onet.TreeNode][]*onet.TreeNode, map[*LocalityNode]map[*LocalityNode]float64) {

	//Slice of Trees to be returned
	Trees := make([]*onet.Tree, 0)

	//Slice of Lists of all the nodes for each tree
	Lists := make([][]*onet.TreeNode, 0)

	//Slice of Parents for each node od each Tree
	Parents := make([]map[*onet.TreeNode][]*onet.TreeNode, 0)

	//Creates a file where we can write
	file, _ := os.Create("Specs/optimized.txt")
	fmt.Fprintf(file, strconv.Itoa(len(all.All))+"\n")
	defer file.Close()

	//w := bufio.NewWriter(file)

	//Prints coordinates of all nodes into the file
	for _, n := range all.All {

		fmt.Fprintf(file, fmt.Sprint(n.X)+" "+fmt.Sprint(n.Y)+"\n")
	}

	//Distance between nodes after Optimisation
	Dist2 := AproximateDistanceOracle(all)

	// AllowedNodes are the nodes that remain in the tree after the optimisation and the filter by radius (Rings)
	AllowedNodes := make(map[string]bool)

	parents := make(map[*onet.TreeNode][]*onet.TreeNode)

	var rootIdxInRoster int
	nrProcessedNodes := 0

	//string represents ServerIdentity
	nodesInTree := make(map[string]*onet.TreeNode)
	roster := make([]*network.ServerIdentity, 0)

	var rootID *network.ServerIdentity

	// create root node
	rootID = all.NameToServerIdentity(rootName)
	root := onet.NewTreeNode(NodeNameToInt(rootName), rootID)
	nodesInTree[root.ServerIdentity.String()] = root

	//Creates Roster*
	for i, k := range all.All {

		if k.Name == rootName {
			rootIdxInRoster = i
		}
		roster = append(roster, all.NameToServerIdentity(k.Name))
	}

	//Remplacing firts element of Roster by the root
	replacement := roster[0]
	roster[0] = rootID
	roster[rootIdxInRoster] = replacement

	// set root children
	parents = CreateAndSetChildren(false, AllowedNodes, all, root, nodesInTree, parents)

	nrProcessedNodes++

	nextLevelNodes := make(map[*onet.TreeNode]bool)
	nextLevelNodesAux := make(map[*onet.TreeNode]bool)

	for _, childNode := range root.Children {

		parents = CreateAndSetChildren(false, AllowedNodes, all, childNode, nodesInTree, parents)

		nrProcessedNodes++

		for _, child := range childNode.Children {
			nextLevelNodesAux[child] = true
		}
	}

	nextLevelNodes = nextLevelNodesAux
	for nrProcessedNodes <= len(all.All) {
		if len(root.Children) == 0 {
			break
		}
		for childNode := range nextLevelNodes {

			parents = CreateAndSetChildren(false, AllowedNodes, all, childNode, nodesInTree, parents)
			nrProcessedNodes++
			for _, child := range childNode.Children {

				nextLevelNodesAux[child] = true
			}
		}
		nextLevelNodes = nextLevelNodesAux
	}

	//Computes Final Roster
	finalRoster := onet.NewRoster(roster)

	//Hashing
	h := sha256.New()
	for _, r := range roster {
		r.Public.MarshalTo(h)
	}

	url := network.NamespaceURL + "tree/" + finalRoster.ID.String() + hex.EncodeToString(h.Sum(nil))

	//Creates Tree
	t := &onet.Tree{
		Roster: finalRoster,
		Root:   root,
		ID:     onet.TreeID(uuid.NewV5(uuid.NamespaceURL, url)),
	}

	//Creates List of Nodes In Tree
	list := make([]*onet.TreeNode, 0)

	for _, v := range nodesInTree {

		list = append(list, v)

	}

	file.Close()

	Lists = append(Lists, list)
	Trees = append(Trees, t)
	Parents = append(Parents, parents)

	return Trees, Lists, Parents, Dist2
}

type ByServerIdentityAlphabetical []*network.ServerIdentity

func (a ByServerIdentityAlphabetical) Len() int {
	return len(a)
}

func (a ByServerIdentityAlphabetical) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a ByServerIdentityAlphabetical) Less(i, j int) bool {
	return a[i].String() < a[j].String()
}

func checkErr(e error) {
	if e != nil && e != io.EOF {
		fmt.Print(e)
		panic(e)
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

func generateSageCodeFromFile(filenameIn string, filenameOut string, graphName string) {

	file, _ := os.Create(filenameOut)
	defer file.Close()
	w := bufio.NewWriter(file)

	N := 0

	log.Lvl1("here")

	readLine := ReadFileLineByLine(filenameIn)

	line := readLine()
	log.Lvl1("here", line)
	//fmt.Println(line)
	if line == "" {
		//fmt.Println("end")
		return
	}

	log.Lvl1("here")
	log.Lvl1(line)
	// write nodes
	N, err := strconv.Atoi(line)
	log.Lvl1(err)
	w.WriteString("g = graphs.EmptyGraph()\n")
	w.WriteString("for i in range(" + line + "):\n")
	w.WriteString("\tg.add_vertex()\n")
	// write positions
	w.WriteString("p = {")
	log.Lvl1("N======", N)

	levels := make([]string, 0)
	for i := 0; i < N; i++ {

		line = readLine()
		coords := strings.Split(line, " ")
		x, y, lvl := coords[0], coords[1], coords[2]
		levels = append(levels, lvl)

		w.WriteString(strconv.Itoa(i) + ": [" + x + "," + y + "]")
		if i != N-1 {
			w.WriteString(", ")
		} else {
			w.WriteString("}\n")
		}
	}
	w.WriteString("g.set_pos(p)\n")

	// write edges
	for true {
		line = readLine()

		if line == "" {
			//fmt.Println("end")
			break
		}

		edge := strings.Split(line, " ")
		log.LLvl1(filenameIn, "line=", line)
		a, b := edge[0], edge[1]

		w.WriteString("g.add_edge(" + a + "," + b + ")\n")
		aint, _ := strconv.Atoi(a)
		bint, _ := strconv.Atoi(b)
		if levels[aint] > levels[bint] {
			w.WriteString("g.set_edge_label(" + a + "," + b + ", str(" + a + "))\n")
		} else {
			w.WriteString("g.set_edge_label(" + a + "," + b + ", str(" + b + "))\n")
		}
	}

	w.WriteString("color_map_aux ={'#000000': [0], '#00FF00': [1], '#0000FF': [2], '#FF0000': [3], '#01FFFE': [4], '#FFA6FE': [5], '#FFDB66': [6], '#006401': [7], '#010067': [8], '#95003A': [9], '#007DB5': [10], '#FF00F6': [11], '#FFEEE8': [12], '#774D00': [13], '#90FB92': [14], '#0076FF': [15], '#D5FF00': [16], '#FF937E': [17], '#6A826C': [18], '#FF029D': [19], '#FE8900': [20], '#7A4782': [21], '#7E2DD2': [22], '#85A900': [23], '#FF0056': [24], '#A42400': [25], '#00AE7E': [26], '#683D3B': [27], '#BDC6FF': [28], '#263400': [29], '#BDD393': [30], '#00B917': [31], '#9E008E': [32], '#001544': [33], '#C28C9F': [34], '#FF74A3': [35], '#01D0FF': [36], '#004754': [37], '#E56FFE': [38], '#788231': [39], '#0E4CA1': [40], '#91D0CB': [41], '#BE9970': [42], '#968AE8': [43], '#BB8800': [44], '#43002C': [45], '#DEFF74': [46], '#00FFC6': [47], '#FFE502': [48], '#620E00': [49], '#008F9C': [50], '#98FF52': [51], '#7544B1': [52], '#B500FF': [53], '#00FF78': [54], '#FF6E41': [55], '#005F39': [56], '#6B6882': [57], '#5FAD4E': [58], '#A75740': [59], '#A5FFD2': [60], '#FFB167': [61], '#009BFF': [62], '#E85EBE': [63]}\n")
	w.WriteString("color_map_reverse_aux = {'0': '#000000', '1': '#00FF00', '2': '#0000FF', '3': '#FF0000', '4': '#01FFFE', '5': '#FFA6FE', '6': '#FFDB66', '7': '#006401', '8': '#010067', '9': '#95003A', '10': '#007DB5', '11': '#FF00F6', '12': '#FFEEE8', '13': '#774D00', '14': '#90FB92', '15': '#0076FF', '16': '#D5FF00', '17': '#FF937E', '18': '#6A826C', '19': '#FF029D', '20': '#FE8900', '21': '#7A4782', '22': '#7E2DD2', '23': '#85A900', '24': '#FF0056', '25': '#A42400', '26': '#00AE7E', '27': '#683D3B', '28': '#BDC6FF', '29': '#263400', '30': '#BDD393', '31': '#00B917', '32': '#9E008E', '33': '#001544', '34': '#C28C9F', '35': '#FF74A3', '36': '#01D0FF', '37': '#004754', '38': '#E56FFE', '39': '#788231', '40': '#0E4CA1', '41': '#91D0CB', '42': '#BE9970', '43': '#968AE8', '44': '#BB8800', '45': '#43002C', '46': '#DEFF74', '47': '#00FFC6', '48': '#FFE502', '49': '#620E00', '50': '#008F9C', '51': '#98FF52', '52': '#7544B1', '53': '#B500FF', '54': '#00FF78', '55': '#FF6E41', '56': '#005F39', '57': '#6B6882', '58': '#5FAD4E', '59': '#A75740', '60': '#A5FFD2', '61': '#FFB167', '62': '#009BFF', '63': '#E85EBE'}\n")
	//w.WriteString("color_map_reverse_aux={'0': '#b25656', '1': '#f21c00', '2': '#991d0c', '3': '#400e08', '4': '#e55440', '5': '#f29488', '6': '#734132', '7': '#d9b1a5', '8': '#f25500', '9': '#73310e', '10': '#ffa270', '11': '#40291c', '12': '#8c7365', '13': '#cc7121', '14': '#593719', '15': '#cc9362', '16': '#e6c8ae', '17': '#ff9500', '18': '#261d11', '19': '#4d4437', '20': '#332400', '21': '#cc9410', '22': '#735717', '23': '#ffedc2', '24': '#ffd829', '25': '#e5d06e', '26': '#807a61', '27': '#8c840b', '28': '#f4ff1f', '29': '#3d400a', '30': '#969956', '31': '#a3bf17', '32': '#527300', '33': '#e2ff99', '34': '#b6ff47', '35': '#4a593d', '36': '#4ab30e', '37': '#273321', '38': '#113306', '39': '#2cff29', '40': '#87ff85', '41': '#9cd9a2', '42': '#5a8c6b', '43': '#046630', '44': '#00bf6c', '45': '#08402e', '46': '#00ffcc', '47': '#008075', '48': '#23d9ca', '49': '#0c2624', '50': '#c2fdff', '51': '#6b8b8c', '52': '#1fddff', '53': '#167b8c', '54': '#22464d', '55': '#005e80', '56': '#82c2d9', '57': '#0aa1ff', '58': '#4d93bf', '59': '#232d33', '60': '#075db3', '61': '#0f2e4d', '62': '#576b80', '63': '#142a66', '64': '#121826', '65': '#425380', '66': '#aec1f2', '67': '#001173', '68': '#092ae6', '69': '#1b2fa6', '70': '#8fa0ff', '71': '#5f63d9', '72': '#060233', '73': '#724399', '74': '#3c284d', '75': '#726180', '76': '#5f068c', '77': '#b68bcc', '78': '#bc25e6', '79': '#210c26', '80': '#e27aff', '81': '#460c4d', '82': '#80006f', '83': '#b347a4', '84': '#ff0ac2', '85': '#ffa3e8', '86': '#593d52', '87': '#99507e', '88': '#990652', '89': '#ff148e', '90': '#400a20', '91': '#b38899', '92': '#ff0a50', '93': '#a60734', '94': '#662537', '95': '#e67797', '96': '#73091b', '97': '#735358', '98': '#261c1d', '99': '#ffc2c5'}\n")

	w.WriteString("color_map_levels = {'0': '#FF0000', '1': '#FFDB66', '2': '#0076FF'}\n")

	// level colors are 6, 3, 15

	w.WriteString("valid_keys = []\n")
	w.WriteString("count=0\n")
	w.WriteString("for v in g.vertices():\n")
	w.WriteString("\tvalid_keys.append(str(count))\n")
	w.WriteString("\tcount += 1\n")

	w.WriteString("levels = [")
	for k, x := range levels {
		w.WriteString(x)
		if k != len(levels)-1 {
			w.WriteString(",")
		}
	}
	w.WriteString("]\n")

	w.WriteString("color_map_reverse =dict((k,color_map_levels[str(levels[int(k)])]) for k in valid_keys if k in color_map_reverse_aux) \n")

	w.WriteString("color_map={}\n")

	w.WriteString("for k in color_map_reverse:\n")
	//w.WriteString("\tcolor_map[color_map_reverse[k]]=[int(k)]\n")
	w.WriteString("\tcolor_map[color_map_reverse[k]] = []\n")

	w.WriteString("for k in color_map_reverse:\n")
	//w.WriteString("\tcolor_map[color_map_reverse[k]]=[int(k)]\n")
	w.WriteString("\tcolor_map[color_map_reverse[k]].append(int(k))\n")

	w.WriteString("print(color_map_reverse)\n")
	w.WriteString("print(color_map)\n")

	//w.WriteString("h = g.plot(color_by_label=color_map_reverse, vertex_colors=color_map)\n")

	w.WriteString("h = g.plot(color_by_label=color_map_reverse, vertex_colors=color_map)\n")
	//w.WriteString("h = g.plot()\n")

	w.WriteString("h.show()\n")
	w.WriteString("save(h, '" + graphName + ".png', axes=False,aspect_ratio=True)\n")
	w.WriteString("os.system('open " + graphName + ".png')\n")

	//save(h, '/tmp/dom.png',axes=False,aspect_ratio=True)
	//os.system('open /tmp/dom.png')

	w.Flush()
}

func visitBuildGraph(all LocalityNodes, crtNode *onet.TreeNode, neighbors *map[string]map[string]bool, visited *map[string]bool, w *bufio.Writer) {
	parentNodeName := all.GetServerIdentityToName(crtNode.ServerIdentity)

	for _, childNode := range crtNode.Children {
		childNodeName := all.GetServerIdentityToName(childNode.ServerIdentity)
		// traversedown
		log.LLvl1(parentNodeName, "on child", childNodeName)

		//(*neighbors)[parentNodeName] = append((*neighbors)[parentNodeName], childNodeName)
		//(*neighbors)[childNodeName] = append((*neighbors)[childNodeName], parentNodeName)

		(*neighbors)[parentNodeName][childNodeName] = true
		(*neighbors)[childNodeName][parentNodeName] = true

		// it'll be an allowed leaf if it has at least one unvisited descendant

		if !(*visited)[childNodeName] {

			log.LLvl1("NOT VISITED!")

			log.LLvl1(parentNodeName, "adding child", childNodeName)

			if w != nil {
				w.WriteString(strconv.Itoa(NodeNameToInt(parentNodeName)) + " " + strconv.Itoa(NodeNameToInt(childNodeName)) + "\n")
			}

			(*visited)[childNodeName] = true
			visitBuildGraph(all, childNode, neighbors, visited, w)
		} else {
			log.LLvl1("VISITED!")
		}

		log.LLvl1("HERE!")
	}
}

func dfsBuildGraph(all LocalityNodes, root *onet.TreeNode, w *bufio.Writer) map[string]map[string]bool {

	// use a list of neighbors
	neighbors := make(map[string]map[string]bool)
	visited := make(map[string]bool, 0)
	for _, node := range all.All {
		neighbors[node.Name] = make(map[string]bool)
		visited[node.Name] = false
	}

	rootName := all.GetServerIdentityToName(root.ServerIdentity)
	log.LLvl1("EXPLORING ", rootName)
	visited[rootName] = true

	crtNode := root
	visitBuildGraph(all, crtNode, &neighbors, &visited, w)

	return neighbors
}

func dfsBuildTree(all LocalityNodes, rootName string, neighbors map[string]map[string]bool, visited *map[string]bool, distances *map[string]float64, nodesInTree *map[string]*onet.TreeNode, depth *map[string]int,
	allowedNodes map[string]bool, allowedLeafs map[string]bool, buildAllowedLeafs bool, radius float64, w *bufio.Writer, failedMode bool) (map[string]bool, bool) {
	innerNodes := make(map[string]bool)
	crtNodeName := rootName

	succeeded := dijkstra(all, crtNodeName, neighbors, visited, distances, nodesInTree, depth, allowedNodes, allowedLeafs, buildAllowedLeafs, radius, failedMode)

	if !succeeded {
		return innerNodes, false
	}
	for name, node := range *nodesInTree {
		if node != nil {
			if len(node.Children) > 0 {
				innerNodes[name] = true
			}
			for _, child := range node.Children {
				childName := all.GetServerIdentityToName(child.ServerIdentity)
				if w != nil {
					w.WriteString(strconv.Itoa(NodeNameToInt(name)) + " " + strconv.Itoa(NodeNameToInt(childName)) + "\n")
				}

			}
		}
	}

	return innerNodes, true

}

func dijkstra(all LocalityNodes, crtNodeName string, neighbors map[string]map[string]bool, visited *map[string]bool, distances *map[string]float64, nodesInTree *map[string]*onet.TreeNode, depth *map[string]int,
	allowedNodes map[string]bool, allowedLeafs map[string]bool, buildAllowedLeafs bool, radius float64, failedMode bool) bool {

	repeat := true

	localVisited := make(map[string]bool)
	for _, node := range all.All {
		localVisited[node.Name] = false
	}

	for repeat {
		repeat = false
		// select smallest distance
		min := 60000.0
		minNodeName := crtNodeName
		for nodeName, dist := range *distances {

			if dist < min && allowedNodes[nodeName] && !localVisited[nodeName] {

				// should nodeName be a leaf? if yes, we cannot select it as inner node
				shouldBeLeaf, ok := allowedLeafs[nodeName]

				if !buildAllowedLeafs && ok && shouldBeLeaf {
					localVisited[nodeName] = true
					continue
				}

				min = dist
				minNodeName = nodeName
			}
			if dist < 60000.0 && allowedNodes[nodeName] && !localVisited[nodeName] {
				repeat = true
			}
		}

		//log.LLvl1("picked node to visit", minNodeName, "repeat=", repeat)

		(*visited)[minNodeName] = true
		localVisited[minNodeName] = true

		// make node
		if (*nodesInTree)[minNodeName] == nil {
			minNodeID := all.NameToServerIdentity(minNodeName)
			minTreeNode := onet.NewTreeNode(NodeNameToInt(minNodeName), minNodeID)
			(*nodesInTree)[minNodeName] = minTreeNode
		}

		for neighborName, exists := range neighbors[minNodeName] {
			log.LLvl1("checking neighbor", neighborName, "exists=", exists, "visited=", (*visited)[neighborName], "distance=", "allowed=", allowedNodes[neighborName], (*distances)[neighborName], "new dist", (*distances)[minNodeName]+ComputeDist(all.GetByName(minNodeName), all.GetByName(neighborName)), "radius", radius)

			if exists && !(*visited)[neighborName] && allowedNodes[neighborName] && len((*nodesInTree)[minNodeName].Children) < 10 {
				log.LLvl1("first passed depth =", (*depth)[minNodeName], "building leafs=", buildAllowedLeafs, "failed mode=", failedMode)
				// TODO 2 is orientative nr
				//if ((*distances)[neighborName] > (*distances)[minNodeName] + ComputeDist(all.GetByName(minNodeName), all.GetByName(neighborName)) && (*depth)[minNodeName] < 2) ||
				if ((*distances)[neighborName] > (*distances)[minNodeName]+ComputeDist(all.GetByName(minNodeName), all.GetByName(neighborName)) && ((!failedMode && (*depth)[minNodeName] < 2) || (failedMode && (*depth)[minNodeName] < 3))) ||
					((*distances)[minNodeName]+ComputeDist(all.GetByName(minNodeName), all.GetByName(neighborName)) <= radius &&
						(((buildAllowedLeafs || (!buildAllowedLeafs && allowedLeafs[neighborName])) && (*depth)[minNodeName] < 2) || (failedMode && (*depth)[minNodeName] < 3))) {

					log.LLvl1("relaxing", minNodeName, "adding child", neighborName)

					(*distances)[neighborName] = (*distances)[minNodeName] + ComputeDist(all.GetByName(minNodeName), all.GetByName(neighborName))
					// make node
					if (*nodesInTree)[neighborName] == nil {
						neighborNodeID := all.NameToServerIdentity(neighborName)
						neighborTreeNode := onet.NewTreeNode(NodeNameToInt(neighborName), neighborNodeID)
						(*nodesInTree)[neighborName] = neighborTreeNode
					}

					(*nodesInTree)[minNodeName].Children = append((*nodesInTree)[minNodeName].Children, (*nodesInTree)[neighborName])

					// the node already had a parent, remove it as a child from that parent
					if (*nodesInTree)[neighborName].Parent != nil {
						idx := -1
						for i, n := range (*nodesInTree)[all.GetServerIdentityToName((*nodesInTree)[neighborName].Parent.ServerIdentity)].Children {
							if all.GetServerIdentityToName(n.ServerIdentity) == neighborName {
								idx = i
								break
							}
						}
						(*nodesInTree)[all.GetServerIdentityToName((*nodesInTree)[neighborName].Parent.ServerIdentity)].Children =
							append((*nodesInTree)[all.GetServerIdentityToName((*nodesInTree)[neighborName].Parent.ServerIdentity)].Children[:idx],
								(*nodesInTree)[all.GetServerIdentityToName((*nodesInTree)[neighborName].Parent.ServerIdentity)].Children[(idx+1):]...)
					}

					(*nodesInTree)[neighborName].Parent = (*nodesInTree)[minNodeName]
					(*depth)[neighborName] = (*depth)[minNodeName] + 1

				}

			}
		}
	}

	for nodeName, dist := range *distances {
		// couldn't visit all nodes within given constraints
		if dist >= 60000.0 && allowedNodes[nodeName] {
			log.LLvl1("PROBL Not all visited")
			return false
		}
		if (*depth)[nodeName] > 2 && allowedNodes[nodeName] && !failedMode {
			log.LLvl1("PROBL Too big depth")
			return false
		}
	}

	for _, node := range *nodesInTree {
		if node != nil && !failedMode {
			// TODO 10 is orientative nr
			if len(node.Children) > 10 {
				log.LLvl1("PROBL Too many children")
				return false
			}
		}
	}

	return true

}

// returns the graph represented as a tree with cycles, rooted at rootName

func buildRootedLocalityGraph(all LocalityNodes, rootName string, radius float64, dist2 map[*LocalityNode]map[*LocalityNode]float64) *onet.TreeNode {
	AllowedNodes := make(map[string]bool)
	var Filterr []*LocalityNode

	Filterr = Filter(all, all.GetByName(rootName), radius, dist2)

	for _, n := range Filterr {
		AllowedNodes[n.Name] = true
	}

	//log.LLvl1("#!@#!@#!@#!@#@!#!@#!@#!@", "root=", rootName, "allowed", AllowedNodes)

	parents := make(map[*onet.TreeNode][]*onet.TreeNode)

	nrProcessedNodes := 0
	nodesInTree := make(map[string]*onet.TreeNode)

	var rootID *network.ServerIdentity

	// create root node
	rootID = all.NameToServerIdentity(rootName)
	root := onet.NewTreeNode(NodeNameToInt(rootName), rootID)
	nodesInTree[root.ServerIdentity.String()] = root

	Filterr = Filter(all, all.GetByName(rootName), radius, dist2)

	// set root children
	parents = CreateAndSetChildren(true, AllowedNodes, all, root, nodesInTree, parents)

	nrProcessedNodes++

	nextLevelNodes := make(map[*onet.TreeNode]bool)
	nextLevelNodesAux := make(map[*onet.TreeNode]bool)

	for _, childNode := range root.Children {

		parents = CreateAndSetChildren(true, AllowedNodes, all, childNode, nodesInTree, parents)

		nrProcessedNodes++

		for _, child := range childNode.Children {
			nextLevelNodesAux[child] = true
		}
	}

	nextLevelNodes = nextLevelNodesAux
	for nrProcessedNodes <= len(all.All) {
		if len(root.Children) == 0 {
			break
		}
		for childNode := range nextLevelNodes {

			parents = CreateAndSetChildren(true, AllowedNodes, all, childNode, nodesInTree, parents)
			nrProcessedNodes++
			for _, child := range childNode.Children {

				nextLevelNodesAux[child] = true
			}
		}
		nextLevelNodes = nextLevelNodesAux
	}
	return root
}

func buildOnetTree(all LocalityNodes, rootName string, rootNode *onet.TreeNode, inTree map[string]*onet.TreeNode) *onet.Tree {
	// build roster
	rootIdxInRoster := 0
	roster := make([]*network.ServerIdentity, 0)
	idx := 0
	for name, node := range inTree {
		if node != nil {
			if name == rootName {
				rootIdxInRoster = idx
			}
			roster = append(roster, node.ServerIdentity)
			idx++
		}
	}

	log.LLvl1(roster)

	// replacing the first element of Roster by the root
	replacement := roster[0]
	roster[0] = all.NameToServerIdentity(rootName)
	roster[rootIdxInRoster] = replacement

	log.LLvl1(roster)

	// deterministic roster order
	// the roster order does not affect the locality graph, because the locality graph is built using Parents
	if len(roster) > 1 {

		// put in rosterAux all elements of roster except the root, which is at index 0
		rosterAux := make([]*network.ServerIdentity, 0)
		for i, x := range roster {
			if i == 0 {
				continue
			}
			rosterAux = append(rosterAux, x)
		}

		log.LLvl1(rosterAux)

		// sort rosterAux
		sort.Sort(ByServerIdentityAlphabetical(rosterAux))

		log.LLvl1(rosterAux)

		// add the sorted elements in roster after the root
		for i := range roster {
			if i == 0 {
				continue
			}
			roster[i] = rosterAux[i-1]
		}
	}

	finalRoster := onet.NewRoster(roster)
	log.LLvl1(finalRoster.List)

	//Hashing
	h := sha256.New()
	for _, r := range roster {
		r.Public.MarshalTo(h)
	}

	url := network.NamespaceURL + "tree/" + finalRoster.ID.String() + hex.EncodeToString(h.Sum(nil))

	//Creates Tree
	t := &onet.Tree{
		Roster: finalRoster,
		Root:   rootNode,
		ID:     onet.TreeID(uuid.NewV5(uuid.NamespaceURL, url)),
	}

	return t
}

func CreateOnetRings(all LocalityNodes, rootName string, dist2 map[*LocalityNode]map[*LocalityNode]float64, levels int) ([][]*onet.Tree, []float64) {

	log.LLvl1("called for root", rootName)

	//Slice of Trees to be returned
	radiuses := generateRadius(10000)
	Trees := make([][]*onet.Tree, len(radiuses))

	for i := range radiuses {
		Trees[i] = make([]*onet.Tree, 0)
	}

	// TODO if you're not a highest level node, then find the first highest level node, build the graph as seen by that node
	// TODO then build your own

	highLevelRootName := ""

	if all.GetByName(rootName).Level == levels-1 {
		highLevelRootName = rootName
	} else {
		for _, node := range all.All {
			if node.Level == levels-1 {
				highLevelRootName = node.Name
				break
			}
		}
	}

	highLevelRootName = "node_19"

	countt := 0

	script, err := os.Create("../outTrees/007-" + rootName + ".sh")
	if err != nil {
		log.Error(err)
	}

	defer script.Close()
	wScript := bufio.NewWriter(script)

	visited := make(map[string]bool)
	distances := make(map[string]float64)
	inTree := make(map[string]*onet.TreeNode)
	depth := make(map[string]int)

	visited2 := make(map[string]bool)
	distances2 := make(map[string]float64)
	inTree2 := make(map[string]*onet.TreeNode)
	depth2 := make(map[string]int)

	for _, node := range all.All {
		visited[node.Name] = false
		distances[node.Name] = 66000.0
		inTree[node.Name] = nil
		depth[node.Name] = 66000

		visited2[node.Name] = false
		distances2[node.Name] = 66000.0
		inTree2[node.Name] = nil
		depth2[node.Name] = 66000
	}

	distances[rootName] = 0.0
	depth[rootName] = 0

	root := buildRootedLocalityGraph(all, highLevelRootName, radiuses[len(radiuses)-1], dist2)
	neighbors := dfsBuildGraph(all, root, nil)
	log.LLvl1("neighbors", neighbors)
	//log.LLvl1("FINISHED")

	// iterate through ring radiuses
	previousRingRosterlen := 0
	for {
		// create global graph
		AllowedNodes := make(map[string]bool)

		//Creates a file where we can write
		fName := "../outTrees/result" + rootName + "-" + strconv.Itoa(countt)
		graphName := "../outTrees/result" + rootName + "-" + strconv.Itoa(countt)
		file, err := os.Create(fName + ".txt")
		if err != nil {
			log.Error(err)
		}

		defer file.Close()

		w := bufio.NewWriter(file)
		w.WriteString(strconv.Itoa(len(all.All)) + "\n")

		//Prints coordinates of all nodes into the file
		for _, n := range all.All {
			w.WriteString(fmt.Sprint(n.X) + " " + fmt.Sprint(n.Y) + " " + fmt.Sprint(n.Level) + "\n")
		}

		// commutative consensus - use the idea of commutative transactions
		for n1, m := range dist2 {
			for n2, d := range m {
				if (n1.Name == rootName || n2.Name == rootName) && d <= radiuses[countt] {
					AllowedNodes[n1.Name] = true
					AllowedNodes[n2.Name] = true

					//log.LLvl1(n1.Name, "->", n2.Name, d)

				}
			}
		}

		log.LLvl1(fName, "allowed nodes", AllowedNodes)

		// attempt several times to compute a tree rooted at rootNode
		var innerNodes map[string]bool
		succeeded := false
		count := 0
		isRelaxedMode := false
		visitedCopy := make(map[string]bool)
		distancesCopy := make(map[string]float64)
		inTreeCopy := make(map[string]*onet.TreeNode)
		depthCopy := make(map[string]int)

		for {
			// make copy of variables from previous radius
			visitedCopy = make(map[string]bool)
			distancesCopy = make(map[string]float64)
			inTreeCopy = make(map[string]*onet.TreeNode)
			depthCopy = make(map[string]int)
			allowedLeafs := make(map[string]bool)

			for k, v := range visited {
				visitedCopy[k] = v
			}

			for k, v := range distances {
				distancesCopy[k] = v
			}

			// deep copy tree
			for k, n := range inTree {
				if n != nil {
					nodeID := all.NameToServerIdentity(k)
					inTreeCopy[k] = onet.NewTreeNode(NodeNameToInt(k), nodeID)
				} else {
					inTreeCopy[k] = nil
				}
			}

			for k, v := range inTree {
				if v != nil {
					for _, c := range v.Children {
						childName := all.GetServerIdentityToName(c.ServerIdentity)
						inTreeCopy[k].AddChild(inTreeCopy[childName])
					}
				}
			}

			for k, v := range depth {
				depthCopy[k] = v
			}

			innerNodes, succeeded = dfsBuildTree(all, rootName, neighbors, &visitedCopy, &distancesCopy, &inTreeCopy, &depthCopy, AllowedNodes, allowedLeafs, true, radiuses[countt], w, isRelaxedMode)

			if succeeded {
				//log.LLvl1("----------1----------- built first tree perfect")
				break
			}

			if count > 20 {
				isRelaxedMode = true
				innerNodes, succeeded = dfsBuildTree(all, rootName, neighbors, &visitedCopy, &distancesCopy, &inTreeCopy, &depthCopy, AllowedNodes, allowedLeafs, true, radiuses[countt], w, isRelaxedMode)
			}

			if succeeded {
				//log.LLvl1("----------2----------- built first tree imprefect")
				break
			}

			count++
		}

		/*
			for name, exists := range innerNodes {
				if exists {
					log.LLvl1(fName, "inner nodes", name)
				}
			}
		*/

		w.Flush()
		file.Close()

		// copy variables back for next iteration
		for k, v := range visitedCopy {
			visited[k] = v
		}

		for k, v := range distancesCopy {
			distances[k] = v
		}

		// deep copy tree
		for k, n := range inTreeCopy {
			if n != nil {
				nodeID := all.NameToServerIdentity(k)
				inTree[k] = onet.NewTreeNode(NodeNameToInt(k), nodeID)
			} else {
				inTree[k] = nil
			}
		}

		for k, v := range inTreeCopy {
			if v != nil {
				for _, c := range v.Children {
					childName := all.GetServerIdentityToName(c.ServerIdentity)
					inTree[k].AddChild(inTree[childName])
				}
			}
		}

		for k, v := range depthCopy {
			depth[k] = v
		}

		// build onet tree
		var tree1, tree2 *onet.Tree

		log.LLvl1("TREE", fName, "succeeded=", succeeded)
		tree1 = buildOnetTree(all, rootName, inTree[rootName], inTree)

		if len(tree1.Roster.List) == previousRingRosterlen || len(tree1.Roster.List) < 2 || !succeeded {
			countt++
			if countt == len(radiuses) {
				break
			}
			continue
		}

		generateSageCodeFromFile(fName+".txt", fName+".sage", graphName)
		wScript.WriteString("sage " + graphName + ".sage\n")

		previousRingRosterlen = len(tree1.Roster.List)
		log.LLvl1("AFTERTREE", fName, "roster len=", len(tree1.Roster.List))

		Trees[countt] = append(Trees[countt], tree1)

		//if len(inTree) != len(AllowedNodes) {
		//	log.Error("different lengths big problem")
		//}

		// build second FT graph
		if len(tree1.Roster.List) > 2 {
			repeats := 0
			nextRootIdx := 0
			nextRootName := ""
			//if nextRootId

			//Creates a file where we can write

			file2, err := os.Create(fName + "2.txt")
			if err != nil {
				log.Error(err)
			}

			defer file2.Close()
			w2 := bufio.NewWriter(file2)
			w2.WriteString(strconv.Itoa(len(all.All)) + "\n")
			//Prints coordinates of all nodes into the file
			for _, n := range all.All {
				w2.WriteString(fmt.Sprint(n.X) + " " + fmt.Sprint(n.Y) + " " + fmt.Sprint(n.Level) + "\n")
			}

			failed := false

			for {
				nextRootIdx = rand.Intn(len(tree1.Roster.List))
				nextRootName = all.GetServerIdentityToName(tree1.Roster.List[nextRootIdx])
				//if nextRootIdx != rootIdxInRoster && innerNodes[nextRootName] == false {
				if nextRootName != rootName && innerNodes[nextRootName] == false {

					log.LLvl1("called for file", fName+"2.txt", "original root", rootName, "new root", nextRootName)
					//log.LLvl1(fName, "next root is", nextRootName)

					distances2[nextRootName] = 0.0
					depth2[nextRootName] = 0

					visited2 := make(map[string]bool)
					distances2 := make(map[string]float64)
					inTree2 := make(map[string]*onet.TreeNode)
					depth2 := make(map[string]int)

					for _, node := range all.All {
						visited2[node.Name] = false
						distances2[node.Name] = 66000.0
						inTree2[node.Name] = nil
						depth2[node.Name] = 66000
					}

					depth2[nextRootName] = 0
					distances2[nextRootName] = 0.0

					succeeded := false

					if repeats > 10 {
						//log.LLvl1("giving up, building approximate tree")
						failed = true
						//_, succeeded = dfsBuildTree(all, nextRootName, neighbors, &visited2, &distances2, &inTree2, &depth2, AllowedNodes, innerNodes, false, radiuses[countt], w2, true)
						//} else {
						//_, succeeded = dfsBuildTree(all, nextRootName, neighbors, &visited2, &distances2, &inTree2, &depth2, AllowedNodes, innerNodes, false, radiuses[countt], w2, false)
					}

					_, succeeded = dfsBuildTree(all, nextRootName, neighbors, &visited2, &distances2, &inTree2, &depth2, AllowedNodes, innerNodes, false, radiuses[countt], w2, failed)

					if succeeded || repeats > 20 {
						if succeeded {
							tree2 = buildOnetTree(all, nextRootName, inTree2[nextRootName], inTree2)
							Trees[countt] = append(Trees[countt], tree2)
							w2.Flush()
							file2.Close()
							generateSageCodeFromFile(fName+"2.txt", fName+"2.sage", graphName+"-"+nextRootName)
							wScript.WriteString("sage " + fName + "2.sage\n")
						}
						break
					}
				}
				repeats++

			}

		}

		countt++
		if countt == len(radiuses) {
			break
		}
	}

	wScript.Flush()

	return Trees, radiuses
}
