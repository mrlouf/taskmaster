#!/bin/bash

TEST_NB=1

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

send_command() {
    local command=$1
    local program=$2
    local sleep_time=$3

    cat << EOF >test
$command $program
exit
EOF

    cat test | ../taskmasterctl 1>/dev/null

    if [ -n "$sleep_time" ]; then
        sleep "$sleep_time"
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
sleep 1

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

echo "shutdown" | ../taskmasterctl 1>/dev/null
wait
sleep 1

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

echo "shutdown" | ../taskmasterctl 1>/dev/null
wait
sleep 1

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 4: Rapid fire — stress test concurrence          #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: Rapid fire — stress test concurrence${NC}"
TEST_NB=$((TEST_NB + 1))

../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
PID_DAEMON=$!
sleep 1

cat << EOF >test
start nginx
status nginx
stop nginx
status nginx
reload
status sleep
start sleep
status sleep
status
exit
EOF

# 10 controllers en parallèle qui spamment des commandes
for i in $(seq 1 10); do
    cat test | ../taskmasterctl 1>/dev/null &
done

echo "shutdown" | ../taskmasterctl 1>/dev/null
wait
sleep 1

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 5: SIGHUP                                        #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: SIGHUP${NC}"
TEST_NB=$((TEST_NB + 1))

../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
DAEMON_PID=$!
sleep 1

cat test | ../taskmasterctl 1>/dev/null &

# SIGHUP en boucle
for i in $(seq 1 5); do
    kill -HUP $DAEMON_PID
    sleep 0.2
done

echo "shutdown" | ../taskmasterctl 1>/dev/null
wait
sleep 1

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 6: external kill during autorestart              #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: external kill during autorestart${NC}"
TEST_NB=$((TEST_NB + 1))

../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
sleep 1

send_command "start" "sleep" 1

for i in $(seq 1 5); do
    pkill -f "sleep 300"
    sleep 0.3
done

sleep 1
send_command "status" "sleep" 1

echo "shutdown" | ../taskmasterctl 1>/dev/null
wait
sleep 1

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Test 7: start/stop loop (may take a while)            #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

echo -e "${GREEN}Running test $TEST_NB: start/stop loop${NC}"
TEST_NB=$((TEST_NB + 1))

../taskmasterd -c ../conf/taskmaster.conf 1>/dev/null &
DAEMON_PID=$!
sleep 1

for i in $(seq 1 20); do
    send_command "start" "sleep" 0
    send_command "stop" "sleep" 0
done

echo "shutdown" | ../taskmasterctl 1>/dev/null
wait

rm test test2