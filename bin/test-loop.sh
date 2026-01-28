#!/bin/bash

set -o pipefail

while true;
do
    date
    make clean && make test|tee -a tests.log
    echo $? >> test_runs.txt
    date
    sleep 5
done
