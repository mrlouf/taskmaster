package logger

import (
	"fmt"
	"os"
)

type Logger struct {
	file *os.File
}

var logfile string = "taskmaster.log"

func NewLogger() (*Logger, error) {
	file, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	return &Logger{file: file}, nil
}

func (l *Logger) Log(message string) error {
	_, err := fmt.Fprintf(l.file, "%s\n", message)
	return err
}

func (l *Logger) Close() error {
	return l.file.Close()
}
