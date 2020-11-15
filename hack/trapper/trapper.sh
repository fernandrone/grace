#!/usr/bin/env bash

# 'Trapper.sh' is a blocking proccess that "listens" for linux termination signals. It has been
# programmed to handle SINGINT and SIGTERM, log when they are trapped and then successfuly
# terminate.
#
# This is used to demonstrate linux termination signals and how to gracefully shutdown a proccess.

trap "handle INT" INT	# 2
trap "handle KILL" KILL # 9 will not work, can't be handled
trap "handle TERM" TERM # 15

handle() {
    echo "Trapped: $1"
    echo "Gracefully shutting down..."
    
    sleep 2
    
    echo "Process shut down"

    exit 0 # Important! We must manually exit with the success status code!
}

echo "Starting process (PID $$)"

sleep infinity & # Sleep for ever and fork into a new proccess
wait             # Wait for the new proccess so the script never finishes
