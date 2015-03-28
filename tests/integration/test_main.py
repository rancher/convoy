#!/usr/bin/python

import subprocess
import os
import json
import pytest

CFG_ROOT = "/tmp/volmgr_test/volmgr"
TEST_ROOT = "/tmp/volmgr_test/"
DATA_FILE = "data.vol"
METADATA_FILE = "metadata.vol"
DATA_DEVICE_SIZE = 1073618944
METADATA_DEVICE_SIZE = 40960000
DD_BLOCK_SIZE = 4096
POOL_NAME = "volmgr_test_pool"
VOLMGR_CMDLINE = ["../../volmgr", "--debug", "--log=/tmp/volmgr.log",
    "--root=" + CFG_ROOT]

DM_DIR = "/dev/mapper"
DM_BLOCK_SIZE = 2097152

VOLUME_SIZE_500M = 524288000
VOLUME_SIZE_100M = 104857600
EXT4_FS = "ext4"

data_dev = ""
metadata_dev = ""

mount_cleanup_list = []
dm_cleanup_list = []

def setup_module():
    if not os.path.exists(TEST_ROOT):
        os.makedirs(TEST_ROOT)
        assert os.path.exists(TEST_ROOT)

    data_file = os.path.join(TEST_ROOT, DATA_FILE)
    metadata_file = os.path.join(TEST_ROOT, METADATA_FILE)
    subprocess.check_call(["dd", "if=/dev/zero", "of=" + data_file,
            "bs=" + str(DD_BLOCK_SIZE), "count=" + str(DATA_DEVICE_SIZE /
            DD_BLOCK_SIZE)])
    assert os.path.exists(os.path.join(TEST_ROOT, DATA_FILE))
    subprocess.check_call(["dd", "if=/dev/zero", "of=" + metadata_file,
            "bs=" + str(DD_BLOCK_SIZE), "count=" + str(METADATA_DEVICE_SIZE /
            DD_BLOCK_SIZE)])
    assert os.path.exists(os.path.join(TEST_ROOT, METADATA_FILE))

    global data_dev
    data_dev = subprocess.check_output(["losetup", "-v", "-f",
            data_file]).strip().split(" ")[3]
    assert data_dev.startswith("/dev/loop")
    global metadata_dev
    metadata_dev = subprocess.check_output(["losetup", "-v", "-f",
            metadata_file]).strip().split(" ")[3]
    assert metadata_dev.startswith("/dev/loop")

def teardown_module():
    subprocess.check_call(["rm", "-rf", CFG_ROOT])
    while mount_cleanup_list:
	subprocess.check_call(["umount", mount_cleanup_list.pop()])

    while dm_cleanup_list:
	subprocess.check_call(["dmsetup", "remove", dm_cleanup_list.pop()])
    subprocess.check_call(["losetup", "-d", data_dev, metadata_dev])
    subprocess.check_call(["rm", "-rf", TEST_ROOT])

def test_init():
    subprocess.check_call(VOLMGR_CMDLINE + ["init", "--driver=devicemapper",
        "--driver-opts", "dm.datadev=" + data_dev,
	"--driver-opts", "dm.metadatadev=" + metadata_dev,
	"--driver-opts", "dm.thinpoolname=" + POOL_NAME])
    dm_cleanup_list.append(POOL_NAME)

def test_info():
    data = subprocess.check_output(VOLMGR_CMDLINE + ["info"])
    info = json.loads(data)
    assert info["Driver"] == "devicemapper"
    assert info["Root"] == os.path.join(CFG_ROOT, "devicemapper")
    assert info["DataDevice"] == data_dev
    assert info["MetadataDevice"] == metadata_dev
    assert info["ThinpoolDevice"] == os.path.join(DM_DIR, POOL_NAME)
    assert info["ThinpoolSize"] == DATA_DEVICE_SIZE
    assert info["ThinpoolBlockSize"] == DM_BLOCK_SIZE

def create_volume(size):
    data = subprocess.check_output(VOLMGR_CMDLINE + ["volume", "create",
	    "--size", str(size)])
    volume = json.loads(data)
    uuid = volume["UUID"]
    assert volume["Size"] == size
    assert volume["Base"] == ""
    return uuid

def delete_volume(uuid):
    subprocess.check_call(VOLMGR_CMDLINE + ["volume", "delete",
	    "--uuid", uuid])

def mount_volume(uuid, need_format):
    volume_mount_dir = os.path.join(TEST_ROOT, uuid)
    if not os.path.exists(volume_mount_dir):
	    os.makedirs(volume_mount_dir)
    assert os.path.exists(volume_mount_dir)
    cmdline = VOLMGR_CMDLINE + ["volume", "mount",
		"--uuid", uuid,
		"--mountpoint", volume_mount_dir,
		"--fs", EXT4_FS]
    if need_format:
	cmdline = cmdline + ["--format"]
    subprocess.check_call(cmdline)
    return volume_mount_dir

def umount_volume(uuid):
    subprocess.check_call(VOLMGR_CMDLINE + ["volume", "umount",
	    "--uuid", uuid])

def list_volumes(uuid = None):
    if uuid is None:
	data = subprocess.check_output(VOLMGR_CMDLINE + ["volume", "list"])
	volumes = json.loads(data)
	return volumes

    data = subprocess.check_output(VOLMGR_CMDLINE + ["volume", "list",
	    "--uuid", uuid])
    volumes = json.loads(data)
    return volumes

def create_snapshot(volume_uuid):
    data = subprocess.check_output(VOLMGR_CMDLINE + ["snapshot", "create",
	    "--volume-uuid", volume_uuid])
    snapshot = json.loads(data)
    assert snapshot["VolumeUUID"] == volume_uuid
    return snapshot["UUID"]

def delete_snapshot(snapshot_uuid, volume_uuid):
    subprocess.check_call(VOLMGR_CMDLINE + ["snapshot", "delete",
	    "--uuid", snapshot_uuid,
	    "--volume-uuid", volume_uuid])

def test_volume_cru():
    uuid1 = create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(uuid1)
    uuid2 = create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(uuid2)

    delete_volume(uuid2)
    dm_cleanup_list.pop()

    delete_volume(uuid1)
    dm_cleanup_list.pop()

def test_volume_mount():
    uuid = create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(uuid)

    # with format
    volume_mount_dir = mount_volume(uuid, True)
    mount_cleanup_list.append(volume_mount_dir)

    test_file = os.path.join(volume_mount_dir, "test")
    f = open(test_file, "w")
    subprocess.check_call(["echo", "This is volume test file"], stdout=f)
    f.close()
    assert os.path.exists(test_file)

    umount_volume(uuid)
    mount_cleanup_list.pop()
    assert not os.path.exists(test_file)

    # without format
    volume_mount_dir = mount_volume(uuid, False)
    mount_cleanup_list.append(volume_mount_dir)
    assert os.path.exists(test_file)

    umount_volume(uuid)
    mount_cleanup_list.pop()
    assert not os.path.exists(test_file)

    delete_volume(uuid)
    dm_cleanup_list.pop()

def test_volume_list():
    uuid1 = create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(uuid1)

    uuid2 = create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(uuid2)

    uuid3 = create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(uuid3)

    volumes = list_volumes(uuid3)
    assert volumes["Volumes"][uuid3]["Size"] == VOLUME_SIZE_100M

    volumes = list_volumes()
    assert volumes["Volumes"][uuid1]["Size"] == VOLUME_SIZE_500M
    assert volumes["Volumes"][uuid2]["Size"] == VOLUME_SIZE_100M
    assert volumes["Volumes"][uuid3]["Size"] == VOLUME_SIZE_100M

    delete_volume(uuid3)
    dm_cleanup_list.pop()

    delete_volume(uuid2)
    dm_cleanup_list.pop()

    delete_volume(uuid1)
    dm_cleanup_list.pop()

def test_snapshot_cru():
    volume_uuid = create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(volume_uuid)

    snapshot_uuid = create_snapshot(volume_uuid)
    delete_snapshot(snapshot_uuid, volume_uuid)

    delete_volume(volume_uuid)
    dm_cleanup_list.pop()

def test_snapshot_list():
    volume_uuid1 = create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(volume_uuid1)

    volume_uuid2 = create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(volume_uuid2)

    snap_vol1_uuid1 = create_snapshot(volume_uuid1)
    snap_vol1_uuid2 = create_snapshot(volume_uuid1)
    snap_vol2_uuid1 = create_snapshot(volume_uuid2)
    snap_vol2_uuid2 = create_snapshot(volume_uuid2)
    snap_vol2_uuid3 = create_snapshot(volume_uuid2)

    volumes = list_volumes(volume_uuid2)
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid1]
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid2]
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid3]

    volumes = list_volumes()
    assert volumes["Volumes"][volume_uuid1]["Snapshots"][snap_vol1_uuid1]
    assert volumes["Volumes"][volume_uuid1]["Snapshots"][snap_vol1_uuid2]
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid1]
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid2]
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid3]

    delete_snapshot(snap_vol1_uuid1, volume_uuid1)
    delete_snapshot(snap_vol1_uuid2, volume_uuid1)
    delete_snapshot(snap_vol2_uuid1, volume_uuid2)
    delete_snapshot(snap_vol2_uuid2, volume_uuid2)
    delete_snapshot(snap_vol2_uuid3, volume_uuid2)

    delete_volume(volume_uuid2)
    dm_cleanup_list.pop()

    delete_volume(volume_uuid1)
    dm_cleanup_list.pop()
