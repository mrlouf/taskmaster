package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/protocol"
)

func main() {
	// Try and catch equivalent in Go
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {

	config, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// * DEBUG
	for name := range config.Programs {
		fmt.Printf("Program '%s'\n", name)
	}

	socket, err := protocol.OpenSocket()
	if err != nil {
		return fmt.Errorf("failed to open socket: %w", err)
	}
	defer func() { // Clean up the socket file on exit
		_ = socket.Close()
		_ = os.Remove("/tmp/taskmaster.sock")
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		os.Remove("/tmp/taskmaster.sock")
		os.Exit(1)
	}()

	// TODO: Add logging instead of printing to stdout
	// logger := logger.New("./.logs/taskmaster.log")

	// TODO: Start the programs
	// go supervisor.StartProgram(config)

	for {

		conn, err := socket.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go protocol.HandleConnection(conn, config)
	}
}
