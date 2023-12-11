#!/usr/bin/env bash
# Like fail_after_time.sh but writes to stderr

cnt=0
while [[ $cnt -le $1 ]]; do
    echo "loop $cnt" 1>&2
    sleep 1
    cnt=$((cnt+1))
done

echo "exiting"

exit "$2"
