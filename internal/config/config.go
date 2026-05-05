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

func setDefaults(config *Config) {

	for name, program := range config.Programs {

		if program.NumProcs == 0 {
			program.NumProcs = 1
		}
		if program.Umask == 0 {
			program.Umask = 0o022
		}
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

func getNodeConfig(file *os.File) (Config, error) {
	var cfg Config

	loader, err := io.ReadAll(file)
	if err != nil {
		return cfg, err
	}
	if err = yaml.Load(loader, &cfg, yaml.WithKnownFields()); err != nil {
		return cfg, err
	}

	setDefaults(&cfg)

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
	_, exists := c.Programs[name]

	return exists
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

	fmt.Printf("[DEBUG] Added program %s to config\n", name)
	fmt.Printf("[DEBUG] Program details: %+v\n", c.Programs[name])
}

func (p *Program) copyProgram() *Program {
	fmt.Printf("[DEBUG] Copy Program\n")
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

func ReloadConfig(Current *Config) (*Config, error) {
	Deletion := &Config{}
	fmt.Printf("[DEBUG] Starting reloading new file\n")
	NewCfg, err := LoadConfig(Current.ConfigPath)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[DEBUG] New config file reloaded\n")
	for name, program := range Current.Programs {
		if !NewCfg.existingProgram(name) {
			fmt.Printf("[DEBUG] Program %s not found in new file, to be deleted\n", name)
			Deletion.addProgram(&program, name)
			fmt.Printf("[DEBUG] Added to Deletion\n")
			delete(Current.Programs, name)
			fmt.Printf("[DEBUG] Deleted\n")
		} else {
			fmt.Printf("[DEBUG] Updating current config\n")
			Current.Programs[name] = NewCfg.Programs[name]
		}
	}
	for name, program := range NewCfg.Programs {
		if !Current.existingProgram(name) {
			fmt.Printf("[DEBUG] Adding new program to config\n")
			Current.addProgram(&program, name)
		}
	}
	return Deletion, nil
}
