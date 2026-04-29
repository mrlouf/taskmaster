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

func handleSigterm(s *supervisor.Supervisor) {
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
	s.Logger.Log("Received SIGHUP signal, reloading...")
	fmt.Printf("[DEBUG]: processing SIGHUP and reloading config\n")

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

	// Handle graceful shutdown on SIGINT and SIGTERM
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

	//moved to keep pid value in the server struct
	//_ = os.Remove("/tmp/taskmasterd.pid")                   //remove if already exists
	//os.WriteFile("/tmp/taskmasterd.pid", []byte(strconv.Itoa(pid)), 0666) //permissions?

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

	supervisor := supervisor.New(cfg, logger)

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
