#!/usr/bin/python

import subprocess
import os
import json

EXT4_FS = "ext4"

class VolumeManager:
    def __init__(self, cmdline, mount_root):
        self.base_cmdline = cmdline
	self.mount_root = mount_root

    def create_volume(self, size):
        data = subprocess.check_output(self.base_cmdline + ["volume", "create",
    	    "--size", str(size)])
        volume = json.loads(data)
        uuid = volume["UUID"]
        assert volume["Size"] == size
        assert volume["Base"] == ""
        return uuid

    def create_volume_with_uuid(self, size, uuid):
        data = subprocess.check_output(self.base_cmdline + ["volume", "create",
            "--size", str(size),
            "--uuid", uuid])
        volume = json.loads(data)
        assert volume["UUID"] == uuid
        assert volume["Size"] == size
        assert volume["Base"] == ""
        return uuid

    def delete_volume(self, uuid):
        subprocess.check_call(self.base_cmdline + ["volume", "delete",
    	    "--uuid", uuid])

    def mount_volume(self, uuid, need_format):
        volume_mount_dir = os.path.join(self.mount_root, uuid)
        if not os.path.exists(volume_mount_dir):
    	    os.makedirs(volume_mount_dir)
        assert os.path.exists(volume_mount_dir)
        cmdline = self.base_cmdline + ["volume", "mount",
    		"--uuid", uuid,
    		"--mountpoint", volume_mount_dir,
    		"--fs", EXT4_FS]
        if need_format:
    	    cmdline = cmdline + ["--format"]

	subprocess.check_call(cmdline)
        return volume_mount_dir

    def umount_volume(self, uuid):
        subprocess.check_call(self.base_cmdline + ["volume", "umount",
    	    "--uuid", uuid])

    def list_volumes(self, uuid = None, snapshot_uuid = None):
        if uuid is None:
    	    data = subprocess.check_output(self.base_cmdline + \
			    ["volume", "list"])
        elif snapshot_uuid is None:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--uuid", uuid])
        else:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--uuid", uuid,
                "--snapshot-uuid", snapshot_uuid])

        volumes = json.loads(data)
        return volumes["Volumes"]

    def create_snapshot(self, volume_uuid):
        data = subprocess.check_output(self.base_cmdline + \
		["snapshot", "create",
    	    	"--volume-uuid", volume_uuid])
        snapshot = json.loads(data)
        assert snapshot["VolumeUUID"] == volume_uuid
        return snapshot["UUID"]

    def create_snapshot_with_uuid(self, volume_uuid, uuid):
        data = subprocess.check_output(self.base_cmdline + \
		["snapshot", "create",
                 "--volume-uuid", volume_uuid,
                 "--uuid", uuid])
        snapshot = json.loads(data)
        assert snapshot["UUID"] == uuid
        assert snapshot["VolumeUUID"] == volume_uuid
        return snapshot["UUID"]

    def delete_snapshot(self, snapshot_uuid, volume_uuid):
        subprocess.check_call(self.base_cmdline + ["snapshot", "delete",
	        "--uuid", snapshot_uuid,
	        "--volume-uuid", volume_uuid])

    def register_vfs_blockstore(self, path):
	data = subprocess.check_output(self.base_cmdline + ["blockstore",
		"register", "--kind", "vfs",
		"--opts", "vfs.path="+path])
	bs = json.loads(data)
	assert bs["Kind"] == "vfs"
	return bs["UUID"]

    def deregister_blockstore(self, uuid):
	subprocess.check_call(self.base_cmdline + ["blockstore", "deregister",
		"--uuid", uuid])

    def add_volume_to_blockstore(self, volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["blockstore", "add",
		"--volume-uuid", volume_uuid,
		"--uuid", bs_uuid])

    def remove_volume_from_blockstore(self, volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["blockstore", "remove",
		"--volume-uuid", volume_uuid,
		"--uuid", bs_uuid])

    def backup_snapshot_to_blockstore(self, snapshot_uuid, volume_uuid,
		    bs_uuid):
	subprocess.check_call(self.base_cmdline + ["snapshot", "backup",
		"--uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--blockstore-uuid", bs_uuid])

    def restore_snapshot_from_blockstore(self, snapshot_uuid,
		    origin_volume_uuid, target_volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["snapshot", "restore",
		"--uuid", snapshot_uuid,
		"--origin-volume-uuid", origin_volume_uuid,
		"--target-volume-uuid", target_volume_uuid,
		"--blockstore-uuid", bs_uuid])

    def remove_snapshot_from_blockstore(self,
		    snapshot_uuid, volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["snapshot", "remove",
		"--uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--blockstore-uuid", bs_uuid])

    def list_blockstore(self, volume_uuid, bs_uuid):
	data = subprocess.check_output(self.base_cmdline + ["blockstore", "list",
		"--volume-uuid", volume_uuid,
		"--uuid", bs_uuid])
        volumes = json.loads(data)
        return volumes["Volumes"]

    def list_blockstore_with_snapshot(self, snapshot_uuid, volume_uuid, bs_uuid):
	data = subprocess.check_output(self.base_cmdline + ["blockstore", "list",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--uuid", bs_uuid])
        volumes = json.loads(data)
        return volumes["Volumes"]
