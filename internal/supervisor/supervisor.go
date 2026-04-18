package supervisor

import (
	"fmt"

	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/logger"
)

type Supervisor struct {
	Config *config.Config
	Logger *logger.Logger
}

func New(config *config.Config, logger *logger.Logger) *Supervisor {

	return &Supervisor{
		Config: config,
		Logger: logger,
	}

}

func (s *Supervisor) Start() {

	// Reminder: config is a map[string]Program

	for name, program := range s.Config.Programs {

		if program.AutoStart {
			s.Logger.Log(fmt.Sprintf("Auto-starting program '%s' with command '%s'", name, program.Command))
		}
	}

}
