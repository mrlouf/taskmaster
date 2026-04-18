package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/logger"
	"github.com/mrlouf/taskmaster/internal/server"
	"github.com/mrlouf/taskmaster/internal/supervisor"
)

func main() {
	// Try and catch equivalent in Go
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger, err := logger.New()
	if err != nil {
		return fmt.Errorf("failed to initialise logger: %w", err)
	}
	defer logger.Close()

	server, err := server.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to initialise server: %w", err)
	}
	supervisor := supervisor.New(cfg, logger)

	go supervisor.Start()

	go server.Start()

	// Handle graceful shutdown on SIGINT and SIGTERM
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	os.Remove("/tmp/taskmaster.sock")
	os.Exit(1)

	return nil

}
