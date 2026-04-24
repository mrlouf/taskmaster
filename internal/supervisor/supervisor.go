package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sync"
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
			s.Logger.Log(fmt.Sprintf("Auto-STARTING program '%s' with command: %s", name, program.Command))
			err := s.startProcess(name)
			if err != nil {
				s.Logger.Log(fmt.Sprintf("Failed to auto-start program '%s': %v", name, err))
			}
		}
	}
}

func (s *Supervisor) handleShutdown() {

	fmt.Printf("[DEBUG] Received shutdown event, STOPPING supervisor...\n")
	s.Logger.Log("Shutting down supervisor...")

	var wg sync.WaitGroup

	for name, process := range s.Processes {

		// Handle the stopping of each process in a separate goroutine
		// instead of sending events to the main event loop
		// to avoid potential deadlocks and ensure a faster shutdown
		wg.Go(func() {

			process.mu.Lock()
			defer process.mu.Unlock()

			// ! Override restart policy to prevent any restarts during shutdown
			process.Config.AutoRestart = "never"

			if process.state == RUNNING ||
				process.state == STARTING ||
				process.state == BACKOFF {

				cfg := s.Config.Programs[name]

				signal := syscall.SIGTERM
				fmt.Printf("Sending signal %d to process '%s' with PID %d\n", signal, name, process.pid)
				process.cmd.Process.Signal(signal)

				select {
				case process.cmd.Err = <-process.done:
					fmt.Printf("[DEBUG] Process '%s' has exited gracefully\n", name)
					s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has exited gracefully", name, process.pid))

				case <-time.After(time.Duration(cfg.StopTime) * time.Second):
					process.cmd.Process.Kill()
					process.cmd.Err = <-process.done

					fmt.Printf("[DEBUG] Process '%s' did not exit gracefully, sent KILL signal\n", name)
					s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d did not exit gracefully, sent KILL signal", name, process.pid))
				}
			}
		})
	}

	wg.Wait()

	fmt.Printf("[DEBUG] Supervisor shutdown complete\n")
	s.Logger.Log("Supervisor shutdown complete")

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

	return fmt.Sprintf("%s\t\t%s pid %d", name, state, pid)

}

func didProcessExitExpectedly(state *os.ProcessState, cfg config.Program) bool {

	if state == nil {
		return false
	}

	exitCode := state.ExitCode()

	return slices.Contains(cfg.ExitCodes, exitCode)
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
		expectedExit := didProcessExitExpectedly(process.cmd.ProcessState, cfg)
		process.mu.Lock()
		if expectedExit {
			fmt.Printf("[DEBUG] Process '%s' has EXITED normally with exit code %d\n", process.Name, process.cmd.ProcessState.ExitCode())
			s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has EXITED normally with exit code %d", process.Name, process.pid, process.cmd.ProcessState.ExitCode()))
			process.state = EXITED
		} else {
			fmt.Printf("[DEBUG] Process '%s' has CRASHED with error: %v\n", process.Name, err)
			s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has CRASHED with error: %v", process.Name, process.pid, err))
			process.state = BACKOFF
		}

		process.mu.Unlock()
		return

	// Timer reaches zero: process is considered ready
	case <-startTimer.C:
		s.Events <- Event{Kind: EventProcessReady, Name: process.Name}
	}

	err := <-waitDone
	process.mu.Lock()
	process.done <- err
	process.state = EXITED
	process.mu.Unlock()
	s.Events <- Event{Kind: EventProcessDied, Name: process.Name, Err: err}
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

func (s *Supervisor) startProcess(name string) error {

	cfg := s.Config.Programs[name]
	process, exists := s.Processes[name]
	if !exists {
		fmt.Printf("[DEBUG] Process '%s' not found in supervisor, STARTING\n", name)
		process = &Process{Name: name, Config: &cfg}
		s.Processes[name] = process
	}

	process.mu.Lock()
	isActive := process.state == RUNNING || process.state == STARTING
	process.mu.Unlock()

	if exists && isActive {
		return fmt.Errorf("process '%s' is already RUNNING or STARTING with PID %d", name, process.pid)
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
	process.mu.Lock()
	process.state = STARTING
	process.mu.Unlock()
	process.done = make(chan error, 1)

	go s.monitorProcess(process, cfg)

	if process.retries > 0 {
		s.Logger.Log(fmt.Sprintf("Restarted process '%s' with PID %d (retry %d)", name, process.pid, process.retries))
	} else {
		s.Logger.Log(fmt.Sprintf("Started process '%s' with PID %d", name, process.pid))
	}

	return nil
}

func (s *Supervisor) handleReady(name string) error {

	process, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("process '%s' not found in supervisor", name)
	}

	if process.state != STARTING {
		return fmt.Errorf("process '%s' is not in STARTING state, cannot transition to ready", name)
	}

	process.state = RUNNING
	s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d is now RUNNING", name, process.pid))

	return nil
}

// Whenever a process has died, whether it EXITED normally, it crashed, it was SIG-killed etc.,
// we will receive an EventProcessDied event which will trigger this handler.
// The handler will check the process state and retry policy to decide whether to attempt a restart,
// mark the process as EXITED, STOPPED or FATAL, and log the event accordingly.
func (s *Supervisor) handleDied(event Event) {

	process, exists := s.Processes[event.Name]
	if !exists {
		s.Logger.Log(fmt.Sprintf("Received process died event for unknown process '%s'", event.Name))
		return
	}

	// * DEBUG
	fmt.Printf("[DEBUG] Process '%s' has died", event.Name)
	if event.Err != nil {
		fmt.Printf(" with error: %v", event.Err)
	}
	fmt.Printf("\n")

	process.mu.Lock()
	defer process.mu.Unlock()

	// * DEBUG
	fmt.Println("\n[DEBUG] Process state before handling death:", process.state.String())
	fmt.Println(process.cmd)
	fmt.Println(process.cmd.ProcessState)
	fmt.Println(process.cmd.ProcessState.ExitCode())
	fmt.Println(event.Err)
	fmt.Println()

	// STOPPING means a stop signal was sent and the process has now EXITED,
	// so we can consider the process as having been STOPPED successfully.
	if process.state == STOPPING || process.state == EXITED {

		fmt.Printf("[DEBUG] Process '%s' has been STOPPED successfully\n", event.Name)
		s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has been STOPPED", event.Name, process.pid))
		if process.state == STOPPING {
			process.state = STOPPED
		}
		process.retries = 0
		process.done = nil
		process.pid = 0

		return
	}

	// If the process was not in the process of being STOPPED,
	// then it means the process has died unexpectedly (crash, SIGKILL, etc.)
	// and we should check the retry policy to decide whether to attempt a restart or mark as FATAL
	if event.Err != nil {
		if process.Config.AutoRestart == "never" {
			fmt.Printf("[DEBUG] Process '%s' has crashed with error: %v. Restart policy is 'never', marking as EXITED\n", event.Name, event.Err)
			s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has crashed: %v and will not be restarted", event.Name, process.pid, event.Err))
			process.state = EXITED
			process.retries = 0
			return
		}
		if process.retries < process.Config.StartRetries {
			fmt.Printf("[DEBUG] Process '%s' has crashed with error: %v. Attempting restart (%d/%d)\n", event.Name, event.Err, process.retries+1, process.Config.StartRetries)
			s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has crashed: %v. Attempting restart (%d/%d)", event.Name, process.pid, event.Err, process.retries+1, process.Config.StartRetries))
			process.retries++
			process.state = BACKOFF

			go func() {
				// * DEBUG: simulate BACKOFF delay before restart
				// TODO: implement BACKOFF algo based on config (fixed delay, exponential BACKOFF, etc.)
				time.Sleep(time.Duration(process.retries) * time.Second)

				event := Event{Kind: EventStartProcess, Name: event.Name}
				s.Events <- event
			}()

			return

		} else {

			fmt.Printf("[DEBUG] Process '%s' has crashed with error: %v. Maximum retries reached (%d/%d), marking as FATAL\n", event.Name, event.Err, process.retries, process.Config.StartRetries)
			s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has crashed: %v. Maximum retries reached (%d/%d), marking as FATAL", event.Name, process.pid, event.Err, process.retries, process.Config.StartRetries))
			process.state = FATAL
			process.retries = 0
			process.pid = 0

			return
		}
	}

	// ? implement restart policy for normal exits if AutoRestart: "always"?

	s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has EXITED", event.Name, process.pid))

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

func (s *Supervisor) stopProcess(name string) error {

	cfg := s.Config.Programs[name]
	process, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("process '%s' not found in supervisor", name)
	}

	if process.state != RUNNING && process.state != STARTING {
		return fmt.Errorf("process '%s' is not RUNNING or STARTING, cannot stop", name)
	}

	signal := getSignalByName(cfg.StopSignal)
	fmt.Printf("Sending signal %s to process '%s' with PID %d\n", cfg.StopSignal, name, process.pid)
	process.cmd.Process.Signal(signal)

	process.mu.Lock()
	process.state = STOPPING
	process.mu.Unlock()

	// Wait for the process to exit gracefully,
	// but if it doesn't exit within the StopTime, force kill it
	select {
	case process.cmd.Err = <-process.done:

	case <-time.After(time.Duration(cfg.StopTime) * time.Second):
		process.cmd.Process.Kill()
		process.cmd.Err = <-process.done
	}

	s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has been STOPPED", name, process.pid))

	return nil

}

func (s *Supervisor) Start() {

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

			/* case EventReloadConfig:
			s.handleReload() */

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
