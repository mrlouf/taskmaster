package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

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

func (s *Supervisor) handleShutdown() { //events.go

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

func (s *Supervisor) startProcess(name string) error { //events.go

	cfg := s.Config.Programs[name]
	process, exists := s.Processes[name]
	if !exists {
		fmt.Printf("[DEBUG] Process '%s' not found in supervisor, starting\n", name)
		process = &Process{
			Name:    name,
			Config:  &cfg,
			state:   STOPPED,
			retries: 0,
		}
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

func (s *Supervisor) handleReady(name string) error { //events.go

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

func (s *Supervisor) handleDied(event Event) { //events.go

	process, exists := s.Processes[event.Name]
	if !exists {
		s.Logger.Log(fmt.Sprintf("Received process died event for unknown process '%s'", event.Name))
		return
	}

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

// The stopProcess function is responsible for stopping a process when a stop event is received.
// It attempts to gracefully stop the process by sending the signal specified in the config,
// changes its state to STOPPING and waiting for it to exit.
// If the process does not exit within the StopTime specified in the config, it SIG-kills it.
func (s *Supervisor) stopProcess(name string) error { //events.go

	cfg := s.Config.Programs[name]
	process, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("process '%s' not found in supervisor", name)
	}

	if process.state != RUNNING && process.state != STARTING && process.state != BACKOFF {
		return fmt.Errorf("process '%s' is not STARTING, RUNNING or BACKOFF - cannot stop", name)
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

	return nil

}
