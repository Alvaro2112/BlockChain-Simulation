package service

import (
	"go.dedis.ch/onet/v3"
	"go.dedis.ch/onet/v3/network"
)

// Multicast is a generic protocol that performs a multicast along a crux-tree
// structure.
type Multicast struct {
	*onet.TreeNodeInstance
	callback func(*network.ServerIdentity)
}

// NewMulticastProtocol returns a protocol instance which can be used to
// register the protocol.
func NewMulticastProtocol(callback func(*network.ServerIdentity)) onet.NewProtocol {
	return func(n *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
		p := &Multicast{
			TreeNodeInstance: n,
			callback:         callback,
		}
		return p, nil
	}
}

// Start should be called on the root to start the protocol.
func (p *Multicast) Start() error {
	return nil
}

// Dispatch runs the protocol.
func (p *Multicast) Dispatch() error {
	defer p.Done()
	return nil
}
