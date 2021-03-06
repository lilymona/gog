package agent

import (
	"crypto/sha1"
	"encoding/json"
	"math/rand"
	"net"
	"os"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/lilymona/gog/arraymap"
	"github.com/lilymona/gog/codec"
	"github.com/lilymona/gog/config"
	log "github.com/lilymona/gog/logging"
	"github.com/lilymona/gog/message"
	"github.com/lilymona/gog/node"
)

// MessageHandler is the message handler.
type MessageHandler func([]byte)

// Agent describes the interface of an agent.
type Agent interface {
	// Serve starts a standalone agent, waiting for
	// incoming connections.
	Serve() error
	// Join joins the peers.
	Join(peerAddrs ...string) error
	// Leave causes the agent to leave the cluster.
	Leave()
	// Broadcast broadcasts a message to the cluster.
	Broadcast(msg []byte) error
	// RegisterMessageHandler registers a user provided callback.
	RegisterMessageHandler(mh MessageHandler)
	// List prints the infomation in two views.
	List() ([]byte, error)
}

// agent implements the Agent interface.
type agent struct {
	// The id of the agent.
	id uint64
	// Configuration.
	cfg *config.Config
	// Active View.
	aView *arraymap.ArrayMap
	// Passive View.
	pView *arraymap.ArrayMap
	// TCP listener.
	ln *net.TCPListener
	// The codec.
	codec codec.Codec
	// Message buffer.
	msgBuffer *arraymap.ArrayMap
	// FaildMessage buffer.
	failmsgBuffer *arraymap.ArrayMap
	// The user message callback.
	msgHandler MessageHandler
}

// view is a struct that encapsulates the active and passive
// view. It is for creating json files.
type view struct {
	// Active View.
	AView *arraymap.ArrayMap `json:"active_view"`
	// Passive View.
	PView *arraymap.ArrayMap `json:"passive_view"`
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func GenID() (n uint64) {
	for n == 0 {
		n = uint64(rand.Int63())
	}
	return
}

// NewAgent creates a new agent.
func NewAgent(cfg *config.Config) Agent {
	// Create a codec and register messages.
	codec := codec.NewProtobufCodec()
	codec.Register(&message.UserMessage{})
	codec.Register(&message.Join{})
	codec.Register(&message.JoinReply{})
	codec.Register(&message.ForwardJoin{})
	codec.Register(&message.Neighbor{})
	codec.Register(&message.NeighborReply{})
	codec.Register(&message.Disconnect{})
	codec.Register(&message.Shuffle{})
	codec.Register(&message.ShuffleReply{})

	return &agent{
		id:            GenID(),
		cfg:           cfg,
		codec:         codec,
		aView:         arraymap.NewArrayMap(),
		pView:         arraymap.NewArrayMap(),
		msgBuffer:     arraymap.NewArrayMap(),
		failmsgBuffer: arraymap.NewArrayMap(),
	}
}

// Serve starts a standalone agent, waiting for
// incoming connections.
func (ag *agent) Serve() error {
	ln, err := net.ListenTCP(ag.cfg.Net, ag.cfg.LocalTCPAddr)
	if err != nil {
		log.Errorf("Serve() Cannot listen %v\n", err)
		return err
	}
	go ag.healLoop()
	go ag.shuffleLoop()
	ag.ln = ln
	ag.serve()
	return nil
}

// serve listens on the TCP listener, waits for incoming connections.
func (ag *agent) serve() {
	for {
		conn, err := ag.ln.AcceptTCP()
		if err != nil {
			log.Errorf("Agent.serve(): Failed to accept\n")
			continue
		}
		// TODO(Yifan): Set read time ount.
		go ag.serveConn(conn)
	}
}

// serveConn() serves a connection.
func (ag *agent) serveConn(conn *net.TCPConn) {
	for {
		msg, err := ag.codec.ReadMsg(conn)
		if err != nil {
			log.Errorf("Agent.serveConn(): Failed to decode message: %v\n", err)
			return
		}
		// Dispatch messages.
		switch t := msg.(type) {
		case *message.Join:
			if ag.handleJoin(conn, msg.(*message.Join)) {
				return
			}
		case *message.Neighbor:
			if ag.handleNeighbor(conn, msg.(*message.Neighbor)) {
				return
			}
		case *message.ShuffleReply:
			ag.handleShuffleReply(msg.(*message.ShuffleReply))
		default:
			log.Errorf("Agent.serveConn(): Unexpected message type: %T\n", t)
			return
		}
	}
}

// serveNode() serves a node's connection.
func (ag *agent) serveNode(node *node.Node) {
	for {
		msg, err := ag.codec.ReadMsg(node.Conn)
		if err != nil {
			log.Errorf("Agent.serveNode(): Failed to decode message: %v\n", err)
			ag.replaceActiveNode(node)
			return
		}
		// Dispatch messages.
		switch t := msg.(type) {
		case *message.ForwardJoin:
			ag.handleForwardJoin(msg.(*message.ForwardJoin))
		case *message.Disconnect:
			ag.replaceActiveNode(node)
			return
		case *message.Shuffle:
			ag.handleShuffle(msg.(*message.Shuffle))
		case *message.UserMessage:
			ag.handleUserMessage(node, msg.(*message.UserMessage))
		default:
			log.Errorf("Agent.serveNode(): Unexpected message type: %T\n", t)
			ag.replaceActiveNode(node)
			return
		}
	}
}

func (ag *agent) healLoop() {
	ticker := time.NewTicker(time.Duration(ag.cfg.HealDuration) * time.Second)
	defer ticker.Stop()
	for t := range ticker.C {
		// ag.aView.Lock()
		// ag.pView.Lock()
		// if ag.aView.Len() < ag.cfg.AViewMinSize {
		// 	if ag.pView.Len() > 0 {
		// 		nd := chooseRandomNode(ag.pView, 0)
		// 		ag.neighbor(nd, message.Neighbor_Low)
		// 	}
		// }
		// ag.aView.Unlock()
		// ag.pView.Unlock()

		ag.aView.RLock()
		len := ag.aView.Len()
		ag.aView.RUnlock()
		if len == 0 {
			log.Warningf("Lost all peers! Join again\n")
			if err := ag.Join(ag.cfg.ShufflePeers()...); err != nil {
				log.Warningf("No available peers, need a new list!")
			}
		}
	}
}

func (ag *agent) shuffleLoop() {
	tick := time.Tick(time.Duration(ag.cfg.ShuffleDuration) * time.Second)
	for {
		select {
		case <-tick:
			ag.aView.RLock()
			ag.pView.RLock()
			if ag.aView.Len() == 0 {
				ag.aView.RUnlock()
				ag.pView.RUnlock()
				continue
			}
			node := chooseRandomNode(ag.aView, 0)
			if node == nil {
				continue
			}
			list := ag.makeShuffleList()
			ag.aView.RUnlock()
			ag.pView.RUnlock()
			go ag.shuffle(node, list)
		}
	}
}

func (ag *agent) makeShuffleList() []*message.Candidate {
	candidates := make([]*message.Candidate, 0, 1+ag.cfg.Ka+ag.cfg.Kp)
	self := &message.Candidate{
		Id:   proto.Uint64(ag.id),
		Addr: proto.String(ag.cfg.AddrStr),
	}
	candidates = append(candidates, self)
	candidates = append(candidates, chooseRandomCandidates(ag.aView, ag.cfg.Ka)...)
	candidates = append(candidates, chooseRandomCandidates(ag.pView, ag.cfg.Kp)...)
	return candidates
}

// addNodeActiveView() adds the node to the active view. If
// the active view is full, it will move one node from the active
// view to the passive view before adding the node.
// If the passive view is also full, it will drop a random node
// in the passive view.
func (ag *agent) addNodeActiveView(node *node.Node) {
	if !ag.aView.Has(node.Id) {
		for ag.aView.Len() >= ag.cfg.AViewMaxSize {
			n := chooseRandomNode(ag.aView, 0)
			ag.aView.Remove(n.Id)
			go ag.disconnect(n)
			ag.addNodePassiveView(n)
			//ag.pView.Add(n.Id, n)
		}
	}
	go ag.serveNode(node)
	if old := ag.aView.Add(node.Id, node); old != nil {
		old.(*node.Node).Conn.Close()
	}
}

// addNodePassiveView() adds a node to the passive view. If
// the passive view is full, it will drop a random node.
func (ag *agent) addNodePassiveView(node *node.Node) {
	if node.Id == ag.id || ag.aView.Has(node.Id) || ag.pView.Has(node.Id) {
		return
	}
	for ag.pView.Len() >= ag.cfg.PViewSize {
		n := chooseRandomNode(ag.pView, 0)
		ag.pView.Remove(n.Id)
	}
	ag.pView.Add(node.Id, node)
}

// replaceActiveNode() replaces a "dead" node in the active
// view with a node randomly chosen from the passive view.
func (ag *agent) replaceActiveNode(node *node.Node) {
	// TODO add the node to passive view instead of removing.
	ag.aView.Lock()
	if !ag.aView.Remove(node.Id) {
		ag.aView.Unlock()
		return
	}
	ag.aView.Unlock()
	node.Conn.Close()

	ag.pView.RLock()
	for {
		nd := chooseRandomNode(ag.pView, 0)
		ag.pView.RUnlock()
		if nd == nil {
			log.Warningf("No nodes in passive view\n")
			break
		}

		if conn, err := ag.connect(nd.Addr); err != nil {
			log.Errorf("Agent.replaceActiveNode(): Failed to connect %s: %v, drop from passive view.", nd.Addr(), err)
			ag.pView.Lock()
			ag.pView.Remove(nd.Id)
			ag.pView.Unlock()
			continue
		} else {
			nd.Conn = conn
		}

		priority := message.Neighbor_Low
		if ag.aView.Len() == 0 {
			message.Neighbor_High
		}
		if accepted, err := ag.neighbor(nd, priority); err != nil {
			log.Errorf("Agent.replaceActiveNode(): Failed to neighbor: %v\n", err)
			nd.Conn.Close()
		} else if accpeted {
			ag.aView.Lock()
			ag.pView.Lock()
			ag.pView.Remove(nd.Id)
			ag.addNodeActiveView(nd)
			ag.aView.Unlock()
			ag.pView.Unlock()
			break
		}
	}

	ag.aView.RLock()
	ag.pView.Lock()
	ag.addNodePassiveView(node)
	ag.pView.Unlock()
	ag.aView.RunLock()

	ag.resendFailedMessages()
}

// Resend failed messages if any.
// NOTE: The view locks should already be held when invoking this function.
func (ag *agent) resendFailedMessages() {

	// Should not use defer unlock to prevent deadlock,
	// because in userMessage() we will probably lock again.
	ag.failmsgBuffer.Lock()
	values := ag.failmsgBuffer.Values()
	ag.failmsgBuffer.RemoveAll()
	ag.failmsgBuffer.Unlock()

	// We have already lock the view, so do not need locks here.
	for _, v := range values {
		log.Debugf("Resending message %v\n", v)
		msg := v.(*message.UserMessage)
		for _, vv := range ag.aView.Values() {
			nd := vv.(*node.Node)
			go ag.userMessage(nd, msg)
		}
	}
	return
}

// handleJoin() handles Join message. If it accepts the request, it will add
// the node in the active view. As specified by the protocol, a node should
// always accept Join requests.
func (ag *agent) handleJoin(conn *net.TCPConn, msg *message.Join) (accept bool) {
	newNode := &node.Node{
		Id:   msg.GetId(),
		Addr: msg.GetAddr(),
		Conn: conn,
	}

	ag.aView.Lock()
	ag.pView.Lock()
	defer ag.aView.Unlock()
	defer ag.pView.Unlock()

	accept = newNode.Id != ag.id && !ag.aView.Has(newNode.Id)

	if err := ag.replyJoin(newNode, accept); err != nil {
		log.Errorf("Agent.handleJoin(): Failed to reply join: %v", err)
		node.Conn.Close()
		return false
	}

	if accept {
		ag.addNodeActiveView(newNode)

		// Send ForwardJoin message to all other the nodes in the active view.
		for _, v := range ag.aView.Values() {
			nd := v.(*node.Node)
			if nd != newNode {
				go ag.forwardJoin(nd, newNode, uint32(rand.Intn(ag.cfg.ARWL)))
			}
		}
	}
	return
}

// handleNeighbor() handles Neighbor message. If the request is high priority,
// the receiver will always accept the request and add the node to its active view.
// If the request is low priority, then the request will only be accepted when
// there are empty slot in the active view.
func (ag *agent) handleNeighbor(conn *net.TCPConn, msg *message.Neighbor) (accept bool) {
	newNode := &node.Node{
		Id:   msg.GetId(),
		Addr: msg.GetAddr(),
		Conn: conn,
	}

	ag.aView.Lock()
	ag.pView.Lock()
	defer ag.aView.Unlock()
	defer ag.pView.Unlock()

	accept = newNode.Id != ag.id && !ag.aView.Has(newNode.Id) && (msg.GetPriority() == message.Neighbor_High || ag.aView.Len() < ag.cfg.AViewMaxSize)

	if err := ag.replyNeighbor(newNode, accept); err != nil {
		log.Errorf("Agent.handleNeighbor(): Failed to reply neighbor: %v", err)
		node.Conn.Close()
		return false
	}
	if accept {
		ag.addNodeActiveView(newNode)
	}
	return
}

// handleForwardJoin() handles the ForwardJoin message, and decides whether
// it will add the original sender to the active view or passive view.
func (ag *agent) handleForwardJoin(msg *message.ForwardJoin) {
	ttl := msg.GetTtl()
	newNode := &node.Node{
		Id:   msg.GetSourceId(),
		Addr: msg.GetSourceAddr(),
	}

	ag.aView.Lock()
	ag.pView.Lock()
	defer ag.aView.Unlock()
	defer ag.pView.Unlock()

	if ttl == 0 || ag.aView.Len() <= 1 { // TODO(yifan): Loose this?
		if ag.id != newNode.Id && !ag.aView.Has(newNode.Id) {
			if conn, err := ag.connect(newNode.Addr); err != nil {
				log.Errorf("Agent.handleForwardJoin(): Failed to connect %s: %v.", newNode.Addr(), err)
			} else {
				newNode.Conn = conn
				if _, err = ag.neighbor(newNode, message.Neighbor_High); err != nil {
					log.Errorf("Agent.handleForwardJoin(): Failed to neighbor: %v", err)
				}
			}
		}
		return
	}
	if ttl == uint32(ag.cfg.PRWL) {
		ag.addNodePassiveView(newNode)
	}
	if node := chooseRandomNode(ag.aView, msg.GetId()); node != nil {
		go ag.forwardJoin(node, newNode, ttl-1)
	}
	return
}

// handleShuffle() handles Shuffle message. It will send back a ShuffleReply
// message and update it's views.
func (ag *agent) handleShuffle(msg *message.Shuffle) {
	ag.aView.Lock()
	ag.pView.Lock()
	defer ag.aView.Unlock()
	defer ag.pView.Unlock()

	ttl := msg.GetTtl()
	if ttl > 0 && ag.aView.Len() > 1 {
		node := chooseRandomNode(ag.aView, msg.GetId())
		msg.Ttl = proto.Uint32(ttl - 1)
		go ag.forwardShuffle(node, msg)
		return
	}

	candidates := msg.GetCandidates()
	replyCandidates := chooseRandomCandidates(ag.pView, len(candidates))
	go ag.shuffleReply(msg, replyCandidates)
	i := 0
	for _, candidate := range candidates {
		node := &node.Node{
			Id:   candidate.GetId(),
			Addr: candidate.GetAddr(),
		}
		//ag.addNodePassiveView(node)
		if node.Id == ag.id || ag.aView.Has(node.Id) || ag.pView.Has(node.Id) {
			continue
		}
		for ag.pView.Len() >= ag.cfg.PViewSize {
			if i < len(replyCandidates) {
				ag.pView.Remove(replyCandidates[i].GetId())
				i++
			} else {
				n := chooseRandomNode(ag.pView, 0)
				ag.pView.Remove(n.Id)
			}
		}
		ag.pView.Add(node.Id, node)
	}
	return
}

// handleShuffleReply() handles ShuffleReply message. It will update it's views.
func (ag *agent) handleShuffleReply(msg *message.ShuffleReply) {
	ag.aView.Lock()
	ag.pView.Lock()
	defer ag.aView.Unlock()
	defer ag.pView.Unlock()

	candidates := msg.GetCandidates()
	for _, candidate := range candidates {
		node := &node.Node{
			Id:   candidate.GetId(),
			Addr: candidate.GetAddr(),
		}
		ag.addNodePassiveView(node)
	}
	return
}

// handleUserMessage() handles user defined messages. It will forward the message
// to the nodes in its active view.
func (ag *agent) handleUserMessage(from *node.Node, msg *message.UserMessage) {
	// Test if the message is stale.
	deadline := msg.GetTs() + time.Millisecond.Nanoseconds()*int64(ag.cfg.MLife)
	now := time.Now().UnixNano()
	if now >= deadline {
		log.Debugf("Message is too old, deadline: %v, now %v\n", deadline, now)
		return
	}

	// Test if the message has been already received.
	hash := hashMessage(msg.GetPayload())

	ag.msgBuffer.Lock()
	defer ag.msgBuffer.Unlock()

	if ag.msgBuffer.Has(hash) {
		purgeDeadline := ag.msgBuffer.GetValueOf(hash)
		if purgeDeadline.(int64) >= now {
			log.Debugf("Message is alread received, and with purge deadline, hash: %v\n", hash)
			return
		}
		ag.msgBuffer.Remove(hash)
	}

	purgeDeadline := now + time.Millisecond.Nanoseconds()*int64(ag.cfg.PurgeDuration)
	ag.msgBuffer.Append(hash, purgeDeadline)

	// Invoke user's message handler.
	go ag.msgHandler(msg.GetPayload())

	ag.aView.Lock()
	defer ag.aView.Unlock()

	for _, v := range ag.aView.Values() {
		nd := v.(*node.Node)
		if nd.Id != from.Id {
			go ag.userMessage(nd, msg)
		}
	}
	return
}

func (ag *agent) connect(peerAddr string) (*net.TCPConn, error) {
	addr, err := net.ResolveTCPAddr(ag.cfg.Net, peerAddr)
	if err != nil {
		// TODO(yifan) log.
		return nil, err
	}
	conn, err := net.DialTCP(ag.cfg.Net, nil, addr)
	if err != nil {
		// TODO(yifan) log.
		return nil, err
	}
	return conn, nil
}

// Join joins the node to the cluster by contacting the nodes provied in the
// list.
func (ag *agent) Join(peerAddrs ...string) error {
	// Append the peer list.
	ag.cfg.Peers = append(ag.cfg.Peers, peerAddrs...)

	for _, peerAddr := range peerAddrs {
		log.Infof("Agent.Join(): Trying to join %s...\n", peerAddr)

		conn, err := ag.connect(peerAddr)
		if err != nil {
			log.Errorf("Agent.Join(): Failed to connect %s: %v\n", peerAddr, err)
		}
		node := &node.Node{Addr: peerAddr, Conn: conn}

		if accepted, err := ag.join(node); err != nil || !accepted {
			log.Errorf("Agent.Join(): Failed to join: accepted:%v, err:%v\n", accepted, err)
			node.Conn.Close()
			continue
		}
		// Successfully Joined.
		log.Infof("Successfully join node %s\n", peerAddr)
		ag.aView.Lock()
		ag.pView.Lock()
		defer ag.aView.Unlock()
		defer ag.pView.Unlock()
		ag.addNodeActiveView(node)
		return nil
	}
	return ErrNoAvailablePeers
}

// Leave causes the agent to leave the cluster.
func (ag *agent) Leave() {
	log.Infof("Agent is leaving...\n")
	os.Exit(0)
}

// Broadcast broadcasts a message to the cluster.
func (ag *agent) Broadcast(payload []byte) error {
	msg := &message.UserMessage{
		Id:      proto.Uint64(ag.id),
		Payload: payload,
		Ts:      proto.Int64(time.Now().UnixNano()),
	}

	ag.aView.Lock()
	defer ag.aView.Unlock()
	for _, v := range ag.aView.Values() {
		nd := v.(*node.Node)
		ag.userMessage(nd, msg)
	}
	return nil
}

// RegisterMessageHandler registers a user provided message callback
// to handle messages.
func (ag *agent) RegisterMessageHandler(mh MessageHandler) {
	ag.msgHandler = mh
}

// List() lists the active view and passive view.
func (ag *agent) List() ([]byte, error) {
	ag.aView.Lock()
	ag.pView.Lock()
	defer ag.aView.Unlock()
	defer ag.pView.Unlock()

	log.Debugf("AView:\n")
	for _, v := range ag.aView.Values() {
		log.Debugf("%v\n", v.(*node.Node))
	}
	log.Debugf("PView:\n")
	for _, v := range ag.pView.Values() {
		log.Debugf("%v\n", v.(*node.Node))
	}

	view := &view{ag.aView, ag.pView}
	return json.Marshal(view)
}

// Helpers

// hashMessage() returns the hash of a user message.
func hashMessage(msg []byte) [sha1.Size]byte {
	return sha1.Sum(msg)
}

// chooseRandomNode() chooses a random node from the active view
// or passive view.
func chooseRandomNode(view *arraymap.ArrayMap, excludeId uint64) *node.Node {
	if view.Len() == 0 {
		return nil
	}
	index := rand.Intn(view.Len())
	nd := view.GetValueAt(index).(*node.Node)
	if nd.Id == excludeId {
		if view.Len() == 1 {
			return nil
		}
		nd = view.GetValueAt((index + 1) % view.Len()).(*node.Node)
	}
	return nd
}

// chooseRandomCandidates() selects n random nodes from the active view
// or passive view. If n > the size of the view, then all nodes are returned.
func chooseRandomCandidates(view *arraymap.ArrayMap, n int) []*message.Candidate {
	if view.Len() == 0 {
		return nil
	}
	if n >= view.Len() {
		n = view.Len()
	}
	candidates := make([]*message.Candidate, n)
	index := rand.Intn(view.Len())
	for i := 0; i < n; i++ {
		nd := view.GetValueAt((index + i) % view.Len()).(*node.Node)
		candidates[i] = &message.Candidate{
			Id:   proto.Uint64(nd.Id),
			Addr: proto.String(nd.Addr),
		}
	}
	return candidates
}
