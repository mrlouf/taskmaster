package supervisor

import (
	"fmt"
	"strings"
)

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
		if i == process.Config.NumProcs-1 {
			str.WriteString("  └── ")
		} else {
			str.WriteString("  ├── ")
		}
		process.mu.Unlock()

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
