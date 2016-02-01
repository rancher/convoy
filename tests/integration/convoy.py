#!/usr/bin/python

import subprocess
import os
import json

class VolumeManager:
    def __init__(self, base_cmdline, mount_root):
        self.base_cmdline = base_cmdline
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

    def start_server_container(self, name, cfg_root, file_root, container, cmdline):
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
        return subprocess.check_call(start_cmdline)

    def stop_server_container(self, name):
        stop_cmdline = ["docker", "rm", "-fv", name]
        return subprocess.check_call(stop_cmdline)

    def check_server_container(self, name):
        check_cmdline = ["docker", "inspect", "-f", "{{.State.Running}}", name]
        output = subprocess.check_output(check_cmdline)
        return output.startswith("true")
 
    def server_info(self):
	return subprocess.check_output(self.base_cmdline + ["info"])

    def create_volume(self, size = "", name = "", backup = "", driver = "",
                    volume_id = "", volume_type = "", iops = "", forvm = False):
        cmd = ["create"]
        if name != "":
            cmd = cmd + [name]
        if size != "":
            cmd = cmd + ["--size", size]
        if backup != "":
            cmd = cmd + ["--backup", backup]
        if driver != "":
            cmd = cmd + ["--driver", driver]
        if volume_id != "":
            cmd = cmd + ["--id", volume_id]
        if volume_type != "":
            cmd = cmd + ["--type", volume_type]
        if iops != "":
            cmd = cmd + ["--iops", iops]
        if forvm:
            cmd = cmd + ["--vm"]
        data = subprocess.check_output(self.base_cmdline + cmd)
        return data.strip()

    def delete_volume(self, volume, ref_only = False):
        cmdline = self.base_cmdline + ["delete", volume]
        if ref_only:
                cmdline += ["--reference"]
        subprocess.check_call(cmdline)

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
        return data.strip()

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

    def create_snapshot(self, volume, name = ""):
        cmd = ["snapshot", "create", volume]
        if name != "":
                cmd += ["--name", name]
        data = subprocess.check_output(self.base_cmdline + cmd)
        return data.strip()

    def delete_snapshot(self, snapshot):
        subprocess.check_call(self.base_cmdline + ["snapshot", "delete",
                snapshot])

    def inspect_snapshot(self, snapshot):
        output = subprocess.check_output(self.base_cmdline + ["snapshot", "inspect",
                snapshot])
        snapshot = json.loads(output)
        return snapshot

    def create_backup(self, snapshot, dest = ""):
        cmdline = self.base_cmdline + ["backup", "create", snapshot]
        if dest != "":
            cmdline += ["--dest", dest]
        data = subprocess.check_output(cmdline)
        return data.strip()

    def delete_backup(self, backup):
	subprocess.check_call(self.base_cmdline + ["backup", "delete", backup])

    def list_backup(self, dest, volume_name = ""):
        cmd = ["backup", "list", dest]
        if volume_name != "":
		cmd += ["--volume-name", volume_name]
	data = subprocess.check_output(self.base_cmdline + cmd)
        backups = json.loads(data)
        return backups

    def inspect_backup(self, backup):
	data = subprocess.check_output(self.base_cmdline + ["backup",
                "inspect", backup])
        backups = json.loads(data)
        return backups
