#!/bin/bash

bash --version
echo "-----"
KUBE_BUILD_PLATFORMS="linux/amd64 windows/amd64"
readonly KUBE_SUPPORTED_SERVER_PLATFORMS=(
    linux/amd64
    linux/arm
    linux/arm64
    linux/s390x
    linux/ppc64le
)
dups() {
    printf "%s\n" "$@" | sort | uniq -d
}
dedup() {
    printf "%s\n" "$@" | sort -u
}
IFS=" " read -ra platforms <<< "${KUBE_BUILD_PLATFORMS}"
mapfile -t platforms <<< "$(dedup "${platforms[@]}")"
mapfile -t KUBE_SERVER_PLATFORMS <<< "$(dups \
    "${platforms[@]}" \
    "${KUBE_SUPPORTED_SERVER_PLATFORMS[@]}" \
    )"
declare -p platforms
declare -p KUBE_SERVER_PLATFORMS