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

func waitForSignals(supervisor *supervisor.Supervisor) {

	// Handle graceful shutdown on SIGINT and SIGTERM
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	supervisor.Logger.Log("Received shutdown signal, exiting...")
	// supervisor.Shutdown() // TODO
	os.Remove("/tmp/taskmaster.sock")
	os.Exit(0)

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

	supervisor := supervisor.New(cfg, logger)

	server, err := server.New(cfg, logger, supervisor)
	if err != nil {
		return fmt.Errorf("failed to initialise server: %w", err)
	}

	go supervisor.Start()

	// ? Server needs to wait for supervisor to be ready to avoid data race
	// ? A better way to do this? Maybe a WaitGroup?
	<-supervisor.Ready

	go server.Start()

	waitForSignals(supervisor)

	return nil

}

func main() {
	// Try and catch equivalent in Go
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
