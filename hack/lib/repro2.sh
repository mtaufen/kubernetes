#!/bin/bash

bash --version
echo "-----"
dedup() {
    printf "%s\n" "$@" | sort -u
}
foo=(a b b c)
declare -p foo
echo "dedup foo"
dedup "${foo[@]}"
echo "mapfile <<< dedup foo"
mapfile -t out <<< $(dedup "${foo[@]}")
declare -p out