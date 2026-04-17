# ════════════════════════════════════════════════════════════
# ════════════════════════════════════════════════════════════

DAEMON := taskmasterd
DAEMON_DIR := ./daemon

CONTROLLER := taskmasterctl
CONTROLLER_DIR := ./controller

all: $(DAEMON) $(CONTROLLER)
	go build -o $(DAEMON) $(DAEMON_DIR)
	go build -o $(CONTROLLER) $(CONTROLLER_DIR)

clean:
	rm -f $(DAEMON) $(CONTROLLER)

re: clean all

.PHONY: all clean re