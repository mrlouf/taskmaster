# taskmaster

A simple process manager inspired by supervisor, written in Go.

## Overview

`taskmaster` provides a lightweight client/server architecture:

- `taskmaster daemon`: starts the daemon (similar to `supervisord`) and manages configured programs.
- `taskmaster ctl`: interactive shell (similar to `supervisorctl`) or one-shot command client.

Configuration is read from a TOML file (default: `taskmaster.toml`).

## Configuration example

```toml
[server]
address = "127.0.0.1:9001"

[[program]]
name = "sleeper"
command = "sleep"
args = ["60"]
autostart = true
```

## Usage

```bash
# Start daemon
taskmaster daemon -config taskmaster.toml

# Interactive shell
taskmaster ctl -config taskmaster.toml

# One-shot command
taskmaster ctl -config taskmaster.toml status
```

Supported control commands: `status`, `start <name>`, `stop <name>`, `restart <name>`, `shutdown`, `help`.
