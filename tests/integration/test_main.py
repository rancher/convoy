#!/usr/bin/python

import subprocess
import os
import json
import pytest

from volmgr import VolumeManager

TEST_ROOT = "/tmp/volmgr_test/"
CFG_ROOT = os.path.join(TEST_ROOT, "volmgr")

BLOCKSTORE_ROOT = os.path.join(TEST_ROOT, "rancher-blockstore")
BLOCKSTORE_CFG = os.path.join(BLOCKSTORE_ROOT, "blockstore.cfg")
BLOCKSTORE_VOLUME_DIR = os.path.join(BLOCKSTORE_ROOT, "volumes")
BLOCKSTORE_PER_VOLUME_CFG = "volume.cfg"
BLOCKSTORE_SNAPSHOTS_DIR = "snapshots"

DD_BLOCK_SIZE = 4096
POOL_NAME = "volmgr_test_pool"
VOLMGR_CMDLINE = ["../../volmgr", "--debug", "--log=/tmp/volmgr.log",
    "--root=" + CFG_ROOT]

DATA_FILE = "data.vol"
METADATA_FILE = "metadata.vol"
DATA_DEVICE_SIZE = 1073618944
METADATA_DEVICE_SIZE = 40960000
DM_DIR = "/dev/mapper"
DM_BLOCK_SIZE = 2097152

VOLUME_SIZE_500M = 524288000
VOLUME_SIZE_100M = 104857600

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

    global v
    v = VolumeManager(VOLMGR_CMDLINE, TEST_ROOT)

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

def test_volume_cru():
    uuid1 = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(uuid1)
    uuid2 = v.create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(uuid2)

    v.delete_volume(uuid2)
    dm_cleanup_list.pop()

    v.delete_volume(uuid1)
    dm_cleanup_list.pop()

def test_volume_mount():
    uuid = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(uuid)

    # with format
    volume_mount_dir = v.mount_volume(uuid, True)
    mount_cleanup_list.append(volume_mount_dir)

    test_file = os.path.join(volume_mount_dir, "test")
    f = open(test_file, "w")
    subprocess.check_call(["echo", "This is volume test file"], stdout=f)
    f.close()
    assert os.path.exists(test_file)

    v.umount_volume(uuid)
    mount_cleanup_list.pop()
    assert not os.path.exists(test_file)

    # without format
    volume_mount_dir = v.mount_volume(uuid, False)
    mount_cleanup_list.append(volume_mount_dir)
    assert os.path.exists(test_file)

    v.umount_volume(uuid)
    mount_cleanup_list.pop()
    assert not os.path.exists(test_file)

    v.delete_volume(uuid)
    dm_cleanup_list.pop()

def test_volume_list():
    uuid1 = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(uuid1)

    uuid2 = v.create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(uuid2)

    uuid3 = v.create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(uuid3)

    volumes = v.list_volumes(uuid3)
    assert volumes["Volumes"][uuid3]["Size"] == VOLUME_SIZE_100M

    volumes = v.list_volumes()
    assert volumes["Volumes"][uuid1]["Size"] == VOLUME_SIZE_500M
    assert volumes["Volumes"][uuid2]["Size"] == VOLUME_SIZE_100M
    assert volumes["Volumes"][uuid3]["Size"] == VOLUME_SIZE_100M

    v.delete_volume(uuid3)
    dm_cleanup_list.pop()

    v.delete_volume(uuid2)
    dm_cleanup_list.pop()

    v.delete_volume(uuid1)
    dm_cleanup_list.pop()

def test_snapshot_cru():
    volume_uuid = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(volume_uuid)

    snapshot_uuid = v.create_snapshot(volume_uuid)
    v.delete_snapshot(snapshot_uuid, volume_uuid)

    v.delete_volume(volume_uuid)
    dm_cleanup_list.pop()

def test_snapshot_list():
    volume_uuid1 = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(volume_uuid1)

    volume_uuid2 = v.create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(volume_uuid2)

    snap_vol1_uuid1 = v.create_snapshot(volume_uuid1)
    snap_vol1_uuid2 = v.create_snapshot(volume_uuid1)
    snap_vol2_uuid1 = v.create_snapshot(volume_uuid2)
    snap_vol2_uuid2 = v.create_snapshot(volume_uuid2)
    snap_vol2_uuid3 = v.create_snapshot(volume_uuid2)

    volumes = v.list_volumes(volume_uuid2)
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid1]
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid2]
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid3]

    volumes = v.list_volumes()
    assert volumes["Volumes"][volume_uuid1]["Snapshots"][snap_vol1_uuid1]
    assert volumes["Volumes"][volume_uuid1]["Snapshots"][snap_vol1_uuid2]
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid1]
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid2]
    assert volumes["Volumes"][volume_uuid2]["Snapshots"][snap_vol2_uuid3]

    v.delete_snapshot(snap_vol1_uuid1, volume_uuid1)
    v.delete_snapshot(snap_vol1_uuid2, volume_uuid1)
    v.delete_snapshot(snap_vol2_uuid1, volume_uuid2)
    v.delete_snapshot(snap_vol2_uuid2, volume_uuid2)
    v.delete_snapshot(snap_vol2_uuid3, volume_uuid2)

    v.delete_volume(volume_uuid2)
    dm_cleanup_list.pop()

    v.delete_volume(volume_uuid1)
    dm_cleanup_list.pop()

def test_blockstore():
    uuid = v.register_vfs_blockstore(TEST_ROOT)

    os.path.exists(BLOCKSTORE_ROOT)
    os.path.exists(BLOCKSTORE_CFG)
    os.path.exists(BLOCKSTORE_VOLUME_DIR)

    bs = json.loads(open(BLOCKSTORE_CFG).read())
    assert bs["UUID"] == uuid
    assert bs["Kind"] == "vfs"

    v.deregister_blockstore(uuid)
