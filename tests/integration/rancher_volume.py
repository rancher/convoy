#!/usr/bin/python

import subprocess
import os
import json

EXT4_FS = "ext4"

class VolumeManager:
    def __init__(self, base_cmdline, mount_root):
        self.base_cmdline = base_cmdline
	self.mount_root = mount_root

    def start_server(self, name, cfg_root, file_root, container, cmdline):
        start_cmdline = ["docker", "run", "--privileged",
                        "--name", name, "-d",
                        "-v", "/etc/ssl/certs:/etc/ssl/certs",
                        "-v", os.path.expanduser("~") + "/.aws:/root/.aws",
                        "-v", "/dev:/host/dev",
                        "-v", "/proc:/host/proc",
                        "-v", cfg_root + ":" + cfg_root,
                        "-v", file_root + ":" + file_root,
			container,
                        ] + cmdline
        subprocess.check_call(start_cmdline)

    def stop_server(self, name):
        stop_cmdline = ["docker", "rm", "-fv", name]
        return subprocess.call(stop_cmdline)

    def check_server(self, name):
        check_cmdline = ["docker", "inspect", "-f", "{{.State.Running}}", name]
        output = subprocess.check_output(check_cmdline)
        return output.startswith("true")

    def server_info(self):
	return subprocess.check_output(self.base_cmdline + ["info"])

    def create_volume(self, size = "", name = "", backup = "", driver = ""):
        cmd = ["create"]
        if name != "":
            cmd = cmd + [name]
        if size != "":
            cmd = cmd + ["--size", size]
        if backup != "":
            cmd = cmd + ["--backup", backup]
        if driver != "":
            cmd = cmd + ["--driver", driver]
        data = subprocess.check_output(self.base_cmdline + cmd)
        volume = json.loads(data)
        if name != "":
            assert volume["Name"] == name
        return volume["UUID"]

    def delete_volume(self, volume):
        subprocess.check_call(self.base_cmdline + ["delete", volume])

    def mount_volume_with_path(self, volume):
        volume_mount_dir = os.path.join(self.mount_root, volume)
        if not os.path.exists(volume_mount_dir):
    	    os.makedirs(volume_mount_dir)
        assert os.path.exists(volume_mount_dir)
        cmdline = self.base_cmdline + ["mount", volume,
    		"--mountpoint", volume_mount_dir]

	subprocess.check_call(cmdline)
        return volume_mount_dir

    def mount_volume(self, volume):
        cmdline = self.base_cmdline + ["mount", volume]

	data = subprocess.check_output(cmdline)
        volume = json.loads(data)
        return volume["MountPoint"]

    def umount_volume(self, volume):
        subprocess.check_call(self.base_cmdline + ["umount", volume])

    def list_volumes(self):
    	data = subprocess.check_output(self.base_cmdline + ["list"])
        volumes = json.loads(data)
        return volumes

    def inspect_volume(self, volume):
        cmd = ["inspect", volume]
    	data = subprocess.check_output(self.base_cmdline + cmd)

        return json.loads(data)

    def create_snapshot(self, volume, snapshot_name = ""):
        cmd = ["snapshot", "create", volume]
        if snapshot_name != "":
                cmd += ["--name", snapshot_name]
        data = subprocess.check_output(self.base_cmdline + cmd)
        snapshot = json.loads(data)
        return snapshot["UUID"]

    def delete_snapshot(self, snapshot):
        subprocess.check_call(self.base_cmdline + ["snapshot", "delete",
                snapshot])

    def inspect_snapshot(self, snapshot):
        output = subprocess.check_output(self.base_cmdline + ["snapshot", "inspect",
                snapshot])
        snapshot = json.loads(output)
        return snapshot

    def create_backup(self, snapshot, dest):
        data = subprocess.check_output(self.base_cmdline + ["backup", "create",
                snapshot,
		"--dest", dest])
        backup = json.loads(data)
        return backup["URL"]

    def delete_backup(self, backup):
	subprocess.check_call(self.base_cmdline + ["backup", "delete", backup])

    def list_backup(self, dest, volume_uuid = ""):
        cmd = ["backup", "list", dest]
        if volume_uuid != "":
		cmd += ["--volume-uuid", volume_uuid]
	data = subprocess.check_output(self.base_cmdline + cmd)
        backups = json.loads(data)
        return backups

    def inspect_backup(self, backup):
	data = subprocess.check_output(self.base_cmdline + ["backup",
                "inspect", backup])
        backups = json.loads(data)
        return backups
