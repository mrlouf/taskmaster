package supervisor

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/protocol"
)

func (s *Supervisor) handleReload(path string) error {
	if ToDel, err := config.ReloadConfig(s.Config, path); err != nil {

		s.Logger.Log(fmt.Sprintf("Failed to reload new config file: %v", err))
		return err

	} else if ToDel != nil {
		for name := range ToDel.Programs {
			s.stopProgram(name)
		}
		ToDel = nil
	}
	s.createProcesses()
	s.autoStartProcesses()
	//using dedicated factored functions above
	// for name, program := range s.Config.Programs {

	// 	// ! And add only processes to new programs,
	// 	// ! otherwise we would add additional processes to existing programs
	// 	if _, exists := s.Processes[name]; exists == false {
	// 		for i := 0; i < program.NumProcs; i++ {
	// 			process := &Process{
	// 				Name:    name,
	// 				Config:  &program,
	// 				state:   STOPPED,
	// 				retries: 0,
	// 				idx:     i,
	// 			}
	// 			s.Processes[name] = append(s.Processes[name], process)

	// 		}
	// 	}

	// 	if program.AutoStart {
	// 		if err := s.startProgram(name); err != nil {
	// 			s.Logger.Log(fmt.Sprintf("Failed to auto-start program during reload '%s': %v", name, err))
	// 		}
	// 	}
	// }
	// for name, program := range s.Config.Programs {
	// 	if program.AutoStart {
	// 		if err := s.startProgram(name); err != nil {
	// 			s.Logger.Log(fmt.Sprintf("Failed to auto-start program during reload '%s': %v", name, err))
	// 		}
	// 	}
	// }
	return nil
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

// Wrapper function to start a program by name, which will in turn
// call the startProcess function for each subprocess of the program.
func (s *Supervisor) startProgram(name string) (error, error) {

	cfg := s.Config.Programs[name]
	processes, exists := s.Processes[name]

	fmt.Println(s.Processes[name]) // is null after reload - should not be

	if !exists {
		return fmt.Errorf("program '%s' not found in taskmasterd\n", name), nil
	}

	var globalErr error

	for _, process := range processes {

		err, warn := s.startProcess(process, cfg)
		if err != nil {
			globalErr = errors.Join(globalErr, fmt.Errorf("failed to start process '%s':\n%w", name, err))
			s.Logger.Log(fmt.Sprintf("Failed to start process '%s': %v", name, err))
		} else if warn != nil {
			globalErr = errors.Join(globalErr, fmt.Errorf("Process '%s' started with warnings:\n%w", name, warn))
			s.Logger.Log(fmt.Sprintf("Process '%s' started with warnings:\n %v", name, warn))
		}
	}
	return nil, globalErr

}

func (s *Supervisor) handleReady(name string, index int) error {

	processes, exists := s.Processes[name]
	if !exists {
		return fmt.Errorf("process '%s' not found in supervisor", name)
	}

	processes[index].mu.Lock()
	defer processes[index].mu.Unlock()

	if processes[index].state != STARTING {
		return fmt.Errorf("process '%s' is not in STARTING state, cannot transition to ready", name)
	}

	processes[index].state = RUNNING
	s.Logger.Log(fmt.Sprintf("Process '%s' with PID %d is now RUNNING", name, processes[index].pid))

	return nil
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
	if index >= len(s.Processes[event.Name]) {
		//s.Logger.Log(fmt.Sprintf("Received process died event for unknown process '%s'", event.Name))
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
			retryNum := process.retries

			go func() {
				// Official supervisor documentation states that the restart strategy is to wait
				// n+1 seconds before each restart attempt, where n is the number of retries already attempted.
				time.Sleep(time.Duration(retryNum) * time.Second)

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

func (s *Supervisor) Start() {

	fmt.Printf("Starting processes from config file located in '%s'\n", s.Config.ConfigPath)
	s.createProcesses()
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

			err, warn := s.startProgram(event.Name)
			if event.RespCh != nil {
				resp := protocol.Response{Ok: err == nil}
				if err != nil {
					resp.Msg = err.Error()
				} else if warn != nil {
					resp.Msg = fmt.Sprintf("Program '%s' started with warnings: %v", event.Name, warn)
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

			err, warn := s.startProcess(s.Processes[event.Name][event.Index], s.Config.Programs[event.Name])
			if event.RespCh != nil {
				resp := protocol.Response{Ok: err == nil}
				if err != nil {
					resp.Msg = err.Error()
				} else if warn != nil {
					resp.Msg = fmt.Sprintf("Process '%s' started with warnings: %v", event.Name, warn)
				} else {
					resp.Msg = fmt.Sprintf("Process '%s' started", event.Name)
				}
				event.RespCh <- resp
			}

		case EventStopProcess:

			err := s.stopProcess(s.Processes[event.Name][event.Index], s.Config.Programs[event.Name])
			if event.RespCh != nil {
				event.RespCh <- protocol.Response{Ok: err == nil}
			}

		case EventReloadConfig:
			err := s.handleReload(event.Name)
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
