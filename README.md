# taskmaster

A process manager for Linux based on [supervisor](https://supervisord.org/index.html#), written in Go

## Description

This project is a job controller for Linux using a daemon/controller architecture.

The daemon is responsible for starting, monitoring and stopping the processes defined in a configuration file. It also listens for commands from the controller, executes and report back the results.

The controller is a CLI tool that allows the user to send commands to the daemon to manage the processes.

The configuration file is a declarative YAML file that defines the processes to be started and their parameters (command, environment variables, working directory, etc.). The daemon reads this configuration file at startup and manages the processes accordingly.

## Features

The daemon and the controller are communicating over a Unix socket using a simple JSON-based protocol. The daemon accepts connections from multiple controllers and handles their requests concurrently with goroutines.

The supported commands are:

- `start <program>`: Start a program defined in the configuration file
- `stop <program>`: Stop a running program
- `restart <program>`: Restart a running program
- `status <program>`: Get the status of a program (running, stopped, etc.)
- `healthcheck`: Check if the daemon is running and responsive
- `reload`: Reload the configuration file and apply any changes to the managed processes
- `shutdown`: Stop the daemon gracefully, allowing it to clean up resources and terminate all managed processes before exiting
- `exit`: Stop the daemon gracefully
- `help`: Display a help message with the list of available commands

## Process States

The daemon manages the state of each process and updates it accordingly. The possible states are:

- `STARTING`: The process is starting. This state is entered when the daemon receives a command to start the process or the process has an autostart policy, and will remain in this state until the start time is reached.
- `RUNNING`: The process is running and healthy. This state is entered when the process has started successfully and has reached the start time specified in the configuration file.
- `STOPPING`: The process is stopping. This state is entered when the daemon receives a command to stop the process and is waiting the specified stop time for the process to terminate gracefully. If the process does not terminate within the stop time, the daemon will forcefully kill the process and transition it to the `STOPPED` state.
- `STOPPED`: The process is not running. This is the initial state before the process is started, and also the state after the process has been stopped.
- `EXITED`: The process has exited. This state is entered when the process has terminated, either successfully, with an error or has received a signal. Depending on the exit status and the restart policy, the daemon may attempt to restart the process.
- `BACKOFF`: The process has failed to start and is waiting before the next retry. This state is entered when the process fails to start, either due to an error or because it exited before reaching the start time. The daemon will wait for a certain backoff time before attempting to restart the process again. If the process continues to fail to start after the maximum number of retries, it will transition to the `FATAL` state.
- `FATAL`: The process has failed to start after the maximum number of retries has been reached. The process will not be restarted anymore and will remain in this state until a manual intervention or a configuration change.

An example of a successful execution of a manually started (no autostart) long-running process would be:

```bash
STOPPED → STARTING → RUNNING
```

An example of a process autostarting and then exiting after normal execution would be:

```bash
STOPPED → STARTING → RUNNING → EXITED
```

An example of a process autostarting and failing to start with two retries would be:

```bash
STOPPED → STARTING → BACKOFF → STARTING → BACKOFF → FATAL
```

## Logs

The daemon logs the output of the managed requests from the controller into a log file with a timestamp for clarity.

The make clean target removes the log file.

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

- [Supervisor Documentation about process states](https://supervisord.org/subprocess.html#process-states)
- [UNIX sockets in Golang](https://dev.to/douglasmakey/understanding-unix-domain-sockets-in-golang-32n8)
