package supervisor

import (
	"os"
	"os/exec"
	"sync"
	"time"

	"taskmaster/internal/config"
	"taskmaster/internal/logger"
	"taskmaster/internal/protocol"
)

type EventKind int

const (
	EventProcessDied EventKind = iota
	EventProcessReady
	EventStartProgram
	EventStopProgram
	EventStartProcess
	EventStopProcess
	EventRestartProgram
	EventReloadConfig
	EventShutdown
	EventStatusRequest
)

type Event struct {
	Kind   EventKind
	Name   string                 // nom du programme ou  reload config file
	Index  int                    // numéro d'instance si numprocs > 1
	RunID  int                    // compteur de run (incrémente à chaque restart)
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
	runID     int
	outFile   *os.File
	errFile   *os.File
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
