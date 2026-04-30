package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strings"

	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/logger"
	"github.com/mrlouf/taskmaster/internal/protocol"
	"github.com/mrlouf/taskmaster/internal/supervisor"
)

type Client struct {
	Socket   net.Conn
	Enc      *json.Encoder
	Dec      *json.Decoder
	Programs []string
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
		if err != nil {
			server.Logger.Log(fmt.Sprintf("handle request error: %v", err))
			var resp protocol.Response
			resp.Ok = false
			resp.Msg = err.Error()
			if err := client.Enc.Encode(resp); err != nil {
				server.Logger.Log(fmt.Sprintf("write error: %v", err))
				return
			}
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

		if req.Name == "" {
			err = HandleAllStatus(client, server)
		} else {
			err = HandleProgramStatus(client, req.Name, server)
		}

	case "restart":

		err = HandleRestart(client, req.Name, server)

	case "reload":

		err = HandleReload(client, server)

	case "shutdown":

		err = HandleShutdown(client, server)

	case "healthcheck":

		err = HandleHealthCheck(client, server)

	case "list":

		err = HandleProgramList(client, server)

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

	fmt.Printf("Sending shutdown request\n")

	var resp protocol.Response
	if err := c.Dec.Decode(&resp); err != nil {
		return fmt.Errorf("recv: %w", err)
	}

	if !resp.Ok {
		return fmt.Errorf("shutdown failed: %s", resp.Msg)
	}

	fmt.Printf("%s\n", resp.Msg)
	fmt.Printf("Goodbye!\n")
	os.Exit(0)

	return nil
}

func HandleShutdown(client Client, server *Server) error {

	var resp protocol.Response
	resp.Ok = true
	resp.Msg = "Daemon is shutting down"

	event := supervisor.Event{
		Kind:   supervisor.EventShutdown,
		RespCh: make(chan protocol.Response),
	}

	server.Supervisor.Events <- event

	resp = <-event.RespCh

	fmt.Printf("Response from supervisor: %s\n", resp.Msg)

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send shutdown response: %w", err)
	}

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
		if strings.Contains(resp.Msg, "RUNNING") {
			return fmt.Errorf("program '%s' is already running", name)
		} else if strings.Contains(resp.Msg, "STARTING") {
			return fmt.Errorf("program '%s' is already starting", name)
		}
		return fmt.Errorf("start command: %s", resp.Msg)
	}

	fmt.Printf("Program '%s' started\n", name)

	return nil
}

func HandleStart(client Client, name string, server *Server) error {

	var resp protocol.Response

	program, exists := server.Config.Programs[name]
	if !exists {

		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)

	} else if strings.Contains(server.Supervisor.GetStatus(name), "RUNNING") {

		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' is already running", name)

	} else if strings.Contains(server.Supervisor.GetStatus(name), "STARTING") {

		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' is already starting", name)

	} else {

		server.Logger.Log(fmt.Sprintf("Starting program '%s' with command: %s", name, program.Command))

		event := supervisor.Event{
			Kind:   supervisor.EventStartProgram,
			Name:   name,
			RespCh: make(chan protocol.Response),
		}

		server.Supervisor.Events <- event
		resp = <-event.RespCh

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
		return fmt.Errorf("stop command: %s", resp.Msg)
	}

	fmt.Printf("Program '%s' stopped\n", name)

	return nil
}

func HandleStop(client Client, name string, server *Server) error {

	var resp protocol.Response

	program, exists := server.Config.Programs[name]
	if !exists {

		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)

	} else if strings.Contains(server.Supervisor.GetStatus(name), "STOPPING") {

		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' is already stopping", name)

	} else if strings.Contains(server.Supervisor.GetStatus(name), "STOPPED") {

		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' is already stopped", name)

	} else {

		server.Logger.Log(fmt.Sprintf("Stopping program '%s' with command: %s", name, program.Command))
		event := supervisor.Event{
			Kind:   supervisor.EventStopProgram,
			Name:   name,
			RespCh: make(chan protocol.Response),
		}

		server.Supervisor.Events <- event
		resp = <-event.RespCh

	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send stop response: %w", err)
	}

	return nil

}

func RequestAllStatus(client Client) error {

	var req protocol.Request
	req.Cmd = "status"

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

	fmt.Printf("%s\n", resp.Msg)

	return nil
}

func RequestProgramStatus(client Client, name string) error {

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

	fmt.Printf("%s\n", resp.Msg)

	return nil
}

func HandleProgramStatus(client Client, name string, server *Server) error {

	var resp protocol.Response

	program, exists := server.Config.Programs[name]
	if !exists {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

		// ? Keep server logs for status requests?
		// ? Useful for debugging but too verbose maybe
		server.Logger.Log(fmt.Sprintf("Getting status of program '%s' with command: %s", name, program.Command))

		resp.Ok = true
		resp.Msg = server.Supervisor.GetStatus(name)
	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send status response: %w", err)
	}

	return nil
}

func HandleAllStatus(client Client, server *Server) error {

	var resp protocol.Response
	resp.Ok = true

	// ! Iterating over maps in Go does not guarantee order,
	// ! so we need to sort the program names in a slice first.
	keys := make([]string, 0, len(server.Config.Programs))
	for name := range server.Config.Programs {
		keys = append(keys, name)
	}

	slices.Sort(keys)

	// Use a string builder to efficiently concatenate status of all programs
	// instead of using the '+' operator which creates multiple intermediate strings and wastes memory
	var b strings.Builder

	for _, name := range keys {
		b.WriteString(server.Supervisor.GetStatus(name))
		b.WriteString("\n")
	}

	resp.Msg = b.String()

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

func RequestProgramList(client Client) ([]string, error) {

	var req protocol.Request
	req.Cmd = "list"

	if err := client.Enc.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send list request: %w", err)
	}

	var resp protocol.Response
	if err := client.Dec.Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to receive list response: %w", err)
	}

	if !resp.Ok {
		return nil, fmt.Errorf("list command failed: %s", resp.Msg)
	}

	programs := strings.Split(resp.Msg, "\n")

	return programs, nil

}

func HandleProgramList(client Client, server *Server) error {

	var resp protocol.Response

	var programNames []string
	for name := range server.Config.Programs {
		programNames = append(programNames, name)
	}

	resp.Ok = true
	resp.Msg = strings.Join(programNames, "\n")

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send list response: %w", err)
	}

	return nil
}
