package service

import (
	"github.com/dedis/paper_nyle/gentree"
	"go.dedis.ch/cothority/v3"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
)

// Client is a structure to communicate with the CoSi
// service
type Client struct {
	*onet.Client
}

// NewClient instantiates a newClient
func NewLocalityClient() *Client {
	return &Client{Client: onet.NewClient(cothority.Suite, ServiceName)}
}

// SignatureRequest sends a CoSi sign request to the Cothority defined by the given
// Roster
func (c *Client) MulticastRequest(r *onet.Roster, root *onet.TreeNode, msg []byte) (*BooleanResponse, error) {

	return nil, nil
}
func (c *Client) InitRequest(dst *network.ServerIdentity, Nodes []*gentree.LocalityNode,
	ServerIdentityToName map[*network.ServerIdentity]string) (*BooleanResponse, error) {

	log.Lvl1("called", dst.String())
	log.Lvl1(Nodes)
	log.Lvl1(ServerIdentityToName)

	serviceReq := &InitRequest{
		Nodes:                Nodes,
		ServerIdentityToName: ServerIdentityToName,
	}

	log.Lvl1("Sending init message to", dst)
	reply := &BooleanResponse{}
	err := c.SendProtobuf(dst, serviceReq, reply)
	log.Lvl1(err, "Done")
	if err != nil {
		return nil, err
	}
	return reply, nil
}
