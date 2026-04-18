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

func HandleConnection(conn net.Conn, config *config.Config) {

	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	for {

		var req Request
		if err := dec.Decode(&req); err != nil {
			// io.EOF = ctl s'est déconnecté proprement, pas une erreur
			if err != io.EOF {
				log.Printf("read error: %v", err)
			}
			return
		}

		resp := handleRequest(conn, req, config)

		if err := enc.Encode(resp); err != nil {
			log.Printf("write error: %v", err)
			return
		}

		if resp != nil {
			log.Printf("Handled request: %s %s - Response: %v", req.Cmd, req.Name, resp)
		} else {
			log.Printf("Handled request: %s %s - No response", req.Cmd, req.Name)
		}

	}
}

func handleRequest(conn net.Conn, req Request, config *config.Config) error {

	var err error

	switch req.Cmd {

	case "start":

		err = HandleStart(conn, req.Name, config)

	case "stop":

		err = HandleStop(conn, req.Name, config)

	case "status":

		err = HandleStatus(conn, req.Name, config)

	case "restart":

		err = HandleRestart(conn, req.Name, config)

	case "reload":

		err = HandleReload(conn, config)

	case "shutdown":

		err = HandleShutdown(conn, config)

	default:
		return fmt.Errorf("unknown command: %s", req.Cmd)
	}

	return err
}

func RequestShutdown(client Client) error {
	_, err := client.Socket.Write([]byte(`{"cmd":"shutdown"}`))
	if err != nil {
		return fmt.Errorf("failed to send shutdown request: %w", err)
	}

	return nil
}

func HandleShutdown(conn net.Conn, config *config.Config) error {

	// TODO: Add graceful shutdown logic here (stop all programs, clean up resources, etc.)
	for name := range config.Programs {
		fmt.Printf("Stopping program '%s'...\n", name)
	}

	_, err := conn.Write([]byte(`{"ok": true, "msg": "Shutting down daemon..."}`))
	if err != nil {
		return fmt.Errorf("failed to send shutdown response: %w", err)
	}

	conn.Close()
	os.Exit(0)

	return nil
}

func RequestStart(client Client, name string) error {

	if name == "" {
		return fmt.Errorf("program name is required for start command")
	}

	_, err := client.Socket.Write([]byte(fmt.Sprintf(`{"cmd":"start","name":"%s"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send start request: %w", err)
	}

	return nil
}

func HandleStart(conn net.Conn, name string, config *config.Config) error {

	program, exists := config.Programs[name]
	if !exists {
		return fmt.Errorf("program not found: %s", name)
	}

	// TODO: Implement start logic for the program
	fmt.Printf("Starting program '%s' with command: %s\n", name, program.Command)

	_, err := conn.Write([]byte(fmt.Sprintf(`{"ok": true, "msg": "Program '%s' started successfully"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send start response: %w", err)
	}

	return nil
}

func RequestStop(client Client, name string) error {

	if name == "" {
		return fmt.Errorf("program name is required for stop command")
	}

	_, err := client.Socket.Write([]byte(fmt.Sprintf(`{"cmd":"stop","name":"%s"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send stop request: %w", err)
	}

	return nil
}

func HandleStop(conn net.Conn, name string, config *config.Config) error {

	program, exists := config.Programs[name]
	if !exists {
		return fmt.Errorf("program not found: %s", name)
	}

	// TODO: Implement stop logic for the program
	fmt.Printf("Stopping program '%s' with command: %s\n", name, program.Command)

	_, err := conn.Write([]byte(fmt.Sprintf(`{"ok": true, "msg": "Program '%s' stopped successfully"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send stop response: %w", err)
	}

	return nil
}

func RequestStatus(client Client, name string) error {

	if name == "" {
		return fmt.Errorf("program name is required for status command")
	}

	_, err := client.Socket.Write([]byte(fmt.Sprintf(`{"cmd":"status","name":"%s"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send status request: %w", err)
	}

	return nil
}

func HandleStatus(conn net.Conn, name string, config *config.Config) error {

	program, exists := config.Programs[name]
	if !exists {
		return fmt.Errorf("program not found: %s", name)
	}

	// TODO: Implement status logic for the program
	fmt.Printf("Getting status of program '%s' with command: %s\n", name, program.Command)

	_, err := conn.Write([]byte(fmt.Sprintf(`{"ok": true, "msg": "Program '%s' is running"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send status response: %w", err)
	}

	return nil
}

func RequestRestart(client Client, name string) error {

	if name == "" {
		return fmt.Errorf("program name is required for restart command")
	}

	_, err := client.Socket.Write([]byte(fmt.Sprintf(`{"cmd":"restart","name":"%s"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send restart request: %w", err)
	}

	return nil
}

func HandleRestart(conn net.Conn, name string, config *config.Config) error {

	program, exists := config.Programs[name]
	if !exists {
		return fmt.Errorf("program not found: %s", name)
	}

	// TODO: Implement restart logic for the program
	fmt.Printf("Restarting program '%s' with command: %s\n", name, program.Command)

	_, err := conn.Write([]byte(fmt.Sprintf(`{"ok": true, "msg": "Program '%s' restarted successfully"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send restart response: %w", err)
	}

	return nil
}

func RequestReload(client Client) error {

	_, err := client.Socket.Write([]byte(`{"cmd":"reload"}`))
	if err != nil {
		return fmt.Errorf("failed to send reload request: %w", err)
	}

	return nil
}

func HandleReload(conn net.Conn, config *config.Config) error {

	// TODO: Implement reload logic
	fmt.Println("Reloading configuration...")

	_, err := conn.Write([]byte(`{"ok": true, "msg": "Configuration reloaded successfully"}`))
	if err != nil {
		return fmt.Errorf("failed to send reload response: %w", err)
	}

	return nil
}
