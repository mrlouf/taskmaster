package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"taskmaster/internal/config"
	"taskmaster/internal/logger"
	"taskmaster/internal/protocol"
	"taskmaster/internal/server"
	"taskmaster/internal/supervisor"
)

func handleSigterm(s *supervisor.Supervisor) {
	// Handle graceful shutdown on SIGINT and SIGTERM
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

func handleSighup(s *supervisor.Supervisor) {
	//To manage SIGHUP signals which leads to a reload command
	s.Logger.Log("Received SIGHUP signal, reloading...")

	event := supervisor.Event{
		Kind:   supervisor.EventReloadConfig,
		RespCh: make(chan protocol.Response),
	}
	s.Events <- event
	resp := <-event.RespCh
	if !resp.Ok {
		s.Logger.Log(fmt.Sprintf("Failed to reload supervisor gracefully: %s", resp.Msg))
	}
}

func waitForSignals(s *supervisor.Supervisor) {
	//Management of SIGTERM, SIGINT and SIGHUP signals. SIGTERM and SIGINT trigger graceful shutdown
	// while SIGHUP triggers a config reload

	for {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		sig := <-c
		switch sig {
		case os.Interrupt, syscall.SIGTERM:
			handleSigterm(s)
		case syscall.SIGHUP:
			handleSighup(s)
		}
	}
}

func run() error {
	var pid int
	pid = os.Getpid()

	cfg, err := config.LoadConfig("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	//logger should start ASAP
	logger, err := logger.New()
	if err != nil {
		return fmt.Errorf("failed to initialise logger: %w", err)
	}
	defer logger.Close()

	supervisor := supervisor.New(cfg, logger, cfg.MaxEvent)

	server, err := server.New(cfg, logger, supervisor, pid)
	if err != nil {
		return fmt.Errorf("failed to initialise server: %w", err)
	}

	go supervisor.Start()

	<-supervisor.Ready

	go server.Start()
	// also we could run the waitingSignals in a specific go channel with a "select {}" to keep an infinite loop. That way you start capturing signals much earlier in the process (from the start)
	waitForSignals(supervisor)

	return nil

}

func main() {
	// Try and catch equivalent in Go
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
