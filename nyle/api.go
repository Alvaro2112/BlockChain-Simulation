package nyle

import (
	"github.com/dedis/paper_nyle/gentree"
	cosi_service "github.com/dedis/paper_nyle/nyle/cosi_service"
	"go.dedis.ch/cothority/v3"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/log"
	"go.dedis.ch/onet/v3/network"
)

// Client is a structure to communicate with the nyle service.
type Client struct {
	*onet.Client
}

// NewClient instantiates a new nyle.Client.
func NewClient() *Client {
	return &Client{Client: onet.NewClient(cothority.Suite, cosi_service.Name)}
}

// InitRequest ...
func (c *Client) InitRequest(dst *network.ServerIdentity, Nodes []*gentree.LocalityNode,
	ServerIdentityToName map[*network.ServerIdentity]string) (*cosi_service.InitResponse, error) {

	log.Lvl1("called", dst.String())
	log.Lvl1(Nodes)
	log.Lvl1(ServerIdentityToName)

	serviceReq := &cosi_service.InitRequest{
		Nodes:                Nodes,
		ServerIdentityToName: ServerIdentityToName,
	}

	log.LLvl1("Sending init message to", dst)
	reply := &cosi_service.InitResponse{}
	err := c.SendProtobuf(dst, serviceReq, reply)
	log.Lvl1(err, "Done with init request")
	if err != nil {
		return nil, err
	}
	return reply, nil
}

// StartCoSi ...
func (c *Client) StartCoSi(dst *network.ServerIdentity, roster *onet.Roster, reqId int) (*cosi_service.BFTCoSiSimulResp, error) {
	req := cosi_service.BFTCoSiSimul{
		Roster: *roster,
		ReqId:  reqId,
	}

	log.Lvl1("Client sending req to", dst.String(), reqId)

	var resp cosi_service.BFTCoSiSimulResp

	err := c.SendProtobuf(dst, &req, &resp)

	log.LLvl1("Client done with ", dst.String(), reqId)

	if err != nil {
		return nil, err
	}
	return &resp, nil
}
