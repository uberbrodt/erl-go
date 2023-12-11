#!/usr/bin/env bash
# byte oriented cmd

while IFS= read -d $'\x04' -r -n "$1" msg; do
  printf '%s' "$msg"
done

