#!/usr/bin/python

import subprocess
import os
import json
import pytest
import uuid
import time

from volmgr import VolumeManager

TEST_ROOT = "/tmp/volmgr_test/"
CFG_ROOT = os.path.join(TEST_ROOT, "volmgr")
MOUNT_ROOT = os.path.join(TEST_ROOT, "mount")
AUTO_MOUNTS_DIR = os.path.join(TEST_ROOT, "auto_mounts")
IMAGES_DIR = os.path.join(TEST_ROOT, "images")
PID_FILE = os.path.join(TEST_ROOT, "volmgr.pid")
LOG_FILE= os.path.join(TEST_ROOT, "volmgr.log")
TEST_IMAGE_FILE = "image.test"
TEST_SNAPSHOT_FILE = "snapshot.test"

BLOCKSTORE_ROOT = os.path.join(TEST_ROOT, "rancher-blockstore")
BLOCKSTORE_CFG = os.path.join(BLOCKSTORE_ROOT, "blockstore.cfg")
BLOCKSTORE_VOLUME_DIR = os.path.join(BLOCKSTORE_ROOT, "volumes")
BLOCKSTORE_PER_VOLUME_CFG = "volume.cfg"
BLOCKSTORE_SNAPSHOTS_DIR = "snapshots"
BLOCKSTORE_IMAGES_DIR = os.path.join(BLOCKSTORE_ROOT, "images")

DD_BLOCK_SIZE = 4096
POOL_NAME = "volmgr_test_pool"
VOLMGR_BINARY = os.path.abspath("../../bin/volmgr")

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

def create_empty_file(filepath, size):
    subprocess.check_call(["dd", "if=/dev/zero", "of=" + filepath,
            "bs=" + str(DD_BLOCK_SIZE), "count=" + str(size /
            DD_BLOCK_SIZE)])
    assert os.path.exists(filepath)

def attach_loopback_dev(filepath):
    dev = subprocess.check_output(["losetup", "-v", "-f",
            filepath]).strip().split(" ")[3]
    assert dev.startswith("/dev/loop")
    return dev

def detach_loopback_dev(dev):
    subprocess.check_output(["losetup", "-d", dev])

def format_dev(dev):
    subprocess.check_call(["mkfs", "-t", "ext4", dev])

def mount_dev(dev, mountpoint):
    subprocess.check_call(["mount", dev, mountpoint])
    mount_cleanup_list.append(mountpoint)

def umount_dev(mountpoint):
    subprocess.check_call(["umount", mountpoint])
    mount_cleanup_list.remove(mountpoint)

def setup_module():
    if os.path.exists(TEST_ROOT):
	subprocess.check_call(["rm", "-rf", TEST_ROOT])

    os.makedirs(TEST_ROOT)
    assert os.path.exists(TEST_ROOT)

    os.makedirs(MOUNT_ROOT)
    assert os.path.exists(MOUNT_ROOT)

    data_file = os.path.join(TEST_ROOT, DATA_FILE)
    create_empty_file(data_file, DATA_DEVICE_SIZE)
    global data_dev
    data_dev = attach_loopback_dev(data_file)

    metadata_file = os.path.join(TEST_ROOT, METADATA_FILE)
    create_empty_file(metadata_file, METADATA_DEVICE_SIZE)
    global metadata_dev
    metadata_dev = attach_loopback_dev(metadata_file)

    global image_file
    image_file = os.path.join(TEST_ROOT, IMAGE_FILE)
    create_empty_file(image_file, VOLUME_SIZE_100M)

    image_dev = attach_loopback_dev(image_file)
    format_dev(image_dev)
    mount_dev(image_dev, MOUNT_ROOT)
    subprocess.check_call(["touch", os.path.join(MOUNT_ROOT, TEST_IMAGE_FILE)])
    umount_dev(MOUNT_ROOT)
    detach_loopback_dev(image_dev)

    global v
    v = VolumeManager(VOLMGR_BINARY, TEST_ROOT)
    v.start_server(PID_FILE, ["server",
        "--root", CFG_ROOT,
        "--log", LOG_FILE,
        "--images-dir", IMAGES_DIR,
        "--mounts-dir", AUTO_MOUNTS_DIR,
        "--driver=devicemapper",
        "--driver-opts", "dm.datadev=" + data_dev,
	"--driver-opts", "dm.metadatadev=" + metadata_dev,
	"--driver-opts", "dm.thinpoolname=" + POOL_NAME])
    dm_cleanup_list.append(POOL_NAME)
    wait_for_daemon()

def detach_all_lodev(keyword):
    output = subprocess.check_output(["losetup", "-a"])
    lines = output.splitlines()
    for line in lines:
        if line.find(keyword) != -1:
            detach_loopback_dev(line.split(":")[0].strip())

def teardown_module():
    v.stop_server(PID_FILE)

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

    detach_all_lodev(TEST_ROOT)

    filenames = os.listdir(CFG_ROOT)
    for filename in filenames:
        assert not filename.startswith('volume')

def wait_for_daemon():
    while True:
        try:
                data = v.server_info()
                break
        except subprocess.CalledProcessError:
                print "Fail to communicate with daemon, retrying"
                time.sleep(1)

    info = json.loads(data)
    assert info["General"]["Driver"] == "devicemapper"
    assert info["General"]["Root"] == CFG_ROOT
    assert info["General"]["ImagesDir"]== IMAGES_DIR 
    assert info["General"]["MountsDir"]== AUTO_MOUNTS_DIR 
    assert info["Driver"]["Driver"] == "devicemapper"
    assert info["Driver"]["Root"] == CFG_ROOT
    assert info["Driver"]["DataDevice"] == data_dev
    assert info["Driver"]["MetadataDevice"] == metadata_dev
    assert info["Driver"]["ThinpoolDevice"] == os.path.join(DM_DIR, POOL_NAME)
    assert info["Driver"]["ThinpoolSize"] == DATA_DEVICE_SIZE
    assert info["Driver"]["ThinpoolBlockSize"] == DM_BLOCK_SIZE

def create_volume(size, uuid = "", base = ""):
    uuid = v.create_volume(size, uuid, base)
    dm_cleanup_list.append(uuid)
    return uuid

def delete_volume(uuid):
    v.delete_volume(uuid)
    dm_cleanup_list.remove(uuid)

def mount_volume(uuid, need_format):
    mount_dir = v.mount_volume(uuid, need_format)
    mount_cleanup_list.append(mount_dir)
    return mount_dir

def mount_volume_auto(uuid, need_format):
    mount_dir = v.mount_volume_auto(uuid, need_format)
    mount_cleanup_list.append(mount_dir)
    return mount_dir

def umount_volume(uuid, mount_dir):
    v.umount_volume(uuid)
    mount_cleanup_list.remove(mount_dir)

def test_volume_cru():
    uuid1 = create_volume(VOLUME_SIZE_500M)
    uuid2 = create_volume(VOLUME_SIZE_100M)

    with pytest.raises(subprocess.CalledProcessError):
        uuid3 = create_volume(VOLUME_SIZE_100M, uuid1)

    specific_uuid = str(uuid.uuid1())

    uuid3 = create_volume(VOLUME_SIZE_100M, specific_uuid)
    assert uuid3 == specific_uuid

    delete_volume(uuid3)
    delete_volume(uuid2)
    delete_volume(uuid1)

def format_volume_and_create_file(uuid, filename):
    # with format
    volume_mount_dir = mount_volume(uuid, True)

    test_file = os.path.join(volume_mount_dir,filename)
    with open(test_file, "w") as f:
	subprocess.check_call(["echo", "This is volume test file"], stdout=f)
    assert os.path.exists(test_file)

    umount_volume(uuid, volume_mount_dir)
    assert not os.path.exists(test_file)

def test_volume_mount():
    uuid = create_volume(VOLUME_SIZE_500M)

    # with format
    filename = "test"
    format_volume_and_create_file(uuid, filename)

    # without format
    volume_mount_dir = mount_volume(uuid, False)
    test_file = os.path.join(volume_mount_dir, filename)
    assert os.path.exists(test_file)

    umount_volume(uuid, volume_mount_dir)
    assert not os.path.exists(test_file)

    # auto mount 
    volume_mount_dir = mount_volume_auto(uuid, False)
    test_file = os.path.join(volume_mount_dir, filename)
    assert os.path.exists(test_file)

    umount_volume(uuid, volume_mount_dir)
    assert not os.path.exists(test_file)

    delete_volume(uuid)

def test_volume_list():
    volumes = v.list_volumes()
    assert len(volumes) == 0

    uuid1 = create_volume(VOLUME_SIZE_500M)
    uuid2 = create_volume(VOLUME_SIZE_100M)
    uuid3 = create_volume(VOLUME_SIZE_100M)

    volumes = v.list_volumes(uuid3)
    assert volumes[uuid3]["Size"] == VOLUME_SIZE_100M

    volumes = v.list_volumes()
    assert volumes[uuid1]["Size"] == VOLUME_SIZE_500M
    assert volumes[uuid2]["Size"] == VOLUME_SIZE_100M
    assert volumes[uuid3]["Size"] == VOLUME_SIZE_100M

    delete_volume(uuid3)
    delete_volume(uuid2)
    delete_volume(uuid1)

def test_snapshot_cru():
    volume_uuid = create_volume(VOLUME_SIZE_500M)

    snapshot_uuid = v.create_snapshot(volume_uuid)
    v.delete_snapshot(snapshot_uuid, volume_uuid)

    delete_volume(volume_uuid)

def test_snapshot_list():
    volume1_uuid = create_volume(VOLUME_SIZE_500M)
    volume2_uuid = create_volume(VOLUME_SIZE_100M)

    snap0_vol1_uuid = str(uuid.uuid1())

    volumes = v.list_volumes(volume1_uuid, snap0_vol1_uuid)
    assert snap0_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]

    v.create_snapshot(volume1_uuid, snap0_vol1_uuid)

    volumes = v.list_volumes(volume1_uuid, snap0_vol1_uuid)
    assert snap0_vol1_uuid in volumes[volume1_uuid]["Snapshots"]

    snap1_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap2_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap1_vol2_uuid = v.create_snapshot(volume2_uuid)
    snap2_vol2_uuid = v.create_snapshot(volume2_uuid)
    snap3_vol2_uuid = v.create_snapshot(volume2_uuid)
    with pytest.raises(subprocess.CalledProcessError):
	v.create_snapshot(volume2_uuid, snap1_vol2_uuid)

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

    delete_volume(volume2_uuid)
    delete_volume(volume1_uuid)

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

    with open(BLOCKSTORE_CFG) as f:
	bs = json.loads(f.read())
    assert bs["UUID"] == uuid
    assert bs["Kind"] == "vfs"

    v.deregister_blockstore(uuid)

    #load blockstore from created one
    blockstore_uuid = v.register_vfs_blockstore(TEST_ROOT)
    assert uuid == blockstore_uuid

    #add volume to blockstore
    volume1_uuid = create_volume(VOLUME_SIZE_500M)

    volume2_uuid = create_volume(VOLUME_SIZE_100M)

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
    res_volume1_uuid = create_volume(VOLUME_SIZE_500M)
    v.restore_snapshot_from_blockstore(snap2_vol1_uuid, volume1_uuid,
		    res_volume1_uuid, blockstore_uuid)
    res_volume1_checksum = get_checksum(os.path.join(DM_DIR, res_volume1_uuid))
    volume1_checksum = get_checksum(os.path.join(DM_DIR, volume1_uuid))
    assert res_volume1_checksum == volume1_checksum

    res_volume2_uuid = create_volume(VOLUME_SIZE_100M)
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

    delete_volume(volume1_uuid)
    delete_volume(volume2_uuid)
    delete_volume(res_volume1_uuid)
    delete_volume(res_volume2_uuid)

def get_image_cfg(uuid):
    return os.path.join(BLOCKSTORE_IMAGES_DIR, uuid + ".json")

def get_image_gz(uuid):
    return os.path.join(BLOCKSTORE_IMAGES_DIR, uuid + ".img.gz")

def get_local_image(uuid):
    return os.path.join(IMAGES_DIR, uuid + ".img")

def test_blockstore_image():
    #load blockstore from created one
    blockstore_uuid = v.register_vfs_blockstore(TEST_ROOT)

    #add/remove image
    global image_file
    image_uuid = v.add_image_to_blockstore(image_file, blockstore_uuid)

    assert os.path.exists(BLOCKSTORE_IMAGES_DIR)
    assert os.path.exists(get_image_cfg(image_uuid))
    assert os.path.exists(get_image_gz(image_uuid))

    v.remove_image_from_blockstore(image_uuid, blockstore_uuid)

    assert not os.path.exists(get_image_cfg(image_uuid))
    assert not os.path.exists(get_image_gz(image_uuid))

    #activate/deactivate image
    image_uuid = v.add_image_to_blockstore(image_file, blockstore_uuid)

    #compressed image cache
    assert os.path.exists(get_local_image(image_uuid)+".gz")

    v.activate_image(image_uuid, blockstore_uuid)
    assert os.path.exists(get_local_image(image_uuid))
    v.deactivate_image(image_uuid, blockstore_uuid)
    assert not os.path.exists(get_local_image(image_uuid))

    #raw image cache
    subprocess.check_call(["cp", image_file, get_local_image(image_uuid)])
    assert os.path.exists(get_local_image(image_uuid))

    v.activate_image(image_uuid, blockstore_uuid)
    assert os.path.exists(get_local_image(image_uuid))
    v.deactivate_image(image_uuid, blockstore_uuid)
    assert not os.path.exists(get_local_image(image_uuid))

    #deactivate would remove the local copy, so this time it would trigger
    # downloading from blockstore
    v.activate_image(image_uuid, blockstore_uuid)
    assert os.path.exists(get_local_image(image_uuid))
    v.deactivate_image(image_uuid, blockstore_uuid)
    assert not os.path.exists(get_local_image(image_uuid))

    v.remove_image_from_blockstore(image_uuid, blockstore_uuid)

def test_image_based_volume():
    #load blockstore from created one
    blockstore_uuid = v.register_vfs_blockstore(TEST_ROOT)

    #add/remove image
    global image_file
    image_uuid = v.add_image_to_blockstore(image_file, blockstore_uuid)

    v.activate_image(image_uuid, blockstore_uuid)

    volume_uuid = create_volume(VOLUME_SIZE_100M, base=image_uuid)

    volume_mount_dir = mount_volume(volume_uuid, False)

    assert os.path.exists(os.path.join(volume_mount_dir, TEST_IMAGE_FILE))
    subprocess.check_call(["touch", os.path.join(volume_mount_dir,
	    TEST_SNAPSHOT_FILE)])
    umount_volume(volume_uuid, volume_mount_dir)

    snapshot_uuid = v.create_snapshot(volume_uuid)
    v.add_volume_to_blockstore(volume_uuid, blockstore_uuid)
    v.backup_snapshot_to_blockstore(snapshot_uuid, volume_uuid, blockstore_uuid)

    new_volume_uuid = create_volume(VOLUME_SIZE_100M, base=image_uuid)

    v.restore_snapshot_from_blockstore(snapshot_uuid, volume_uuid,
            new_volume_uuid, blockstore_uuid)

    new_volume_mount_dir = mount_volume(new_volume_uuid, False)

    assert os.path.exists(os.path.join(new_volume_mount_dir, TEST_IMAGE_FILE))
    assert os.path.exists(os.path.join(new_volume_mount_dir, TEST_SNAPSHOT_FILE))

    umount_volume(new_volume_uuid, new_volume_mount_dir)

    v.remove_snapshot_from_blockstore(snapshot_uuid, volume_uuid,
            blockstore_uuid)
    v.delete_snapshot(snapshot_uuid, volume_uuid)

    v.remove_volume_from_blockstore(volume_uuid, blockstore_uuid)
    delete_volume(volume_uuid)

    delete_volume(new_volume_uuid)

    v.deactivate_image(image_uuid, blockstore_uuid)
    v.remove_image_from_blockstore(image_uuid, blockstore_uuid)

