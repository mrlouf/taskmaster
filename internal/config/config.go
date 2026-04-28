package config

import (
	// "errors"
	"flag"
	"fmt"
	"io"
	"os"

	"go.yaml.in/yaml/v4"
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

func getConfFilePath() string {
	// error management?

	var path string
	flag.StringVar(&path, "c", "./taskmaster.conf", "Path to config file")
	flag.StringVar(&path, "config", "./taskmaster.conf", "Path to config file")
	flag.Parse()
	return path

}

func getNodeConfig(file *os.File) (Config, error) {
	var cfg Config

	// loader, err := yaml.NewLoader(file)
	// if err != nil {
	// 	return config, err
	// }
	loader, err := io.ReadAll(file)
	if err != nil {
		return cfg, err
	}
	if err = yaml.Load(loader, &cfg, yaml.WithKnownFields()); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func LoadConfig() (*Config, error) {
	//open file
	path := getConfFilePath()
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config file '%s': %w", path, err)
	}
	cfg, err := getNodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("error while parsing file '%s': %w", path, err)
	}
	if err = validate(&cfg); err != nil {
		// fmt.Printf("err %v", err)
		return nil, fmt.Errorf("configuration file format error '%s':\n%w", path, err)
	}
	return &cfg, nil
}
