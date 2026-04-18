package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chzyer/readline"

	"github.com/mrlouf/taskmaster/internal/protocol"
)

func gracefulExit(rl *readline.Instance) {

	if rl != nil {
		rl.Close()
	}
	os.Exit(0)
}

func parseCommand(line string) (protocol.Request, error) {

	parts := strings.Split(strings.TrimSpace(line), " ")
	if len(parts) == 0 {
		return protocol.Request{}, fmt.Errorf("empty command")
	}

	req := protocol.Request{
		Cmd: parts[0],
	}

	if req.Cmd == "help" ||
		req.Cmd == "reload" ||
		req.Cmd == "shutdown" ||
		req.Cmd == "healthcheck" ||
		req.Cmd == "exit" {

		if len(parts) > 1 {
			fmt.Printf("Warning: Command '%s' does not take any arguments, ignoring extra input\n", req.Cmd)
		}

		return req, nil
	}

	if len(parts) > 1 {
		req.Name = parts[1]
	}

	if req.Cmd == "start" ||
		req.Cmd == "stop" ||
		req.Cmd == "status" ||
		req.Cmd == "restart" {

		if req.Name == "" {
			return protocol.Request{}, fmt.Errorf("Error: command '%s' requires a program name", req.Cmd)
		} else if len(parts) > 2 {
			fmt.Printf("Warning: Command '%s' does not handle multiple arguments, ignoring extra input\n", req.Cmd)
		}
	}

	return req, nil

}

func handleRequest(req protocol.Request, client protocol.Client) error {

	cmd := req.Cmd
	name := req.Name

	var err error

	switch cmd {

	case "start":

		return protocol.RequestStart(client, name)

	case "stop":

		return protocol.RequestStop(client, name)

	case "status":

		return protocol.RequestStatus(client, name)

	case "restart":

		return protocol.RequestRestart(client, name)

	case "reload":

		return protocol.RequestReload(client)

	case "shutdown":

		return protocol.RequestShutdown(client)

	case "healthcheck":

		return protocol.RequestHealthCheck(client)

	case "help":

		fmt.Println("Available commands: start, stop, status, restart, reload, shutdown, help, healthcheck, exit")

	case "exit":

		fmt.Println("Goodbye!")
		os.Exit(0)

	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}

	if err != nil {
		return fmt.Errorf("failed to send %s command: %w", cmd, err)
	}

	return nil
}

func connectToSocket() (protocol.Client, error) {

	var c protocol.Client

	socket, err := net.Dial("unix", "/tmp/taskmaster.sock")
	if err != nil {
		count := 0
		for {
			time.Sleep(1 * time.Second)
			socket, err = net.Dial("unix", "/tmp/taskmaster.sock")
			if err == nil {
				break
			}
			count++
			if count >= 5 {
				return c, fmt.Errorf("failed to connect to socket after 5 attempts: %w", err)
			}
		}
	}

	c.Socket = socket
	c.Dec = json.NewDecoder(socket)
	c.Enc = json.NewEncoder(socket)

	return c, nil
}

func run() error {

	rl, err := readline.New("taskmasterctl> ")
	if err != nil {
		return fmt.Errorf("failed to initialise readline: %w", err)
	}
	defer rl.Close()

	// ? Necessary? Readline handles SIGINT and EOF internally
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("Goodbye!")
		gracefulExit(rl)
	}()

	client, err := connectToSocket()
	if err != nil {
		return fmt.Errorf("failed to connect to socket: %w", err)
	}

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			fmt.Println("Goodbye!")
			gracefulExit(rl)
		}
		if err != nil {
			return fmt.Errorf("failed to read line: %w", err)
		}

		req, err := parseCommand(line)
		if err != nil {
			fmt.Printf("%v\n", err)
			continue
		}

		err = handleRequest(req, client)

		if err != nil {
			fmt.Printf("Error handling request: %v\n", err)
		}
	}
}

func main() {

	if err := run(); err != nil {
		log.Fatal(err)
	}

}
