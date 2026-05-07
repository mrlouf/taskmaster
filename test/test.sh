#!/bin/bash

TEST_NB=0

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

assert_status() {
    local program=$1
    local expected=$2
    local result=$(echo "status $program" | ../taskmasterctl 2>/dev/null)
    
    if echo "$result" | grep -q "$expected"; then
        echo -e "${GREEN}PASS${NC}: $program is $expected"
    else
        echo -e "${RED}FAIL${NC}: $program expected $expected, got: $result"
        FAILED=$((FAILED + 1))
    fi
}

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 1: Single controller                             #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: Single controller${NC}"
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

wait

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 2: Two controllers, same commands                #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: Double controllers, same commands${NC}"
TEST_NB=$((TEST_NB + 1))

# Restart daemon after shutdown.
../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
sleep 1

# Run the same test through two controllers at the same time.
cat test | ../taskmasterctl 1>/dev/null &
cat test | ../taskmasterctl 1>/dev/null &

wait

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 3: Two controllers, different commands           #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: Double controllers, different commands${NC}"
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

wait

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 4: Rapid fire — stress test concurrence          #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: Rapid fire — stress test concurrence${NC}"
TEST_NB=$((TEST_NB + 1))

../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
sleep 1

# 10 controllers en parallèle qui spamment des commandes
for i in $(seq 1 10); do
    cat test | ../taskmasterctl 1>/dev/null &
done
wait
killall taskmasterd
sleep 1

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 5: SIGHUP                                        #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: SIGHUP${NC}"
TEST_NB=$((TEST_NB + 1))

../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
DAEMON_PID=$!
sleep 1

# envoyer des commandes en continu
cat test | ../taskmasterctl 1>/dev/null &

# pendant ce temps, spammer des SIGHUP
for i in $(seq 1 5); do
    kill -HUP $DAEMON_PID
    sleep 0.2
done
wait
kill $DAEMON_PID
sleep 1

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 6: external kill during autorestart              #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: external kill during autorestart${NC}"
TEST_NB=$((TEST_NB + 1))

../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
DAEMON_PID=$!
sleep 1

# démarrer un process avec autorestart: always
echo "start sleep" | ../taskmasterctl 1>/dev/null
sleep 1

# le killer plusieurs fois rapidement
for i in $(seq 1 5); do
    pkill -f "sleep 999"
    sleep 0.3
done

sleep 2
echo "status sleep" | ../taskmasterctl
kill $DAEMON_PID
sleep 1

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 7: start/stop loop                               #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: start/stop loop${NC}"
TEST_NB=$((TEST_NB + 1))

../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
DAEMON_PID=$!
sleep 1

for i in $(seq 1 20); do
    echo "start sleep" | ../taskmasterctl 1>/dev/null
    echo "stop sleep"  | ../taskmasterctl 1>/dev/null
done

kill $DAEMON_PID
sleep 1

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Clean up                                              #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

# Stop the daemon.
killall taskmasterd

# Clean up
rm taskmaster.log
rm test
rm test2