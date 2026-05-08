package config

import (
	"errors"
	"fmt"
)

var ErrInvalidPath = errors.New("detected forbidden null character")
var ErrNumSPos = errors.New("must be a positive integer > 0")
var ErrNumPos = errors.New("must be a positive integer >= 0")
var ErrUmask = errors.New("invalid umask code")
var ErrAutoStart = errors.New("invalid keyword for AutoStart")
var ErrAutoRestart = errors.New("invalid keyword for AutoRestart")
var ErrStopSignal = errors.New("invalid keyword for StopSignal")
var ErrEnvFormat = errors.New("invalid env format")

func isnotnullchar(str string) error {
	for i := 0; i < len(str); i++ {
		if str[i] == 0 {
			return ErrInvalidPath
		}
	}
	return nil
}

func ispositivedigit(input any, strict bool) error {
	switch n := input.(type) {
	case int:
		if strict == true && n <= 0 {
			return ErrNumSPos
		} else if strict == false && n < 0 {
			return ErrNumPos
		}
	case []int:
		for x := range n {
			if strict == true && x <= 0 {
				return ErrNumSPos
			} else if strict == false && x < 0 {
				return ErrNumPos
			}
		}
	}
	return nil
}

func validautorestart(str string) error {
	var allowed = []string{"always", "unexpected", "never"}
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
		if err := ispositivedigit(program.NumProcs, true); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field NumProcs: %w", name, err))
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
		if err := ispositivedigit(program.ExitCodes, false); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field ExitCodes: %w", name, err))
		}
		if err := ispositivedigit(program.StartRetries, false); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field StartRetries: %w", name, err))
		}
		if err := ispositivedigit(program.StartTime, false); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field StartTime: %w", name, err))
		}
		if err := validstopsignal(program.StopSignal); err != nil {
			errs = append(errs, fmt.Errorf("Program %s, Field StopSignal: %w", name, err))
		}
		if err := ispositivedigit(program.StopTime, false); err != nil {
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
