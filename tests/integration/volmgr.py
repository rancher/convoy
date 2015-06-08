#!/usr/bin/python

import subprocess
import os
import json

EXT4_FS = "ext4"

class VolumeManager:
    def __init__(self, binary, mount_root):
        self.base_cmdline = [binary]
	self.mount_root = mount_root

    def start_server(self, pidfile, cmdline):
        start_cmdline = ["start-stop-daemon", "-S", "-b", "-m", "-p", pidfile,
			"--exec"] + self.base_cmdline + ["--"] + cmdline
        subprocess.check_call(start_cmdline)

    def stop_server(self, pidfile):
        stop_cmdline = ["start-stop-daemon", "-K","-p", pidfile, "-x"] + self.base_cmdline
        subprocess.check_call(stop_cmdline)

    def server_info(self):
	return subprocess.check_output(self.base_cmdline + ["info"])

    def create_volume(self, size, uuid = "", base = ""):
        cmd = ["volume", "create", "--size", str(size)]
        if uuid != "":
            cmd = cmd + ["--volume-uuid", uuid]
        if base != "":
            cmd = cmd + ["--image-uuid", base]
        data = subprocess.check_output(self.base_cmdline + cmd)
        volume = json.loads(data)
        if uuid != "":
            assert volume["UUID"] == uuid
        assert volume["Size"] == size
        return volume["UUID"]

    def delete_volume(self, uuid):
        subprocess.check_call(self.base_cmdline + ["volume", "delete",
            "--volume-uuid", uuid])

    def mount_volume(self, uuid, need_format):
        volume_mount_dir = os.path.join(self.mount_root, uuid)
        if not os.path.exists(volume_mount_dir):
    	    os.makedirs(volume_mount_dir)
        assert os.path.exists(volume_mount_dir)
        cmdline = self.base_cmdline + ["volume", "mount",
                "--volume-uuid", uuid,
    		"--mountpoint", volume_mount_dir,
    		"--fs", EXT4_FS]
        if need_format:
    	    cmdline = cmdline + ["--format"]

	subprocess.check_call(cmdline)
        return volume_mount_dir

    def umount_volume(self, uuid):
        subprocess.check_call(self.base_cmdline + ["volume", "umount",
            "--volume-uuid", uuid])

    def list_volumes(self, uuid = None, snapshot_uuid = None):
        if uuid is None:
    	    data = subprocess.check_output(self.base_cmdline + \
			    ["volume", "list"])
        elif snapshot_uuid is None:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--volume-uuid", uuid])
        else:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--volume-uuid", uuid,
                "--snapshot-uuid", snapshot_uuid])

        volumes = json.loads(data)
        return volumes["Volumes"]

    def create_snapshot(self, volume_uuid, uuid = ""):
        cmd = ["snapshot", "create", "--volume-uuid", volume_uuid]
        if uuid != "":
            cmd = cmd + ["--snapshot-uuid", uuid]
        data = subprocess.check_output(self.base_cmdline + cmd)
        snapshot = json.loads(data)
        if uuid != "":
            assert snapshot["UUID"] == uuid
        assert snapshot["VolumeUUID"] == volume_uuid
        return snapshot["UUID"]

    def delete_snapshot(self, snapshot_uuid, volume_uuid):
        subprocess.check_call(self.base_cmdline + ["snapshot", "delete",
	        "--snapshot-uuid", snapshot_uuid,
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
		"--blockstore-uuid", uuid])

    def add_volume_to_blockstore(self, volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["blockstore",
                "add-volume",
		"--volume-uuid", volume_uuid,
		"--blockstore-uuid", bs_uuid])

    def remove_volume_from_blockstore(self, volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["blockstore",
                "remove-volume",
		"--volume-uuid", volume_uuid,
		"--blockstore-uuid", bs_uuid])

    def backup_snapshot_to_blockstore(self, snapshot_uuid, volume_uuid,
		    bs_uuid):
	subprocess.check_call(self.base_cmdline + ["snapshot", "backup",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--blockstore-uuid", bs_uuid])

    def restore_snapshot_from_blockstore(self, snapshot_uuid,
		    origin_volume_uuid, target_volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["snapshot", "restore",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", origin_volume_uuid,
		"--target-volume-uuid", target_volume_uuid,
		"--blockstore-uuid", bs_uuid])

    def remove_snapshot_from_blockstore(self,
		    snapshot_uuid, volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["snapshot", "remove",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--blockstore-uuid", bs_uuid])

    def list_volume_blockstore(self, volume_uuid, bs_uuid):
	data = subprocess.check_output(self.base_cmdline + ["blockstore",
                "list-volume",
		"--volume-uuid", volume_uuid,
		"--blockstore-uuid", bs_uuid])
        volumes = json.loads(data)
        return volumes["Volumes"]

    def list_volume_blockstore_with_snapshot(self,
            snapshot_uuid, volume_uuid, bs_uuid):
	data = subprocess.check_output(self.base_cmdline + ["blockstore",
                "list-volume",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--blockstore-uuid", bs_uuid])
        volumes = json.loads(data)
        return volumes["Volumes"]

    def add_image_to_blockstore(self, image_file, bs_uuid):
        data = subprocess.check_output(self.base_cmdline + ["blockstore",
                "add-image",
                "--image-file", image_file,
                "--blockstore-uuid", bs_uuid])
        image = json.loads(data)
        return image["UUID"]

    def remove_image_from_blockstore(self, image_uuid, bs_uuid):
        subprocess.check_call(self.base_cmdline + ["blockstore",
                "remove-image",
                "--image-uuid", image_uuid,
                "--blockstore-uuid", bs_uuid])

    def activate_image(self, image_uuid, bs_uuid):
        subprocess.check_call(self.base_cmdline + ["blockstore",
                "activate-image",
                "--image-uuid", image_uuid,
                "--blockstore-uuid", bs_uuid])

    def deactivate_image(self, image_uuid, bs_uuid):
        subprocess.check_call(self.base_cmdline + ["blockstore",
                "deactivate-image",
                "--image-uuid", image_uuid,
                "--blockstore-uuid", bs_uuid])

