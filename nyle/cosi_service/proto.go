package service

import (
	"time"

	"github.com/dedis/paper_nyle/nyle/simplechain"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/network"
)

func init() {
	network.RegisterMessages(&AddTxReq{}, &AddTxResp{},
		&BFTCoSiSimul{}, &BFTCoSiSimulResp{})
}

// AddTxReq is an add transaction request.
type AddTxReq struct {
	simplechain.Tx
}

// AddTxResp is the response of AddTxReq.
type AddTxResp struct {
}

// BFTCoSiSimul is the message to start the bftcosi simulation.
type BFTCoSiSimul struct {
	Roster onet.Roster
	ReqId  int
}

// BFTCoSiSimulResp is the response of the simulation.
type BFTCoSiSimulResp struct {
	Duration time.Duration
}
