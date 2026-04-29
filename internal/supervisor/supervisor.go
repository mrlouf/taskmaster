package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

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

	idx       int
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
	Processes map[string][]*Process
	Events    chan Event
	Ready     chan bool
}

func New(config *config.Config, logger *logger.Logger) *Supervisor {

	return &Supervisor{
		Config:    config,
		Logger:    logger,
		Processes: make(map[string][]*Process),
		Events:    make(chan Event, 100), // ! 100 is arbitrary
		Ready:     make(chan bool, 1),
	}

}

func (s *Supervisor) autoStartProcesses() {

	// * This could be optimised using a worker pool to gather all programs in parallel
	// * using goroutines and a WaitGroup, but it could also be risky and probably overkill
	for name, program := range s.Config.Programs {

		for i := 0; i < program.NumProcs; i++ {
			process := &Process{
				Name:    name,
				Config:  &program,
				state:   STOPPED,
				retries: 0,
				idx:     i,
			}
			s.Processes[name] = append(s.Processes[name], process)

		}

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

	fmt.Printf("[DEBUG] Received shutdown event, STOPPING supervisor...\n")
	s.Logger.Log("Shutting down supervisor...")

	var wg sync.WaitGroup

	for name, processes := range s.Processes {

		for _, process := range processes {

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
	}

	wg.Wait()

	fmt.Printf("[DEBUG] Supervisor shutdown complete\n")
	s.Logger.Log("Supervisor shutdown complete")

}

func (s *Supervisor) GetStatus(name string) string {

	processes, exists := s.Processes[name]
	if !exists {
		return fmt.Sprintf("'%s' not found", name)
	}

	status := fmt.Sprintf("'%s':\n", name)

	for i, process := range processes {

		process.mu.Lock()
		state := process.state.String()
		pid := process.pid
		process.mu.Unlock()
		status += fmt.Sprintf("  - Instance %d with PID %d is in state %s\n", i, pid, state)
	}

	return status

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
		s.Events <- Event{Kind: EventProcessDied, Name: process.Name, Index: process.idx, Err: err}
		return

	// Timer reaches zero: process is considered ready
	case <-startTimer.C:
		s.Events <- Event{Kind: EventProcessReady, Name: process.Name, Index: process.idx}
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

func (s *Supervisor) startProcess(name string) error {

	cfg := s.Config.Programs[name]
	processes, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("process '%s' not found in taskmasterd\n", name)
	}

	for _, process := range processes {

		process.mu.Lock()
		isActive := process.state == RUNNING || process.state == STARTING
		process.mu.Unlock()

		if exists && isActive {
			return fmt.Errorf("process '%s' is already RUNNING or STARTING with PID %d", name, process.pid)
		}

		args := strings.Fields(cfg.Command)
		cmd := exec.Command(args[0], args[1:]...)

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
	}

	return nil
}

func (s *Supervisor) handleReady(name string, index int) error {

	processes, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("process '%s' not found in supervisor", name)
	}

	if processes[index].state != STARTING {
		return fmt.Errorf("process '%s' is not in STARTING state, cannot transition to ready", name)
	}

	processes[index].state = RUNNING
	s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d is now RUNNING", name, processes[index].pid))

	return nil
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
func (s *Supervisor) handleDied(event Event, index int) {

	processes, exists := s.Processes[event.Name]
	if !exists {
		s.Logger.Log(fmt.Sprintf("Received process died event for unknown process '%s'", event.Name))
		return
	}

	process := processes[index]

	process.mu.Lock()
	defer process.mu.Unlock()

	restartPolicy := process.Config.AutoRestart
	expectedExit := didProcessExitExpectedly(process.cmd.ProcessState, *process.Config)

	switch restartPolicy {

	case "never":

		fmt.Printf("[DEBUG] Process '%s' has terminated.\n", event.Name)
		s.Logger.Log(fmt.Sprintf("Process '%s' has terminated.", event.Name))
		process.state = EXITED
		process.retries = 0
		process.pid = 0

	case "always":

		fmt.Printf("[DEBUG] Process '%s' has terminated. Restart policy is 'always', attempting restart\n", event.Name)
		s.Logger.Log(fmt.Sprintf("Process '%s' has terminated. Restart policy is 'always', attempting restart", event.Name))
		event := Event{Kind: EventStartProcess, Name: event.Name}
		s.Events <- event

	case "unexpected":

		if expectedExit {

			// Expected exit: mark process as EXITED
			fmt.Printf("[DEBUG] Process '%s' has terminated with expected exit code %d.\n", event.Name, process.cmd.ProcessState.ExitCode())
			s.Logger.Log(fmt.Sprintf("Process '%s' has terminated with expected exit code %d.", event.Name, process.cmd.ProcessState.ExitCode()))
			process.state = EXITED
			process.retries = 0
			process.pid = 0

		} else if process.retries < process.Config.StartRetries {

			// Unexpected exit: mark process as BACKOFF if terminated during startup
			// else mark as EXITED and attempt restart if retries are still available
			if process.state == STARTING {
				process.state = BACKOFF
			} else {
				process.state = EXITED
			}

			fmt.Printf("[DEBUG] Process '%s' has terminated. Attempting restart (%d/%d)\n", event.Name, process.retries+1, process.Config.StartRetries)
			s.Logger.Log(fmt.Sprintf("Process '%s' has terminated. Attempting restart (%d/%d)", event.Name, process.retries+1, process.Config.StartRetries))

			process.retries++

			go func() {
				// Official supervisor documentation states that the restart strategy is to wait
				// n+1 seconds before each restart attempt, where n is the number of retries already attempted.
				time.Sleep(time.Duration(process.retries) * time.Second)

				event := Event{Kind: EventStartProcess, Name: event.Name}
				s.Events <- event
			}()

		} else {

			// Unexpected exit and no more retries available: mark process as FATAL
			fmt.Printf("[DEBUG] Process '%s' has terminated. Restart attempts exhausted (%d/%d), marking as FATAL\n", event.Name, process.retries, process.Config.StartRetries)
			s.Logger.Log(fmt.Sprintf("Process '%s' has terminated. Restart attempts exhausted (%d/%d), marking as FATAL", event.Name, process.retries, process.Config.StartRetries))
			process.state = FATAL
			process.pid = 0
			process.retries = 0

		}
	}
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

// The stopProcess function is responsible for stopping a process when a stop event is received.
// It attempts to gracefully stop the process by sending the signal specified in the config,
// changes its state to STOPPING and waiting for it to exit.
// If the process does not exit within the StopTime specified in the config, it SIG-kills it.
func (s *Supervisor) stopProcess(name string) error {

	cfg := s.Config.Programs[name]
	processes, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("process '%s' not found in supervisor", name)
	}

	for _, process := range processes {

		process.mu.Lock()
		isActive := process.state == RUNNING || process.state == STARTING || process.state == BACKOFF
		process.mu.Unlock()

		if !isActive {
			return fmt.Errorf("process '%s' is not RUNNING, STARTING or BACKOFF - cannot stop", name)
		}

		process.mu.Lock()
		process.state = STOPPING
		process.mu.Unlock()

		signal := getSignalByName(cfg.StopSignal)
		fmt.Printf("Sending signal %s to process '%s' with PID %d\n", cfg.StopSignal, name, process.pid)
		process.cmd.Process.Signal(signal)

		// Wait for the process to exit gracefully,
		// but if it doesn't exit within the StopTime, force kill it
		select {
		case process.cmd.Err = <-process.done:

		case <-time.After(time.Duration(cfg.StopTime) * time.Second):
			process.cmd.Process.Kill()
			process.cmd.Err = <-process.done
		}

		// Reset process state and metadata after it has stopped
		process.mu.Lock()
		process.state = STOPPED
		process.retries = 0
		process.done = nil
		process.pid = 0
		process.mu.Unlock()

		fmt.Printf("[DEBUG] Process '%s' has been STOPPED\n", name)
		s.Logger.Log(fmt.Sprintf("Process '%s' has been STOPPED", name))

	}

	return nil

}

func (s *Supervisor) Start() {

	s.autoStartProcesses()

	s.Ready <- true

	for event := range s.Events {
		switch event.Kind {

		case EventProcessDied:

			fmt.Printf("[DEBUG] Received process died event for program '%s'\n", event.Name)
			s.handleDied(event, event.Index)

		case EventProcessReady:

			fmt.Printf("[DEBUG] Received process ready event for program '%s'\n", event.Name)
			err := s.handleReady(event.Name, event.Index)
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
