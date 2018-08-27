package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"

	"github.com/lilymona/gog/agent"
	"github.com/lilymona/gog/config"
	log "github.com/lilymona/gog/logging"
)

const (
	listURL      = "/api/list"
	joinURL      = "/api/join"
	broadcastURL = "/api/broadcast"
	configURL    = "/api/config"
	leaveURL     = "/api/leave"
)

var (
	errInvalidMethod = errors.New("server: Invalid method")
)

// RESTServer handles RESTful requests for gog agent.
type RESTServer struct {
	cfg *config.Config
	ag  agent.Agent
	mux *http.ServeMux
}

// NewServer creates a new RESTful server for gog agent.
// It will also starts the agent server.
func NewServer(cfg *config.Config) *http.Server {
	handler := NewRESTServer(cfg)
	return &http.Server{
		Addr:    cfg.RESTAddrStr,
		Handler: handler,
	}
}

// NewRESTServer creates an http.Handler to handle HTTP requests.
func NewRESTServer(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()
	ag := agent.NewAgent(cfg)
	rh := &RESTServer{cfg, ag, mux}
	rh.RegisterAPI(mux)

	// Register a user message handler.
	ag.RegisterMessageHandler(rh.UserMessagHandler)

	// Start the agent server.
	go func() {
		if err := ag.Serve(); err != nil {
			log.Fatalf("server.NewServer(): Agent failed to serve: %v\n", err)
		}
	}()
	return rh
}

// registerAPI registers the api urls.
func (rh *RESTServer) RegisterAPI(mux *http.ServeMux) {
	mux.HandleFunc(listURL, rh.List)
	mux.HandleFunc(joinURL, rh.Join)
	mux.HandleFunc(broadcastURL, rh.Broadcast)
	mux.HandleFunc(configURL, rh.Config)
	mux.HandleFunc(leaveURL, rh.Leave)
	return
}

// List lists the views.
func (rh *RESTServer) List(w http.ResponseWriter, r *http.Request) {
	b, err := rh.ag.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(b))
	return
}

// Join joins the agent to a cluster.
func (rh *RESTServer) Join(w http.ResponseWriter, r *http.Request) {
	var peers []string

	if r.Method != "POST" {
		http.Error(w, errInvalidMethod.Error(), http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	peer := r.Form.Get("peer")

	// Join a single peer.
	if peer != "" {
		if err := rh.ag.Join(peer); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	// Join a cluster.
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(b, &peers); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := rh.ag.Join(peers...); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	return
}

// Broadcast broadcasts the message to the cluster
func (rh *RESTServer) Broadcast(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	msg := r.Form.Get("message")
	if msg != "" {
		log.Infof("Broadcasting: %s\n", msg)
		if err := rh.ag.Broadcast([]byte(msg)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	return
}

// Config get/set the current configuration.
func (rh *RESTServer) Config(w http.ResponseWriter, r *http.Request) {
	b, err := json.Marshal(rh.cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, string(b))
}

// Leave makes the agent to exit.
func (rh *RESTServer) Leave(w http.ResponseWriter, r *http.Request) {
	rh.ag.Leave()
}

// UserMessagHandler is the handler for user messages. It will run a script
// specified by the configuration.
func (rh *RESTServer) UserMessagHandler(msg []byte) {
	if rh.cfg.UserMsgHandler == "" {
		return
	}
	cmd := exec.Command(rh.cfg.UserMsgHandler, string(msg))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Errorf("server.UserMessageHandler(): Failed to run command: %v\n", err)
	}
}

// ServeHTTP implements the http.Handler for RESTServer.
// It will get the handler from mux and invoke the handler.
func (rh *RESTServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h, _ := rh.mux.Handler(r)
	h.ServeHTTP(w, r)
}
