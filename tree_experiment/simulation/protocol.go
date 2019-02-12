package main

/*
The simulation-file can be used with the `cothority/simul` and be run either
locally or on deterlab. Contrary to the `test` of the protocol, the simulation
is much more realistic, as it tests the protocol on different nodes, and not
only in a test-environment.

The Setup-method is run once on the client and will create all structures
and slices necessary to the simulation. It also receives a 'dir' argument
of a directory where it can write files. These files will be copied over to
the simulation so that they are available.

The Run-method is called only once by the root-node of the tree defined in
Setup. It should run the simulation in different rounds. It can also
measure the time each run takes.

In the Node-method you can read the files that have been created by the
'Setup'-method.
*/

import (
	"github.com/BurntSushi/toml"
	"github.com/dedis/paper_nyle/gentree"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/app"
	"go.dedis.ch/onet/v3/log"
	//"fmt"
	"github.com/dedis/paper_nyle/tree_experiment/service"
)

func init() {
	onet.SimulationRegister(service.ServiceName, NewSimulationProtocol)
	//	onet.GlobalProtocolRegister("TemplateProtocol", NewSimulationProtocol)
}

// SimulationProtocol implements onet.Simulation.
type SimulationProtocol struct {
	onet.SimulationBFTree

	Nodes   gentree.LocalityNodes
	Parents map[string][]string
	//ServerIdentityToNode map[*onet.SendServerIdentity]string
}

// NewSimulationProtocol is used internally to register the simulation (see the init()
// function above).
func NewSimulationProtocol(config string) (onet.Simulation, error) {
	es := &SimulationProtocol{}
	_, err := toml.Decode(config, es)
	if err != nil {
		return nil, err
	}
	return es, nil
}

// Setup implements onet.Simulation.
func (s *SimulationProtocol) Setup(dir string, hosts []string) (*onet.SimulationConfig, error) {

	app.Copy(dir, "nodes.txt")

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
func (s *SimulationProtocol) Node(config *onet.SimulationConfig) error {
	index, _ := config.Roster.Search(config.Server.ServerIdentity.ID)
	if index < 0 {
		log.Fatal("Didn't find this node in roster")
	}
	log.Lvl3("Initializing node-index", index)
	return s.SimulationBFTree.Node(config)
}

// Run implements onet.Simulation.
func (s *SimulationProtocol) Run(config *onet.SimulationConfig) error {
	//size := config.Tree.Size()

	s.ReadNodeInfo(true)
	// create maps of nodes to server identities and vice versa
	mymap := s.InitializeMaps(config, true)

	client := service.NewLocalityClient()

	// send init request to all nodes

	for _, dst := range config.Roster.List {
		client.InitRequest(dst, s.Nodes.All, mymap)
	}

	// TODO send multicast request

	/*
		proto, err := config.Overlay.CreateProtocol(protocol.LocalityProtocolName, s.CreateOnetLPTree(strRoot), onet.NilServiceID)
		checkErr(err)
	*/
	//	checkErr(err)

	//proto.Start()

	/*
		wrapper := proto.(*protocol.LocalityPresProtocol)
		wrapper.Parents = ....

		proto.Start()
	*/

	/*

		log.Lvl2("Size is:", size, "rounds:", s.Rounds)
		for round := 0; round < s.Rounds; round++ {
			log.Lvl1("Starting round", round)
			round := monitor.NewTimeMeasure("round")
			p, err := config.Overlay.CreateProtocol("Template", config.Tree,
				onet.NilServiceID)
			if err != nil {
				return err
			}
			go p.Start()
			children := <-p.(*protocol.TemplateProtocol).ChildCount
			round.Record()
			if children != size {
				return errors.New("Didn't get " + strconv.Itoa(size) +
					" children")
			}
		}

	*/
	return nil
}
