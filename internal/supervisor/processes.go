package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/mrlouf/taskmaster/internal/config"
)

func (s *Supervisor) createProcesses() {
	var deleted int
	var added int
	for name, program := range s.Config.Programs {
		NumProcs := program.NumProcs
		if len(s.Processes[name]) < NumProcs || len(s.Processes[name]) == 0 {
			x := len(s.Processes[name])
			for i := x; i < NumProcs; i++ {
				process := &Process{
					Name:    name,
					Config:  &program,
					state:   STOPPED,
					retries: 0,
					//idx:     s.getMaxIdx(name) + 1,
				}
				s.Processes[name] = append(s.Processes[name], process)
				added++
			}
			s.updateIdx(name)
		} else if len(s.Processes[name]) > NumProcs {
			deleted += s.sizedownProcesses(name, len(s.Processes[name])-NumProcs)
			s.updateIdx(name)
		}
		// for i := 0; i < program.NumProcs; i++ {
		// 	process := &Process{
		// 		Name:    name,
		// 		Config:  &program,
		// 		state:   STOPPED,
		// 		retries: 0,
		// 		idx:     i,
		// 	}
		// 	s.Processes[name] = append(s.Processes[name], process)
		// }
	}
	fmt.Printf("%d processes were added and %d were deleted\n", added, deleted)
}

func (s *Supervisor) getStdfile(filename string) (*os.File, error) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (s *Supervisor) startProcess(process *Process, cfg config.Program) (error, string) {

	process.mu.Lock()
	isActive := process.state == RUNNING || process.state == STARTING
	name := process.Name
	pid := process.pid
	process.mu.Unlock()

	if isActive {
		return fmt.Errorf("process '%s' is already RUNNING or STARTING with PID %d", name, pid), ""
	}

	args := strings.Fields(cfg.Command)
	cmd := exec.Command(args[0], args[1:]...)

	var warn strings.Builder

	env := os.Environ()
	env = append(env, convertEnvMapToSlice(cfg.Env)...)
	cmd.Env = env
	cmd.Dir = cfg.WorkingDir

	if outfile, warn_out := s.getStdfile(cfg.Stdout); warn_out != nil {
		s.Logger.Log(fmt.Sprintf("Error while opening StdOut path '%s' for Program '%s': '%v'\n Defaulting to standard output", cfg.Stdout, process.Name, warn_out))
		warn.WriteString("Defaulting to standard output")
		cmd.Stdout = os.Stdout
	} else {
		cmd.Stdout = outfile
	}

	if errfile, warn_err := s.getStdfile(cfg.Stderr); warn_err != nil {
		s.Logger.Log(fmt.Sprintf("Error while opening StdErr path '%s' for Program '%s': '%v'\n Defaulting to standard error output", cfg.Stderr, process.Name, warn_err))
		warn.WriteString("Defaulting to standard error")
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stderr = errfile
	}

	// Umask is set process-wide and could theoretically race with file creation
	// in other goroutines (e.g. logger). In practice this is safe because all
	// critical operations go through the event loop sequentially, and the logger
	// file is opened once at startup.
	old := syscall.Umask(cfg.Umask)
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start process '%s': %w", process.Name, err), ""
	}
	syscall.Umask(old)

	process.mu.Lock()
	process.cmd = cmd
	process.pid = cmd.Process.Pid
	process.startedAt = time.Now()
	process.state = STARTING
	process.done = make(chan error, 1)
	process.runID++
	runID := process.runID
	process.mu.Unlock()

	go s.monitorProcess(process, cfg, runID)

	if process.retries > 0 {
		s.Logger.Log(fmt.Sprintf("Restarted process '%s' with PID %d (retry %d)", process.Name, process.pid, process.retries))
	} else {
		s.Logger.Log(fmt.Sprintf("Started process '%s' with PID %d", process.Name, process.pid))
	}
	return nil, warn.String()
}

func (s *Supervisor) autoStartProcesses() {
	for name, program := range s.Config.Programs {
		if program.AutoStart {
			s.Logger.Log(fmt.Sprintf("Auto-starting program '%s' with command: %s", name, program.Command))
			err, warn := s.startProgram(name)
			if err != nil {
				s.Logger.Log(fmt.Sprintf("Failed to auto-start program '%s': %v", name, err))
			} else if warn != "" {
				s.Logger.Log(fmt.Sprintf("Program '%s' auto-stared with following warnings: '%v'", name, warn))
			}
		}
	}
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

// The monitorProcess function is responsible for monitoring the lifecycle
// of a process after it has been started. It only monitors and reports the death
// or the readiness of the process, but the State transition is the responsibility
// of the event handlers (handleReady and handleDied).
func (s *Supervisor) monitorProcess(process *Process, cfg config.Program, runID int) {
	startTimer := time.NewTimer(time.Duration(cfg.StartTime) * time.Second)

	process.mu.Lock()
	cmd := process.cmd
	done := process.done
	process.mu.Unlock()

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	select {
	// Process is done before start timer expires: could be a crash or a very fast process
	// In both cases we consider the process as having died, and we won't transition to ready state
	// The error from Wait() will be sent to the process.done channel
	// and the handleDied handler will decide what to do based on process state and retry policy
	case err := <-waitDone:
		startTimer.Stop()
		done <- err
		s.Events <- Event{Kind: EventProcessDied, Name: process.Name, Index: process.idx, RunID: runID, Err: err}
		return
	// Timer reaches zero: process is considered ready
	case <-startTimer.C:
		s.Events <- Event{Kind: EventProcessReady, Name: process.Name, Index: process.idx, RunID: runID}
	}

	err := <-waitDone
	done <- err
	s.Events <- Event{Kind: EventProcessDied, Name: process.Name, Index: process.idx, RunID: runID, Err: err}

}
