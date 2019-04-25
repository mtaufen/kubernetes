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
echo "while <<< dedup foo"
out=()
i=0
declare -p IFS
while read -r line; do
    echo "$i"
    echo "$line"
    out[$i]="$line"
    i=$((i + 1))
done <<< "$(dedup "${foo[@]}")"
declare -p out
declare -p i