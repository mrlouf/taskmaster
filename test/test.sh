#!/bin/bash

# Generate a series of commands to test the controller.
cat << EOF >test
start sleep
stop sleep
status
reload
status
start nginx
reload
stop nginx
status
reload
status nginx
status sleep
status
reload
stop nginx
shutdown
EOF

# Start the daemon in the background.
../taskmasterd -c ../conf/taskmaster.conf &
sleep 1


# Run the test commands through one controller.
cat test | ../taskmasterctl

# Restart daemon after shutdown.
../taskmasterd -c ../conf/taskmaster.conf &
sleep 1

# Run the same test through two controllers at the same time.
cat test | ../taskmasterctl &
cat test | ../taskmasterctl

# Stop the daemon.
killall taskmasterd

# Clean up
rm test 