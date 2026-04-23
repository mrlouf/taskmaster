package config

import (
	"errors"
	"fmt"
)

var ErrInvalidPath = errors.New("detected forbidden null character")
var ErrNumProc = errors.New("NumProcs must be a positive integer > 0")
var ErrUmask = errors.New("invalid umask code")
var ErrAutoRestart = errors.New("invalid keyword for AutoRestart")
var ErrStopSignal = errors.New("invalid keyword for StopSignal")
var ErrEnvFormat = errors.New("invalid env format")

func isnotnullchar(str string) error {
	for c := range str {
		if c == 0 {
			return ErrInvalidPath
		}
	}
	return nil
}

func ispositivedigit(input any) error {
	switch n := input.(type) {
	case int:
		if n <= 0 {
			return ErrNumProc
		}
	case []int:
		for x := range n {
			if x <= 0 {
				return ErrNumProc
			}
		}
	}
	return nil
}

func validautorestart(str string) error {
	var allowed = []string{"true", "false", "unexpected"}
	for _, value := range allowed {
		if str == value {
			return nil
		}
	}
	return ErrAutoRestart
}

func validstopsignal(str string) error {
	var allowed = []string{"TERM", "HUP", "INT", "QUIT", "KILL", "USR1", "USR2"}
	for _, value := range allowed {
		if str == value {
			return nil
		}
	}
	return ErrStopSignal
}

func validenv(str map[string]string) error {
	for key, value := range str {
		if key == "" || value == "" {
			return ErrEnvFormat
		}
	}
	return nil
}

// func validumask(n int) error {

// 	return nil
// }

func validate(config *Config) error {
	var errs []error

	for name, program := range config.Programs {
		if err := isnotnullchar(program.Command); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field Command: %w", name, err))
		}
		// if err := validumask(program.Umask); err != nil {
		// 	errs = append(errs, fmt.Errorf("Program %s, Field Umask: %w", name, err))
		// }
		if err := isnotnullchar(program.WorkingDir); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field WorkingDir: %w", name, err))
		}
		if err := validautorestart(program.AutoRestart); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field AutoRestart: %w", name, err))
		}
		if err := ispositivedigit(program.ExitCodes); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field ExitCodes: %w", name, err))
		}
		if err := ispositivedigit(program.StartRetries); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field StartRetries: %w", name, err))
		}
		if err := ispositivedigit(program.StartTime); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field StartTime: %w", name, err))
		}
		if err := validstopsignal(program.StopSignal); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field StopSignal: %w", name, err))
		}
		if err := ispositivedigit(program.StopTime); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field StopTime: %w", name, err))
		}
		if err := isnotnullchar(program.Stdout); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field Stdout: %w", name, err))
		}
		if err := isnotnullchar(program.Stderr); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field Stderr: %w", name, err))
		}
		if err := validenv(program.Env); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field Env: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
