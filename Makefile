# ════════════════════════════════════════════════════════════
# ════════════════════════════════════════════════════════════

DAEMON := taskmasterd
DAEMON_DIR := ./cmd/taskmasterd

CONTROLLER := taskmasterctl
CONTROLLER_DIR := ./cmd/taskmasterctl

all: $(DAEMON) $(CONTROLLER)
	go build -o $(DAEMON) $(DAEMON_DIR)
	go build -o $(CONTROLLER) $(CONTROLLER_DIR)

$(DAEMON): $(DAEMON_DIR)/*.go
	go build -o $@ $(DAEMON_DIR)

$(CONTROLLER): $(CONTROLLER_DIR)/*.go
	go build -o $@ $(CONTROLLER_DIR)

clean:
	rm -f $(DAEMON) $(CONTROLLER)
	rm -f /tmp/taskmaster.sock
	rm -rf ./tmp
	rm -rf ./.logs
	rm -f taskmaster.log

log:
	rm -f taskmaster.log

re: clean all

# First kill any running process of the daemon and controller, then
# start daemon in background and controller in foreground with Air for live reloading.
# Logs are saved in the .logs directory.
dev: pkill
	@mkdir -pv ./.logs
	air -c .air.daemon.toml &> ./.logs/daemon.log &
	@sleep 1 # wait for the daemon to start
	air -c .air.controller.toml &> ./.logs/controller.log

pkill:
	-@pkill -f "air -c .air.daemon.toml" &>/dev/null || true
	-@pkill -f "air -c .air.controller.toml" &>/dev/null || true
	-@pkill -f $(DAEMON) &>/dev/null || true
	-@pkill -f $(CONTROLLER) &>/dev/null || true
	rm -f /tmp/taskmaster.sock

.PHONY: all clean re dev pkill