#!/usr/bin/python

import argparse
import subprocess
import json

RANCHER_VOLUME_BINARY = "rancher-volume"

parser = argparse.ArgumentParser()
parser.add_argument("--new-volume-name", dest="new_volume_name", help="new volume name",
		required=True)
parser.add_argument("--snapshot-uuid", dest="snapshot_uuid", help="oldsnapshot",
		required=True)
parser.add_argument("--volume-uuid", dest="volume_uuid", help="old volume uuid",
		required=True)
parser.add_argument("--objectstore-uuid", dest="object_store", help="object store uuid",
		required=True)

options = parser.parse_args()

#create new volume
cmd = [RANCHER_VOLUME_BINARY, "volume", "create",
		"--volume-name", options.new_volume_name]
data = subprocess.check_output(cmd)
volume = json.loads(data)
new_volume_uuid = volume["UUID"]

#restore to the new volume
cmd = [RANCHER_VOLUME_BINARY, "snapshot", "restore",
		"--snapshot-uuid", options.snapshot_uuid,
		"--volume-uuid", options.volume_uuid,
		"--objectstore-uuid", options.object_store,
		"--target-volume-uuid", new_volume_uuid]
subprocess.check_call(cmd)

print "Restoration complete."
print "New volume uuid is", new_volume_uuid
print "New volume name is", options.new_volume_name
