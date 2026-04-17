package config

import (
	"fmt"
	"os"
)

type Program struct {
	Name         string
	Command      string
	NumProcs     int
	Umask        int
	WorkingDir   string
	AutoStart    bool
	AutoRestart  bool
	ExitCodes    []int
	StartRetries int
	StartTime    int
	StopSignal   string
	StopTime     int
	StdoutLog    string
	StderrLog    string
	Env          map[string]string
}

type Config struct {
	programs []Program
}

func LoadConfig() (*Config, error) {
	// For the moment, we just check if there is a conf file
	// in the current folder, and if not, we return an error
	_, err := os.Stat("taskmaster.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}

	return &Config{}, nil
}
