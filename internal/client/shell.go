package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
)

type response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func RunShell(in io.Reader, out io.Writer, address string) error {
	reader := bufio.NewReader(in)
	for {
		if _, err := fmt.Fprint(out, "taskmasterctl> "); err != nil {
			return err
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "quit" || line == "exit" {
			return nil
		}

		msg, err := SendCommand(address, line)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, msg); err != nil {
			return err
		}
	}
}

func SendCommand(address, command string) (string, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, strings.TrimSpace(command)+"\n"); err != nil {
		return "", err
	}

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", err
	}

	var res response
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &res); err != nil {
		return "", err
	}
	if !res.OK {
		return "", fmt.Errorf(res.Message)
	}
	return res.Message, nil
}
