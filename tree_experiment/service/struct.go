package service

import "go.dedis.ch/onet/v3"

/*
Struct holds the messages that will be sent around in the protocol. You have
to define each message twice: once the actual message, and a second time
with the `*onet.TreeNode` embedded. The latter is used in the handler-function
so that it can find out who sent the message.
*/

// Name can be used from other packages to refer to this protocol.
const LocalityProtocolName = "LocalityPresProtocol"

// Announce is used to pass a message to all children.
type Announce struct {
	Message int
}

// StructAnnounce just contains Announce and the data necessary to identify and
// process the message in the sda framework.
type StructAnnounce struct {
	*onet.TreeNode
	Announce
}

// Reply returns the count of all children.
type Reply struct {
	Received int
}

// StructReply just contains Reply and the data necessary to identify and
// process the message in the sda framework.
type StructReply struct {
	*onet.TreeNode
	Reply
}
