package agent

import (
	"errors"

	log "github.com/lilymona/gog/logging"
	"github.com/lilymona/gog/message"
	"github.com/lilymona/gog/node"

	"github.com/gogo/protobuf/proto"
)

var (
	ErrInvalidMessageType = errors.New("Invalid message type")
	ErrNoAvailablePeers   = errors.New("No available peers")
)

// disconnect() sends a Disconnect message to the node and close the connection.
// TODO(yifan): cache the connection.
func (ag *agent) disconnect(node *node.Node) {
	msg := &message.Disconnect{Id: proto.Uint64(ag.id)}
	ag.codec.WriteMsg(msg, node.Conn) // TODO record err log.
	node.Conn.Close()
}

// forwardJoin() sends a ForwardJoin message to the node. The message
// will include the Id and Addr of the source node, as the receiver might
// use these information to establish a connection.
func (ag *agent) forwardJoin(node, newNode *node.Node, ttl uint32) {
	msg := &message.ForwardJoin{
		Id:         proto.Uint64(ag.id),
		SourceId:   proto.Uint64(newNode.Id),
		SourceAddr: proto.String(newNode.Addr),
		Ttl:        proto.Uint32(ttl),
	}
	if err := ag.codec.WriteMsg(msg, node.Conn); err != nil {
		node.Conn.Close()
	}
}

// join() sends a Join message, and wait for the reply.
func (ag *agent) join(node *node.Node) (bool, error) {
	msg := &message.Join{
		Id:   proto.Uint64(ag.id),
		Addr: proto.String(ag.cfg.AddrStr),
	}
	if err := ag.codec.WriteMsg(msg, node.Conn); err != nil {
		return false, err
	}
	recvMsg, err := ag.codec.ReadMsg(node.Conn)
	if err != nil {
		// TODO(yifan) log.
		return false, err
	}
	reply, ok := recvMsg.(*message.JoinReply)
	if !ok {
		return false, ErrInvalidMessageType
	}
	node.Id = reply.GetId()
	return reply.GetAccept(), nil
}

// replyJoin() sends a the JoinReply message to the node.
func (ag *agent) replyJoin(node *node.Node, accept bool) error {
	msg := &message.JoinReply{
		Id:     proto.Uint64(ag.id),
		Accept: proto.Bool(accept),
	}
	return ag.codec.WriteMsg(msg, node.Conn)
}

// neighbor() sends a Neighbor message, and wait for the reply.
func (ag *agent) neighbor(node *node.Node, priority message.Neighbor_Priority) (bool, error) {
	msg := &message.Neighbor{
		Id:       proto.Uint64(ag.id),
		Addr:     proto.String(ag.cfg.AddrStr),
		Priority: priority.Enum(),
	}
	if err := ag.codec.WriteMsg(msg, node.Conn); err != nil {
		// TODO(yifan) log.
		return false, err
	}
	recvMsg, err := ag.codec.ReadMsg(node.Conn)
	if err != nil {
		// TODO(yifan) log.
		return false, err
	}
	reply, ok := recvMsg.(*message.NeighborReply)
	if !ok {
		return false, ErrInvalidMessageType
	}

	return reply.GetAccept(), nil
}

// replyNeighbor() sends a the NeighborReply message to the node.
func (ag *agent) replyNeighbor(node *node.Node, accept bool) error {
	msg := &message.NeighborReply{
		Id:     proto.Uint64(ag.id),
		Accept: proto.Bool(accept),
	}
	return ag.codec.WriteMsg(msg, node.Conn)
}

// userMessage() sends a user message to the node.
func (ag *agent) userMessage(node *node.Node, msg proto.Message) {
	if err := ag.codec.WriteMsg(msg, node.Conn); err != nil {
		log.Errorf("Agent.userMessage(): Write msg error: %v", err)
		// Record this message, so we can resend it later.
		umsg := msg.(*message.UserMessage)
		hash := hashMessage(umsg.GetPayload())

		ag.failmsgBuffer.Lock()
		ag.failmsgBuffer.Append(hash, msg)
		ag.failmsgBuffer.Unlock()

		node.Conn.Close()
	}
}

func (ag *agent) forwardShuffle(node *node.Node, msg *message.Shuffle) {
	msg.Id = proto.Uint64(ag.id)
	if err := ag.codec.WriteMsg(msg, node.Conn); err != nil {
		node.Conn.Close()
	}
}

func (ag *agent) shuffleReply(msg *message.Shuffle, candidates []*message.Candidate) error {
	// TODO use existing tcp.
	conn, err := ag.connect(msg.GetAddr())
	if err != nil {
		log.Errorf("Agent.shuffleReply(): Failed to connect %s: %v", msg.GetAddr(), err)
		return err
	}
	defer conn.Close()
	reply := &message.ShuffleReply{
		Id:         proto.Uint64(ag.id),
		Candidates: candidates,
	}
	if err := ag.codec.WriteMsg(reply, conn); err != nil {
		// TODO log
		return err
	}
	return nil
}

func (ag *agent) shuffle(node *node.Node, candidates []*message.Candidate) {
	msg := &message.Shuffle{
		Id:         proto.Uint64(ag.id),
		SourceId:   proto.Uint64(ag.id),
		Addr:       proto.String(ag.cfg.AddrStr),
		Candidates: candidates,
		Ttl:        proto.Uint32(uint32(ag.cfg.SRWL)),
	}
	if err := ag.codec.WriteMsg(msg, node.Conn); err != nil {
		node.Conn.Close()
	}
}
