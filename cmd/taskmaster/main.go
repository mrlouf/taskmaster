package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mrlouf/taskmaster/internal/client"
	"github.com/mrlouf/taskmaster/internal/config"
	"github.com/mrlouf/taskmaster/internal/daemon"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: taskmaster <daemon|ctl> [flags]")
	}

	switch os.Args[1] {
	case "daemon":
		return runDaemon(os.Args[2:])
	case "ctl":
		return runClient(os.Args[2:])
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func runDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	configPath := fs.String("config", "taskmaster.toml", "path to TOML configuration file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	manager := daemon.NewManager(cfg.Programs)
	if err := manager.StartAutostart(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := daemon.NewServer(cfg.Server.Address, manager, cancel)
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Serve(ctx)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		_ = sig
		cancel()
	case err := <-serverErr:
		if err != nil {
			return err
		}
	}

	srv.Close()
	manager.StopAll()
	return nil
}

func runClient(args []string) error {
	fs := flag.NewFlagSet("ctl", flag.ContinueOnError)
	configPath := fs.String("config", "taskmaster.toml", "path to TOML configuration file")
	addr := fs.String("addr", "", "daemon address (overrides config)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	target := *addr
	if target == "" {
		cfg, err := config.Load(*configPath)
		if err != nil {
			return err
		}
		target = cfg.Server.Address
	}

	rest := fs.Args()
	if len(rest) > 0 {
		resp, err := client.SendCommand(target, strings.Join(rest, " "))
		if err != nil {
			return err
		}
		fmt.Println(resp)
		return nil
	}

	return client.RunShell(os.Stdin, os.Stdout, target)
}
