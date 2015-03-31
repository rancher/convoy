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
    if os.path.exists(TEST_ROOT):
	subprocess.check_call(["rm", "-rf", TEST_ROOT])

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
    while mount_cleanup_list:
	code = subprocess.call(["umount", mount_cleanup_list.pop()])
        if code != 0:
            print "Something wrong when tearing down, error {}, continuing", code

    while dm_cleanup_list:
	code = subprocess.call(["dmsetup", "remove", dm_cleanup_list.pop()])
        if code != 0:
            print "Something wrong when tearing down, error {}, continuing", code

    code = subprocess.call(["losetup", "-d", data_dev, metadata_dev])
    if code != 0:
        print "Something wrong when tearing down, error {}, continuing", code

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
    dm_cleanup_list.remove(uuid1)

    v.delete_volume(uuid1)
    dm_cleanup_list.remove(uuid2)

def format_volume_and_create_file(uuid, filename):
    # with format
    volume_mount_dir = v.mount_volume(uuid, True)
    mount_cleanup_list.append(volume_mount_dir)

    test_file = os.path.join(volume_mount_dir,filename)
    with open(test_file, "w") as f:
	subprocess.check_call(["echo", "This is volume test file"], stdout=f)
    assert os.path.exists(test_file)

    v.umount_volume(uuid)
    mount_cleanup_list.remove(volume_mount_dir)
    assert not os.path.exists(test_file)

def test_volume_mount():
    uuid = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(uuid)

    # with format
    filename = "test"
    format_volume_and_create_file(uuid, filename)

    # without format
    volume_mount_dir = v.mount_volume(uuid, False)
    mount_cleanup_list.append(volume_mount_dir)
    test_file = os.path.join(volume_mount_dir, filename)
    assert os.path.exists(test_file)

    v.umount_volume(uuid)
    mount_cleanup_list.remove(volume_mount_dir)
    assert not os.path.exists(test_file)

    v.delete_volume(uuid)
    dm_cleanup_list.remove(uuid)

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
    dm_cleanup_list.remove(uuid3)

    v.delete_volume(uuid2)
    dm_cleanup_list.remove(uuid2)

    v.delete_volume(uuid1)
    dm_cleanup_list.remove(uuid1)

def test_snapshot_cru():
    volume_uuid = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(volume_uuid)

    snapshot_uuid = v.create_snapshot(volume_uuid)
    v.delete_snapshot(snapshot_uuid, volume_uuid)

    v.delete_volume(volume_uuid)
    dm_cleanup_list.remove(volume_uuid)

def test_snapshot_list():
    volume1_uuid = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(volume1_uuid)

    volume2_uuid = v.create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(volume2_uuid)

    snap1_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap2_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap1_vol2_uuid = v.create_snapshot(volume2_uuid)
    snap2_vol2_uuid = v.create_snapshot(volume2_uuid)
    snap3_vol2_uuid = v.create_snapshot(volume2_uuid)

    volumes = v.list_volumes(volume2_uuid)
    assert volumes["Volumes"][volume2_uuid]["Snapshots"][snap1_vol2_uuid]
    assert volumes["Volumes"][volume2_uuid]["Snapshots"][snap2_vol2_uuid]
    assert volumes["Volumes"][volume2_uuid]["Snapshots"][snap3_vol2_uuid]

    volumes = v.list_volumes()
    assert volumes["Volumes"][volume1_uuid]["Snapshots"][snap1_vol1_uuid]
    assert volumes["Volumes"][volume1_uuid]["Snapshots"][snap2_vol1_uuid]
    assert volumes["Volumes"][volume2_uuid]["Snapshots"][snap1_vol2_uuid]
    assert volumes["Volumes"][volume2_uuid]["Snapshots"][snap2_vol2_uuid]
    assert volumes["Volumes"][volume2_uuid]["Snapshots"][snap3_vol2_uuid]

    v.delete_snapshot(snap1_vol1_uuid, volume1_uuid)
    v.delete_snapshot(snap2_vol1_uuid, volume1_uuid)
    v.delete_snapshot(snap1_vol2_uuid, volume2_uuid)
    v.delete_snapshot(snap2_vol2_uuid, volume2_uuid)
    v.delete_snapshot(snap3_vol2_uuid, volume2_uuid)

    v.delete_volume(volume2_uuid)
    dm_cleanup_list.remove(volume2_uuid)

    v.delete_volume(volume1_uuid)
    dm_cleanup_list.remove(volume1_uuid)

def get_volume_dir(uuid):
    return os.path.join(BLOCKSTORE_VOLUME_DIR, uuid[:2], uuid[2:4], uuid)

def get_volume_cfg(uuid):
    return os.path.join(get_volume_dir(uuid), BLOCKSTORE_PER_VOLUME_CFG)

def get_snapshot_dir(snapshot_uuid, volume_uuid):
    return os.path.join(get_volume_dir(volume_uuid), BLOCKSTORE_SNAPSHOTS_DIR)

def get_snapshot_cfg(snapshot_uuid, volume_uuid):
    return  os.path.join(get_snapshot_dir(snapshot_uuid, volume_uuid),
            "snapshot_" + snapshot_uuid +".cfg")

def get_checksum(filename):
    output = subprocess.check_output(["sha512sum", filename]).decode()
    return output.split(" ")[0]

def test_blockstore():
    #create blockstore
    uuid = v.register_vfs_blockstore(TEST_ROOT)

    assert os.path.exists(BLOCKSTORE_ROOT)
    assert os.path.exists(BLOCKSTORE_CFG)
    assert os.path.exists(BLOCKSTORE_VOLUME_DIR)

    with open(BLOCKSTORE_CFG) as f:
	bs = json.loads(f.read())
    assert bs["UUID"] == uuid
    assert bs["Kind"] == "vfs"

    v.deregister_blockstore(uuid)

    #load blockstore from created one
    blockstore_uuid = v.register_vfs_blockstore(TEST_ROOT)
    assert uuid == blockstore_uuid

    #add volume to blockstore
    volume1_uuid = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(volume1_uuid)

    volume2_uuid = v.create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(volume2_uuid)

    v.add_volume_to_blockstore(volume1_uuid, blockstore_uuid)
    volume1_cfg_path = os.path.join(get_volume_dir(volume1_uuid), BLOCKSTORE_PER_VOLUME_CFG)
    assert os.path.exists(volume1_cfg_path)

    v.add_volume_to_blockstore(volume2_uuid, uuid)
    volume2_cfg_path = os.path.join(get_volume_dir(volume2_uuid), BLOCKSTORE_PER_VOLUME_CFG)
    assert os.path.exists(volume2_cfg_path)

    #first snapshots
    snap1_vol1_uuid = v.create_snapshot(volume1_uuid)
    v.backup_snapshot_to_blockstore(snap1_vol1_uuid, volume1_uuid,
		    blockstore_uuid)
    with open(get_snapshot_cfg(snap1_vol1_uuid, volume1_uuid)) as f:
	snap1_vol1 = json.loads(f.read())
    assert snap1_vol1["ID"] == snap1_vol1_uuid
    assert len(snap1_vol1["Blocks"]) == 0

    snap1_vol2_uuid = v.create_snapshot(volume2_uuid)
    v.backup_snapshot_to_blockstore(snap1_vol2_uuid, volume2_uuid,
		    blockstore_uuid)
    with open(get_snapshot_cfg(snap1_vol2_uuid, volume2_uuid)) as f:
	snap1_vol2 = json.loads(f.read())
    assert snap1_vol2["ID"] == snap1_vol2_uuid
    assert len(snap1_vol2["Blocks"]) == 0

    #second snapshots
    format_volume_and_create_file(volume1_uuid, "test-vol1-v1")
    snap2_vol1_uuid = v.create_snapshot(volume1_uuid)
    v.backup_snapshot_to_blockstore(snap2_vol1_uuid, volume1_uuid,
		    blockstore_uuid)
    with open(get_snapshot_cfg(snap2_vol1_uuid, volume1_uuid)) as f:
	snap2_vol1 = json.loads(f.read())
    assert snap2_vol1["ID"] == snap2_vol1_uuid
    assert len(snap2_vol1["Blocks"]) != 0

    format_volume_and_create_file(volume2_uuid, "test-vol2-v2")
    snap2_vol2_uuid = v.create_snapshot(volume2_uuid)
    v.backup_snapshot_to_blockstore(snap2_vol2_uuid, volume2_uuid,
		    blockstore_uuid)
    with open(get_snapshot_cfg(snap2_vol2_uuid, volume2_uuid)) as f:
	snap2_vol2 = json.loads(f.read())
    assert snap2_vol2["ID"] == snap2_vol2_uuid
    assert len(snap2_vol2["Blocks"]) != 0

    #restore snapshot
    res_volume1_uuid = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(res_volume1_uuid)
    v.restore_snapshot_from_blockstore(snap2_vol1_uuid, volume1_uuid,
		    res_volume1_uuid, blockstore_uuid)
    res_volume1_checksum = get_checksum(os.path.join(DM_DIR, res_volume1_uuid))
    volume1_checksum = get_checksum(os.path.join(DM_DIR, volume1_uuid))
    assert res_volume1_checksum == volume1_checksum

    res_volume2_uuid = v.create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(res_volume2_uuid)
    v.restore_snapshot_from_blockstore(snap2_vol2_uuid, volume2_uuid,
		    res_volume2_uuid, blockstore_uuid)
    res_volume2_checksum = get_checksum(os.path.join(DM_DIR, res_volume2_uuid))
    volume2_checksum = get_checksum(os.path.join(DM_DIR, volume2_uuid))
    assert res_volume2_checksum == volume2_checksum

    #remove snapshots from blockstore
    v.remove_snapshot_from_blockstore(snap2_vol1_uuid, volume1_uuid, blockstore_uuid)
    assert not os.path.exists(get_snapshot_cfg(snap2_vol1_uuid, volume1_uuid))

    v.remove_snapshot_from_blockstore(snap2_vol2_uuid, volume2_uuid, blockstore_uuid)
    assert not os.path.exists(get_snapshot_cfg(snap2_vol2_uuid, volume2_uuid))

    #remove volume from blockstore
    v.remove_volume_from_blockstore(volume1_uuid, blockstore_uuid)
    assert not os.path.exists(get_volume_cfg(volume1_uuid))

    v.remove_volume_from_blockstore(volume2_uuid, blockstore_uuid)
    assert not os.path.exists(get_volume_cfg(volume2_uuid))
