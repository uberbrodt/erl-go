#!/usr/bin/env bash

echo "started ext program"
while IFS= read -r line; do
  printf '%s\n' "$line"
done < /dev/stdin


