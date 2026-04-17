# taskmaster

A process manager for Linux based on [supervisor](https://supervisord.org/index.html#), written in Go

## Description

This project is a process manager for Linux using a daemon and a controller.

The daemon is responsible for starting, monitoring and stopping the processes defined in a configuration file. It also listens for commands from the controller, executes and report back the results.

The controller is a command-line tool that allows the user to send commands to the daemon to manage the processes.

The configuration file is a declarative YAML file that defines the processes to be started and their parameters (command, environment variables, working directory, etc.). The daemon reads this configuration file at startup and manages the processes accordingly.

## Features

The daemon and the controller are communicating over a Unix socket using a simple JSON-based protocol.

### Development tools

To help with the development, I am using the [Air tool for live reloading](https://github.com/air-verse/air). Air watches for changes in the source code and automatically recompiles and restart the binary.

To install Air, you can run:

```bash
go install github.com/air-verse/air@latest
```

Since Air is not natively supporting multi-binary projects, I have declared a target in the Makefile that makes Air run the daemon in the background first, then the controller in the foreground. This allows me to have both the daemon and the controller running with live reloading in the same terminal:

```bash
make dev
```

#### References

- [UNIX sockets in Golang](https://dev.to/douglasmakey/understanding-unix-domain-sockets-in-golang-32n8)