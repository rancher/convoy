#!/bin/bash

set -e

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

if [ $write_to_disk -eq 0 ]; then
        exit
fi

last_partition=$(fdisk -l $dev|tail -n 1)
echo $last_partition
if [[ "$last_partition" != *"Device"* ]]; then
        echo "$dev already partitioned, won't to start new partition"
        exit 1
fi

echo "Start partitioning of $dev"

(echo n; echo p; echo 1; echo; echo $datadev_sectors; \
        echo n; echo p; echo 2; echo; echo; echo w) | fdisk $dev

echo "Complete the partition of $dev"
fdisk -l $dev

echo "Now you can start Convoy Daemon using: "
echo
echo "convoy daemon --drivers devicemapper --driver-opts dm.datadev=${dev}1 --driver-opts dm.metadatadev=${dev}2"
