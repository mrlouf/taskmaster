package server

import (
	"fmt"

	"taskmaster/internal/protocol"
)

func RequestSetDebugFlag(client Client) error {

	var req protocol.Request
	req.Cmd = "debug"

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

func HandleSetDebugFlag(client Client, server *Server) error {

	server.Logger.SetDebugFlag()

	var resp protocol.Response
	resp.Ok = true

	val := server.Logger.GetDebugFlag()

	if val {
		resp.Msg = "Debug flag enabled"
	} else {
		resp.Msg = "Debug flag disabled"
	}

	if err := client.Enc.Encode(resp); err != nil {
		return fmt.Errorf("failed to send status request: %w", err)
	}

	return nil

}
