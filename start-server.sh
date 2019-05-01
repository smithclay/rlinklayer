#!/bin/bash -e

function ifup {
    if [[ ! -d /sys/class/net/${1} ]]; then
        printf '[bootstrap] No such interface: %s\n' "$1" >&2
        return 1
    else
        [[ $(</sys/class/net/${1}/operstate) == up ]]
    fi
}

unamestr=`uname`
if [[ "$unamestr" != 'Linux' ]]; then
   echo 'Error: This script only runs on Linux'
   exit 1
fi

IF_NAME=tap0
IF_TYPE=tap

if ifup ${IF_NAME}; then
    echo ${IF_NAME} online
else
    echo [bootstrap] ${IF_NAME} device does not exist, creating...
    ip tuntap add user root mode ${IF_TYPE} ${IF_NAME}
    ip link set ${IF_NAME} up
    ip addr add 192.168.1.1/24 dev ${IF_NAME}
fi

if [[ ${IF_TYPE} = 'tap' ]]; then
     # TODO: Instead of setting get this value and pass to program
     ip link set ${IF_NAME} addr 74:74:74:74:74:74
     if [[ -z "${GO_DEBUG}" ]]; then
        /go/src/github.com/smithclay/rlinklayer/server -tap
     else
        /go/bin/dlv --listen=:2345 --headless=true --api-version=2 exec /go/src/github.com/smithclay/rlinklayer/server -- -tap
     fi
else
    ./server
fi
