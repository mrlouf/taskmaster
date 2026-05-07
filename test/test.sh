#!/bin/bash

TEST_NB=0

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 1: Single controller                             #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -ne "$GREEN"
echo -e "Running test $TEST_NB: Single controller"
echo -ne "$NC"
TEST_NB=$((TEST_NB + 1))

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
../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
sleep 1

# Run the test commands through one controller.
cat test | ../taskmasterctl 1>/dev/null &

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 2: Two controllers, same commands                #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -ne "$GREEN"
echo -e "Running test $TEST_NB: Double controllers, same commands"
echo -ne "$NC"
TEST_NB=$((TEST_NB + 1))

# Restart daemon after shutdown.
../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
sleep 1

# Run the same test through two controllers at the same time.
cat test | ../taskmasterctl 1>/dev/null &
cat test | ../taskmasterctl 1>/dev/null &

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 3: Two controllers, different commands           #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -ne "$GREEN"
echo -e "Running test $TEST_NB: Double controllers, different commands"
echo -ne "$NC"
TEST_NB=$((TEST_NB + 1))

# Restart daemon after shutdown.
../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
sleep 1

# Generate a different series of commands to test the second controller.
cat << EOF >test2
start nginx
status nginx
stop nginx
status nginx
reload
status sleep
start sleep
status sleep
status
shutdown
EOF

# Run the same test through two controllers at the same time.
cat test | ../taskmasterctl 1>/dev/null &
cat test2 | ../taskmasterctl 1>/dev/null &

# Stop the daemon.
killall taskmasterd

# Clean up
rm taskmaster.log
rm test
rm test1
rm test2