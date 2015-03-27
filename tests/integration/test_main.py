#!/usr/bin/python

import subprocess
import os
import json

ROOT_DIR = "/tmp/volmgr_test/volmgr"
DATA_DIR = "/tmp/volmgr_test/"
DATA_FILE = "data.vol"
METADATA_FILE = "metadata.vol"
DATA_DEVICE_SIZE = 1073618944 
METADATA_DEVICE_SIZE = 40960000
DD_BLOCK_SIZE = 4096
POOL_NAME = "volmgr_test_pool"
VOLMGR_CMDLINE = ["../../volmgr", "--debug", "--log=/tmp/volmgr.log",
    "--root=" + ROOT_DIR]

DM_DIR = "/dev/mapper"
DM_BLOCK_SIZE = 2097152

VOLUME_SIZE = 524288000

data_dev = ""
metadata_dev = ""

def setup_module():
    if not os.path.exists(DATA_DIR):
        os.makedirs(DATA_DIR)
        assert os.path.exists(DATA_DIR)

    data_file = os.path.join(DATA_DIR, DATA_FILE)
    metadata_file = os.path.join(DATA_DIR, METADATA_FILE)
    subprocess.check_call(["dd", "if=/dev/zero", "of=" + data_file,
            "bs=" + str(DD_BLOCK_SIZE), "count=" + str(DATA_DEVICE_SIZE /
            DD_BLOCK_SIZE)])
    assert os.path.exists(os.path.join(DATA_DIR, DATA_FILE))
    subprocess.check_call(["dd", "if=/dev/zero", "of=" + metadata_file,
            "bs=" + str(DD_BLOCK_SIZE), "count=" + str(METADATA_DEVICE_SIZE /
            DD_BLOCK_SIZE)])
    assert os.path.exists(os.path.join(DATA_DIR, METADATA_FILE))
    
    global data_dev
    data_dev = subprocess.check_output(["losetup", "-v", "-f",
            data_file]).strip().split(" ")[3]
    assert data_dev.startswith("/dev/loop")
    global metadata_dev
    metadata_dev = subprocess.check_output(["losetup", "-v", "-f",
            metadata_file]).strip().split(" ")[3]
    assert metadata_dev.startswith("/dev/loop")

def teardown_module():
    subprocess.check_call(["rm", "-rf", ROOT_DIR])

    subprocess.check_call(["dmsetup", "remove", POOL_NAME])
    subprocess.check_call(["losetup", "-d", data_dev, metadata_dev])

def test_init():
    subprocess.check_call(VOLMGR_CMDLINE + ["init", "--driver=devicemapper",
        "--driver-opts", "dm.datadev=" + data_dev,
	"--driver-opts", "dm.metadatadev=" + metadata_dev,
	"--driver-opts", "dm.thinpoolname=" + POOL_NAME])

def test_info():
    data = subprocess.check_output(VOLMGR_CMDLINE + ["info"])
    info = json.loads(data)
    assert info["Driver"] == "devicemapper"
    assert info["Root"] == os.path.join(ROOT_DIR, "devicemapper")
    assert info["DataDevice"] == data_dev
    assert info["MetadataDevice"] == metadata_dev
    assert info["ThinpoolDevice"] == os.path.join(DM_DIR, POOL_NAME)
    assert info["ThinpoolSize"] == DATA_DEVICE_SIZE
    assert info["ThinpoolBlockSize"] == DM_BLOCK_SIZE

