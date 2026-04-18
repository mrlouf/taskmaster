package logger

import (
	"fmt"
	"os"
	"time"
)

type Logger struct {
	file *os.File
}

var logfile string = "taskmaster.log"

func New() (*Logger, error) {
	file, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	return &Logger{file: file}, nil
}

func (l *Logger) Log(message string) error {

	timestamp := fmt.Sprintf("%s", time.Now().Format(time.RFC3339))
	_, err := fmt.Fprintf(l.file, "%s %s\n", timestamp, message)
	return err
}

func (l *Logger) Close() error {
	return l.file.Close()
}
