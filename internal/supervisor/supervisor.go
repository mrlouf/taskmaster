package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/logger"
	"github.com/mrlouf/taskmaster/internal/protocol"
)

type State int

const (
	STOPPED State = iota
	STARTING
	RUNNING
	STOPPING
	EXITED
	BACKOFF
	FATAL
)

func (s State) String() string {
	switch s {
	case STOPPED:
		return "STOPPED"
	case STARTING:
		return "STARTING"
	case RUNNING:
		return "RUNNING"
	case STOPPING:
		return "STOPPING"
	case EXITED:
		return "EXITED"
	case BACKOFF:
		return "BACKOFF"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

type Process struct {
	Name   string
	Config *config.Program

	cmd       *exec.Cmd
	state     State
	mu        sync.Mutex
	pid       int
	startedAt time.Time
	done      chan error
	retries   int
}

type Supervisor struct {
	Config    *config.Config
	Logger    *logger.Logger
	Processes map[string]*Process
	Events    chan Event
	Ready     chan bool
}

func New(config *config.Config, logger *logger.Logger) *Supervisor {

	return &Supervisor{
		Config:    config,
		Logger:    logger,
		Processes: make(map[string]*Process),
		Events:    make(chan Event, 100),
		Ready:     make(chan bool, 1),
	}
}

func (s *Supervisor) handleReload() error {
	if ToDel, err := config.ReloadConfig(s.Config); err != nil {
		return err
	} else if ToDel != nil {
		for name := range ToDel.Programs {
			s.stopProcess(name)
		}
		ToDel = nil
	}
	for name, program := range s.Config.Programs {
		if program.AutoStart {
			if err := s.startProcess(name); err != nil {
				s.Logger.Log(fmt.Sprintf("Failed to auto-start program during reload'%s': %v", name, err))
			}
		}
	}
	return nil
}

func (s *Supervisor) autoStartProcesses() {

	// * This could be optimised using a worker pool to gather all programs in parallel
	// * using goroutines and a WaitGroup, but it could also be risky and probably overkill
	for name, program := range s.Config.Programs {

		process := &Process{
			Name:    name,
			Config:  &program,
			state:   STOPPED,
			retries: 0,
		}
		s.Processes[name] = process

		if program.AutoStart {
			s.Logger.Log(fmt.Sprintf("Auto-starting program '%s' with command: %s", name, program.Command))
			err := s.startProcess(name)
			if err != nil {
				s.Logger.Log(fmt.Sprintf("Failed to auto-start program '%s': %v", name, err))
			}
		}
	}
}

func (s *Supervisor) GetStatus(name string) string {

	process, exists := s.Processes[name]
	if !exists {
		return fmt.Sprintf("'%s' not found", name)
	}

	process.mu.Lock()
	state := process.state.String()
	pid := process.pid
	process.mu.Unlock()

	return fmt.Sprintf("%s\t\t\t%s pid %d", name, state, pid)

}

// The monitorProcess function is responsible for monitoring the lifecycle
// of a process after it has been started. It only monitors and reports the death
// or the readiness of the process, but the State transition is the responsibility
// of the event handlers (handleReady and handleDied).
func (s *Supervisor) monitorProcess(process *Process, cfg config.Program) {
	startTimer := time.NewTimer(time.Duration(cfg.StartTime) * time.Second)

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- process.cmd.Wait()
	}()

	select {
	// Process is done before start timer expires: could be a crash or a very fast process
	// In both cases we consider the process as having died, and we won't transition to ready state
	// The error from Wait() will be sent to the process.done channel
	// and the handleDied handler will decide what to do based on process state and retry policy
	case err := <-waitDone:

		startTimer.Stop()
		process.done <- err
		s.Events <- Event{Kind: EventProcessDied, Name: process.Name, Err: err}
		return

	// Timer reaches zero: process is considered ready
	case <-startTimer.C:
		s.Events <- Event{Kind: EventProcessReady, Name: process.Name}
	}

	err := <-waitDone
	process.done <- err

	// s.Events <- Event{Kind: EventProcessDied, Name: process.Name, Err: err}
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

func didProcessExitExpectedly(state *os.ProcessState, cfg config.Program) bool {

	if state == nil {
		return false
	}

	exitCode := state.ExitCode()

	return slices.Contains(cfg.ExitCodes, exitCode)
}

// Whenever a process dies, the monitorProcess will send an EventProcessDied event
// to the main event loop which will the trigger this handler. Its role is to triage
// the process's death and decide whether it should be revived or not based on the state,
// the restart policy and the number of retries already attempted.

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

func (s *Supervisor) Start() {

	fmt.Printf("Starting processes from config file located in '%s'\n", s.Config.ConfigPath)
	s.autoStartProcesses()
	s.Ready <- true

	for event := range s.Events {
		switch event.Kind {

		case EventProcessDied:

			fmt.Printf("[DEBUG] Received process died event for program '%s'\n", event.Name)
			s.handleDied(event)

		case EventProcessReady:

			fmt.Printf("[DEBUG] Received process ready event for program '%s'\n", event.Name)
			err := s.handleReady(event.Name)
			if err != nil {
				s.Logger.Log(fmt.Sprintf("Failed to handle ready event for program '%s': %v", event.Name, err))
			}

		case EventStartProcess:

			fmt.Printf("[DEBUG] Received start event for program '%s'\n", event.Name)
			err := s.startProcess(event.Name)
			if event.RespCh != nil {
				resp := protocol.Response{Ok: err == nil}
				if err != nil {
					resp.Msg = err.Error()
				} else {
					resp.Msg = fmt.Sprintf("Program '%s' started", event.Name)
				}
				event.RespCh <- resp
			}

		case EventStopProcess:

			fmt.Printf("[DEBUG] Received stop event for program '%s'\n", event.Name)
			err := s.stopProcess(event.Name)
			if event.RespCh != nil {
				event.RespCh <- protocol.Response{Ok: err == nil}
			}

		case EventReloadConfig:
			fmt.Printf("[DEBUG] Received reload event\n")
			err := s.handleReload()
			if event.RespCh != nil {
				event.RespCh <- protocol.Response{Ok: err == nil}
			}

		case EventShutdown:

			fmt.Printf("[DEBUG] Received shutdown event\n")
			s.handleShutdown()
			if event.RespCh != nil {
				event.RespCh <- protocol.Response{Ok: true, Msg: "Shut down complete"}
			}
			os.Exit(0)
		}
	}
}
