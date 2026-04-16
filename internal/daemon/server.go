package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
)

type Server struct {
	address string
	manager *Manager
	cancel  context.CancelFunc

	mu       sync.Mutex
	listener net.Listener
}

type response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func NewServer(address string, manager *Manager, cancel context.CancelFunc) *Server {
	return &Server{address: address, manager: manager, cancel: cancel}
}

func (s *Server) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	defer ln.Close()

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if isNetworkClosedError(err) {
				return nil
			}
			return err
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		command := strings.TrimSpace(scanner.Text())
		res := s.handleCommand(command)
		payload, err := json.Marshal(res)
		if err != nil {
			_, _ = io.WriteString(conn, `{"ok":false,"message":"internal error"}`+"\n")
			return
		}
		_, _ = conn.Write(append(payload, '\n'))
		if command == "shutdown" {
			return
		}
	}
}

func (s *Server) handleCommand(line string) response {
	line = strings.TrimSpace(line)
	if line == "" {
		return response{OK: false, Message: "empty command"}
	}

	parts := strings.Fields(line)
	cmd := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}

	switch cmd {
	case "help":
		return response{OK: true, Message: "commands: status, start <name>, stop <name>, restart <name>, shutdown, help"}
	case "status":
		statuses := s.manager.Status()
		if len(statuses) == 0 {
			return response{OK: true, Message: "no programs configured"}
		}
		var b strings.Builder
		for _, st := range statuses {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			if st.PID > 0 {
				_, _ = fmt.Fprintf(&b, "%s\t%s\tpid=%d", st.Name, st.State, st.PID)
			} else {
				_, _ = fmt.Fprintf(&b, "%s\t%s", st.Name, st.State)
			}
		}
		return response{OK: true, Message: b.String()}
	case "start":
		if arg == "" {
			return response{OK: false, Message: "usage: start <name>"}
		}
		if err := s.manager.Start(arg); err != nil {
			return response{OK: false, Message: err.Error()}
		}
		return response{OK: true, Message: "started " + arg}
	case "stop":
		if arg == "" {
			return response{OK: false, Message: "usage: stop <name>"}
		}
		if err := s.manager.Stop(arg); err != nil {
			return response{OK: false, Message: err.Error()}
		}
		return response{OK: true, Message: "stopped " + arg}
	case "restart":
		if arg == "" {
			return response{OK: false, Message: "usage: restart <name>"}
		}
		if err := s.manager.Restart(arg); err != nil {
			return response{OK: false, Message: err.Error()}
		}
		return response{OK: true, Message: "restarted " + arg}
	case "shutdown":
		s.cancel()
		return response{OK: true, Message: "daemon stopping"}
	default:
		return response{OK: false, Message: "unknown command"}
	}
}

func isNetworkClosedError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, net.ErrClosed)
}
