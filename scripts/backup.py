#!/usr/bin/python

import argparse
import subprocess
import json

RANCHER_VOLUME_BINARY = "rancher-volume"

parser = argparse.ArgumentParser()
parser.add_argument("--volume-name", dest="volume_name", help="volume name",
		required=True)
parser.add_argument("--object-store", dest="object_store", help="object store uuid",
		required=True)

options = parser.parse_args()

#create snapshot
cmd = [RANCHER_VOLUME_BINARY, "snapshot", "create",
	"--volume-name", options.volume_name]
data = subprocess.check_output(cmd)
snapshot = json.loads(data)
snapshot_uuid = snapshot["UUID"]
volume_uuid = snapshot["VolumeUUID"]

#add volume of snapshot to object store
cmd = [RANCHER_VOLUME_BINARY, "objectstore", "add-volume",
	"--volume-uuid", volume_uuid,
	"--objectstore-uuid", options.object_store]
subprocess.check_call(cmd)

#backup snapshot to object store
cmd = [RANCHER_VOLUME_BINARY, "snapshot", "backup",
	"--snapshot-uuid", snapshot_uuid,
        "--volume-uuid", volume_uuid,
	"--objectstore-uuid", options.object_store]
subprocess.check_call(cmd)

#print out cmdline for restore
cmd = ["restore.py", "--snapshot-uuid", snapshot_uuid,
	"--volume-uuid", volume_uuid,
	"--objectstore-uuid", options.object_store,
	"--new-volume-name", "<new_volume_name>"]
print " ".join(cmd)
