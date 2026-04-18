package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/logger"
	"github.com/mrlouf/taskmaster/internal/protocol"
	"github.com/mrlouf/taskmaster/internal/supervisor"
)

type Client struct {
	Socket net.Conn
	Enc    *json.Encoder
	Dec    *json.Decoder
}

type Server struct {
	Config     *config.Config
	Logger     *logger.Logger
	Supervisor *supervisor.Supervisor
	Socket     net.Listener
}

func New(config *config.Config, logger *logger.Logger, supervisor *supervisor.Supervisor) (*Server, error) {

	socket, err := OpenSocket()
	if err != nil {
		return nil, err
	}

	return &Server{
		Config:     config,
		Logger:     logger,
		Socket:     socket,
		Supervisor: supervisor,
	}, nil
}

func OpenSocket() (net.Listener, error) {

	// Remove existing socket file if it exists, ignore error if it doesn't
	_ = os.Remove("/tmp/taskmaster.sock")

	socket, err := net.Listen("unix", "/tmp/taskmaster.sock")
	if err != nil {
		return nil, err
	}

	return socket, nil
}

func (server *Server) Start() {

	for {

		conn, err := server.Socket.Accept()
		if err != nil {
			server.Logger.Log(fmt.Sprintf("Failed to accept connection: %v", err))
			continue
		}

		client := Client{
			Socket: conn,
			Dec:    json.NewDecoder(conn),
			Enc:    json.NewEncoder(conn),
		}

		go HandleConnection(client, server)
	}
}

func HandleConnection(client Client, server *Server) {

	defer client.Socket.Close()

	for {

		var req protocol.Request
		if err := client.Dec.Decode(&req); err != nil {
			// io.EOF = normal client disconnect, log other errors
			if err != io.EOF {
				server.Logger.Log(fmt.Sprintf("read error: %v", err))
			}
			return
		}

		err := handleRequest(client, req, server)

		// * DEBUG
		if err != nil {
			fmt.Printf("handle request error: %v\n", err)
		} else {
			fmt.Printf("handled request: %s %s\n", req.Cmd, req.Name)
		}
	}
}

func handleRequest(client Client, req protocol.Request, server *Server) error {

	var err error

	switch req.Cmd {

	case "start":

		err = HandleStart(client, req.Name, server)

	case "stop":

		err = HandleStop(client, req.Name, server)

	case "status":

		err = HandleStatus(client, req.Name, server)

	case "restart":

		err = HandleRestart(client, req.Name, server)

	case "reload":

		err = HandleReload(client, server)

	case "shutdown":

		err = HandleShutdown(client, server)

	case "healthcheck":

		err = HandleHealthCheck(client, server)

	default:
		return fmt.Errorf("unknown command: %s", req.Cmd)
	}

	return err
}

func RequestShutdown(c Client) error {

	var req protocol.Request
	req.Cmd = "shutdown"

	if err := c.Enc.Encode(req); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	var resp protocol.Response
	if err := c.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("recv: %w", err)
	}
	return nil
}

func HandleShutdown(client Client, server *Server) error {

	var resp protocol.Response
	resp.Ok = true
	resp.Msg = "Daemon is shutting down"

	server.Supervisor.Events <- supervisor.Event{Kind: supervisor.EventShutdown}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send shutdown response: %w", err)
	}

	os.Exit(0)
	return nil
}

func RequestStart(client Client, name string) error {

	var req protocol.Request
	req.Cmd = "start"
	req.Name = name

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("failed to send start request: %w", err)
	}

	var resp protocol.Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("failed to receive start response: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("start command: %s", resp.Msg)
	}

	fmt.Printf("Successful start: %s\n", resp.Msg)

	return nil
}

func HandleStart(client Client, name string, server *Server) error {

	var resp protocol.Response

	program, exists := server.Config.Programs[name]
	if !exists {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

		server.Logger.Log(fmt.Sprintf("Starting program '%s' with command: %s", name, program.Command))

		event := supervisor.Event{
			Kind:   supervisor.EventStartProcess,
			Name:   name,
			RespCh: make(chan protocol.Response),
		}

		server.Supervisor.Events <- event
		resp = <-event.RespCh

		/* 		if event.RespCh != nil {
		   			resp.Msg = fmt.Sprintf("Program '%s' started successfully", name)
		   		} else {
		   			resp.Msg = fmt.Sprintf("Failed to start program '%s'", name)
		   		} */

	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send start response: %w", err)
	}

	return nil
}

func RequestStop(client Client, name string) error {

	var req protocol.Request
	req.Cmd = "stop"
	req.Name = name

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("failed to send stop request: %w", err)
	}

	var resp protocol.Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("failed to receive stop response: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("stop command failed: %s", resp.Msg)
	}

	fmt.Printf("Successful stop: %s\n", resp.Msg)

	return nil
}

func HandleStop(client Client, name string, server *Server) error {

	var resp protocol.Response

	program, exists := server.Config.Programs[name]
	if !exists {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

		// TODO: Implement stop logic for the program
		server.Logger.Log(fmt.Sprintf("Stopping program '%s' with command: %s", name, program.Command))
		server.Supervisor.Events <- supervisor.Event{Kind: supervisor.EventStopProcess, Name: name}

		resp.Ok = true
		resp.Msg = fmt.Sprintf("Program '%s' stopped successfully", name)
	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send stop response: %w", err)
	}

	return nil

}

func RequestStatus(client Client, name string) error {

	var req protocol.Request
	req.Cmd = "status"
	req.Name = name

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("failed to send status request: %w", err)
	}

	var resp protocol.Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("failed to receive status response: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("status command failed: %s", resp.Msg)
	}

	fmt.Printf("Status of program '%s': %s\n", name, resp.Msg)

	return nil
}

func HandleStatus(client Client, name string, server *Server) error {

	var resp protocol.Response

	program, exists := server.Config.Programs[name]
	if !exists {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

		// TODO: Implement status logic for the program
		server.Logger.Log(fmt.Sprintf("Getting status of program '%s' with command: %s", name, program.Command))

		resp.Ok = true
		resp.Msg = fmt.Sprintf("Program '%s' is running", name)
	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send status response: %w", err)
	}

	return nil
}

func RequestRestart(client Client, name string) error {

	var req protocol.Request
	req.Cmd = "restart"
	req.Name = name

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("failed to send restart request: %w", err)
	}

	var resp protocol.Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("failed to receive restart response: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("restart command failed: %s", resp.Msg)
	}

	fmt.Printf("Successful restart: %s\n", resp.Msg)

	return nil
}

func HandleRestart(client Client, name string, server *Server) error {

	var resp protocol.Response

	program, exists := server.Config.Programs[name]
	if !exists {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

		// TODO: Implement restart logic for the program
		server.Logger.Log(fmt.Sprintf("Restarting program '%s' with command: %s", name, program.Command))

		resp.Ok = true
		resp.Msg = fmt.Sprintf("Program '%s' restarted successfully", name)

	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send restart response: %w", err)
	}

	return nil
}

func RequestReload(client Client) error {

	var req protocol.Request
	req.Cmd = "reload"

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("failed to send reload request: %w", err)
	}

	var resp protocol.Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("failed to receive reload response: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("reload command failed: %s", resp.Msg)
	}

	fmt.Printf("Successful reload: %s\n", resp.Msg)

	return nil
}

func HandleReload(client Client, server *Server) error {

	// TODO: Implement reload logic
	server.Logger.Log("Reloading configuration...")

	var resp protocol.Response
	resp.Ok = true
	resp.Msg = "Configuration reloaded successfully"

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send reload response: %w", err)
	}

	return nil
}

func RequestHealthCheck(client Client) error {

	var req protocol.Request
	req.Cmd = "healthcheck"

	if err := client.Enc.Encode(req); err != nil {
		return fmt.Errorf("send: %w", err)
	}

	var resp protocol.Response
	if err := client.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("recv: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("healthcheck failed: %s", resp.Msg)
	}

	fmt.Printf("Healthcheck successful: %s\n", resp.Msg)

	return nil
}

func HandleHealthCheck(client Client, server *Server) error {

	var resp protocol.Response
	resp.Ok = true
	resp.Msg = "Daemon is healthy"

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send healthcheck response: %w", err)
	}

	return nil
}
