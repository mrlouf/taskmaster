#!/bin/bash

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}\n************************"
echo -e "Starting data race tests"
echo -e "************************\n${NC}"

# bash -c "./data-race.sh"

echo -e "${GREEN}\n******************************"
echo -e "Starting state assertion tests"
echo -e "******************************\n${NC}"

bash -c "./state-assert.sh"

# Stop the daemon.
killall taskmasterd

# Clean up
rm taskmaster.log