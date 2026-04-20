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

	// channel qui reçoit la mort du process
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- process.cmd.Wait()
	}()

	// phase STARTING — on attend soit la mort soit le starttime
	select {
	case err := <-waitDone:
		// mort pendant STARTING → BACKOFF
		startTimer.Stop()
		process.done <- err
		s.Events <- Event{Kind: EventProcessDied, Name: process.Name, Err: err}
		return

	case <-startTimer.C:
		// a survécu assez longtemps → RUNNING
		s.Events <- Event{Kind: EventProcessReady, Name: process.Name}
	}

	// phase RUNNING — on attend juste la mort
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

func (s *Supervisor) handleDied(event Event) {

	process, exists := s.Processes[event.Name]
	if !exists {
		s.Logger.Log(fmt.Sprintf("Received process died event for unknown process '%s'", event.Name))
		return
	}

	fmt.Printf("Process '%s' has died\n", event.Name)

	process.state = Exited
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
	case <-process.done:

	case <-time.After(time.Duration(cfg.StopTime) * time.Second):
		// toujours vivant → SIGKILL
		process.cmd.Process.Kill()
		<-process.done
	}

	fmt.Printf("Does not reach here - get blocked on process.done channel\n")

	process.state = Stopped
	return nil

}

func (s *Supervisor) Start() {

	s.autoStartProcesses()

	/* 	go func() {

		for {
			fmt.Println("Active processes in supervisor:")
			for name, process := range s.Processes {

				fmt.Printf("- %s: PID %d, State %d\n", name, process.pid, process.state)
			}
			time.Sleep(2 * time.Second)
		}
	}() */

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
