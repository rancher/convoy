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

    def create_volume(self, size = "", base = "", name = "",
                    need_format = False):
        cmd = ["volume", "create"]
        if size != "":
            cmd = cmd + ["--size", size]
        if base != "":
            cmd = cmd + ["--image-uuid", base]
        if name != "":
            cmd = cmd + ["--name", name]
        if need_format:
            cmd = cmd + ["--format"]
        data = subprocess.check_output(self.base_cmdline + cmd)
        volume = json.loads(data)
        if name != "":
            assert volume["Name"] == name
        return volume["UUID"]

    def delete_volume(self, volume):
        subprocess.check_call(self.base_cmdline + ["volume", "delete",
            ] + _get_volume(volume))

    def mount_volume(self, volume, need_format):
        volume_mount_dir = os.path.join(self.mount_root, volume)
        if not os.path.exists(volume_mount_dir):
    	    os.makedirs(volume_mount_dir)
        assert os.path.exists(volume_mount_dir)
        cmdline = self.base_cmdline + ["volume", "mount",
    		"--mountpoint", volume_mount_dir,
    		"--fs", EXT4_FS] + _get_volume(volume)
        if need_format:
    	    cmdline = cmdline + ["--format"]

	subprocess.check_call(cmdline)
        return volume_mount_dir

    def mount_volume_auto(self, volume, need_format):
        cmdline = self.base_cmdline + ["volume", "mount",
    		"--fs", EXT4_FS] + _get_volume(volume)
        if need_format:
    	    cmdline = cmdline + ["--format"]

	data = subprocess.check_output(cmdline)
        volume = json.loads(data)
        return volume["MountPoint"]

    def umount_volume(self, volume):
        subprocess.check_call(self.base_cmdline + ["volume", "umount",
            ] + _get_volume(volume))

    def list_volumes(self, uuid = None, snapshot_uuid = None):
        if uuid is None:
    	    data = subprocess.check_output(self.base_cmdline + \
			    ["volume", "list"])
        elif snapshot_uuid is None:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--volume", uuid])
        else:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--volume", uuid,
                "--snapshot-uuid", snapshot_uuid])

        volumes = json.loads(data)
        return volumes["Volumes"]

    def list_volumes_by_name(self, name = None, snapshot_uuid = None):
        if name is None:
    	    data = subprocess.check_output(self.base_cmdline + \
			    ["volume", "list"])
        elif snapshot_uuid is None:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--volume", name])
        else:
            data = subprocess.check_output(self.base_cmdline + ["volume", "list",
                "--volume", name,
                "--snapshot-uuid", snapshot_uuid])

        volumes = json.loads(data)
        return volumes["Volumes"]

    def create_snapshot(self, volume, snapshot_name = ""):
        cmd = ["snapshot", "create"] + _get_volume(volume)
        if snapshot_name != "":
                cmd += ["--snapshot-name", snapshot_name]
        data = subprocess.check_output(self.base_cmdline + cmd)
        snapshot = json.loads(data)
        return snapshot["UUID"]

    def delete_snapshot(self, snapshot_uuid, volume):
        subprocess.check_call(self.base_cmdline + ["snapshot", "delete",
	        "--snapshot-uuid", snapshot_uuid] + _get_volume(volume))

    def register_vfs_objectstore(self, path):
	data = subprocess.check_output(self.base_cmdline + ["objectstore",
		"register", "--kind", "vfs",
		"--opts", "vfs.path="+path])
	bs = json.loads(data)
	assert bs["Kind"] == "vfs"
	return bs["UUID"]

    def register_s3_objectstore(self, region, bucket, path):
	data = subprocess.check_output(self.base_cmdline + ["objectstore",
		"register", "--kind", "s3",
		"--opts", "s3.region="+region,
		"--opts", "s3.bucket="+bucket,
		"--opts", "s3.path="+path])
	bs = json.loads(data)
	assert bs["Kind"] == "s3"
	return bs["UUID"]

    def deregister_objectstore(self, uuid):
	subprocess.check_call(self.base_cmdline + ["objectstore", "deregister",
		"--objectstore-uuid", uuid])

    def add_volume_to_objectstore(self, volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["objectstore",
                "add-volume",
		"--volume-uuid", volume_uuid,
		"--objectstore-uuid", bs_uuid])

    def remove_volume_from_objectstore(self, volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["objectstore",
                "remove-volume",
		"--volume-uuid", volume_uuid,
		"--objectstore-uuid", bs_uuid])

    def backup_snapshot_to_objectstore(self, snapshot_uuid, volume_uuid,
		    bs_uuid):
	subprocess.check_call(self.base_cmdline + ["snapshot", "backup",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--objectstore-uuid", bs_uuid])

    def restore_snapshot_from_objectstore(self, snapshot_uuid,
		    origin_volume_uuid, target_volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["snapshot", "restore",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", origin_volume_uuid,
		"--target-volume-uuid", target_volume_uuid,
		"--objectstore-uuid", bs_uuid])

    def remove_snapshot_from_objectstore(self,
		    snapshot_uuid, volume_uuid, bs_uuid):
	subprocess.check_call(self.base_cmdline + ["snapshot", "remove-backup",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--objectstore-uuid", bs_uuid])

    def list_volume_objectstore(self, volume_uuid, bs_uuid):
	data = subprocess.check_output(self.base_cmdline + ["objectstore",
                "list-volume",
		"--volume-uuid", volume_uuid,
		"--objectstore-uuid", bs_uuid])
        volumes = json.loads(data)
        return volumes["Volumes"]

    def list_volume_objectstore_with_snapshot(self,
            snapshot_uuid, volume_uuid, bs_uuid):
	data = subprocess.check_output(self.base_cmdline + ["objectstore",
                "list-volume",
		"--snapshot-uuid", snapshot_uuid,
		"--volume-uuid", volume_uuid,
		"--objectstore-uuid", bs_uuid])
        volumes = json.loads(data)
        return volumes["Volumes"]

    def add_image_to_objectstore(self, image_file, bs_uuid):
        data = subprocess.check_output(self.base_cmdline + ["objectstore",
                "add-image",
                "--image-file", image_file,
                "--objectstore-uuid", bs_uuid])
        image = json.loads(data)
        return image["UUID"]

    def remove_image_from_objectstore(self, image_uuid, bs_uuid):
        subprocess.check_call(self.base_cmdline + ["objectstore",
                "remove-image",
                "--image-uuid", image_uuid,
                "--objectstore-uuid", bs_uuid])

    def activate_image(self, image_uuid, bs_uuid):
        subprocess.check_call(self.base_cmdline + ["objectstore",
                "activate-image",
                "--image-uuid", image_uuid,
                "--objectstore-uuid", bs_uuid])

    def deactivate_image(self, image_uuid, bs_uuid):
        subprocess.check_call(self.base_cmdline + ["objectstore",
                "deactivate-image",
                "--image-uuid", image_uuid,
                "--objectstore-uuid", bs_uuid])

    def list_objectstores(self, store_uuid = ""):
        cmd = ["objectstore", "list"]
        if store_uuid != "":
            cmd += ["--objectstore-uuid", store_uuid]
	data = subprocess.check_output(self.base_cmdline + cmd)
        stores = json.loads(data)
        return stores["ObjectStores"]

