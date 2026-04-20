package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	// SignalNum()

	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/logger"
	"github.com/mrlouf/taskmaster/internal/protocol"
)

type EventKind int

const (
	EventProcessDied EventKind = iota
	EventProcessReady
	EventStartProcess
	EventStopProcess
	EventRestartProcess
	EventReloadConfig
	EventShutdown
	EventStatusRequest
)

type Event struct {
	Kind   EventKind
	Name   string                 // nom du programme
	Index  int                    // numéro d'instance si numprocs > 1
	Err    error                  // pour EventProcessDied
	RespCh chan protocol.Response // pour les commandes qui attendent une réponse
}

type State int

const (
	Stopped State = iota
	Starting
	Running
	Stopping
	Exited
	Fatal
)

func (s State) String() string {
	switch s {
	case Stopped:
		return "Stopped"
	case Starting:
		return "Starting"
	case Running:
		return "Running"
	case Stopping:
		return "Stopping"
	case Exited:
		return "Exited"
	case Fatal:
		return "Fatal"
	default:
		return "Unknown"
	}
}

type Process struct {
	Name   string
	Config *config.Program

	cmd       *exec.Cmd
	state     State
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
}

func New(config *config.Config, logger *logger.Logger) *Supervisor {

	return &Supervisor{
		Config:    config,
		Logger:    logger,
		Processes: make(map[string]*Process),
		Events:    make(chan Event, 100),
	}

}

func (s *Supervisor) autoStartProcesses() {

	for name, program := range s.Config.Programs {

		process := &Process{
			Name:    name,
			Config:  &program,
			state:   Stopped,
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

func (s *Supervisor) handleShutdown() {

	s.Logger.Log("Shutting down supervisor...")

	/* 	for name := range s.Processes {
		s.stopProcess(name)
	} */

	s.Logger.Log("Supervisor shutdown complete")

}

func (s *Supervisor) GetStatus(name string) string {

	process, exists := s.Processes[name]
	if !exists {
		return fmt.Sprintf("Program '%s' not found", name)
	}

	return fmt.Sprintf("Program '%s' is in state %s with PID %d", name, process.state, process.pid)

}

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
	s.Events <- Event{Kind: EventProcessDied, Name: process.Name, Err: err}
}

// Helper function to convert env map to slice of "KEY=VALUE" strings
// Needed since exec.Cmd.Env expects a slice of strings in the format "KEY=VALUE"
func convertEnvMapToSlice(envMap map[string]string) []string {

	env := make([]string, 0, len(envMap))
	for key, value := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	return env
}

func (s *Supervisor) startProcess(name string) error {

	cfg := s.Config.Programs[name]
	process, exists := s.Processes[name]
	if !exists {
		fmt.Printf("Process '%s' not found in supervisor, starting\n", name)
		process = &Process{Name: name, Config: &cfg}
		s.Processes[name] = process
	}

	if exists && (process.state == Running || process.state == Starting) {
		return fmt.Errorf("process '%s' is already running or starting with PID %d", name, process.pid)
	}

	cmd := exec.Command("/bin/sh", "-c", cfg.Command)

	env := os.Environ()
	env = append(env, convertEnvMapToSlice(cfg.Env)...)
	cmd.Env = env
	cmd.Dir = cfg.WorkingDir

	// TODO: handle stdout/stderr redirection to files if specified in config
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start process '%s': %w", name, err)
	}

	process.cmd = cmd
	process.pid = cmd.Process.Pid
	process.startedAt = time.Now()
	process.state = Starting
	process.done = make(chan error, 1)

	process.retries++

	go s.monitorProcess(process, cfg)

	s.Logger.Log(fmt.Sprintf("Started process '%s' with PID %d", name, process.pid))

	return nil
}

func (s *Supervisor) handleReady(name string) error {

	process, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("process '%s' not found in supervisor", name)
	}

	if process.state != Starting {
		return fmt.Errorf("process '%s' is not in starting state, cannot transition to ready", name)
	}

	process.state = Running
	s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d is now running", name, process.pid))

	return nil
}

// Whenever a process has died, whether it exited normally, it crashed, it was SIG-killed etc.,
// we will receive an EventProcessDied event which will trigger this handler.
// The handler will check the process state and retry policy to decide whether to attempt a restart,
// mark the process as exited, stopped or fatal, and log the event accordingly.
func (s *Supervisor) handleDied(event Event) {

	process, exists := s.Processes[event.Name]
	if !exists {
		s.Logger.Log(fmt.Sprintf("Received process died event for unknown process '%s'", event.Name))
		return
	}

	fmt.Printf("[DEBUG] Process '%s' has died\n", event.Name)

	if process.state == Stopping {
		s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has been stopped", event.Name, process.pid))
		process.state = Stopped
		process.retries = 0
		return
	}

	if process.retries >= process.Config.StartRetries {
		s.Logger.Log(fmt.Sprintf("Process '%s' has exceeded retry limit (%d), marking as fatal", event.Name, process.Config.StartRetries))
		process.state = Fatal
		return
	}

	s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has died: %v. Attempting restart (%d/%d)", event.Name, process.pid, process.cmd.Err, process.retries, process.Config.StartRetries))

	err := s.startProcess(event.Name)
	if err != nil {
		s.Logger.Log(fmt.Sprintf("Failed to restart process '%s': %v", event.Name, err))
		// ? Mark process as fatal if restart fails?
	}

	// If the process has exited without error, consider it normal exit and set state to Exited
	// and do not attempt to restart unless the process is configured to always restart (Restart: "always")
	if process.state == Stopping {
		s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has been stopped", event.Name, process.pid))
		process.state = Stopped
	} else {
		s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has exited normally", event.Name, process.pid))
		process.state = Exited
	}
	// ? implement restart policy for normal exits if Restart: "always"?

	s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has exited", event.Name, process.pid))

}

func (s *Supervisor) stopProcess(name string) error {

	cfg := s.Config.Programs[name]
	process, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("process '%s' not found in supervisor", name)
	}

	if process.state != Running && process.state != Starting {
		return fmt.Errorf("process '%s' is not running or starting, cannot stop", name)
	}

	signal := syscall.SIGTERM
	fmt.Printf("Sending signal %d to process '%s' with PID %d\n", signal, name, process.pid)
	process.cmd.Process.Signal(signal)
	process.state = Stopping

	select {
	case process.cmd.Err = <-process.done:

	case <-time.After(time.Duration(cfg.StopTime) * time.Second):
		// toujours vivant → SIGKILL
		process.cmd.Process.Kill()
		process.cmd.Err = <-process.done
	}

	s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has been stopped", name, process.pid))

	return nil

}

func (s *Supervisor) Start() {

	s.autoStartProcesses()

	for event := range s.Events {
		switch event.Kind {

		case EventProcessDied:

			fmt.Printf("Received process died event for program '%s'\n", event.Name)
			s.handleDied(event)

		case EventProcessReady:

			fmt.Printf("Received process ready event for program '%s'\n", event.Name)
			err := s.handleReady(event.Name)
			if err != nil {
				s.Logger.Log(fmt.Sprintf("Failed to handle ready event for program '%s': %v", event.Name, err))
			}

		case EventStartProcess:

			fmt.Printf("Received start event for program '%s'\n", event.Name)
			err := s.startProcess(event.Name)
			if event.RespCh != nil {
				resp := protocol.Response{Ok: err == nil}
				if err != nil {
					resp.Msg = err.Error()
				} else {
					resp.Msg = fmt.Sprintf("Program '%s' started successfully", event.Name)
				}
				event.RespCh <- resp
			}

		case EventStopProcess:

			fmt.Printf("Received stop event for program '%s'\n", event.Name)
			err := s.stopProcess(event.Name)
			if event.RespCh != nil {
				event.RespCh <- protocol.Response{Ok: err == nil}
			}

			/* case EventReloadConfig:
			s.handleReload() */

		case EventShutdown:
			s.handleShutdown()
			return
		}
	}
}
