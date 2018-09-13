package config

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strings"
)

// Config describes the config of the system.
type Config struct {
	// Net should be tcp4 or tcp6.
	Net string `json:"net"`
	// AddrStr is the local address string.
	AddrStr string `json:"address"`
	// Peers is peer list.
	Peers []string `json:"-"`
	// LocalTCPAddr is TCP address parsed from
	// Net and AddrStr.
	LocalTCPAddr *net.TCPAddr `json:"-"`
	// AViewMinSize is the minimum size of the active view.
	AViewMinSize int `json:"active_view_min"`
	// AViewMaxSize is the maximum size of the active view.
	AViewMaxSize int `json:"active_view_max"`
	// PViewSize is the size of the passive view.
	PViewSize int `json:"passive_view"`
	// Ka is the number of nodes to choose from active view
	// when shuffling views.
	Ka int `json:"ka"`
	// Kp is the number of nodes to choose from passive view
	// when shuffling views.
	Kp int `json:"kb"`
	// Active Random Walk Length.
	ARWL int `json:"arwl"`
	// Passive Random Walk Length.
	PRWL int `json:"prwl"`
	// Shuffle Random Walk Length.
	SRWL int `json:"srwl"`
	// Message life.
	MLife int `json:"message_life"`
	// Shuffle Duration in seconds.
	ShuffleDuration int `json:"shuffle_duration"`
	// Heal Duration in seconds.
	HealDuration int `json:"heal_duration"`
	// The REST server address.
	RESTAddrStr string `json:"rest_addr"`
	// The path to user message handler(script).
	UserMsgHandler string `json:"user_message_handler"`
	// The duration to purge message buffer.
	PurgeDuration int `json:"purge_duration"`
}

func ParseConfig() (*Config, error) {
	var peerStr string
	var peerFile string

	cfg := new(Config)

	flag.StringVar(&cfg.Net, "net", "tcp", "The network protocol")
	flag.StringVar(&cfg.AddrStr, "addr", ":8424", "The address the agent listens on")

	flag.StringVar(&peerFile, "peer-file", "", "Peer list file")
	flag.StringVar(&peerStr, "peers", "", "Comma-separated list of peers")

	flag.IntVar(&cfg.AViewMinSize, "min-aview-size", 3, "The minimum size of the active view")
	flag.IntVar(&cfg.AViewMaxSize, "max-aview-size", 5, "The maximum size of the active view")
	flag.IntVar(&cfg.PViewSize, "pview-size", 30, "The size of the passive view")

	flag.IntVar(&cfg.Ka, "ka", 1, "The number of active nodes to shuffle")
	flag.IntVar(&cfg.Kp, "kp", 3, "The number of passive nodes to shuffle")

	flag.IntVar(&cfg.ARWL, "arwl", 5, "The active random walk length")
	flag.IntVar(&cfg.PRWL, "prwl", 3, "The passive random walk length")
	flag.IntVar(&cfg.SRWL, "srwl", 5, "The shuffle random walk length")

	flag.IntVar(&cfg.MLife, "msg-life", 5000, "The default message life (milliseconds)")
	flag.IntVar(&cfg.ShuffleDuration, "shuffle-duration", 5, "The default shuffle duration (seconds)")
	flag.IntVar(&cfg.HealDuration, "heal", 1, "The default heal duration (seconds)")
	flag.StringVar(&cfg.RESTAddrStr, "rest-addr", ":9424", "The address of the REST server")
	flag.StringVar(&cfg.UserMsgHandler, "user-message-handler", "", "The path to the user message handler script")
	flag.IntVar(&cfg.PurgeDuration, "purge-duration", 5000, "The default purge duration (milliseconds)")

	flag.Parse()

	// Check configuration.
	if peerStr != "" {
		cfg.Peers = strings.Split(peerStr, ",")
	}
	if peerFile != "" {
		peers, err := parsePeerFile(peerFile)
		if err != nil {
			return nil, err
		}
		cfg.Peers = peers
	}

	// Check agent server address.
	tcpAddr, err := net.ResolveTCPAddr(cfg.Net, cfg.AddrStr)
	if err != nil {
		return nil, err
	}
	cfg.LocalTCPAddr = tcpAddr

	// Check REST API address.
	_, err = net.ResolveTCPAddr(cfg.Net, cfg.RESTAddrStr)
	if err != nil {
		return nil, err
	}

	// Check User Message Handler.
	if cfg.UserMsgHandler != "" {
		_, err = exec.LookPath(cfg.UserMsgHandler)
		if err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func parsePeerFile(path string) ([]string, error) {
	var peers []string
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &peers); err != nil {
		return nil, err
	}
	return peers, nil
}

func (cfg *Config) ShufflePeers() []string {
	shuffledPeers := make([]string, len(cfg.Peers))
	copy(shuffledPeers, cfg.Peers)
	for i := range shuffledPeers {
		if i == 0 {
			continue
		}
		swapIndex := rand.Intn(i)
		shuffledPeers[i], shuffledPeers[swapIndex] = shuffledPeers[swapIndex], shuffledPeers[i]
	}
	return shuffledPeers
}
