package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/mrlouf/taskmaster/internal/config"
)

type ProcessStatus struct {
	Name  string
	State string
	PID   int
}

type process struct {
	cfg           config.Program
	cmd           *exec.Cmd
	state         string
	pid           int
	stopRequested bool
}

type Manager struct {
	mu        sync.Mutex
	processes map[string]*process
}

func NewManager(programs []config.Program) *Manager {
	p := make(map[string]*process, len(programs))
	for _, prog := range programs {
		copyProg := prog
		p[prog.Name] = &process{cfg: copyProg, state: "stopped"}
	}
	return &Manager{processes: p}
}

func (m *Manager) StartAutostart() error {
	m.mu.Lock()
	names := make([]string, 0, len(m.processes))
	for name, proc := range m.processes {
		if proc.cfg.Autostart {
			names = append(names, name)
		}
	}
	m.mu.Unlock()

	for _, name := range names {
		if err := m.Start(name); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Start(name string) error {
	m.mu.Lock()
	proc, ok := m.processes[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown program %q", name)
	}
	if proc.state == "running" {
		m.mu.Unlock()
		return nil
	}

	cmd := exec.Command(proc.cfg.Command, proc.cfg.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if proc.cfg.Directory != "" {
		cmd.Dir = proc.cfg.Directory
	}
	if err := cmd.Start(); err != nil {
		m.mu.Unlock()
		return err
	}

	proc.cmd = cmd
	proc.state = "running"
	proc.pid = cmd.Process.Pid
	proc.stopRequested = false
	m.mu.Unlock()

	go m.waitProcess(name, cmd)
	return nil
}

func (m *Manager) waitProcess(name string, cmd *exec.Cmd) {
	err := cmd.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()

	proc := m.processes[name]
	if proc == nil || proc.cmd != cmd {
		return
	}

	proc.cmd = nil
	proc.pid = 0
	if proc.stopRequested {
		proc.state = "stopped"
		proc.stopRequested = false
		return
	}
	if err != nil {
		proc.state = "exited"
		return
	}
	proc.state = "stopped"
}

func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	proc, ok := m.processes[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown program %q", name)
	}
	if proc.cmd == nil || proc.state != "running" {
		m.mu.Unlock()
		return nil
	}

	currentCmd := proc.cmd
	proc.stopRequested = true
	proc.state = "stopping"
	m.mu.Unlock()

	if currentCmd.Process == nil {
		return errors.New("process not started")
	}

	_ = currentCmd.Process.Signal(syscall.SIGTERM)
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		m.mu.Lock()
		finished := proc.cmd == nil
		m.mu.Unlock()
		if finished {
			return nil
		}
		select {
		case <-timeout.C:
			_ = currentCmd.Process.Kill()
			return nil
		case <-ticker.C:
		}
	}
}

func (m *Manager) Restart(name string) error {
	if err := m.Stop(name); err != nil {
		return err
	}
	return m.Start(name)
}

func (m *Manager) StopAll() {
	for _, status := range m.Status() {
		_ = m.Stop(status.Name)
	}
}

func (m *Manager) Status() []ProcessStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	statuses := make([]ProcessStatus, 0, len(m.processes))
	for name, proc := range m.processes {
		statuses = append(statuses, ProcessStatus{Name: name, State: proc.state, PID: proc.pid})
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})
	return statuses
}
