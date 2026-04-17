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

re: clean all

.PHONY: all clean re