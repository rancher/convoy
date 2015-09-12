#!/bin/bash

if [ $# -eq 0 ]; then
        echo "Usage: $0 [--write-to-disk] [-m|--max-thins 10000] <dev>"
        exit
fi

if [ ! $(command -v blockdev) ]; then
        echo "Cannot find \"blockdev\" command"
        exit 1
fi

if [ ! $(command -v fdisk) ]; then
        echo "Cannot find \"fdisk\" command"
        exit 1
fi

if [ ! $(command -v convoy-pdata_tools) ]; then
        echo "Cannot find \"convoy-pdata_tools\" command"
        exit 1
fi

write_to_disk=0
dev=""
maxthins=10000
while test $# -gt 0; do
        case "$1" in
                --write-to-disk)
                        write_to_disk=1
                        shift
                        ;;
                -m|--max-thins)
                        shift
                        maxthins=$1
                        shift
                        ;;
                *)
                        dev=$1
                        shift
                        ;;
        esac
done

if [ ! -e $dev ]; then
        echo "Cannot find device $dev"
        exit 1
fi

devsize=$(blockdev --getsize64 $dev)
echo "$dev size is $devsize bytes"
echo "Maximum volume and snapshot counts is $maxthins"

metadev_size=$(convoy-pdata_tools thin_metadata_size \
        -b 2m -s ${devsize}b -m ${maxthins} -u b -n)
echo "Metadata Device size would be $metadev_size bytes"

datadev_size=$(($devsize-$metadev_size))
echo "Data Device size would be $datadev_size bytes"

datadev_sectors=$((datadev_size/512))
echo "Data Device would be $datadev_sectors sectors"

last_line=$(fdisk -l $dev|tail -n 1)
if [[ "$last_line" != *"Device"* && "$last_line" != "" ]]; then
        echo "$dev already partitioned, can't start partition"
        exit 1
fi

if [ $write_to_disk -eq 0 ]; then
        exit
fi

echo "Start partitioning of $dev"

(echo n; echo p; echo 1; echo; echo $datadev_sectors; \
        echo n; echo p; echo 2; echo; echo; echo w) | fdisk $dev > /dev/null

echo
echo "Complete the partition of $dev"

tempfile=$(mktemp)
fdisk -l $dev | tee $tempfile
datadev=$(cat $tempfile| tail -n 2| head -n 1 |cut -d " " -f 1)
metadev=$(cat $tempfile| tail -n 1| cut -d " " -f 1)

echo "Now you can start Convoy Daemon using: "
echo
echo "convoy daemon --drivers devicemapper --driver-opts dm.datadev=${datadev} --driver-opts dm.metadatadev=${metadev}"
