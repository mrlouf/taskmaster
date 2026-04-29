package config

import (
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
	Programs   map[string]Program `yaml:"programs"`
	ConfigPath string
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

	loader, err := io.ReadAll(file)
	if err != nil {
		return cfg, err
	}
	if err = yaml.Load(loader, &cfg, yaml.WithKnownFields()); err != nil {
		return cfg, err
	}
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
	if err = validate(&cfg); err != nil {
		// fmt.Printf("err %v", err)
		return nil, fmt.Errorf("configuration file format error '%s':\n%w", path, err)
	}
	return &cfg, nil
}

func (c *Config) existingProgram(name string) bool {
	if _, ok := c.Programs[name]; ok {
		return true
	}
	return false
}

// func (p *Program) updateProgram(new *Program) {
// 	p.Command = new.Command
// 	p.NumProcs = new.NumProcs
// 	p.Umask = new.Umask
// 	p.WorkingDir = new.WorkingDir
// 	p.AutoStart = new.AutoStart
// 	p.AutoRestart = new.AutoRestart
// 	p.StartRetries = new.StartRetries
// 	p.StartTime = new.StartTime
// 	p.StopSignal = new.StopSignal
// 	p.StopTime = new.StopTime
// 	p.Stdout = new.Stdout
// 	p.Stderr = new.Stderr
// 	p.ExitCodes = make(IntOrSlice)
// 	for k, v := range new.ExitCodes {
// 		p.ExitCodes[k] = v
// 	}
// 	p.Env := make(map[string]string)
// 	for k, v := range new.Env {
// 		p.Env[k] = v
// 	}
// }

func (c *Config) addProgram(p *Program, name string) {
	if c.Programs == nil {
		c.Programs = make(map[string]Program)
	}
	newProg := p.copyProgram()
	c.Programs[name] = *newProg
}

func (p *Program) copyProgram() *Program {
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
		for k, v := range p.ExitCodes {
			copyprog.ExitCodes[k] = v
		}
	}
	if p.Env != nil {
		copyprog.Env = make(map[string]string, len(p.Env))
		for k, v := range p.Env {
			copyprog.Env[k] = v
		}
	}
	return copyprog
}

func ReloadConfig(Current *Config) *Config {
	var Deletion *Config

	NewCfg, err := LoadConfig(Current.ConfigPath)
	if err != nil {
		return nil
	}
	for name, program := range Current.Programs {
		if !NewCfg.existingProgram(name) {
			Deletion.addProgram(&program, name)
			delete(Current.Programs, name)
		} else {
			Current.Programs[name] = NewCfg.Programs[name]
		}
	}
	for name, program := range NewCfg.Programs {
		if !Current.existingProgram(name) {
			Current.addProgram(&program, name)
		}
	}
	return Deletion
}
