#!/usr/bin/env bash

echo "started"

function handle_ctrlc()
{
    echo "handled SIGINT"
    exit 0
}

# trapping the SIGINT signal
trap handle_ctrlc SIGINT




cnt=0
while [[ $cnt -le $1 ]]; do
    echo "loop $cnt"
    sleep 1
    cnt=$((cnt+1))

done

echo "exiting"

exit "$2"
