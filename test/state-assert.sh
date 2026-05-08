#!/bin/bash

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

FAILED=0

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Helper functions                                      #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

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
# Start tests                                           #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#

# Start Daemon
../taskmasterd -c ../conf/taskmaster.conf 2>&1>/dev/null &
sleep 1

send_command "start" "sleep" 1

assert_status "sleep" "RUNNING"
assert_status "nginx" "STOPPED"
assert_status "k3d" "STOPPED"

send_command "stop" "sleep" 0
assert_status "sleep" "STOPPED"

send_command "start" "nginx" 10
assert_status "nginx" "FATAL"

# Add a new program configuration and reload.

cat << EOF >>../conf/taskmaster.conf

  sleep1:
    cmd: "sleep 300"
    numprocs: 2
    umask: 077
    workingdir: /tmp
    autostart: true
    autorestart: always
    exitcodes: 
      - 0
      - 1
    startretries: 0
    starttime: 1
    stopsignal: HUP
    stoptime: 10
    stdout: /tmp/sleep.stdout
    stderr: /tmp/sleep.stderr
EOF

send_command "reload" "" 2
send_command "status" "" 1

assert_status "sleep1" "RUNNING"

# Remove the new program configuration and reload.
# head -n -17 ../conf/taskmaster.conf > ../conf/taskmaster.conf.tmp && mv ../conf/taskmaster.conf.tmp ../conf/taskmaster.conf
tac ../conf/taskmaster.conf | sed '1,17d' | tac > ../conf/taskmaster.conf.tmp && mv ../conf/taskmaster.conf.tmp ../conf/taskmaster.conf


# Reload the configuration
send_command "reload" "" 2
send_command "status" "" 1

assert_status "sleep1" "not found"

#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#
# Print output                                          #
#~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~*~#


echo -e "${GREEN}\n********************************"
echo -e "State assertion tests completed:"
if [ $FAILED -eq 0 ]; then
    echo -e "All tests passed!${NC}"
else
    echo -e "${RED}$FAILED tests failed.${NC}"
fi
echo -e "${GREEN}********************************\n${NC}"

rm test