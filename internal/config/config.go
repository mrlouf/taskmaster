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

	setDefaults(&cfg)

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

// func validate(config *Config) error {
// 	if len(config.Programs) != 1 {
// 		return errors.New("yaml document root structure error")
// 	}
// 	if prog, ok := config.Programs["programs"] ; ok == false{
// 		return errors.New("missing 'programs' key value")
// 	}
// 	for i := 0; i < len(prog); i++ {

// 	}

// 	return nil

// }

// func LoadConfig() (*Config, error) {

// 	conf_file := getConfFile()

// 	_, err := os.Stat(conf_file)
// 	if err != nil {
// 		return nil, fmt.Errorf("config file '%s': %w", conf_file, err)
// 	}

// 	file, err := os.ReadFile(conf_file)
// 	if err != nil {
// 		return nil, fmt.Errorf("config file '%s': %w", conf_file, err)
// 	}

// 	var cfg Config
// 	err = yaml.Unmarshal(file, &cfg)
// 	if err != nil {
// 		return nil, fmt.Errorf("config file '%s': %w", conf_file, err)
// 	}

// 	err = cfg.validate()
// 	if err != nil {
// 		return nil, fmt.Errorf("config file '%s': %w", conf_file, err)
// 	}

// 	return &cfg, nil

// }

// func (cfg *Config) validate() error {
// 	for name, program := range cfg.Programs {
// 		if program.Command == "" {
// 			return fmt.Errorf("program '%s' has an empty command", name)
// 		}
// 		if program.NumProcs < 1 {
// 			return fmt.Errorf("program '%s' must have at least 1 process", name)
// 		}
// 		if program.Umask < 0 || program.Umask > 0o777 {
// 			return fmt.Errorf("program '%s' has an invalid umask: %o", name, program.Umask)
// 		}
// 		if program.StartRetries < 0 {
// 			return fmt.Errorf("program '%s' has a negative startretries value", name)
// 		}
// 		if program.StartTime < 0 {
// 			return fmt.Errorf("program '%s' has a negative starttime value", name)
// 		}
// 		if program.StopTime < 0 {
// 			return fmt.Errorf("program '%s' has a negative stoptime value", name)
// 		}
// 	}
// 	return nil
// }
