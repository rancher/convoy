#!/usr/bin/python

import subprocess
import os
import json
import pytest
import uuid
import time
import sys
import threading

from rancher_volume import VolumeManager

TEST_ROOT = "/tmp/rancher-volume_test/"
CFG_ROOT = os.path.join(TEST_ROOT, "rancher-volume")
MOUNT_ROOT = os.path.join(TEST_ROOT, "mount")
AUTO_MOUNTS_DIR = os.path.join(TEST_ROOT, "auto_mounts")
IMAGES_DIR = os.path.join(TEST_ROOT, "images")
PID_FILE = os.path.join(TEST_ROOT, "rancher-volume.pid")
LOG_FILE= os.path.join(TEST_ROOT, "rancher-volume.log")
TEST_IMAGE_FILE = "image.test"
TEST_SNAPSHOT_FILE = "snapshot.test"

TEST_THREAD_COUNT = 100

OBJECTSTORE_ROOT = os.path.join(TEST_ROOT, "rancher-objectstore")
OBJECTSTORE_CFG = os.path.join(OBJECTSTORE_ROOT, "objectstore.cfg")
OBJECTSTORE_VOLUME_DIR = os.path.join(OBJECTSTORE_ROOT, "volumes")
OBJECTSTORE_PER_VOLUME_CFG = "volume.cfg"
OBJECTSTORE_SNAPSHOTS_DIR = "snapshots"
OBJECTSTORE_IMAGES_DIR = os.path.join(OBJECTSTORE_ROOT, "images")

OBJECTSTORE_PATH1 = os.path.join(TEST_ROOT, "store1")
OBJECTSTORE_PATH2 = os.path.join(TEST_ROOT, "store2")
OBJECTSTORE_PATH3 = os.path.join(TEST_ROOT, "store3")

ENV_TEST_AWS_ACCESS_KEY = "RANCHER_TEST_AWS_ACCESS_KEY_ID"
ENV_TEST_AWS_SECRET_KEY = "RANCHER_TEST_AWS_SECRET_ACCESS_KEY"
ENV_TEST_AWS_REGION     = "RANCHER_TEST_AWS_REGION"
ENV_TEST_AWS_BUCKET     = "RANCHER_TEST_AWS_BUCKET"
S3_PATH = "test/volume/"

DD_BLOCK_SIZE = 4096
POOL_NAME = "rancher_volume_test_pool"
RANCHER_VOLUME_BINARY = os.path.abspath("../../bin/rancher-volume")

DATA_FILE = "data.vol"
METADATA_FILE = "metadata.vol"
IMAGE_FILE = "test.img"
DATA_DEVICE_SIZE = 1073618944
METADATA_DEVICE_SIZE = 40960000
DM_DIR = "/dev/mapper"
DM_BLOCK_SIZE = 2097152
EMPTY_FILE_SIZE = 104857600

DEFAULT_VOLUME_SIZE = "1073741824"
VOLUME_SIZE_500M_Bytes = "524288000"
VOLUME_SIZE_500M = "500M"
VOLUME_SIZE_100M = "104857600"

RANDOM_VALID_UUID = "0bd0bc5f-f3ad-4e1b-9283-98adb3ef38f4"

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
    create_empty_file(image_file, EMPTY_FILE_SIZE)

    image_dev = attach_loopback_dev(image_file)
    format_dev(image_dev)
    mount_dev(image_dev, MOUNT_ROOT)
    subprocess.check_call(["touch", os.path.join(MOUNT_ROOT, TEST_IMAGE_FILE)])
    umount_dev(MOUNT_ROOT)
    detach_loopback_dev(image_dev)

    global v
    v = VolumeManager(RANCHER_VOLUME_BINARY, TEST_ROOT)
    v.start_server(PID_FILE, ["server",
        "--root", CFG_ROOT,
        "--log", LOG_FILE,
        "--images-dir", IMAGES_DIR,
        "--mounts-dir", AUTO_MOUNTS_DIR,
        "--default-volume-size", DEFAULT_VOLUME_SIZE,
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
    code = v.stop_server(PID_FILE)
    if code != 0:
        print "Something wrong when tearing down, continuing with code ", code

    while mount_cleanup_list:
	code = subprocess.call(["umount", mount_cleanup_list.pop()])
        if code != 0:
            print "Something wrong when tearing down, continuing with code", code

    while dm_cleanup_list:
	code = subprocess.call(["dmsetup", "remove", dm_cleanup_list.pop()])
        if code != 0:
            print "Something wrong when tearing down, continuing with code ", code

    code = subprocess.call(["losetup", "-d", data_dev, metadata_dev])
    if code != 0:
        print "Something wrong when tearing down, continuing with code", code

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
                print "Fail to communicate with daemon"
                if v.check_server(PID_FILE) != 0:
                    print "Server failed to start"
                    sys.exit(1)
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

@pytest.yield_fixture(autouse=True)
def check_test():
    yield
    filenames = os.listdir(CFG_ROOT)
    for filename in filenames:
        assert not filename.startswith('volume')

def create_volume(size = "", base = "", name = ""):
    uuid = v.create_volume(size, base, name)
    dm_cleanup_list.append(uuid)
    return uuid

def delete_volume(uuid, name = ""):
    if name == "":
        v.delete_volume(uuid)
    else:
        v.delete_volume(name)
    dm_cleanup_list.remove(uuid)

def mount_volume(uuid):
    mount_dir = v.mount_volume(uuid)
    mount_cleanup_list.append(mount_dir)
    return mount_dir

def mount_volume_auto(uuid):
    mount_dir = v.mount_volume_auto(uuid)
    mount_cleanup_list.append(mount_dir)
    return mount_dir

def umount_volume(uuid, mount_dir):
    v.umount_volume(uuid)
    mount_cleanup_list.remove(mount_dir)

def test_volume_crud():
    uuid1 = create_volume(VOLUME_SIZE_500M)
    uuid2 = create_volume(VOLUME_SIZE_100M)
    uuid3 = create_volume()

    delete_volume(uuid3, uuid3[:6])
    delete_volume(uuid2)
    delete_volume(uuid1)

def test_volume_name():
    vol_name1 = "vol1"
    vol_name2 = "vol2"
    vol_uuid = create_volume(name=vol_name1)
    vols = v.list_volumes()
    assert vols[vol_uuid]["Name"] == vol_name1
    assert vols[vol_uuid]["CreatedTime"] != ""

    with pytest.raises(subprocess.CalledProcessError):
        new_uuid = create_volume(name=vol_name1)

    delete_volume(vol_uuid, vol_name1)
    vols = v.list_volumes()
    assert vol_uuid not in vols

    vol_uuid1 = create_volume(name=vol_name1)
    vol_uuid2 = create_volume(name=vol_name2)
    assert vol_uuid1 != vol_uuid

    vols = v.list_volumes()
    assert vols[vol_uuid1]["Name"] == vol_name1
    assert vols[vol_uuid2]["Name"] == vol_name2
    assert vols[vol_uuid1]["CreatedTime"] != ""
    assert vols[vol_uuid2]["CreatedTime"] != ""
    delete_volume(vol_uuid1, vol_name1)
    delete_volume(vol_uuid2, vol_name2)

def mount_volume_and_create_file(uuid, filename):
    # with format
    volume_mount_dir = mount_volume(uuid)

    test_file = os.path.join(volume_mount_dir,filename)
    with open(test_file, "w") as f:
	subprocess.check_call(["echo", "This is volume test file"], stdout=f)
    assert os.path.exists(test_file)

    umount_volume(uuid, volume_mount_dir)
    assert not os.path.exists(test_file)

def test_volume_mount():
    uuid = create_volume()

    # with format
    filename = "test"
    mount_volume_and_create_file(uuid, filename)

    # without format
    volume_mount_dir = mount_volume(uuid)
    test_file = os.path.join(volume_mount_dir, filename)
    assert os.path.exists(test_file)

    with pytest.raises(subprocess.CalledProcessError):
        volume_mount_dir = mount_volume(uuid)

    umount_volume(uuid, volume_mount_dir)
    assert not os.path.exists(test_file)

    with pytest.raises(subprocess.CalledProcessError):
        umount_volume(uuid, volume_mount_dir)

    # auto mount
    volume_mount_dir = mount_volume_auto(uuid)
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
    uuid3 = create_volume()

    volumes = v.list_volumes(uuid3)
    assert volumes[uuid3]["Size"] == int(DEFAULT_VOLUME_SIZE)

    volumes = v.list_volumes()
    assert volumes[uuid1]["Size"] == int(VOLUME_SIZE_500M_Bytes)
    assert volumes[uuid2]["Size"] == int(VOLUME_SIZE_100M)
    assert volumes[uuid3]["Size"] == int(DEFAULT_VOLUME_SIZE)

    delete_volume(uuid3)
    delete_volume(uuid2)
    delete_volume(uuid1)

def test_snapshot_crud():
    volume_uuid = create_volume(VOLUME_SIZE_500M, name="vol1")

    snapshot_uuid = v.create_snapshot(volume_uuid)
    v.delete_snapshot(snapshot_uuid)

    delete_volume(volume_uuid)

    # delete snapshot automatically with volume
    volume_uuid = create_volume(VOLUME_SIZE_500M, name="vol1")
    snap1 = v.create_snapshot(volume_uuid)
    snap2 = v.create_snapshot(volume_uuid)
    snap3 = v.create_snapshot(volume_uuid)

    v.delete_snapshot(snap1)
    v.delete_snapshot(snap2[:6])
    delete_volume(volume_uuid)

    volume_uuid = create_volume(VOLUME_SIZE_500M)
    snap1 = v.create_snapshot(volume_uuid, "snap1")
    snap2 = v.create_snapshot(volume_uuid, "snap2")
    snap3 = v.create_snapshot(volume_uuid, "snap3")
    v.delete_snapshot("snap1")
    v.delete_snapshot("snap2")
    delete_volume(volume_uuid)

def test_snapshot_list():
    volume1_uuid = create_volume(VOLUME_SIZE_500M)
    volume2_uuid = create_volume(VOLUME_SIZE_100M)

    snap0_vol1_uuid = str(uuid.uuid1())

    volumes = v.list_volumes(volume1_uuid, snap0_vol1_uuid)
    assert snap0_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]

    snap0_vol1_uuid = v.create_snapshot(volume1_uuid)

    volumes = v.list_volumes(volume1_uuid, snap0_vol1_uuid)
    assert snap0_vol1_uuid in volumes[volume1_uuid]["Snapshots"]

    snap1_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap2_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap1_vol2_uuid = v.create_snapshot(volume2_uuid, "snap1_vol2")
    snap2_vol2_uuid = v.create_snapshot(volume2_uuid, "snap2_vol2")
    snap3_vol2_uuid = v.create_snapshot(volume2_uuid, "snap3_vol2")

    volumes = v.list_volumes(volume2_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert volumes[volume2_uuid]["Snapshots"][snap1_vol2_uuid]["Name"] == "snap1_vol2"
    assert volumes[volume2_uuid]["Snapshots"][snap1_vol2_uuid]["CreatedTime"] != ""
    assert snap2_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert volumes[volume2_uuid]["Snapshots"][snap2_vol2_uuid]["Name"] == "snap2_vol2"
    assert volumes[volume2_uuid]["Snapshots"][snap2_vol2_uuid]["CreatedTime"] != ""
    assert snap3_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert volumes[volume2_uuid]["Snapshots"][snap3_vol2_uuid]["Name"] == "snap3_vol2"
    assert volumes[volume2_uuid]["Snapshots"][snap3_vol2_uuid]["CreatedTime"] != ""

    volumes = v.list_volumes()
    assert snap0_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap2_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap2_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap3_vol2_uuid in volumes[volume2_uuid]["Snapshots"]

    v.delete_snapshot(snap0_vol1_uuid)

    volumes = v.list_volumes(volume1_uuid, snap0_vol1_uuid)
    assert snap0_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]

    v.delete_snapshot(snap1_vol1_uuid)
    v.delete_snapshot(snap2_vol1_uuid)
    v.delete_snapshot(snap1_vol2_uuid)
    v.delete_snapshot(snap2_vol2_uuid)
    v.delete_snapshot(snap3_vol2_uuid)

    delete_volume(volume2_uuid)
    delete_volume(volume1_uuid)

def get_volume_dir(uuid):
    return os.path.join(OBJECTSTORE_VOLUME_DIR, uuid[:2], uuid[2:4], uuid)

def get_volume_cfg(uuid):
    return os.path.join(get_volume_dir(uuid), OBJECTSTORE_PER_VOLUME_CFG)

def get_snapshot_dir(snapshot_uuid, volume_uuid):
    return os.path.join(get_volume_dir(volume_uuid), OBJECTSTORE_SNAPSHOTS_DIR)

def get_snapshot_cfg(snapshot_uuid, volume_uuid):
    return  os.path.join(get_snapshot_dir(snapshot_uuid, volume_uuid),
            "snapshot_" + snapshot_uuid +".cfg")

def get_checksum(filename):
    output = subprocess.check_output(["sha512sum", filename]).decode()
    return output.split(" ")[0]

def test_restore_with_original_removed():
    objectstore_uuid = v.register_vfs_objectstore(TEST_ROOT)
    volume1_uuid = create_volume(VOLUME_SIZE_500M)
    v.add_volume_to_objectstore(volume1_uuid, objectstore_uuid)
    mount_volume_and_create_file(volume1_uuid, "test-vol1-v1")
    snap1_vol1_uuid = v.create_snapshot(volume1_uuid)
    v.backup_snapshot_to_objectstore(snap1_vol1_uuid, objectstore_uuid)
    volume1_checksum = get_checksum(os.path.join(DM_DIR, volume1_uuid))
    delete_volume(volume1_uuid)

    res_volume1_uuid = create_volume(VOLUME_SIZE_500M)
    v.restore_snapshot_from_objectstore(snap1_vol1_uuid, volume1_uuid,
		    res_volume1_uuid, objectstore_uuid)
    res_volume1_checksum = get_checksum(os.path.join(DM_DIR, res_volume1_uuid))
    assert res_volume1_checksum == volume1_checksum
    delete_volume(res_volume1_uuid)

    v.deregister_objectstore(objectstore_uuid)

def test_vfs_objectstore():
    #create objectstore
    uuid = v.register_vfs_objectstore(TEST_ROOT)

    assert os.path.exists(OBJECTSTORE_ROOT)
    assert os.path.exists(OBJECTSTORE_CFG)

    with open(OBJECTSTORE_CFG) as f:
	bs = json.loads(f.read())
    assert bs["UUID"] == uuid
    assert bs["Kind"] == "vfs"

    v.deregister_objectstore(uuid)

    #load objectstore from created one
    objectstore_uuid = v.register_vfs_objectstore(TEST_ROOT)
    assert uuid == objectstore_uuid

    process_objectstore_test(objectstore_uuid, True)

def register_s3_objectstore():
    region = os.environ[ENV_TEST_AWS_REGION]
    bucket = os.environ[ENV_TEST_AWS_BUCKET]
    path = S3_PATH

    # slash is not allowed for prefix of path
    with pytest.raises(subprocess.CalledProcessError):
        v.register_s3_objectstore(region, bucket, "/test")

    return v.register_s3_objectstore(region, bucket, path)

@pytest.mark.s3
def test_s3_objectstore():
    #create objectstore
    uuid = register_s3_objectstore()
    v.deregister_objectstore(uuid)

    #load objectstore from created one
    objectstore_uuid = register_s3_objectstore()
    assert uuid == objectstore_uuid

    process_objectstore_test(objectstore_uuid, False)

def process_objectstore_test(objectstore_uuid, is_vfs):
    #add volume to objectstore
    volume1_uuid = create_volume(VOLUME_SIZE_500M)
    volume2_uuid = create_volume(VOLUME_SIZE_100M)

    volumes = v.list_volume_objectstore(volume1_uuid, objectstore_uuid)
    assert len(volumes) == 0
    volumes = v.list_volume_objectstore_with_snapshot(RANDOM_VALID_UUID, volume1_uuid, objectstore_uuid)
    assert len(volumes) == 0

    v.add_volume_to_objectstore(volume1_uuid, objectstore_uuid)
    # test idempotency
    v.add_volume_to_objectstore(volume1_uuid, objectstore_uuid)
    if is_vfs:
        volume1_cfg_path = os.path.join(get_volume_dir(volume1_uuid), OBJECTSTORE_PER_VOLUME_CFG)
        assert os.path.exists(volume1_cfg_path)

    volumes = v.list_volume_objectstore_with_snapshot(RANDOM_VALID_UUID, volume1_uuid, objectstore_uuid)
    assert len(volumes) == 1
    assert volumes[volume1_uuid]["Size"] == int(VOLUME_SIZE_500M_Bytes)
    assert volumes[volume1_uuid]["Base"] == ""
    assert len(volumes[volume1_uuid]["Snapshots"]) == 0

    v.add_volume_to_objectstore(volume2_uuid, objectstore_uuid)
    # test idempotency
    v.add_volume_to_objectstore(volume2_uuid, objectstore_uuid)
    if is_vfs:
        volume2_cfg_path = os.path.join(get_volume_dir(volume2_uuid), OBJECTSTORE_PER_VOLUME_CFG)
        assert os.path.exists(volume2_cfg_path)

    # remove volume from objectstore, it should be added automatically with backup
    v.remove_volume_from_objectstore(volume1_uuid, objectstore_uuid)
    v.remove_volume_from_objectstore(volume2_uuid, objectstore_uuid)

    #first snapshots
    snap1_vol1_uuid = v.create_snapshot(volume1_uuid)
    v.backup_snapshot_to_objectstore(snap1_vol1_uuid, objectstore_uuid)
    if is_vfs:
        with open(get_snapshot_cfg(snap1_vol1_uuid, volume1_uuid)) as f:
            snap1_vol1 = json.loads(f.read())
        assert snap1_vol1["ID"] == snap1_vol1_uuid
        assert len(snap1_vol1["Blocks"]) != 0

    volumes = v.list_volume_objectstore_with_snapshot(snap1_vol1_uuid, volume1_uuid, objectstore_uuid)
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]

    snap1_vol2_uuid = v.create_snapshot(volume2_uuid, "snap1_vol2")
    v.backup_snapshot_to_objectstore("snap1_vol2", objectstore_uuid)
    if is_vfs:
        with open(get_snapshot_cfg(snap1_vol2_uuid, volume2_uuid)) as f:
            snap1_vol2 = json.loads(f.read())
        assert snap1_vol2["ID"] == snap1_vol2_uuid
        assert len(snap1_vol2["Blocks"]) != 0

    #list snapshots
    volumes = v.list_volume_objectstore(volume1_uuid, objectstore_uuid)
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    volumes = v.list_volume_objectstore(volume2_uuid, objectstore_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    volumes = v.list_volume_objectstore_with_snapshot(snap1_vol2_uuid, volume2_uuid, objectstore_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]

    #second snapshots
    mount_volume_and_create_file(volume1_uuid, "test-vol1-v1")
    snap2_vol1_uuid = v.create_snapshot(volume1_uuid)
    v.backup_snapshot_to_objectstore(snap2_vol1_uuid, objectstore_uuid)
    if is_vfs:
        with open(get_snapshot_cfg(snap2_vol1_uuid, volume1_uuid)) as f:
            snap2_vol1 = json.loads(f.read())
        assert snap2_vol1["ID"] == snap2_vol1_uuid
        assert len(snap2_vol1["Blocks"]) != 0

    mount_volume_and_create_file(volume2_uuid, "test-vol2-v2")
    snap2_vol2_uuid = v.create_snapshot(volume2_uuid)
    v.backup_snapshot_to_objectstore(snap2_vol2_uuid, objectstore_uuid)
    if is_vfs:
        with open(get_snapshot_cfg(snap2_vol2_uuid, volume2_uuid)) as f:
            snap2_vol2 = json.loads(f.read())
        assert snap2_vol2["ID"] == snap2_vol2_uuid
        assert len(snap2_vol2["Blocks"]) != 0

    #dupcliate snapshot backup should fail
    with pytest.raises(subprocess.CalledProcessError):
        v.backup_snapshot_to_objectstore(snap2_vol2_uuid, objectstore_uuid)

    #list snapshots again
    volumes = v.list_volume_objectstore(volume1_uuid, objectstore_uuid)
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap2_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    volumes = v.list_volume_objectstore(volume2_uuid, objectstore_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap2_vol2_uuid in volumes[volume2_uuid]["Snapshots"]

    #restore snapshot
    res_volume1_uuid = create_volume(VOLUME_SIZE_500M)
    v.restore_snapshot_from_objectstore(snap2_vol1_uuid, volume1_uuid,
		    res_volume1_uuid, objectstore_uuid)
    res_volume1_checksum = get_checksum(os.path.join(DM_DIR, res_volume1_uuid))
    volume1_checksum = get_checksum(os.path.join(DM_DIR, volume1_uuid))
    assert res_volume1_checksum == volume1_checksum

    res_volume2_uuid = create_volume(VOLUME_SIZE_100M)
    v.restore_snapshot_from_objectstore(snap2_vol2_uuid, volume2_uuid,
		    res_volume2_uuid, objectstore_uuid)
    res_volume2_checksum = get_checksum(os.path.join(DM_DIR, res_volume2_uuid))
    volume2_checksum = get_checksum(os.path.join(DM_DIR, volume2_uuid))
    assert res_volume2_checksum == volume2_checksum

    #remove snapshots from objectstore
    v.remove_snapshot_from_objectstore(snap2_vol1_uuid, volume1_uuid, objectstore_uuid)
    if is_vfs:
        assert not os.path.exists(get_snapshot_cfg(snap2_vol1_uuid, volume1_uuid))

    v.remove_snapshot_from_objectstore(snap2_vol2_uuid, volume2_uuid, objectstore_uuid)
    if is_vfs:
        assert not os.path.exists(get_snapshot_cfg(snap2_vol2_uuid, volume2_uuid))

    #list snapshots again
    volumes = v.list_volume_objectstore(volume1_uuid, objectstore_uuid)
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap2_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]
    volumes = v.list_volume_objectstore_with_snapshot(snap1_vol1_uuid, volume1_uuid, objectstore_uuid)
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    volumes = v.list_volume_objectstore_with_snapshot(snap2_vol1_uuid, volume1_uuid, objectstore_uuid)
    assert snap1_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]
    assert snap2_vol1_uuid not in volumes[volume1_uuid]["Snapshots"]

    volumes = v.list_volume_objectstore(volume2_uuid, objectstore_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap2_vol2_uuid not in volumes[volume2_uuid]["Snapshots"]
    volumes = v.list_volume_objectstore_with_snapshot(snap1_vol2_uuid, volume2_uuid, objectstore_uuid)
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    volumes = v.list_volume_objectstore_with_snapshot(snap2_vol2_uuid, volume2_uuid, objectstore_uuid)
    assert snap1_vol2_uuid not in volumes[volume2_uuid]["Snapshots"]
    assert snap2_vol2_uuid not in volumes[volume2_uuid]["Snapshots"]

    #remove volume from objectstore
    v.remove_volume_from_objectstore(volume1_uuid, objectstore_uuid)
    if is_vfs:
        assert not os.path.exists(get_volume_cfg(volume1_uuid))

    v.remove_volume_from_objectstore(volume2_uuid, objectstore_uuid)
    if is_vfs:
        assert not os.path.exists(get_volume_cfg(volume2_uuid))

    v.delete_snapshot(snap1_vol1_uuid)
    v.delete_snapshot(snap2_vol1_uuid)
    v.delete_snapshot(snap1_vol2_uuid)
    v.delete_snapshot(snap2_vol2_uuid)

    delete_volume(volume1_uuid)
    delete_volume(volume2_uuid)
    delete_volume(res_volume1_uuid)
    delete_volume(res_volume2_uuid)

    v.deregister_objectstore(objectstore_uuid)

def get_image_cfg(uuid):
    return os.path.join(OBJECTSTORE_IMAGES_DIR, uuid + ".json")

def get_image_gz(uuid):
    return os.path.join(OBJECTSTORE_IMAGES_DIR, uuid + ".img.gz")

def get_local_image(uuid):
    return os.path.join(IMAGES_DIR, uuid + ".img")

def test_vfs_objectstore_image():
    #load objectstore from created one
    objectstore_uuid = v.register_vfs_objectstore(TEST_ROOT)
    process_objectstore_image(objectstore_uuid, True)

@pytest.mark.s3
def test_s3_objectstore_image():
    #load objectstore from created one
    objectstore_uuid = register_s3_objectstore()
    process_objectstore_image(objectstore_uuid, False)

def process_objectstore_image(objectstore_uuid, is_vfs):
    #add/remove image
    global image_file
    image_uuid = v.add_image_to_objectstore(image_file, objectstore_uuid)

    if is_vfs:
	assert os.path.exists(OBJECTSTORE_IMAGES_DIR)
	assert os.path.exists(get_image_cfg(image_uuid))
	assert os.path.exists(get_image_gz(image_uuid))

    v.remove_image_from_objectstore(image_uuid, objectstore_uuid)

    if is_vfs:
	assert not os.path.exists(get_image_cfg(image_uuid))
	assert not os.path.exists(get_image_gz(image_uuid))

    #activate/deactivate image
    image_uuid = v.add_image_to_objectstore(image_file, objectstore_uuid)

    #compressed image cache
    if is_vfs:
	assert os.path.exists(get_local_image(image_uuid)+".gz")

    v.activate_image(image_uuid, objectstore_uuid)
    if is_vfs:
	assert os.path.exists(get_local_image(image_uuid))
    v.deactivate_image(image_uuid, objectstore_uuid)
    if is_vfs:
	assert not os.path.exists(get_local_image(image_uuid))

    #raw image cache
    subprocess.check_call(["cp", image_file, get_local_image(image_uuid)])
    if is_vfs:
	assert os.path.exists(get_local_image(image_uuid))

    v.activate_image(image_uuid, objectstore_uuid)
    if is_vfs:
	assert os.path.exists(get_local_image(image_uuid))
    v.deactivate_image(image_uuid, objectstore_uuid)
    if is_vfs:
	assert not os.path.exists(get_local_image(image_uuid))

    #deactivate would remove the local copy, so this time it would trigger
    # downloading from objectstore
    v.activate_image(image_uuid, objectstore_uuid)
    if is_vfs:
	assert os.path.exists(get_local_image(image_uuid))
    v.deactivate_image(image_uuid, objectstore_uuid)
    if is_vfs:
	assert not os.path.exists(get_local_image(image_uuid))

    v.remove_image_from_objectstore(image_uuid, objectstore_uuid)
    v.deregister_objectstore(objectstore_uuid)

def test_vfs_image_based_volume():
    #load objectstore from created one
    objectstore_uuid = v.register_vfs_objectstore(TEST_ROOT)
    process_image_based_volume(objectstore_uuid)

@pytest.mark.s3
def test_s3_image_based_volume():
    #load objectstore from created one
    objectstore_uuid = register_s3_objectstore()
    process_image_based_volume(objectstore_uuid)

def process_image_based_volume(objectstore_uuid):
    #add/remove image
    global image_file
    image_uuid = v.add_image_to_objectstore(image_file, objectstore_uuid)

    v.activate_image(image_uuid, objectstore_uuid)

    volume_uuid = create_volume(VOLUME_SIZE_100M, base=image_uuid)

    volume_mount_dir = mount_volume(volume_uuid)

    assert os.path.exists(os.path.join(volume_mount_dir, TEST_IMAGE_FILE))
    subprocess.check_call(["touch", os.path.join(volume_mount_dir,
	    TEST_SNAPSHOT_FILE)])
    umount_volume(volume_uuid, volume_mount_dir)

    snapshot_uuid = v.create_snapshot(volume_uuid)
    v.add_volume_to_objectstore(volume_uuid, objectstore_uuid)
    # idempotency
    v.add_volume_to_objectstore(volume_uuid, objectstore_uuid)
    v.add_volume_to_objectstore(volume_uuid, objectstore_uuid)
    v.backup_snapshot_to_objectstore(snapshot_uuid, objectstore_uuid)

    new_volume_uuid = create_volume(VOLUME_SIZE_100M, base=image_uuid)

    v.restore_snapshot_from_objectstore(snapshot_uuid, volume_uuid,
            new_volume_uuid, objectstore_uuid)

    new_volume_mount_dir = mount_volume(new_volume_uuid)

    assert os.path.exists(os.path.join(new_volume_mount_dir, TEST_IMAGE_FILE))
    assert os.path.exists(os.path.join(new_volume_mount_dir, TEST_SNAPSHOT_FILE))

    umount_volume(new_volume_uuid, new_volume_mount_dir)

    v.remove_snapshot_from_objectstore(snapshot_uuid, volume_uuid,
            objectstore_uuid)
    v.delete_snapshot(snapshot_uuid)

    v.remove_volume_from_objectstore(volume_uuid, objectstore_uuid)
    delete_volume(volume_uuid)

    delete_volume(new_volume_uuid)

    v.deactivate_image(image_uuid, objectstore_uuid)
    v.remove_image_from_objectstore(image_uuid, objectstore_uuid)

    v.deregister_objectstore(objectstore_uuid)

def test_list_objectstores():
    os.makedirs(OBJECTSTORE_PATH1)
    os.makedirs(OBJECTSTORE_PATH2)
    os.makedirs(OBJECTSTORE_PATH3)

    store1 = v.register_vfs_objectstore(OBJECTSTORE_PATH1)
    store2 = v.register_vfs_objectstore(OBJECTSTORE_PATH2)
    store3 = v.register_vfs_objectstore(OBJECTSTORE_PATH3)

    stores = v.list_objectstores()
    # we may have leftover from previous test cases, don't fail because of that
    assert len(stores) >= 3
    assert stores[store1]["UUID"] == store1
    assert stores[store1]["Kind"] == "vfs"

    assert stores[store2]["UUID"] == store2
    assert stores[store2]["Kind"] == "vfs"

    assert stores[store3]["UUID"] == store3
    assert stores[store3]["Kind"] == "vfs"

    stores = v.list_objectstores(store1)
    assert len(stores) == 1
    assert stores[store1]["UUID"] == store1
    assert stores[store1]["Kind"] == "vfs"

    with pytest.raises(subprocess.CalledProcessError):
        stores = v.list_objectstores(RANDOM_VALID_UUID)

    v.deregister_objectstore(store1)
    v.deregister_objectstore(store2)
    v.deregister_objectstore(store3)

#def create_delete_volume_thread():
#    uuid = v.create_volume()
#    v.delete_volume(uuid)
#
#def test_create_volume_in_parallel():
#    threads = []
#    for i in range(TEST_THREAD_COUNT):
#        threads.append(threading.Thread(target = create_delete_volume_thread))
#        threads[i].start()
#
#    for i in range(TEST_THREAD_COUNT):
#        threads[i].join()

