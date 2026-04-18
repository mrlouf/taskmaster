package supervisor

import (
	"fmt"
	"os/exec"
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
	Stopped State = iota
	Starting
	Running
	Stopping
	Exited
	Fatal
)

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

func (s *Supervisor) monitorProcess(process *Process, cfg config.Program) {

	err := process.cmd.Wait()
	process.done <- err

	go func() {
		time.Sleep(time.Second * time.Duration(cfg.StartTime))
		s.Events <- Event{Kind: EventProcessReady, Name: process.Name}
	}()
	go func() {
		time.Sleep(time.Second * time.Duration(cfg.StopTime))
		s.Events <- Event{Kind: EventStopProcess, Name: process.Name}
	}()

	if err != nil {
		s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d exited with error: %v", process.Name, process.pid, err))
	} else {
		s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d exited successfully", process.Name, process.pid))
	}

}

// Helper function to convert env map to slice of "KEY=VALUE" strings
// Needed since exec.Cmd.Env expects a slice of strings in the format "KEY=VALUE"
func convertEnvMapToSlice(envMap map[string]string) []string {

	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
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

	cmd.Env = convertEnvMapToSlice(cfg.Env)
	cmd.Dir = cfg.WorkingDir
	cmd.Stdout = nil
	cmd.Stderr = nil

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

	process.state = Exited
	s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d has exited", event.Name, process.pid))

}

func (s *Supervisor) stopProcess(name string) error {

	process, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("process '%s' not found in supervisor", name)
	}

	if process.state != Running && process.state != Starting {
		return fmt.Errorf("process '%s' is not running or starting, cannot stop", name)
	}

	err := process.cmd.Process.Kill()
	if err != nil {
		return fmt.Errorf("failed to stop process '%s' with PID %d: %w", name, process.pid, err)
	}

	process.state = Stopping
	s.Events <- Event{Kind: EventProcessDied, Name: name}

	s.Logger.Log(fmt.Sprintf("Sent kill signal to process '%s' with PID %d", name, process.pid))

	return nil
}

func (s *Supervisor) Start() {

	s.autoStartProcesses()

	go func() {

		for {
			fmt.Println("Active processes in supervisor:")
			for name, process := range s.Processes {

				fmt.Printf("- %s: PID %d, State %d\n", name, process.pid, process.state)
			}
			time.Sleep(2 * time.Second)
		}
	}()

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
