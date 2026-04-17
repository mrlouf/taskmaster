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

	"github.com/mrlouf/taskmaster/internal/protocol/handlers"
)

func gracefulExit(rl *readline.Instance) {

	if rl != nil {
		rl.Close()
	}
	os.Exit(0)
}

func handleCommand(line string, socket net.Conn) error {

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

		err = handlers.RequestStart(socket, name)

	case "stop":

		err = handlers.RequestStop(socket, name)

	case "status":

		err = handlers.RequestStatus(socket, name)

	case "restart":

		err = handlers.RequestRestart(socket, name)

	case "reload":

		err = handlers.RequestReload(socket)

	case "shutdown":

		err = handlers.RequestShutdown(socket)

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
				return fmt.Errorf("failed to connect to socket after 5 attempts: %w", err)
			}
		}
	}
	defer socket.Close()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := socket.Read(buf)
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
		err = handleCommand(line, socket)
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
