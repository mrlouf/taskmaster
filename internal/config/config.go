package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type Program struct {
	Command      string            `yaml:"cmd"`
	NumProcs     int               `yaml:"numprocs"`
	Umask        int               `yaml:"umask"`
	WorkingDir   string            `yaml:"workingdir"`
	AutoStart    bool              `yaml:"autostart"`
	AutoRestart  bool              `yaml:"autorestart"`
	ExitCodes    []int             `yaml:"exitcodes"`
	StartRetries int               `yaml:"startretries"`
	StartTime    int               `yaml:"starttime"`
	StopSignal   string            `yaml:"stopsignal"`
	StopTime     int               `yaml:"stoptime"`
	StdoutLog    string            `yaml:"stdout"`
	StderrLog    string            `yaml:"stderr"`
	Env          map[string]string `yaml:"env"`
}

type Config struct {
	programs map[string]Program `yaml:"programs"`
}

func LoadConfig() (*Config, error) {

	_, err := os.Stat("taskmaster.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}

	file, err := os.Open("taskmaster.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var content []byte
	_, err = file.Read(content)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	err = yaml.Unmarshal(content, &config)

	fmt.Printf("Loaded config: %+v\n", config)

	return &Config{
		programs: make(map[string]Program),
	}, nil

}
