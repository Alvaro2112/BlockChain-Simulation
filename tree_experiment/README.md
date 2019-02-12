# Experiment for fault tolerance in trees
This directory contains the experiment of fault-tolerant trees. Normally, when
we build an overlay in the form of a stretch-tree, if the higher-level nodes
fail, then a lot of the messages will be lost. To overcome that problem, we
build multiple trees and send our messages in all of them to mitigate node
failure. This experiment explores the relationship between the level of fault
tolerance (the number of node failures) and the overhead in terms of bandwidth.

## Implementation ideas
We assume every node knows the overlay structure. We need a data structure to
capture this strcuture, let's call it `OverlayGraphs`. To send a message from
`A` to `B`, `A` extracts some paths from `OverlayGraphs` that leads to `B`. A
path is a `onet.Tree`, that can be used to create a protocol. Note that for
sending each message, `A` should extract multiple paths in case one or more of
them are faulty. The protocol is basically a chain of `p.SendToChildren`.

Before the start of the simulation, we'll pick some faulty that do not forward
any messages. To start the simulation, every node sends a message with a unique
ID to another node selected at random (in the simplest and non-local case).
We'll record every send and every successful receive and check the proportion
of missing messages.
