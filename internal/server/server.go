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
	Pid        int
}

func New(config *config.Config, logger *logger.Logger, supervisor *supervisor.Supervisor, pid int) (*Server, error) {

	socket, err := OpenSocket()
	if err != nil {
		return nil, err
	}

	return &Server{
		Config:     config,
		Logger:     logger,
		Socket:     socket,
		Supervisor: supervisor,
		Pid:        pid,
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

		err = HandleReload(client, req.Name, server)

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
