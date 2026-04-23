package config

import "errors"

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
	var err [13]error
	//how to add line number
	for _, program := range config.Programs {
		err[0] = isnotnullchar(program.Command)
		err[1] = ispositivedigit(program.NumProcs)
		// err[2] = validumask(program.Umask)
		err[3] = isnotnullchar(program.WorkingDir)
		err[4] = validautorestart(program.AutoRestart)
		err[5] = ispositivedigit(program.ExitCodes)
		err[6] = ispositivedigit(program.StartRetries)
		err[7] = ispositivedigit(program.StartTime)
		err[8] = validstopsignal(program.StopSignal)
		err[9] = ispositivedigit(program.StopTime)
		err[10] = isnotnullchar(program.Stdout)
		err[11] = isnotnullchar(program.Stderr)
		err[12] = validenv(program.Env)
	}

	return nil
}
