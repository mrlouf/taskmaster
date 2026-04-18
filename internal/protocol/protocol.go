package protocol

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"github.com/mrlouf/taskmaster/internal/config"
)

type Client struct {
	Socket net.Conn
	Enc    *json.Encoder
	Dec    *json.Decoder
}

type Request struct {
	Cmd  string `json:"cmd"` // "start", "stop", "status", "restart", "reload", "shutdown"
	Name string `json:"name"`
}

type Response struct {
	Ok  bool   `json:"ok"`
	Msg string `json:"msg"`
}

func OpenSocket() (net.Listener, error) {

	// Remove existing socket file if it exists, ignore error if it doesn't exist
	_ = os.Remove("/tmp/taskmaster.sock")

	socket, err := net.Listen("unix", "/tmp/taskmaster.sock")
	if err != nil {
		return nil, err
	}

	return socket, nil
}

func HandleConnection(client Client, config *config.Config) {

	defer client.Socket.Close()

	for {

		var req Request
		if err := client.Dec.Decode(&req); err != nil {
			// io.EOF = normal client disconnect, log other errors
			if err != io.EOF {
				log.Printf("read error: %v", err)
			}
			return
		}

		err := handleRequest(client, req, config)

		// * DEBUG
		if err != nil {
			log.Printf("handle request error: %v", err)
		} else {
			log.Printf("handled request: %s %s", req.Cmd, req.Name)
		}
	}
}

func handleRequest(client Client, req Request, config *config.Config) error {

	var err error

	switch req.Cmd {

	case "start":

		err = HandleStart(client, req.Name, config)

	case "stop":

		err = HandleStop(client, req.Name, config)

	case "status":

		err = HandleStatus(client, req.Name, config)

	case "restart":

		err = HandleRestart(client, req.Name, config)

	case "reload":

		err = HandleReload(client, config)

	case "shutdown":

		err = HandleShutdown(client, config)

	case "healthcheck":

		err = HandleHealthCheck(client, config)

	default:
		return fmt.Errorf("unknown command: %s", req.Cmd)
	}

	return err
}

func RequestShutdown(c Client) error {

	var req Request
	req.Cmd = "shutdown"

	if err := c.Enc.Encode(req); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	var resp Response
	if err := c.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("recv: %w", err)
	}
	return nil
}

func HandleShutdown(client Client, config *config.Config) error {

	// TODO: Add graceful shutdown logic here (stop all programs, clean up resources, etc.)
	for name := range config.Programs {
		fmt.Printf("Stopping program '%s'...\n", name)
	}

	var resp Response
	resp.Ok = true
	resp.Msg = "Taskmaster is shutting down"

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send shutdown response: %w", err)
	}

	client.Socket.Close()
	// ? Remove socket file on shutdown?
	os.Exit(0)

	return nil
}

func RequestStart(client Client, name string) error {

	var req Request
	req.Cmd = "start"
	req.Name = name

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("failed to send start request: %w", err)
	}

	var resp Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("failed to receive start response: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("start command failed: %s", resp.Msg)
	}

	fmt.Printf("Successful start: %s\n", resp.Msg)

	return nil
}

func HandleStart(client Client, name string, config *config.Config) error {

	var resp Response

	program, exists := config.Programs[name]
	if !exists {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

		// TODO: Implement start logic for the program
		fmt.Printf("Starting program '%s' with command: %s\n", name, program.Command)

		resp.Ok = true
		resp.Msg = fmt.Sprintf("Program '%s' started successfully", name)
	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send start response: %w", err)
	}

	return nil
}

func RequestStop(client Client, name string) error {

	var req Request
	req.Cmd = "stop"
	req.Name = name

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("failed to send stop request: %w", err)
	}

	var resp Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("failed to receive stop response: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("stop command failed: %s", resp.Msg)
	}

	fmt.Printf("Successful stop: %s\n", resp.Msg)

	return nil
}

func HandleStop(client Client, name string, config *config.Config) error {

	var resp Response

	program, exists := config.Programs[name]
	if !exists {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

		// TODO: Implement stop logic for the program
		fmt.Printf("Stopping program '%s' with command: %s\n", name, program.Command)

		resp.Ok = true
		resp.Msg = fmt.Sprintf("Program '%s' stopped successfully", name)
	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send stop response: %w", err)
	}

	return nil

}

func RequestStatus(client Client, name string) error {

	var req Request
	req.Cmd = "status"
	req.Name = name

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("failed to send status request: %w", err)
	}

	var resp Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("failed to receive status response: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("status command failed: %s", resp.Msg)
	}

	fmt.Printf("Status of program '%s': %s\n", name, resp.Msg)

	return nil
}

func HandleStatus(client Client, name string, config *config.Config) error {

	var resp Response

	program, exists := config.Programs[name]
	if !exists {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

		// TODO: Implement status logic for the program
		fmt.Printf("Getting status of program '%s' with command: %s\n", name, program.Command)

		resp.Ok = true
		resp.Msg = fmt.Sprintf("Program '%s' is running", name)
	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send status response: %w", err)
	}

	return nil
}

func RequestRestart(client Client, name string) error {

	var req Request
	req.Cmd = "restart"
	req.Name = name

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("failed to send restart request: %w", err)
	}

	var resp Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("failed to receive restart response: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("restart command failed: %s", resp.Msg)
	}

	fmt.Printf("Successful restart: %s\n", resp.Msg)

	return nil
}

func HandleRestart(client Client, name string, config *config.Config) error {

	var resp Response

	program, exists := config.Programs[name]
	if !exists {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

		// TODO: Implement restart logic for the program
		fmt.Printf("Restarting program '%s' with command: %s\n", name, program.Command)

		resp.Ok = true
		resp.Msg = fmt.Sprintf("Program '%s' restarted successfully", name)

	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send restart response: %w", err)
	}

	return nil
}

func RequestReload(client Client) error {

	var req Request
	req.Cmd = "reload"

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("failed to send reload request: %w", err)
	}

	var resp Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("failed to receive reload response: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("reload command failed: %s", resp.Msg)
	}

	fmt.Printf("Successful reload: %s\n", resp.Msg)

	return nil
}

func HandleReload(client Client, config *config.Config) error {

	// TODO: Implement reload logic
	fmt.Println("Reloading configuration...")

	var resp Response
	resp.Ok = true
	resp.Msg = "Configuration reloaded successfully"

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send reload response: %w", err)
	}

	return nil
}

func RequestHealthCheck(client Client) error {

	var req Request
	req.Cmd = "healthcheck"

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("send: %w", err)
	}

	var resp Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("recv: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("healthcheck failed: %s", resp.Msg)
	}

	fmt.Printf("Healthcheck successful: %s\n", resp.Msg)

	return nil
}

func HandleHealthCheck(client Client, config *config.Config) error {

	var resp Response
	resp.Ok = true
	resp.Msg = "Daemon is healthy"

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send healthcheck response: %w", err)
	}

	return nil
}
