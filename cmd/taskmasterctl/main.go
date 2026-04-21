package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/chzyer/readline"

	"github.com/mrlouf/taskmaster/internal/protocol"
	"github.com/mrlouf/taskmaster/internal/server"
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
		req.Cmd == "restart" ||
		req.Cmd == "status" {

		if req.Name == "" && req.Cmd != "status" {
			return protocol.Request{}, fmt.Errorf("Error: command '%s' requires a program name", req.Cmd)
		} else if len(parts) > 2 {

			// Use a string builder to concatenate all arguments after the command
			// instead of just using the '+' operator which creates multiple intermediate strings
			// and wastes memory. This is more efficient especially for long program names with spaces.
			var args strings.Builder
			for i := 1; i < len(parts); i++ {
				args.WriteString("," + parts[i])
			}
			req.Name = strings.TrimPrefix(args.String(), ",")
		}
	}

	return req, nil

}

func handleRequest(req protocol.Request, client server.Client) error {

	cmd := req.Cmd
	name := req.Name

	var err error

	switch cmd {

	case "start":

		return server.RequestStart(client, name)

	case "stop":

		return server.RequestStop(client, name)

	case "status":

		if name == "" {
			return server.RequestAllStatus(client)
		} else {
			return server.RequestProgramStatus(client, name)
		}

	case "restart":

		return server.RequestRestart(client, name)

	case "reload":

		return server.RequestReload(client)

	case "shutdown":

		return server.RequestShutdown(client)

	case "healthcheck":

		return server.RequestHealthCheck(client)

	case "help":

		fmt.Printf(`  __                 __                            __                
_/  |______    _____|  | __ _____ _____    _______/  |_  ___________ 
\   __\__  \  /  ___/  |/ //     \\__  \  /  ___/\   __\/ __ \_  __ \
 |  |  / __ \_\___ \|    <|  Y Y  \/ __ \_\___ \  |  | \  ___/|  | \/
 |__| (____  /____  >__|_ \__|_|  (____  /____  > |__|  \___  >__|   
           \/     \/     \/     \/     \/     \/            \/       
Usage:
 start <programs>	: start one or multiple programs
 stop <programs>	: stop one or multiple programs
 status [programs]	: display the status of one or multiple programs.
 				Display all programs if no name is provided
 restart <programs>	: restart one or multiple programs
 reload			: reload the configuration
 shutdown		: shutdown the server
 help			: display this help message
 healthcheck		: perform a health check
 exit			: exit the controller

`)

		return nil

	case "exit":

		fmt.Println("Goodbye!")
		os.Exit(0)

	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}

	return fmt.Errorf("failed to send %s command: %w", cmd, err)

}

func connectToSocket() (server.Client, error) {

	var c server.Client

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
				return c, fmt.Errorf("connection failed after 5 attempts: %w", err)
			}
		}
	}

	c.Socket = socket
	c.Dec = json.NewDecoder(socket)
	c.Enc = json.NewEncoder(socket)

	return c, nil
}

func run() error {

	/* 	// ? Necessary? Readline handles SIGINT and EOF internally
	   	c := make(chan os.Signal, 1)
	   	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	   	go func() {
	   		<-c
	   		fmt.Println("Goodbye!")
	   		gracefulExit(rl)
	   	}() */

	client, err := connectToSocket()
	if err != nil {
		return fmt.Errorf("cannot connect to socket: %w\nIs the daemon running?", err)
	}

	rl, err := readline.New("taskmasterctl> ")
	if err != nil {
		return fmt.Errorf("failed to initialise readline: %w", err)
	}
	defer rl.Close()

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
			fmt.Printf("Error: %v\n", err)
			continue
		}

		for job := range strings.SplitSeq(req.Name, (",")) {
			job = strings.TrimSpace(job)
			req.Name = job
			err = handleRequest(req, client)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}
	}
}

func main() {

	if err := run(); err != nil {
		log.Fatal(err)
	}

}
