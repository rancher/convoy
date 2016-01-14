#!/usr/bin/python

import json
import os
import pytest
import shutil
import subprocess
import threading
import time
import uuid

from convoy import VolumeManager

TEST_ROOT = "/tmp/convoy_test/"
CFG_ROOT = os.path.join(TEST_ROOT, "convoy")
PID_FILE = os.path.join(TEST_ROOT, "convoy.pid")
LOG_FILE = os.path.join(TEST_ROOT, "convoy.log")
TEST_SNAPSHOT_FILE = "snapshot.test"

CONTAINER_NAME = "convoy-test"
CONTAINER = "yasker/convoy"
CONVOY_CONTAINER_CMD = ["docker", "exec", CONTAINER_NAME, "convoy"]

CONVOY_BINARY = [os.path.abspath("../../bin/convoy")]

DM = "devicemapper"
DM_ROOT = os.path.join(CFG_ROOT, DM)

TEST_THREAD_COUNT = 100
TEST_LOOP_COUNT = 100

VFS_BACKUP_DIR = os.path.join(TEST_ROOT, "Backup")
VFS_DEST = "vfs://" + VFS_BACKUP_DIR

VFS = "vfs"
VFS_ROOT = os.path.join(CFG_ROOT, VFS)
VFS_VOLUME_PATH = os.path.join(TEST_ROOT, "vfs-volumes")

EBS = "ebs"

ENV_TEST_AWS_REGION = "CONVOY_TEST_AWS_REGION"
ENV_TEST_AWS_BUCKET = "CONVOY_TEST_AWS_BUCKET"
S3_PATH = "test/volume/"

DD_BLOCK_SIZE = 4096
POOL_NAME = "convoy_test_pool"

DATA_FILE = "data.vol"
METADATA_FILE = "metadata.vol"
DATA_DEVICE_SIZE = 2147483648
METADATA_DEVICE_SIZE = 52428800
DM_DIR = "/dev/mapper"
DM_BLOCK_SIZE = 2097152
EMPTY_FILE_SIZE = 104857600

DEFAULT_VOLUME_SIZE = "1073741824"
VOLUME_SIZE_IOPS = "5G"
VOLUME_IOPS = "100"
VOLUME_SIZE_BIG_Bytes = "2147483648"
VOLUME_SIZE_BIG = "2G"
VOLUME_SIZE_SMALL = "1073741824"

VOLUME_SIZE_6M = "6M"

EBS_DEFAULT_VOLUME_TYPE = "standard"

VM_IMAGE_FILE = "disk.img"

data_dev = ""
metadata_dev = ""

mount_cleanup_list = []
dm_cleanup_list = []
volume_cleanup_list = []

test_ebs = False
test_container = False

v = None


def create_empty_file(filepath, size):
    subprocess.check_call(["truncate", "-s", str(size), filepath])
    assert os.path.exists(filepath)


def attach_loopback_dev(filepath):
    dev = subprocess.check_output(["losetup", "-v", "-f",
                                   filepath]).strip().split(" ")[3]
    assert dev.startswith("/dev/loop")
    return dev


def detach_loopback_dev(dev):
    subprocess.check_output(["losetup", "-d", dev])


def mount_dev(dev, mountpoint):
    subprocess.check_call(["mount", dev, mountpoint])
    mount_cleanup_list.append(mountpoint)


def umount_dev(mountpoint):
    subprocess.check_call(["umount", mountpoint])
    mount_cleanup_list.remove(mountpoint)


def setup_module():
    global test_ebs
    test_ebs = pytest.config.getoption("ebs")

    global test_container
    test_container = pytest.config.getoption("container")

    if os.path.exists(TEST_ROOT):
        subprocess.check_call(["rm", "-rf", TEST_ROOT])

    os.makedirs(TEST_ROOT)
    assert os.path.exists(TEST_ROOT)

    os.makedirs(VFS_BACKUP_DIR)
    assert os.path.exists(VFS_BACKUP_DIR)

    data_file = os.path.join(TEST_ROOT, DATA_FILE)
    create_empty_file(data_file, DATA_DEVICE_SIZE)
    global data_dev
    data_dev = attach_loopback_dev(data_file)

    metadata_file = os.path.join(TEST_ROOT, METADATA_FILE)
    create_empty_file(metadata_file, METADATA_DEVICE_SIZE)
    global metadata_dev
    metadata_dev = attach_loopback_dev(metadata_file)

    global v
    cmdline = []
    if test_container:
        v = VolumeManager(CONVOY_CONTAINER_CMD, TEST_ROOT)
        cmdline = ["convoy-start",
                   "--mnt-ns", "/host/proc/1/ns/mnt"]
    else:
        v = VolumeManager(CONVOY_BINARY, TEST_ROOT)
        cmdline = ["daemon"]
    cmdline += [
        "--root", CFG_ROOT,
        "--log", LOG_FILE,
        "--drivers=" + DM,
        "--driver-opts", "dm.datadev=" + data_dev,
        "--driver-opts", "dm.metadatadev=" + metadata_dev,
        "--driver-opts", "dm.thinpoolname=" + POOL_NAME,
        "--driver-opts", "dm.defaultvolumesize=" + DEFAULT_VOLUME_SIZE,
        "--drivers=" + VFS,
        "--driver-opts", "vfs.path=" + VFS_VOLUME_PATH]
    if test_ebs:
        cmdline += ["--drivers=ebs",
                    "--driver-opts",
                    "ebs.defaultvolumesize=" + DEFAULT_VOLUME_SIZE,
                    "--driver-opts",
                    "ebs.defaultvolumetype=" + EBS_DEFAULT_VOLUME_TYPE]
    if test_container:
        v.start_server_container(CONTAINER_NAME, CFG_ROOT,
                                 TEST_ROOT, CONTAINER, cmdline)
    else:
        v.start_server(PID_FILE, cmdline)
    dm_cleanup_list.append(POOL_NAME)
    wait_for_daemon()


def detach_all_lodev(keyword):
    output = subprocess.check_output(["losetup", "-a"])
    lines = output.splitlines()
    for line in lines:
        if line.find(keyword) != -1:
            detach_loopback_dev(line.split(":")[0].strip())


def teardown_module():
    if test_container:
        code = v.stop_server_container(CONTAINER_NAME)
    else:
        code = v.stop_server(PID_FILE)
    if code != 0:
        print("Something wrong when tearing down, continuing with code ", code)

    while mount_cleanup_list:
        code = subprocess.call(["umount", mount_cleanup_list.pop()])
        if code != 0:
            print("Something wrong when tearing down, continuing with code",
                  code)
    while dm_cleanup_list:
        code = subprocess.call(["dmsetup", "remove", "--retry",
                               dm_cleanup_list.pop()])
        if code != 0:
            print("Something wrong when tearing down, continuing with code ",
                  code)

    code = subprocess.call(["dmsetup", "remove", "--retry", POOL_NAME])
    if code != 0:
        print("Something wrong when tearing down, continuing with code ", code)

    code = subprocess.call(["losetup", "-d", data_dev, metadata_dev])
    if code != 0:
        print("Something wrong when tearing down, continuing with code", code)

    detach_all_lodev(TEST_ROOT)


def wait_for_daemon():
    while True:
        try:
            data = v.server_info()
            break
        except subprocess.CalledProcessError:
            print("Fail to communicate with daemon")
            check_result = 0
            if test_container:
                check_result = v.check_server_container(CONTAINER_NAME)
            else:
                check_result = v.check_server(PID_FILE)
            if check_result != 0:
                print("Server failed to start")
                teardown_module()
                assert False
            time.sleep(1)

    info = json.loads(data)
    success = True
    try:
        success = bool(success and DM in info["General"]["DriverList"])
        success = bool(success and VFS in info["General"]["DriverList"])
        success = bool(success and info["General"]["Root"] == CFG_ROOT)
        success = bool(success and info["General"]["DefaultDriver"] == DM)
        success = bool(success and info[DM]["Driver"] == "devicemapper")
        success = bool(success and info[DM]["Root"] == DM_ROOT)
        success = bool(success and info[DM]["DataDevice"] == data_dev)
        success = bool(success and info[DM]["MetadataDevice"] == metadata_dev)
        success = bool(success and info[DM]["ThinpoolDevice"] ==
                       os.path.join(DM_DIR, POOL_NAME))
        success = bool(success and info[DM]["ThinpoolSize"] ==
                       str(DATA_DEVICE_SIZE))
        success = bool(success and info[DM]["ThinpoolBlockSize"] ==
                       str(DM_BLOCK_SIZE))
        success = bool(success and info[DM]["DefaultVolumeSize"] ==
                       DEFAULT_VOLUME_SIZE)
        success = bool(success and info[VFS]["Root"] == VFS_ROOT)
        success = bool(success and info[VFS]["Path"] == VFS_VOLUME_PATH)
        if test_ebs:
            success = bool(success and info[EBS]["DefaultVolumeSize"] ==
                           DEFAULT_VOLUME_SIZE)
            success = bool(success and info[EBS]["DefaultVolumeType"] ==
                           EBS_DEFAULT_VOLUME_TYPE)

    except Exception:
        success = False

    if not success:
        teardown_module()
        assert False


@pytest.yield_fixture(autouse=True)
def cleanup_test():
    yield
    filenames = os.listdir(CFG_ROOT)
    leftover_volumes = []
    for filename in filenames:
        if filename.startswith('volume'):
            leftover_volumes.append(filename)
    while volume_cleanup_list:
        v = volume_cleanup_list.pop()
        try:
            delete_volume(v)
        except Exception:
            print("Failed to delete volume ", v)
    if len(leftover_volumes) != 0:
        print(leftover_volumes)
        assert False


def create_volume(size="", name="", backup="", driver="",
                  volume_id="", volume_type="", iops="", forvm=False):
    uuid = v.create_volume(size, name, backup, driver,
                           volume_id, volume_type, iops, forvm)
    if driver == "" or driver == DM:
        dm_cleanup_list.append(uuid)
    volume_cleanup_list.append(uuid)
    return uuid


def delete_volume(uuid, name="", ref_only=False):
    if name == "":
        v.delete_volume(uuid, ref_only)
    else:
        v.delete_volume(name, ref_only)
    try:
        dm_cleanup_list.remove(uuid)
    except ValueError:
        pass
    volume_cleanup_list.remove(uuid)


def mount_volume_with_path(uuid):
    mount_dir = v.mount_volume_with_path(uuid)
    mount_cleanup_list.append(mount_dir)
    return mount_dir


def mount_volume(uuid):
    mount_dir = v.mount_volume(uuid)
    mount_cleanup_list.append(mount_dir)
    return mount_dir


def umount_volume(uuid, mount_dir):
    v.umount_volume(uuid)
    mount_cleanup_list.remove(mount_dir)


def test_volume_crud():
    volume_crud_test(DM, vmTest=False)
    volume_crud_test(VFS, sizeTest=False)


def volume_crud_test(drv, sizeTest=True, vmTest=True):
    uuid1 = create_volume(driver=drv)
    uuid2 = create_volume(driver=drv)

    if sizeTest:
        uuid3 = create_volume(VOLUME_SIZE_BIG, driver=drv)
        uuid4 = create_volume(VOLUME_SIZE_SMALL, driver=drv)
        delete_volume(uuid4)
        delete_volume(uuid3)

    if vmTest:
        uuid3 = create_volume(driver=drv, forvm=True)
        uuid4 = create_volume(driver=drv, forvm=True)
        delete_volume(uuid4)
        delete_volume(uuid3)

    delete_volume(uuid2, uuid2[:6])
    delete_volume(uuid1)


@pytest.mark.skipif(not pytest.config.getoption("ebs"),
                    reason="--ebs was not specified")
def test_ebs_volume_crud():
    uuid1 = create_volume(driver=EBS)
    uuid2 = create_volume(size=VOLUME_SIZE_SMALL,
                          driver=EBS,
                          volume_type="gp2")
    uuid3 = create_volume(size=VOLUME_SIZE_IOPS,
                          driver=EBS,
                          volume_type="io1",
                          iops="100")

    volume3 = v.inspect_volume(uuid3)
    ebs_volume_id3 = volume3["DriverInfo"]["EBSVolumeID"]

    delete_volume(uuid3, ref_only=True)

    uuid3 = create_volume(driver=EBS, volume_id=ebs_volume_id3)

    delete_volume(uuid3)
    delete_volume(uuid2)
    delete_volume(uuid1)


def test_vfs_delete_volume_ref_only():
    uuid = create_volume(driver=VFS)
    insp = v.inspect_volume(uuid)
    path = insp["DriverInfo"]["Path"]

    assert os.path.exists(path)
    filename = "testfile"
    test_file = os.path.join(path, filename)
    with open(test_file, "w") as f:
        subprocess.check_call(["echo", "This is volume test file"], stdout=f)
    assert os.path.exists(test_file)

    delete_volume(uuid, ref_only=True)
    assert os.path.exists(test_file)
    os.remove(test_file)


def test_volume_name():
    volume_name_test(DM)
    volume_name_test(VFS)


def volume_name_test(drv):
    vol_name1 = "vol1"
    vol_name2 = "vol2"
    vol_uuid = create_volume(name=vol_name1, driver=drv)
    vols = v.list_volumes()
    assert vols[vol_uuid]["Name"] == vol_name1
    assert vols[vol_uuid]["Driver"] == drv
    assert vols[vol_uuid]["CreatedTime"] != ""

    with pytest.raises(subprocess.CalledProcessError):
        create_volume(name=vol_name1, driver=drv)

    with pytest.raises(subprocess.CalledProcessError):
        create_volume(driver="randomdriver")

    delete_volume(vol_uuid, vol_name1)
    vols = v.list_volumes()
    assert vol_uuid not in vols

    vol_uuid1 = create_volume(name=vol_name1, driver=drv)
    vol_uuid2 = create_volume(name=vol_name2, driver=drv)
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

    test_file = os.path.join(volume_mount_dir, filename)
    with open(test_file, "w") as f:
        subprocess.check_call(["echo", "This is volume test file"], stdout=f)
    assert os.path.exists(test_file)

    umount_volume(uuid, volume_mount_dir)
    # Doesn't work with current VFS implmentation, since it won't really mount
    # assert not os.path.exists(test_file)


def test_volume_mount():
    volume_mount_test(DM)
    if test_ebs:
        volume_mount_test(EBS)
    # skip the vfs mount test because we only pass the original volume path as
    # mount path, not really done any mount work now


def volume_mount_test(drv):
    uuid = create_volume(driver=drv)

    # with format
    filename = "test"
    mount_volume_and_create_file(uuid, filename)

    # without format
    volume_mount_dir = mount_volume_with_path(uuid)
    test_file = os.path.join(volume_mount_dir, filename)
    assert os.path.exists(test_file)

    umount_volume(uuid, volume_mount_dir)
    assert not os.path.exists(test_file)

    # auto mount
    volume_mount_dir = mount_volume(uuid)
    test_file = os.path.join(volume_mount_dir, filename)
    assert os.path.exists(test_file)

    umount_volume(uuid, volume_mount_dir)
    assert not os.path.exists(test_file)

    delete_volume(uuid)


def test_volume_vm_mount():
    volume_vm_test(VFS)


def volume_vm_test(drv):
    uuid = create_volume(driver=drv, size=VOLUME_SIZE_SMALL, forvm=True)
    mount_dir = mount_volume(uuid)

    image_filepath = os.path.join(mount_dir, VM_IMAGE_FILE)
    assert os.path.exists(image_filepath)
    size = os.stat(image_filepath).st_size
    assert str(size) == VOLUME_SIZE_SMALL

    umount_volume(uuid, mount_dir)
    delete_volume(uuid)


def test_volume_list():
    volume_list_driver_test(DM)
    volume_list_driver_test(VFS, False)
    if test_ebs:
        volume_list_driver_test(EBS)


def volume_list_driver_test(drv, check_size=True):
    volumes = v.list_volumes()
    assert len(volumes) == 0

    uuid1 = create_volume(driver=drv)
    uuid2 = create_volume(driver=drv)
    if check_size:
        uuid3 = create_volume(VOLUME_SIZE_BIG, driver=drv)
        uuid4 = create_volume(VOLUME_SIZE_SMALL, driver=drv)

    volume = v.inspect_volume(uuid1)
    assert volume["UUID"] == uuid1
    if check_size:
        assert volume["DriverInfo"]["Size"] == DEFAULT_VOLUME_SIZE
    volume = v.inspect_volume(uuid2)
    assert volume["UUID"] == uuid2
    if check_size:
        assert volume["DriverInfo"]["Size"] == DEFAULT_VOLUME_SIZE

    if check_size:
        volumes = v.list_volumes()
        assert volumes[uuid1]["DriverInfo"]["Size"] == DEFAULT_VOLUME_SIZE
        assert volumes[uuid2]["DriverInfo"]["Size"] == DEFAULT_VOLUME_SIZE
        assert volumes[uuid3]["DriverInfo"]["Size"] == VOLUME_SIZE_BIG_Bytes
        assert volumes[uuid4]["DriverInfo"]["Size"] == VOLUME_SIZE_SMALL

        delete_volume(uuid4)
        delete_volume(uuid3)

    delete_volume(uuid2)
    delete_volume(uuid1)


def test_snapshot_crud():
    snapshot_crud_test(DM)
    snapshot_crud_test(VFS)


def snapshot_crud_test(driver):
    volume_uuid = create_volume(VOLUME_SIZE_SMALL, name="vol1", driver=driver)

    snapshot_uuid = v.create_snapshot(volume_uuid)
    v.delete_snapshot(snapshot_uuid)

    delete_volume(volume_uuid)

    # delete snapshot automatically with volume
    volume_uuid = create_volume(VOLUME_SIZE_SMALL, name="vol1", driver=driver)
    snap1 = v.create_snapshot(volume_uuid)
    snap2 = v.create_snapshot(volume_uuid)
    snap3 = v.create_snapshot(volume_uuid)

    v.delete_snapshot(snap1)
    v.delete_snapshot(snap2[:6])
    v.delete_snapshot(snap3)
    delete_volume(volume_uuid)


def test_snapshot_name():
    snapshot_name_test(DM)
    snapshot_name_test(VFS)


def snapshot_name_test(driver):
    volume_uuid = create_volume(VOLUME_SIZE_SMALL, driver=driver)

    snap1_name = "snap1"
    snap1_uuid = v.create_snapshot(volume_uuid, name=snap1_name)

    vols = v.list_volumes()
    s = vols[volume_uuid]["Snapshots"][snap1_uuid]
    assert s["Name"] == snap1_name
    assert s["DriverInfo"]["Driver"] == driver
    assert s["CreatedTime"] != ""

    with pytest.raises(subprocess.CalledProcessError):
        v.create_snapshot(volume_uuid, name=snap1_name)

    v.delete_snapshot(snap1_uuid)
    delete_volume(volume_uuid)


def test_snapshot_list():
    snapshot_list_test(DM)
    snapshot_list_test(VFS, False)


def snapshot_list_test(driver, check_size=True):
    volume1_uuid = create_volume(VOLUME_SIZE_SMALL,
                                 name="volume1",
                                 driver=driver)
    volume2_uuid = create_volume(VOLUME_SIZE_BIG, driver=driver)

    with pytest.raises(subprocess.CalledProcessError):
        snapshot = v.inspect_snapshot(str(uuid.uuid1()))

    with pytest.raises(subprocess.CalledProcessError):
        volume = v.inspect_snapshot(str(uuid.uuid1()))

    snap0_vol1_uuid = v.create_snapshot(volume1_uuid, "snap0_vol1")

    snapshot = v.inspect_snapshot("snap0_vol1")
    assert snapshot["UUID"] == snap0_vol1_uuid
    assert snapshot["VolumeUUID"] == volume1_uuid
    assert snapshot["VolumeName"] == "volume1"
    if check_size:
        assert str(snapshot["DriverInfo"]["Size"]) == VOLUME_SIZE_SMALL
    assert snapshot["Name"] == "snap0_vol1"

    snap1_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap2_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap1_vol2_uuid = v.create_snapshot(volume2_uuid, "snap1_vol2")
    snap2_vol2_uuid = v.create_snapshot(volume2_uuid, "snap2_vol2")
    snap3_vol2_uuid = v.create_snapshot(volume2_uuid, "snap3_vol2")

    volume = v.inspect_volume(volume2_uuid)
    assert snap1_vol2_uuid in volume["Snapshots"]
    assert volume["Snapshots"][snap1_vol2_uuid]["Name"] == "snap1_vol2"
    assert volume["Snapshots"][snap1_vol2_uuid]["CreatedTime"] != ""
    assert snap2_vol2_uuid in volume["Snapshots"]
    assert volume["Snapshots"][snap2_vol2_uuid]["Name"] == "snap2_vol2"
    assert volume["Snapshots"][snap2_vol2_uuid]["CreatedTime"] != ""
    assert snap3_vol2_uuid in volume["Snapshots"]
    assert volume["Snapshots"][snap3_vol2_uuid]["Name"] == "snap3_vol2"
    assert volume["Snapshots"][snap3_vol2_uuid]["CreatedTime"] != ""

    volumes = v.list_volumes()
    assert snap0_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap1_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap2_vol1_uuid in volumes[volume1_uuid]["Snapshots"]
    assert snap1_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap2_vol2_uuid in volumes[volume2_uuid]["Snapshots"]
    assert snap3_vol2_uuid in volumes[volume2_uuid]["Snapshots"]

    v.delete_snapshot(snap0_vol1_uuid)

    with pytest.raises(subprocess.CalledProcessError):
        snapshot = v.inspect_snapshot(snap0_vol1_uuid)

    v.delete_snapshot(snap1_vol1_uuid)
    v.delete_snapshot(snap2_vol1_uuid)
    v.delete_snapshot(snap1_vol2_uuid)
    v.delete_snapshot(snap2_vol2_uuid)
    v.delete_snapshot(snap3_vol2_uuid)

    delete_volume(volume2_uuid)
    delete_volume(volume1_uuid)


@pytest.mark.skipif(not pytest.config.getoption("ebs"),
                    reason="--ebs was not specified")
def test_ebs_snapshot_backup():
    volume_uuid = create_volume(size=VOLUME_SIZE_SMALL,
                                name="ebs_volume",
                                driver=EBS)

    mount_volume_and_create_file(volume_uuid, "test-vol1-v1")
    snap1_uuid = v.create_snapshot("ebs_volume", "snap1")
    volume = v.inspect_volume("ebs_volume")
    snap1 = v.inspect_snapshot("snap1")
    assert snap1["UUID"] == snap1_uuid
    assert snap1["VolumeUUID"] == volume_uuid
    assert snap1["VolumeName"] == "ebs_volume"
    assert snap1["Name"] == "snap1"
    assert str(snap1["DriverInfo"]["Size"]) == VOLUME_SIZE_SMALL
    assert (snap1["DriverInfo"]["EBSVolumeID"] ==
            volume["DriverInfo"]["EBSVolumeID"])
    assert snap1["DriverInfo"]["Size"] == volume["DriverInfo"]["Size"]

    backup_url = v.create_backup(snap1_uuid)
    backup = v.inspect_backup(backup_url)
    assert backup["EBSVolumeID"] == volume["DriverInfo"]["EBSVolumeID"]
    assert backup["EBSSnapshotID"] == snap1["DriverInfo"]["EBSSnapshotID"]
    assert backup["Size"] == snap1["DriverInfo"]["Size"]

    v.delete_backup(backup_url)
    v.delete_snapshot("snap1")
    delete_volume(volume_uuid)


def create_delete_volume():
    uuid = v.create_volume(size=VOLUME_SIZE_6M)
    snap = v.create_snapshot(uuid)
    v.delete_snapshot(snap)
    v.delete_volume(uuid)


# uses default driver which is device mapper
def test_create_volume_in_parallel():
    threads = []
    for i in range(TEST_THREAD_COUNT):
        threads.append(threading.Thread(target=create_delete_volume))
        threads[i].start()

    for i in range(TEST_THREAD_COUNT):
        threads[i].join()


def test_create_volume_in_sequence():
    for i in range(TEST_LOOP_COUNT):
        create_delete_volume()


def compress_volume(volume_uuid):
    mountpoint = mount_volume(volume_uuid)
    zipfile = os.path.join(TEST_ROOT, volume_uuid)
    shutil.make_archive(zipfile, "zip", mountpoint)
    umount_volume(volume_uuid, mountpoint)
    return zipfile + ".zip"


def get_volume_checksum(volume_uuid, driver):
    f = ""
    if driver == VFS:
        f = compress_volume(volume_uuid)
    else:  # DM/EBS
        f = v.inspect_volume(volume_uuid)["DriverInfo"]["Device"]
    output = subprocess.check_output(["sha512sum", f]).decode()

    if driver == "VFS" and f != "":
        os.remove(f)
    return output.split(" ")[0]


def check_restore(origin_vol, restored_vol, driver):
    volume_checksum = get_volume_checksum(origin_vol, driver)
    restore_checksum = get_volume_checksum(restored_vol, driver)
    assert volume_checksum == restore_checksum


def test_backup_create_restore_only():
    process_restore_with_original_removed(VFS, VFS_DEST)
    process_restore_with_original_removed(DM, VFS_DEST)
    if test_ebs:
        process_restore_with_original_removed(EBS)


def process_restore_with_original_removed(driver, dest=""):
    volume1_uuid = create_volume(size=VOLUME_SIZE_BIG, driver=driver)
    mount_volume_and_create_file(volume1_uuid, "test-vol1-v1")
    snap1_vol1_uuid = v.create_snapshot(volume1_uuid)
    bak = v.create_backup(snap1_vol1_uuid, dest)
    volume1_checksum = get_volume_checksum(volume1_uuid, driver)
    delete_volume(volume1_uuid)

    if driver == DM:
        # cannot specify different size with backup
        with pytest.raises(subprocess.CalledProcessError):
            res_volume1_uuid = create_volume(VOLUME_SIZE_SMALL,
                                             "res-vol1",
                                             bak,
                                             driver=driver)

    res_volume1_uuid = create_volume(name="res-vol1",
                                     backup=bak,
                                     driver=driver)
    res_volume1_checksum = get_volume_checksum(res_volume1_uuid, driver)
    assert res_volume1_checksum == volume1_checksum
    delete_volume(res_volume1_uuid)

    v.delete_backup(bak)


def test_duplicate_backup():
    process_duplicate_backup_test(VFS_DEST, VFS)
    process_duplicate_backup_test(VFS_DEST, DM)


def process_duplicate_backup_test(dest, driver):
    volume_uuid = create_volume(size=VOLUME_SIZE_BIG, driver=driver)
    mount_volume_and_create_file(volume_uuid, "volume_snap_test")
    snap_uuid = v.create_snapshot(volume_uuid)
    volume_checksum = get_volume_checksum(volume_uuid, driver)

    bak1 = v.create_backup(snap_uuid, dest)
    bak2 = v.create_backup(snap_uuid, dest)

    res2 = create_volume(backup=bak2, driver=driver)
    res2_checksum = get_volume_checksum(res2, driver=driver)
    assert res2_checksum == volume_checksum

    v.delete_backup(bak2)

    res1 = create_volume(backup=bak1, driver=driver)
    res1_checksum = get_volume_checksum(res1, driver=driver)
    assert res1_checksum == volume_checksum

    v.delete_backup(bak1)
    delete_volume(res2)
    delete_volume(res1)
    delete_volume(volume_uuid)


def test_vfs_objectstore():
    vfs_objectstore_test(VFS)
    vfs_objectstore_test(DM)


def vfs_objectstore_test(driver):
    process_objectstore_test(VFS_DEST, driver)


@pytest.mark.skipif(not pytest.config.getoption("s3"),
                    reason="--s3 was not specified")
def test_s3_objectstore():
    s3_objectstore_test(VFS)
    s3_objectstore_test(DM)


def s3_objectstore_test(driver):
    process_objectstore_test(get_s3_dest(), driver)
    process_objectstore_test(get_s3_dest(S3_PATH), driver)


def get_s3_dest(path=""):
    region = os.environ[ENV_TEST_AWS_REGION]
    bucket = os.environ[ENV_TEST_AWS_BUCKET]
    return "s3://" + bucket + "@" + region + "/" + path


def unescape_url(url):
    return url.replace("\\u0026", "&").replace("u0026", "&")


def process_objectstore_test(dest, driver):
    # make sure objectstore is empty
    backups = v.list_backup(dest)
    assert len(backups) == 0

    # add volume to objectstore
    volume1_uuid = create_volume(VOLUME_SIZE_BIG, "volume1", driver=driver)
    volume1 = v.inspect_volume("volume1")
    volume2_uuid = create_volume(VOLUME_SIZE_SMALL, "volume2", driver=driver)

    with pytest.raises(subprocess.CalledProcessError):
        backups = v.list_backup(dest, volume1_uuid)

    # first snapshots
    snap1_vol1_uuid = v.create_snapshot(volume1_uuid, "snap1_vol1")
    snap1_vol1 = v.inspect_snapshot("snap1_vol1")
    snap1_vol1_bak = v.create_backup("snap1_vol1", dest)

    backups = v.list_backup(dest, volume1_uuid)
    assert len(backups) == 1
    backup = backups[unescape_url(snap1_vol1_bak)]
    assert backup["DriverName"] == driver
    assert backup["VolumeUUID"] == volume1["UUID"]
    assert backup["VolumeName"] == volume1["Name"]
    if "Size" in volume1["DriverInfo"]:
        assert backup["VolumeSize"] == volume1["DriverInfo"]["Size"]
    assert backup["VolumeCreatedAt"] == volume1["CreatedTime"]
    assert backup["SnapshotUUID"] == snap1_vol1["UUID"]
    assert backup["SnapshotName"] == snap1_vol1["Name"]
    assert backup["SnapshotCreatedAt"] == snap1_vol1["CreatedTime"]
    assert backup["CreatedTime"] != ""

    backup = v.inspect_backup(snap1_vol1_bak)
    assert backup["DriverName"] == driver
    assert backup["VolumeUUID"] == volume1["UUID"]
    assert backup["VolumeName"] == volume1["Name"]
    if "Size" in volume1["DriverInfo"]:
        assert backup["VolumeSize"] == volume1["DriverInfo"]["Size"]
    assert backup["VolumeCreatedAt"] == volume1["CreatedTime"]
    assert backup["SnapshotUUID"] == snap1_vol1["UUID"]
    assert backup["SnapshotName"] == snap1_vol1["Name"]
    assert backup["SnapshotCreatedAt"] == snap1_vol1["CreatedTime"]
    assert backup["CreatedTime"] != ""

    snap1_vol2_uuid = v.create_snapshot(volume2_uuid, "snap1_vol2")
    snap1_vol2_bak = v.create_backup("snap1_vol2", dest)

    # list snapshots
    backups = v.list_backup(dest, volume2_uuid)
    assert len(backups) == 1

    backup = v.inspect_backup(snap1_vol2_bak)
    assert backup["VolumeUUID"] == volume2_uuid
    assert backup["SnapshotUUID"] == snap1_vol2_uuid

    # second snapshots
    mount_volume_and_create_file(volume1_uuid, "test-vol1-v1")
    snap2_vol1_uuid = v.create_snapshot(volume1_uuid)
    snap2_vol1_bak = v.create_backup(snap2_vol1_uuid, dest)

    mount_volume_and_create_file(volume2_uuid, "test-vol2-v2")
    snap2_vol2_uuid = v.create_snapshot(volume2_uuid)
    snap2_vol2_bak = v.create_backup(snap2_vol2_uuid, dest)

    # list snapshots again
    backups = v.list_backup(dest)
    assert len(backups) == 4
    assert backups[unescape_url(snap1_vol1_bak)]["DriverName"] == driver
    assert backups[unescape_url(snap1_vol1_bak)]["VolumeUUID"] == volume1_uuid
    assert (backups[unescape_url(snap1_vol1_bak)]["SnapshotUUID"] ==
            snap1_vol1_uuid)
    assert backups[unescape_url(snap2_vol1_bak)]["DriverName"] == driver
    assert backups[unescape_url(snap2_vol1_bak)]["VolumeUUID"] == volume1_uuid
    assert (backups[unescape_url(snap2_vol1_bak)]["SnapshotUUID"] ==
            snap2_vol1_uuid)
    assert backups[unescape_url(snap1_vol2_bak)]["DriverName"] == driver
    assert backups[unescape_url(snap1_vol2_bak)]["VolumeUUID"] == volume2_uuid
    assert (backups[unescape_url(snap1_vol2_bak)]["SnapshotUUID"] ==
            snap1_vol2_uuid)
    assert backups[unescape_url(snap2_vol2_bak)]["DriverName"] == driver
    assert backups[unescape_url(snap2_vol2_bak)]["VolumeUUID"] == volume2_uuid
    assert (backups[unescape_url(snap2_vol2_bak)]["SnapshotUUID"] ==
            snap2_vol2_uuid)

    backups = v.list_backup(dest, volume1_uuid)
    assert len(backups) == 2
    assert backups[unescape_url(snap1_vol1_bak)]["VolumeUUID"] == volume1_uuid
    assert (backups[unescape_url(snap1_vol1_bak)]["SnapshotUUID"] ==
            snap1_vol1_uuid)
    assert backups[unescape_url(snap2_vol1_bak)]["VolumeUUID"] == volume1_uuid
    assert (backups[unescape_url(snap2_vol1_bak)]["SnapshotUUID"] ==
            snap2_vol1_uuid)

    backups = v.list_backup(dest, volume2_uuid)
    assert len(backups) == 2
    assert backups[unescape_url(snap1_vol2_bak)]["VolumeUUID"] == volume2_uuid
    assert (backups[unescape_url(snap1_vol2_bak)]["SnapshotUUID"] ==
            snap1_vol2_uuid)
    assert backups[unescape_url(snap2_vol2_bak)]["VolumeUUID"] == volume2_uuid
    assert (backups[unescape_url(snap2_vol2_bak)]["SnapshotUUID"] ==
            snap2_vol2_uuid)

    # restore snapshot
    res_volume1_uuid = create_volume(name="res-vol1", backup=snap2_vol1_bak,
                                     driver=driver)
    check_restore(volume1_uuid, res_volume1_uuid, driver)

    res_volume2_uuid = create_volume(backup=snap2_vol2_bak, driver=driver)
    check_restore(volume2_uuid, res_volume2_uuid, driver)

    # remove snapshots from objectstore
    v.delete_backup(snap2_vol1_bak)
    v.delete_backup(snap2_vol2_bak)

    # list snapshots again
    backups = v.list_backup(dest)
    assert len(backups) == 2
    assert backups[unescape_url(snap1_vol1_bak)]["DriverName"] == driver
    assert backups[unescape_url(snap1_vol1_bak)]["VolumeUUID"] == volume1_uuid
    assert (backups[unescape_url(snap1_vol1_bak)]["SnapshotUUID"] ==
            snap1_vol1_uuid)
    assert backups[unescape_url(snap1_vol2_bak)]["DriverName"] == driver
    assert backups[unescape_url(snap1_vol2_bak)]["VolumeUUID"] == volume2_uuid
    assert (backups[unescape_url(snap1_vol2_bak)]["SnapshotUUID"] ==
            snap1_vol2_uuid)

    backups = v.list_backup(dest, volume1_uuid)
    assert len(backups) == 1
    backup = backups[unescape_url(snap1_vol1_bak)]
    assert backup["DriverName"] == driver
    assert backup["VolumeUUID"] == volume1_uuid
    assert backup["SnapshotUUID"] == snap1_vol1_uuid

    backup = v.inspect_backup(snap1_vol1_bak)
    assert backup["DriverName"] == driver
    assert backup["VolumeUUID"] == volume1_uuid
    assert backup["SnapshotUUID"] == snap1_vol1_uuid

    backups = v.list_backup(dest, volume2_uuid)
    assert len(backups) == 1
    backup = backups[unescape_url(snap1_vol2_bak)]
    assert backup["DriverName"] == driver
    assert backup["VolumeUUID"] == volume2_uuid
    assert backup["SnapshotUUID"] == snap1_vol2_uuid

    backup = v.inspect_backup(snap1_vol2_bak)
    assert backup["DriverName"] == driver
    assert backup["VolumeUUID"] == volume2_uuid
    assert backup["SnapshotUUID"] == snap1_vol2_uuid

    # remove snapshots from objectstore
    v.delete_backup(snap1_vol2_bak)
    v.delete_backup(snap1_vol1_bak)

    v.delete_snapshot(snap1_vol1_uuid)
    v.delete_snapshot(snap2_vol1_uuid)
    v.delete_snapshot(snap1_vol2_uuid)
    v.delete_snapshot(snap2_vol2_uuid)

    delete_volume(volume1_uuid)
    delete_volume(volume2_uuid)
    delete_volume(res_volume1_uuid)
    delete_volume(res_volume2_uuid)


def test_cross_restore_error_checking():
    vfs_vol_uuid = create_volume(driver=VFS)
    vfs_snap_uuid = v.create_snapshot(vfs_vol_uuid)
    vfs_backup = v.create_backup(vfs_snap_uuid, VFS_DEST)

    dm_vol_uuid = create_volume(size=VOLUME_SIZE_SMALL, driver=DM)
    dm_snap_uuid = v.create_snapshot(dm_vol_uuid)
    dm_backup = v.create_backup(dm_snap_uuid, VFS_DEST)

    with pytest.raises(subprocess.CalledProcessError):
        create_volume(driver=VFS, backup=dm_backup)

    with pytest.raises(subprocess.CalledProcessError):
        create_volume(driver=DM, backup=vfs_backup)

    vfs_res = create_volume(driver=VFS, backup=vfs_backup)
    dm_res = create_volume(driver=DM, backup=dm_backup)

    delete_volume(vfs_vol_uuid)
    delete_volume(vfs_res)
    delete_volume(dm_vol_uuid)
    delete_volume(dm_res)
