package supervisor

import (
	"errors"
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
	EventStartProgram
	EventStopProgram
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

func (s *Supervisor) handleReload() error {

	if ToDel, err := config.ReloadConfig(s.Config); err != nil {

		s.Logger.Log(fmt.Sprintf("Failed to reload new config file: %v", err))
		return err

	} else if ToDel != nil {
		for name := range ToDel.Programs {
			s.stopProgram(name)
		}
		ToDel = nil
	}

	// ! Adding the programs in the new config is not enough,
	// ! we also need to create the corresponding processes!
	for name, program := range s.Config.Programs {

		// ! And add only processes to new programs,
		// ! otherwise we would add additional processes to existing programs
		if _, exists := s.Processes[name]; exists == false {
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
		}

		if program.AutoStart {
			if err := s.startProgram(name); err != nil {
				s.Logger.Log(fmt.Sprintf("Failed to auto-start program during reload '%s': %v", name, err))
			}
		}
	}
	return nil
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
			err := s.startProgram(name)
			if err != nil {
				s.Logger.Log(fmt.Sprintf("Failed to auto-start program '%s': %v", name, err))
			}
		}
	}
}

func (s *Supervisor) handleShutdown() { //events.go

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

				if process.state == RUNNING ||
					process.state == STARTING ||
					process.state == BACKOFF {

					cfg := s.Config.Programs[name]

					signal := syscall.SIGTERM
					s.Logger.Log(fmt.Sprintf("Sending signal %d to process '%s' with PID %d", signal, name, process.pid))
					process.cmd.Process.Signal(signal)

					select {
					case process.cmd.Err = <-process.done:
						s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has exited gracefully", name, process.pid))

					case <-time.After(time.Duration(cfg.StopTime) * time.Second):
						process.cmd.Process.Kill()
						process.cmd.Err = <-process.done

						s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d did not exit gracefully, sent KILL signal", name, process.pid))
					}
					process.state = STOPPED
					process.retries = 0
					process.done = nil
					process.pid = 0
				}
			})
		}
	}

	wg.Wait()

	s.Logger.Log("Supervisor shutdown complete")

}

func (s *Supervisor) GetStatus(name string) string {

	processes, exists := s.Processes[name]
	if !exists {
		return fmt.Sprintf("'%s' not found", name)
	}

	str := strings.Builder{}
	str.WriteString(fmt.Sprintf("'%s':\n", name))

	for i, process := range processes {

		process.mu.Lock()
		state := process.state.String()
		pid := process.pid
		process.mu.Unlock()
		if i == process.Config.NumProcs-1 {
			str.WriteString("  └── ")
		} else {
			str.WriteString("  ├── ")
		}

		str.WriteString(fmt.Sprintf("process %d %s", i, state))
		if pid != 0 {
			str.WriteString(fmt.Sprintf(": PID %d\n", pid))
		} else {
			str.WriteString("\n")
		}
	}

	status := str.String()

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

	s.Events <- Event{Kind: EventProcessDied, Name: process.Name, Index: process.idx, Err: err}

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

// Wrapper function to start a program by name, which will in turn
// call the startProcess function for each subprocess of the program.
func (s *Supervisor) startProgram(name string) error {

	cfg := s.Config.Programs[name]
	processes, exists := s.Processes[name]

	fmt.Println(s.Processes[name]) // is null after reload - should not be

	if !exists {
		return fmt.Errorf("program '%s' not found in taskmasterd\n", name)
	}

	var globalErr error

	for _, process := range processes {

		err := s.startProcess(process, cfg)
		if err != nil {
			globalErr = errors.Join(globalErr, fmt.Errorf("failed to start process '%s': %w", name, err))
			s.Logger.Log(fmt.Sprintf("Failed to start process '%s': %v", name, err))
		}
	}

	return globalErr
}

func (s *Supervisor) startProcess(process *Process, cfg config.Program) error {

	process.mu.Lock()
	isActive := process.state == RUNNING || process.state == STARTING
	process.mu.Unlock()

	if isActive {
		return fmt.Errorf("process '%s' is already RUNNING or STARTING with PID %d", process.Name, process.pid)
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
		return fmt.Errorf("failed to start process '%s': %w", process.Name, err)
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
		s.Logger.Log(fmt.Sprintf("Restarted process '%s' with PID %d (retry %d)", process.Name, process.pid, process.retries))
	} else {
		s.Logger.Log(fmt.Sprintf("Started process '%s' with PID %d", process.Name, process.pid))
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

	// Ignore if the process is manually stopping or has already been stopped
	if process.state == STOPPING || process.state == STOPPED {
		return
	}

	restartPolicy := process.Config.AutoRestart
	expectedExit := didProcessExitExpectedly(process.cmd.ProcessState, *process.Config)

	switch restartPolicy {

	case "never":

		s.Logger.Log(fmt.Sprintf("Process '%s' has terminated.", event.Name))
		process.state = EXITED
		process.retries = 0
		process.pid = 0

	case "always":

		s.Logger.Log(fmt.Sprintf("Process '%s' has terminated. Restart policy is 'always', attempting restart", event.Name))

		process.state = EXITED
		process.retries = 0
		process.pid = 0

		event := Event{Kind: EventStartProcess, Name: event.Name, Index: event.Index}
		s.Events <- event

	case "unexpected":

		if expectedExit {

			// Expected exit: mark process as EXITED
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

			s.Logger.Log(fmt.Sprintf("Process '%s' has terminated. Attempting restart (%d/%d)", event.Name, process.retries+1, process.Config.StartRetries))

			process.retries++

			go func() {
				// Official supervisor documentation states that the restart strategy is to wait
				// n+1 seconds before each restart attempt, where n is the number of retries already attempted.
				time.Sleep(time.Duration(process.retries) * time.Second)

				event := Event{Kind: EventStartProcess, Name: event.Name, Index: event.Index}
				s.Events <- event
			}()

		} else {

			// Unexpected exit and no more retries available: mark process as FATAL
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

// Wrapper function to stop a program by name, which will in turn
// call the stopProcess function for each instance of the program.
func (s *Supervisor) stopProgram(name string) error {

	processes, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("program '%s' not found in supervisor", name)
	}
	cfg := s.Config.Programs[name]

	for _, process := range processes {
		err := s.stopProcess(process, cfg)
		if err != nil {
			return err
		}
	}

	return nil
}

// The stopProcess function is responsible for stopping a specific subprocess.
// It attempts to gracefully stop the process by sending the signal specified in the config,
// changes its state to STOPPING and waiting for it to exit.
// If the process does not exit within the StopTime specified in the config, it SIG-kills it.
func (s *Supervisor) stopProcess(process *Process, cfg config.Program) error {

	process.mu.Lock()
	isActive := process.state == RUNNING || process.state == STARTING || process.state == BACKOFF
	process.mu.Unlock()

	if !isActive {
		return fmt.Errorf("process '%s' is not RUNNING, STARTING or BACKOFF - cannot stop", process.Name)
	}

	process.mu.Lock()
	process.state = STOPPING
	process.mu.Unlock()

	signal := getSignalByName(cfg.StopSignal)
	s.Logger.Log(fmt.Sprintf("Sending signal %s to process '%s' with PID %d", cfg.StopSignal, process.Name, process.pid))
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

	s.Logger.Log(fmt.Sprintf("Process '%s' has been STOPPED", process.Name))

	return nil

}

func (s *Supervisor) Start() {

	fmt.Printf("Starting processes from config file located in '%s'\n", s.Config.ConfigPath)
	s.autoStartProcesses()
	s.Ready <- true

	for event := range s.Events {
		switch event.Kind {

		case EventProcessDied:

			s.handleDied(event, event.Index)

		case EventProcessReady:

			err := s.handleReady(event.Name, event.Index)
			if err != nil {
				s.Logger.Log(fmt.Sprintf("Failed to handle ready event for program '%s': %v", event.Name, err))
			}

		case EventStartProgram:

			err := s.startProgram(event.Name)
			if event.RespCh != nil {
				resp := protocol.Response{Ok: err == nil}
				if err != nil {
					resp.Msg = err.Error()
				} else {
					resp.Msg = fmt.Sprintf("Program '%s' started", event.Name)
				}
				event.RespCh <- resp
			}

		case EventStopProgram:

			err := s.stopProgram(event.Name)
			if event.RespCh != nil {
				event.RespCh <- protocol.Response{Ok: err == nil}
			}

		case EventStartProcess:

			err := s.startProcess(s.Processes[event.Name][event.Index], s.Config.Programs[event.Name])
			if event.RespCh != nil {
				event.RespCh <- protocol.Response{Ok: err == nil}
			}

		case EventStopProcess:

			err := s.stopProcess(s.Processes[event.Name][event.Index], s.Config.Programs[event.Name])
			if event.RespCh != nil {
				event.RespCh <- protocol.Response{Ok: err == nil}
			}

		case EventReloadConfig:

			err := s.handleReload()
			if event.RespCh != nil {
				if err != nil {
					event.RespCh <- protocol.Response{Ok: false, Msg: err.Error()}
				} else {
					event.RespCh <- protocol.Response{Ok: true, Msg: "Config reloaded successfully"}
				}
			}

		case EventShutdown:

			s.handleShutdown()
			if event.RespCh != nil {
				event.RespCh <- protocol.Response{Ok: true, Msg: "Shut down complete"}
			}
			// os.Exit(0)
		}
	}
}
