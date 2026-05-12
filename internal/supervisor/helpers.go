package supervisor

import (
	"fmt"
	"os"
	"slices"
	"syscall"

	"github.com/mrlouf/taskmaster/internal/config"
)

func (s *Supervisor) updateIdx(name string) {
	// s.bigmu.Lock()
	// defer s.bigmu.Unlock()
	for i := 0; i < len(s.Processes[name]); i++ {
		s.Processes[name][i].mu.Lock()
		s.Processes[name][i].idx = i
		s.Processes[name][i].mu.Unlock()
	}
}

func didProcessExitExpectedly(state *os.ProcessState, cfg config.Program) bool {

	if state == nil {
		return false
	}

	exitCode := state.ExitCode()

	return slices.Contains(cfg.ExitCodes, exitCode)
}

func (s *Supervisor) sizedownProcesses(name string, n int) int {
	var deleted int
	//since I would touch the whole Process map slice in supervisor, I use a mutex in the Supervisor struct directly
	// s.bigmu.Lock()
	// defer s.bigmu.Unlock()
	//processes := s.Processes[name]
	if len(s.Processes[name]) == 0 {
		return deleted
	}
	//deadlocks with 2 imbricated locks?
	x := len(s.Processes[name])
	for i := x - 1; i >= 0 && n > 0; i-- {
		s.Processes[name][i].mu.Lock()
		isActive := s.Processes[name][i].state == RUNNING || s.Processes[name][i].state == STARTING || s.Processes[name][i].state == BACKOFF
		s.Processes[name][i].mu.Unlock()
		//first removing any process not running or in starting condition
		if isActive == false {
			//switching last value of the slice with the process to remove from the slice
			s.Processes[name][i], s.Processes[name][len(s.Processes[name])-1] = s.Processes[name][len(s.Processes[name])-1], s.Processes[name][i]

			//removing the last element
			s.Processes[name] = s.Processes[name][:len(s.Processes[name])-1]
			n--
			deleted++

		}

	}
	if len(s.Processes[name]) == 0 {
		return deleted
	}
	//if n > 0 we need to stop some other processes, the running ones starting from the last
	if n > 0 {
		x := len(s.Processes[name])
		for i := x - 1; i >= 0 && n > 0; i-- {
			// s.Processes[name][i].mu.Lock()
			//stopping process
			if err := s.stopProcess(s.Processes[name][i], s.Config.Programs[name]); err != nil {
				continue
			}
			//switching last value of the slice with the process to remove from the slice
			s.Processes[name][i], s.Processes[name][len(s.Processes[name])-1] = s.Processes[name][len(s.Processes[name])-1], s.Processes[name][i]
			//s.Processes[name][i].mu.Unlock()
			//removing the last element
			s.Processes[name] = s.Processes[name][:len(s.Processes[name])-1]
			n--
			deleted++

		}
	}
	return deleted
}

// Helper function to convert env map to slice of "KEY=VALUE" strings
// Needed since exec.Cmd.Env expects a slice of strings in the format "KEY=VALUE"
// but our config uses a map[string]string for environment variables
func convertEnvMapToSlice(envMap map[string]string) []string {

	env := make([]string, 0, len(envMap))
	for key, value := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	return env
}

func getSignalByName(name string) syscall.Signal {

	switch name {
	case "TERM":
		return syscall.SIGTERM
	case "HUP":
		return syscall.SIGHUP
	case "INT":
		return syscall.SIGINT
	case "KILL":
		return syscall.SIGKILL
	case "USR1":
		return syscall.SIGUSR1
	case "USR2":
		return syscall.SIGUSR2
	default:
		return syscall.SIGTERM // default to SIGTERM if unknown signal is specified
	}
}
