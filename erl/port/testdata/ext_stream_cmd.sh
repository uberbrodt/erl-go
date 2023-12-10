#!/usr/bin/env bash

printf "started ext program\0"
while IFS= read -r -d $'\0' msg; do
  printf '%s\0' "$msg"
done

