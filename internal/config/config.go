package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/mrlouf/taskmaster/internal/logger"
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
	Mu         sync.Mutex
	Programs   map[string]Program `yaml:"programs"`
	ConfigPath string
}

func getConfFilePath() string {

	var path string
	flag.StringVar(&path, "c", "./taskmaster.conf", "Path to config file")
	flag.StringVar(&path, "config", "./taskmaster.conf", "Path to config file")
	flag.Parse()
	return path

}

func setDefaults(config *Config) {

	for name, program := range config.Programs {

		if program.NumProcs == 0 {
			program.NumProcs = 1
		}
		/* 		if program.Umask == 0 {
			program.Umask = 0o022
		} */
		if program.WorkingDir == "" {
			program.WorkingDir = "/tmp"
		}
		if program.AutoRestart == "" {
			program.AutoRestart = "unexpected"
		}
		if len(program.ExitCodes) == 0 {
			program.ExitCodes = []int{0}
		}
		if program.StopSignal == "" {
			program.StopSignal = "TERM"
		}
		if program.StopTime == 0 {
			program.StopTime = 10
		}
		if program.Stdout == "" {
			program.Stdout = fmt.Sprintf("/tmp/%s.stdout", program.Command)
		}
		if program.Stderr == "" {
			program.Stderr = fmt.Sprintf("/tmp/%s.stderr", program.Command)
		}
		// * Note: since program is a copy of the struct in the map,
		// * we need to assign it back to the map after modifying it.
		config.Programs[name] = program
	}

}

func getNodeConfig(file *os.File) (*Config, error) {

	cfg := &Config{}

	loader, err := io.ReadAll(file)
	if err != nil {
		return cfg, err
	}
	if err = yaml.Load(loader, &cfg, yaml.WithKnownFields()); err != nil {
		return cfg, err
	}

	setDefaults(cfg)

	return cfg, nil
}

func LoadConfig(path string) (*Config, error) {
	//open file
	if path == "" {
		path = getConfFilePath()
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config file '%s': %w", path, err)
	}
	cfg, err := getNodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("error while parsing file '%s': %w", path, err)
	}
	cfg.ConfigPath = path
	if err = validate(cfg); err != nil {
		// fmt.Printf("err %v", err)
		return nil, fmt.Errorf("configuration file format error '%s':\n%w", path, err)
	}
	return cfg, nil
}

func (c *Config) existingProgram(name string) bool {
	_, exists := c.Programs[name]

	return exists
}

func (c *Config) addProgram(p *Program, name string, logger logger.Logger) {
	if c.Programs == nil {
		c.Programs = make(map[string]Program)
	}
	newProg := p.copyProgram(logger)
	c.Programs[name] = *newProg

	logger.Log(fmt.Sprintf("[DEBUG] Added program %s to config\n", name))
	logger.Log(fmt.Sprintf("[DEBUG] Program details: %+v\n", c.Programs[name]))
}

func (p *Program) copyProgram(logger logger.Logger) *Program {
	logger.Log("[DEBUG] Copy Program\n")
	copyprog := &Program{
		Command:      p.Command,
		NumProcs:     p.NumProcs,
		Umask:        p.Umask,
		WorkingDir:   p.WorkingDir,
		AutoStart:    p.AutoStart,
		AutoRestart:  p.AutoRestart,
		StartRetries: p.StartRetries,
		StartTime:    p.StartTime,
		StopSignal:   p.StopSignal,
		StopTime:     p.StopTime,
		Stdout:       p.Stdout,
		Stderr:       p.Stderr,
	}
	if p.ExitCodes != nil {
		copyprog.ExitCodes = make(IntOrSlice, len(p.ExitCodes))
		copy(copyprog.ExitCodes, p.ExitCodes)
	}
	if p.Env != nil {
		copyprog.Env = make(map[string]string, len(p.Env))
		for k, v := range p.Env {
			copyprog.Env[k] = v
		}
	}
	return copyprog
}

func ReloadConfig(Current *Config, path string, logger logger.Logger) (*Config, error) {
	Deletion := &Config{}
	if path == "" {
		path = Current.ConfigPath
	}
	NewCfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	logger.Log("New config file reloaded\n")

	toBeDeleted := make(map[string]Program)

	for name, program := range Current.Programs {
		if !NewCfg.existingProgram(name) {
			Current.Mu.Lock()
			logger.Log(fmt.Sprintf("Program %s not found in new file, to be deleted\n", name))
			Deletion.addProgram(&program, name, logger)
			logger.Log("Added to Deletion\n")
			toBeDeleted[name] = program
			Current.Mu.Unlock()
		} else {
			Current.Mu.Lock()
			logger.Log(fmt.Sprintf("Updating current config of %s\n", name))
			Current.Programs[name] = NewCfg.Programs[name]
			Current.Mu.Unlock()
		}
	}
	for name := range toBeDeleted {
		delete(Current.Programs, name)
		logger.Log("Deleted\n")
	}
	for name, program := range NewCfg.Programs {
		if !Current.existingProgram(name) {
			Current.Mu.Lock()
			logger.Log("Adding new program to config\n")
			Current.addProgram(&program, name, logger)
			Current.Mu.Unlock()
		}
	}
	Current.ConfigPath = path
	return Deletion, nil
}
