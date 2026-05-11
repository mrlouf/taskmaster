package server

import (
	"fmt"
	"strings"

	"github.com/mrlouf/taskmaster/internal/protocol"
	"github.com/mrlouf/taskmaster/internal/supervisor"
)

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

	_, exists := server.Config.Programs[name]
	if !exists && name != "" {
		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)
	} else {

		event := supervisor.Event{
			Kind:   supervisor.EventRestartProgram,
			Name:   name,
			RespCh: make(chan protocol.Response),
		}

		server.Supervisor.Events <- event
		resp = <-event.RespCh

	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send restart response: %w", err)
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

	if resp.Ok {
		prefix := "Program '" + name + "' started with warnings:"
		if warnMsg, exists := strings.CutPrefix(resp.Msg, prefix); exists {
			return fmt.Errorf("program '%s' started but with following warnings:\n%s", name, strings.TrimSpace(warnMsg))
		}
		fmt.Printf("Program '%s' started\n", name)
	} else if !resp.Ok {
		return fmt.Errorf("start command: %s", resp.Msg)
	}
	return nil
}

func HandleStart(client Client, name string, server *Server) error {

	var resp protocol.Response

	server.Config.Mu.Lock()
	program, exists := server.Config.Programs[name]
	server.Config.Mu.Unlock()

	if !exists {

		resp.Ok = false
		resp.Msg = fmt.Sprintf("Program '%s' not found", name)

		// } else if strings.Contains(server.Supervisor.GetStatus(name), "RUNNING") {

		// 	resp.Ok = false
		// 	resp.Msg = fmt.Sprintf("Program '%s' is already running", name)

		// } else if strings.Contains(server.Supervisor.GetStatus(name), "STARTING") {

		// 	resp.Ok = false
		// 	resp.Msg = fmt.Sprintf("Program '%s' is already starting", name)

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

func RequestReload(client Client, path string) error {

	var req protocol.Request
	req.Cmd = "reload"
	req.Name = path

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

func HandleReload(client Client, path string, server *Server) error {

	server.Logger.Log("Reloading configuration...")

	event := supervisor.Event{
		Kind:   supervisor.EventReloadConfig,
		Name:   path,
		RespCh: make(chan protocol.Response),
	}
	server.Supervisor.Events <- event

	/* 	pid := server.Pid
	   	if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
	   		return fmt.Errorf("failed to send SIGHUP signal to %d: %w", pid, err)
	   	} */
	var resp protocol.Response
	resp = <-event.RespCh

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send reload response: %w", err)
	}

	return nil
}
