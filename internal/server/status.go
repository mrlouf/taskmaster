package server

import (
	"fmt"
	"slices"
	"strings"

	"taskmaster/internal/protocol"
)

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

	fmt.Printf("%s", resp.Msg)

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

	fmt.Printf("\n%s\n", resp.Msg)

	return nil
}

func HandleProgramStatus(client Client, name string, server *Server) error {

	var resp protocol.Response

	server.Config.Mu.Lock()
	_, exists := server.Config.Programs[name]
	server.Config.Mu.Unlock()

	if !exists {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

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

	server.Config.Mu.Lock()
	programs := server.Config.Programs

	// ! Iterating over maps in Go does not guarantee order,
	// ! so we need to sort the program names in a slice first.
	keys := make([]string, 0, len(programs))
	for name := range programs {
		keys = append(keys, name)
	}

	server.Config.Mu.Unlock()

	slices.Sort(keys)

	// Use a string builder to efficiently concatenate status of all programs
	// instead of using the '+' operator which creates multiple intermediate strings and wastes memory
	var b strings.Builder

	b.WriteString("\n")
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
