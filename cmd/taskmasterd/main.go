package main

import (
	"fmt"
	"log"

	"github.com/mrlouf/taskmaster/internal/config"
)

func main() {
	// Try and catch equivalent in Go
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {

	config, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Config loaded successfully: %+v\n", config)

	return nil
}
