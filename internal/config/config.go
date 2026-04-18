package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type IntOrSlice []int

// Custom unmarshal function to handle both single int and slice of ints
// in the YAML configuration, such as for the "exitcodes" field.
func (i *IntOrSlice) UnmarshalYAML(value *yaml.Node) error {
	var single int
	if err := value.Decode(&single); err == nil {
		*i = []int{single}
		return nil
	}

	var slice []int
	if err := value.Decode(&slice); err != nil {
		return fmt.Errorf("failed to decode IntOrSlice: %w", err)
	}
	*i = slice
	return nil
}

type Program struct {
	Command      string            `yaml:"cmd"`
	NumProcs     int               `yaml:"numprocs"`
	Umask        int               `yaml:"umask"`
	WorkingDir   string            `yaml:"workingdir"`
	AutoStart    bool              `yaml:"autostart"`
	AutoRestart  string            `yaml:"autorestart"`
	ExitCodes    IntOrSlice        `yaml:"exitcodes"`
	StartRetries int               `yaml:"startretries"`
	StartTime    int               `yaml:"starttime"`
	StopSignal   string            `yaml:"stopsignal"`
	StopTime     int               `yaml:"stoptime"`
	Stdout       string            `yaml:"stdout"`
	Stderr       string            `yaml:"stderr"`
	Env          map[string]string `yaml:"env"`
}

type Config struct {
	Programs map[string]Program `yaml:"programs"`
}

func LoadConfig() (*Config, error) {

	_, err := os.Stat("./taskmaster.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}

	file, err := os.ReadFile("./taskmaster.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	err = yaml.Unmarshal(file, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	err = cfg.validate()
	if err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	return &cfg, nil

}

func (cfg *Config) validate() error {
	for name, program := range cfg.Programs {
		if program.Command == "" {
			return fmt.Errorf("program '%s' has an empty command", name)
		}
		if program.NumProcs < 1 {
			return fmt.Errorf("program '%s' must have at least 1 process", name)
		}
		if program.Umask < 0 || program.Umask > 0o777 {
			return fmt.Errorf("program '%s' has an invalid umask: %o", name, program.Umask)
		}
		if program.StartRetries < 0 {
			return fmt.Errorf("program '%s' has a negative startretries value", name)
		}
		if program.StartTime < 0 {
			return fmt.Errorf("program '%s' has a negative starttime value", name)
		}
		if program.StopTime < 0 {
			return fmt.Errorf("program '%s' has a negative stoptime value", name)
		}
	}
	return nil
}
