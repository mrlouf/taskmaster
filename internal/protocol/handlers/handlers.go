package handlers

import (
	"fmt"
	"net"
)

func RequestShutdown(conn net.Conn) error {
	_, err := conn.Write([]byte(`{"cmd":"shutdown"}`))
	if err != nil {
		return fmt.Errorf("failed to send shutdown request: %w", err)
	}

	return nil
}

func RequestStart(conn net.Conn, name string) error {

	if name == "" {
		return fmt.Errorf("program name is required for start command")
	}

	_, err := conn.Write([]byte(fmt.Sprintf(`{"cmd":"start","name":"%s"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send start request: %w", err)
	}

	return nil
}

func RequestStop(conn net.Conn, name string) error {

	if name == "" {
		return fmt.Errorf("program name is required for stop command")
	}

	_, err := conn.Write([]byte(fmt.Sprintf(`{"cmd":"stop","name":"%s"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send stop request: %w", err)
	}

	return nil
}

func RequestStatus(conn net.Conn, name string) error {

	if name == "" {
		return fmt.Errorf("program name is required for status command")
	}

	_, err := conn.Write([]byte(fmt.Sprintf(`{"cmd":"status","name":"%s"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send status request: %w", err)
	}

	return nil
}

func RequestRestart(conn net.Conn, name string) error {

	if name == "" {
		return fmt.Errorf("program name is required for restart command")
	}

	_, err := conn.Write([]byte(fmt.Sprintf(`{"cmd":"restart","name":"%s"}`, name)))
	if err != nil {
		return fmt.Errorf("failed to send restart request: %w", err)
	}

	return nil
}

func RequestReload(conn net.Conn) error {

	_, err := conn.Write([]byte(`{"cmd":"reload"}`))
	if err != nil {
		return fmt.Errorf("failed to send reload request: %w", err)
	}

	return nil
}
