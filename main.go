package main

import (
	"github.com/lilymona/gog/config"
	log "github.com/lilymona/gog/logging"
	"github.com/lilymona/gog/rest"
)

func main() {
	cfg, err := config.ParseConfig()
	if err != nil {
		log.Fatalf("Failed to parse configuration: %v\n", err)
	}

	srv := rest.NewServer(cfg)
	log.Infof("Starting server...\n")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start server: %v\n", err)
	}
	return
}
