package protocol

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	cfg "github.com/mrlouf/taskmaster/internal/config"
)

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

func HandleConnection(conn net.Conn, config *cfg.Config) {
	defer conn.Close()

	buf := make([]byte, 1024)

	n, err := conn.Read(buf)
	if err != nil {
		// TODO: Send error response to client, write to log, etc.
		return
	}

	// * DEBUG
	fmt.Printf("Received raw data: %s", string(buf[:n]))

	var req Request
	err = json.Unmarshal(buf[:n], &req)
	if err != nil {
		// TODO: Send error response to client, write to log, etc.
		return
	}

	// * DEBUG
	fmt.Printf("Unmarshalled request: %+v\n", req)

	config = nil

}
