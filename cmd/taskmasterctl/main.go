package main

import (
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

func handleCommand(line string, client protocol.Client) error {

	cmd := strings.Split(line, " ")[0]

	name := ""
	len := len(strings.Split(line, " "))
	if len > 1 {
		name = strings.Split(line, " ")[1]
		if len > 2 {
			return fmt.Errorf("Multiple arguments are not supported (yet)")
		}
	}

	var err error

	switch cmd {

	case "start":

		err = protocol.RequestStart(client, name)

	case "stop":

		err = protocol.RequestStop(client, name)

	case "status":

		err = protocol.RequestStatus(client, name)

	case "restart":

		err = protocol.RequestRestart(client, name)

	case "reload":

		err = protocol.RequestReload(client)

	case "shutdown":

		err = protocol.RequestShutdown(client)

	case "help":

		fmt.Println("Available commands: start, stop, status, restart, reload, shutdown, help")

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

func connectToSocket(c *protocol.Client) (protocol.Client, error) {

	socket, err := net.Dial("unix", "/tmp/taskmaster.sock")
	if err != nil {
		count := 0
		for {
			time.Sleep(1 * time.Second)
			socket, err = net.Dial("unix", "/tmp/taskmaster.sock")
			if err == nil {
				return protocol.Client{Socket: socket}, nil
			}
			count++
			if count >= 5 {
				return protocol.Client{}, fmt.Errorf("failed to connect to socket after 5 attempts: %w", err)
			}
		}
	}
	return protocol.Client{Socket: socket}, nil
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

	client := protocol.Client{}
	_, err = connectToSocket(&client)
	if err != nil {
		return fmt.Errorf("failed to connect to socket: %w", err)
	}

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := client.Socket.Read(buf)
			if err != nil {
				log.Printf("Failed to read from socket: %v", err)
				return
			}
			fmt.Printf("Received response: %s\n", string(buf[:n]))
		}
	}()

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			gracefulExit(rl)
		}
		if err != nil {
			return fmt.Errorf("failed to read line: %w", err)
		}
		err = handleCommand(line, client)
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
