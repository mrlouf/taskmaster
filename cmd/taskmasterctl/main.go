package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chzyer/readline"
)

func gracefulExit(rl *readline.Instance) {
	fmt.Println("Received interrupt signal, exiting...")
	rl.Close()
	os.Exit(0)
}

func handleCommand(line string) error {

	cmd := strings.Split(line, " ")[0]
	name := strings.TrimSpace(strings.TrimPrefix(line, cmd))

	switch cmd {

	case "start":
		if name == "" {
			return fmt.Errorf("program name is required for start command")
		}
		fmt.Println("Starting program...", name)
	case "stop":
		if name == "" {
			return fmt.Errorf("program name is required for stop command")
		}
		fmt.Println("Stopping program...", name)
	case "status":
		if name == "" {
			return fmt.Errorf("program name is required for status command")
		}
		fmt.Println("Program status: ...", name)
	case "restart":
		if name == "" {
			return fmt.Errorf("program name is required for restart command")
		}
		fmt.Println("Restarting program...", name)
	case "reload":
		fmt.Println("Reloading configuration...")
	case "shutdown":
		fmt.Println("Shutting down taskmasterd...")
	case "help":
		fmt.Println("Available commands: start, stop, status, restart, reload, shutdown, help")
	case "exit":
		fmt.Println("Goodbye!")
		os.Exit(0)
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
	return nil
}

func run() error {

	rl, err := readline.New("taskmasterctl> ")
	if err != nil {
		return fmt.Errorf("failed to initialise readline: %w", err)
	}
	defer rl.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		gracefulExit(rl)
	}()

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			gracefulExit(rl)
		}
		if err != nil {
			return fmt.Errorf("failed to read line: %w", err)
		}
		err = handleCommand(line)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}
}

func main() {

	if err := run(); err != nil {
		log.Fatal(err)
	}

}
