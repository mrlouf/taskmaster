package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/logger"
	"github.com/mrlouf/taskmaster/internal/protocol"
	"github.com/mrlouf/taskmaster/internal/server"
	"github.com/mrlouf/taskmaster/internal/supervisor"
)

func waitForSignals(s *supervisor.Supervisor) {

	// Handle graceful shutdown on SIGINT and SIGTERM
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	s.Logger.Log("Received shutdown signal, exiting...")

	event := supervisor.Event{
		Kind:   supervisor.EventShutdown,
		RespCh: make(chan protocol.Response),
	}

	s.Events <- event
	resp := <-event.RespCh
	if !resp.Ok {
		s.Logger.Log(fmt.Sprintf("Failed to shutdown supervisor gracefully: %s", resp.Msg))
	}

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
