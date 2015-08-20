#!/bin/bash

source ${CATTLE_HOME:-/var/lib/cattle}/common/scripts.sh

trap "touch $CATTLE_HOME/.pyagent-stamp" exit

cd $(dirname $0)

mkdir -p ${CATTLE_HOME}/bin

cp bin/convoy bin/convoy-pdata_tools ${CATTLE_HOME}/bin

chmod +x ${CATTLE_HOME}/bin/convoy
chmod +x ${CATTLE_HOME}/bin/convoy-pdata_tools
