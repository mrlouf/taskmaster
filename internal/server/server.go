package server

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/logger"
	"github.com/mrlouf/taskmaster/internal/protocol"
)

type Server struct {
	Config *config.Config
	Logger *logger.Logger
	Socket net.Listener
}

func New(config *config.Config, logger *logger.Logger) (*Server, error) {
	socket, err := OpenSocket()
	if err != nil {
		return nil, err
	}

	return &Server{
		Config: config,
		Logger: logger,
		Socket: socket,
	}, nil
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

func (server *Server) Start() {

	cfg := server.Config
	logger := server.Logger

	for {

		conn, err := server.Socket.Accept()
		if err != nil {
			server.Logger.Log(fmt.Sprintf("Failed to accept connection: %v", err))
			continue
		}

		client := protocol.Client{
			Socket: conn,
			Dec:    json.NewDecoder(conn),
			Enc:    json.NewEncoder(conn),
		}

		go protocol.HandleConnection(client, cfg, logger)
	}
}
