package service

import (
	"github.com/dedis/paper_nyle/gentree"
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/network"
)

//Represents The actual graph that will be linked to the Binary Tree of the Protocol
type GraphTree struct {
	Tree        *onet.Tree
	ListOfNodes []*onet.TreeNode
	Parents     map[*onet.TreeNode][]*onet.TreeNode
}

type InitRequest struct {
	Nodes                []*gentree.LocalityNode
	ServerIdentityToName map[*network.ServerIdentity]string
}

// SignatureResponse is what the Cosi service will reply to clients.
type InitResponse struct {
}

type StartBFTCosiReq struct {
	SenderName string
	ReqId      int
}

type ReplyBFTCosiReq struct {
	ReqId int
}



