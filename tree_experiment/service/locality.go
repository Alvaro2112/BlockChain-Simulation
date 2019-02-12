package service

import (
	"fmt"
	"io"

	"go.dedis.ch/onet/v3"
)

func checkErr(e error) {
	if e != nil && e != io.EOF {
		fmt.Print(e)
		panic(e)
	}
}

//Coumputes A Binary Tree Based On A Graph
func (s *Service) CreateBinaryTreeFromGraphTree(GraphTree GraphTree) *onet.Tree {

	BinaryTreeRoster := GraphTree.Tree.Roster
	Tree := BinaryTreeRoster.GenerateBinaryTree()

	return Tree
}
