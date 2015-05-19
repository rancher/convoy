#!/usr/bin/python

import subprocess
import os
import json
import pytest
import uuid

from volmgr import VolumeManager

TEST_ROOT = "/tmp/volmgr_test/"
CFG_ROOT = os.path.join(TEST_ROOT, "volmgr")

BLOCKSTORE_ROOT = os.path.join(TEST_ROOT, "rancher-blockstore")
BLOCKSTORE_CFG = os.path.join(BLOCKSTORE_ROOT, "blockstore.cfg")
BLOCKSTORE_VOLUME_DIR = os.path.join(BLOCKSTORE_ROOT, "volumes")
BLOCKSTORE_PER_VOLUME_CFG = "volume.cfg"
BLOCKSTORE_SNAPSHOTS_DIR = "snapshots"
BLOCKSTORE_IMAGES_DIR = os.path.join(BLOCKSTORE_ROOT, "images")

DD_BLOCK_SIZE = 4096
POOL_NAME = "volmgr_test_pool"
VOLMGR_CMDLINE = ["../../bin/volmgr", "--debug", "--log=/tmp/volmgr.log",
    "--root=" + CFG_ROOT]

DATA_FILE = "data.vol"
METADATA_FILE = "metadata.vol"
IMAGE_FILE = "test.img"
DATA_DEVICE_SIZE = 1073618944
METADATA_DEVICE_SIZE = 40960000
DM_DIR = "/dev/mapper"
DM_BLOCK_SIZE = 2097152

VOLUME_SIZE_500M = 524288000
VOLUME_SIZE_100M = 104857600

data_dev = ""
metadata_dev = ""
image_file = ""

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

    global image_file
    image_file = os.path.join(TEST_ROOT, IMAGE_FILE)
    subprocess.check_call(["dd", "if=/dev/zero", "of=" + image_file,
            "bs=" + str(DD_BLOCK_SIZE), "count=" + str(VOLUME_SIZE_100M /
            DD_BLOCK_SIZE)])

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

    filenames = os.listdir(CFG_ROOT)
    for filename in filenames:
        assert not filename.startswith('volume')

def test_init():
    subprocess.check_call(VOLMGR_CMDLINE + ["init",
        "--images-dir", "/tmp/volmgr_images",
        "--driver=devicemapper",
        "--driver-opts", "dm.datadev=" + data_dev,
	"--driver-opts", "dm.metadatadev=" + metadata_dev,
	"--driver-opts", "dm.thinpoolname=" + POOL_NAME])
    dm_cleanup_list.append(POOL_NAME)

def test_info():
    data = subprocess.check_output(VOLMGR_CMDLINE + ["info"])
    info = json.loads(data)
    assert info["Driver"] == "devicemapper"
    assert info["Root"] == CFG_ROOT
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

    with pytest.raises(subprocess.CalledProcessError):
        uuid3 = v.create_volume_with_uuid(VOLUME_SIZE_100M, uuid1)

    specific_uuid = str(uuid.uuid1())

    uuid3 = v.create_volume_with_uuid(VOLUME_SIZE_100M, specific_uuid)
    dm_cleanup_list.append(uuid3)
    assert uuid3 == specific_uuid

    v.delete_volume(uuid3)
    dm_cleanup_list.remove(uuid3)

    v.delete_volume(uuid2)
    dm_cleanup_list.remove(uuid2)

    v.delete_volume(uuid1)
    dm_cleanup_list.remove(uuid1)

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
    volumes = v.list_volumes()
    assert len(volumes) == 0

    uuid1 = v.create_volume(VOLUME_SIZE_500M)
    dm_cleanup_list.append(uuid1)

    uuid2 = v.create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(uuid2)

    uuid3 = v.create_volume(VOLUME_SIZE_100M)
    dm_cleanup_list.append(uuid3)

    volumes = v.list_volumes(uuid3)
    assert volumes[uuid3]["Size"] == VOLUME_SIZE_100M

    volumes = v.list_volumes()
    assert volumes[uuid1]["Size"] == VOLUME_SIZE_500M
    assert volumes[uuid2]["Size"] == VOLUME_SIZE_100M
    assert volumes[uuid3]["Size"] == VOLUME_SIZE_100M

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

    snap0_vol1_uuid = str(uuid.uuid1())

    volumes = v.list_volumes(volume1_uuid, snap0_vol1_uuid)
    assert snap0_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]

    v.create_snapshot_with_uuid(volume1_uuid, snap0_vol1_uuid)

    volumes = v.list_volumes(volume1_uuid, snap0_vol1_uuid)
    assert snap0_vol1_uuid in volumes[volume1_uuid]["Snapshots"]

    snap1_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap2_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap1_vol2_uuid = v.create_snapshot(volume2_uuid)
    snap2_vol2_uuid = v.create_snapshot(volume2_uuid)
    snap3_vol2_uuid = v.create_snapshot(volume2_uuid)
    with pytest.raises(subprocess.CalledProcessError):
	v.create_snapshot_with_uuid(volume2_uuid, snap1_vol2_uuid)

    volumes = v.list_volumes(volume2_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap2_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap3_vol2_uuid in volumes[volume2_uuid]["Snapshots"]

    volumes = v.list_volumes()
    assert snap0_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap2_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap2_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap3_vol2_uuid in volumes[volume2_uuid]["Snapshots"]

    v.delete_snapshot(snap0_vol1_uuid, volume1_uuid)

    volumes = v.list_volumes(volume1_uuid, snap0_vol1_uuid)
    assert snap0_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]

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

    volumes = v.list_volume_blockstore(volume1_uuid, blockstore_uuid)
    assert len(volumes) == 0
    volumes = v.list_volume_blockstore_with_snapshot("random_string", volume1_uuid, blockstore_uuid)
    assert len(volumes) == 0

    v.add_volume_to_blockstore(volume1_uuid, blockstore_uuid)
    volume1_cfg_path = os.path.join(get_volume_dir(volume1_uuid), BLOCKSTORE_PER_VOLUME_CFG)
    assert os.path.exists(volume1_cfg_path)

    volumes = v.list_volume_blockstore_with_snapshot("random_string", volume1_uuid, blockstore_uuid)
    assert len(volumes) == 1
    assert volumes[volume1_uuid]["Size"] == VOLUME_SIZE_500M
    assert volumes[volume1_uuid]["Base"] == ""
    assert len(volumes[volume1_uuid]["Snapshots"]) == 0

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

    volumes = v.list_volume_blockstore_with_snapshot(snap1_vol1_uuid, volume1_uuid, blockstore_uuid)
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]

    snap1_vol2_uuid = v.create_snapshot(volume2_uuid)
    v.backup_snapshot_to_blockstore(snap1_vol2_uuid, volume2_uuid,
		    blockstore_uuid)
    with open(get_snapshot_cfg(snap1_vol2_uuid, volume2_uuid)) as f:
	snap1_vol2 = json.loads(f.read())
    assert snap1_vol2["ID"] == snap1_vol2_uuid
    assert len(snap1_vol2["Blocks"]) == 0

    #list snapshots
    volumes = v.list_volume_blockstore(volume1_uuid, blockstore_uuid)
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    volumes = v.list_volume_blockstore(volume2_uuid, blockstore_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    volumes = v.list_volume_blockstore_with_snapshot(snap1_vol2_uuid, volume2_uuid, blockstore_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]

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

    #dupcliate snapshot backup should fail
    with pytest.raises(subprocess.CalledProcessError):
        v.backup_snapshot_to_blockstore(snap2_vol2_uuid, volume2_uuid,
                blockstore_uuid)

    #list snapshots again
    volumes = v.list_volume_blockstore(volume1_uuid, blockstore_uuid)
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap2_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    volumes = v.list_volume_blockstore(volume2_uuid, blockstore_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap2_vol2_uuid in volumes[volume2_uuid]["Snapshots"]

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

    #list snapshots again
    volumes = v.list_volume_blockstore(volume1_uuid, blockstore_uuid)
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap2_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]
    volumes = v.list_volume_blockstore_with_snapshot(snap1_vol1_uuid, volume1_uuid, blockstore_uuid)
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    volumes = v.list_volume_blockstore_with_snapshot(snap2_vol1_uuid, volume1_uuid, blockstore_uuid)
    assert snap1_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]
    assert snap2_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]

    volumes = v.list_volume_blockstore(volume2_uuid, blockstore_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap2_vol2_uuid not in volumes[volume2_uuid]["Snapshots"]
    volumes = v.list_volume_blockstore_with_snapshot(snap1_vol2_uuid, volume2_uuid, blockstore_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    volumes = v.list_volume_blockstore_with_snapshot(snap2_vol2_uuid, volume2_uuid, blockstore_uuid)
    assert snap1_vol2_uuid not in volumes[volume2_uuid]["Snapshots"]
    assert snap2_vol2_uuid not in volumes[volume2_uuid]["Snapshots"]

    #remove volume from blockstore
    v.remove_volume_from_blockstore(volume1_uuid, blockstore_uuid)
    assert not os.path.exists(get_volume_cfg(volume1_uuid))

    v.remove_volume_from_blockstore(volume2_uuid, blockstore_uuid)
    assert not os.path.exists(get_volume_cfg(volume2_uuid))

    v.delete_snapshot(snap1_vol1_uuid, volume1_uuid)
    v.delete_snapshot(snap2_vol1_uuid, volume1_uuid)
    v.delete_snapshot(snap1_vol2_uuid, volume2_uuid)
    v.delete_snapshot(snap2_vol2_uuid, volume2_uuid)

    v.delete_volume(volume1_uuid)
    dm_cleanup_list.remove(volume1_uuid)

    v.delete_volume(volume2_uuid)
    dm_cleanup_list.remove(volume2_uuid)

    v.delete_volume(res_volume1_uuid)
    dm_cleanup_list.remove(res_volume1_uuid)

    v.delete_volume(res_volume2_uuid)
    dm_cleanup_list.remove(res_volume2_uuid)

def get_image_cfg(uuid):
    return os.path.join(BLOCKSTORE_IMAGES_DIR, uuid + ".json")

def get_image_gz(uuid):
    return os.path.join(BLOCKSTORE_IMAGES_DIR, uuid + ".img.gz")

def test_blockstore_image():
    #load blockstore from created one
    blockstore_uuid = v.register_vfs_blockstore(TEST_ROOT)

    global image_file
    image_uuid = v.add_image_to_blockstore(image_file, blockstore_uuid)

    assert os.path.exists(BLOCKSTORE_IMAGES_DIR)
    assert os.path.exists(get_image_cfg(image_uuid))
    assert os.path.exists(get_image_gz(image_uuid))

    v.remove_image_from_blockstore(image_uuid, blockstore_uuid)

    assert os.path.exists(BLOCKSTORE_IMAGES_DIR)
    assert not os.path.exists(get_image_cfg(image_uuid))
    assert not os.path.exists(get_image_gz(image_uuid))

