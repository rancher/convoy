#!/usr/bin/python

import subprocess
import os
import json

EXT4_FS = "ext4"

def _get_volume(volume):
    return ["--volume", volume]

class VolumeManager:
    def __init__(self, binary, mount_root):
        self.base_cmdline = [binary]
	self.mount_root = mount_root

    def start_server(self, pidfile, cmdline):
        start_cmdline = ["start-stop-daemon", "-S", "-b", "-m", "-p", pidfile,
			"--exec"] + self.base_cmdline + ["--"] + cmdline
        subprocess.check_call(start_cmdline)

    def stop_server(self, pidfile):
        stop_cmdline = ["start-stop-daemon", "-K", "-p", pidfile, "-x"] + self.base_cmdline
        return subprocess.call(stop_cmdline)

    def check_server(self, pidfile):
        check_cmdline = ["start-stop-daemon", "-T", "-p", pidfile]
        return subprocess.call(check_cmdline)

    def server_info(self):
	return subprocess.check_output(self.base_cmdline + ["info"])

    def create_volume(self, size = "", name = ""):
        cmd = ["volume", "create"]
        if size != "":
            cmd = cmd + ["--size", size]
        if name != "":
            cmd = cmd + ["--name", name]
        data = subprocess.check_output(self.base_cmdline + cmd)
        volume = json.loads(data)
        if name != "":
            assert volume["Name"] == name
        return volume["UUID"]

    def delete_volume(self, volume):
        subprocess.check_call(self.base_cmdline + ["volume", "delete",
            ] + _get_volume(volume))

    def mount_volume(self, volume):
        volume_mount_dir = os.path.join(self.mount_root, volume)
        if not os.path.exists(volume_mount_dir):
    	    os.makedirs(volume_mount_dir)
        assert os.path.exists(volume_mount_dir)
        cmdline = self.base_cmdline + ["volume", "mount",
    		"--mountpoint", volume_mount_dir] + _get_volume(volume)

	subprocess.check_call(cmdline)
        return volume_mount_dir

    def mount_volume_auto(self, volume):
        cmdline = self.base_cmdline + ["volume", "mount"] + _get_volume(volume)

	data = subprocess.check_output(cmdline)
        volume = json.loads(data)
        return volume["MountPoint"]

    def umount_volume(self, volume):
        subprocess.check_call(self.base_cmdline + ["volume", "umount",
            ] + _get_volume(volume))

    def list_volumes(self, uuid = None, snapshot = None):
        if uuid is None:
    	    data = subprocess.check_output(self.base_cmdline + \
			    ["volume", "list"])
        elif snapshot is None:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--volume", uuid])
        else:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--volume", uuid,
                "--snapshot", snapshot])

        volumes = json.loads(data)
        return volumes["Volumes"]

    def list_volumes_by_name(self, name = None, snapshot = None):
        if name is None:
    	    data = subprocess.check_output(self.base_cmdline + \
			    ["volume", "list"])
        elif snapshot is None:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--volume", name])
        else:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--volume", name,
                "--snapshot", snapshot])

        volumes = json.loads(data)
        return volumes["Volumes"]

    def create_snapshot(self, volume, snapshot_name = ""):
        cmd = ["snapshot", "create"] + _get_volume(volume)
        if snapshot_name != "":
                cmd += ["--name", snapshot_name]
        data = subprocess.check_output(self.base_cmdline + cmd)
        snapshot = json.loads(data)
        return snapshot["UUID"]

    def delete_snapshot(self, snapshot):
        subprocess.check_call(self.base_cmdline + ["snapshot", "delete",
	        "--snapshot", snapshot])

    def backup_snapshot_to_objectstore(self, snapshot_uuid, dest_url):
	subprocess.check_call(self.base_cmdline + ["snapshot", "backup",
		"--snapshot", snapshot_uuid,
		"--dest-url", dest_url])

    def restore_snapshot_from_objectstore(self, snapshot_uuid,
		    origin_volume_uuid, target_volume_uuid, dest_url):
	subprocess.check_call(self.base_cmdline + ["snapshot", "restore",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", origin_volume_uuid,
		"--target-volume-uuid", target_volume_uuid,
		"--dest-url", dest_url])

    def remove_snapshot_from_objectstore(self,
		    snapshot_uuid, volume_uuid, dest_url):
	subprocess.check_call(self.base_cmdline + ["snapshot", "remove-backup",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--dest-url", dest_url])

    def list_volume_objectstore(self, volume_uuid, dest_url):
	data = subprocess.check_output(self.base_cmdline + ["objectstore",
                "list-volume",
		"--volume-uuid", volume_uuid,
		"--dest-url", dest_url])
        volumes = json.loads(data)
        return volumes["Volumes"]

    def list_volume_objectstore_with_snapshot(self,
            snapshot_uuid, volume_uuid, dest_url):
	data = subprocess.check_output(self.base_cmdline + ["objectstore",
                "list-volume",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--dest-url", dest_url])
        volumes = json.loads(data)
        return volumes["Volumes"]
