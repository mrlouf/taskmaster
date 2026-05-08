package server

import (
	"fmt"
	"os"
	"strings"

	"github.com/mrlouf/taskmaster/internal/protocol"
	"github.com/mrlouf/taskmaster/internal/supervisor"
)

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

	os.Exit(0) // controller exit

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

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send shutdown response: %w", err)
	}

	os.Exit(0) // daemon exit

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

	server.Config.Mu.Lock()
	program, exists := server.Config.Programs[name]
	server.Config.Mu.Unlock()
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
