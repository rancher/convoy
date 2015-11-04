#!/usr/bin/python

import argparse
import sys
import subprocess
import os.path
import time
import requests
import urllib2

from flask import Flask
from flask_restful import reqparse, Api, Resource

app = Flask(__name__)
api = Api(app)

CHAP_USERID = "convoy"
CHAP_PASSWORD = "shorthorn"

ISCSIADM = ["nsenter", "--net=/host/proc/1/ns/net", "iscsiadm"]
MDADM = ["nsenter", "--mount=/host/proc/1/ns/mnt", "mdadm"]

parser = reqparse.RequestParser()
parser.add_argument("peers", help="replica peers' ip addresses, separate by comma",
            required=True)
parser.add_argument("device", help="raid device", required=True)

def GetInitiatorName():
    name = None
    with open("/etc/iscsi/initiatorname.iscsi") as f:
	lines = f.readlines()
	for line in lines:
	    if line.startswith("InitiatorName="):
	    	name = line.split("=")[1].strip()
	    	break
    if name == None:
    	print "Cannot find initiator name "
    	assert False
    return name

def ACLAddToPortal(ip):
    initiator = GetInitiatorName()
    url = "http://" + ip + ":3140/v1/target/acl"
    payload = {"initiator" : initiator}
    print payload
    response = requests.post(url, params=payload)
    return response.text, response.status_code

def ACLRemoveFromPortal(ip):
    initiator = GetInitiatorName()
    url = "http://" + ip + ":3140/v1/target/acl"
    payload = {"initiator" : initiator}
    print payload
    response = requests.delete(url, params=payload)
    return response.text, response.status_code

def TargetGet(ip):
    result = subprocess.check_output(ISCSIADM + ["-m", "discovery", "-t",
		"sendtargets", "-p", ip])
    target = None
    for line in result.split('\n'):
	if line.startswith(ip + ":3260"):
    	    target = line.split(" ")[1].strip()
            break
    if target == None:
    	print "Cannot find target from ip " + ip
    	assert False
    return target

def TargetLogin(ip):
    target = TargetGet(ip)
    if target == None:
	return None
    print target
    msg, code = ACLAddToPortal(ip)
    print msg
    if code >= 400:
	return None
    subprocess.check_call(ISCSIADM + ["-m", "node", "--targetname",  target,
		"--op", "update",
		"--name", "node.session.auth.authmethod",
		"--value", "CHAP"])
    subprocess.check_call(ISCSIADM + ["-m", "node", "--targetname",  target,
		"--op", "update",
		"--name", "node.session.auth.username",
		"--value", CHAP_USERID])
    subprocess.check_call(ISCSIADM + ["-m", "node", "--targetname",  target,
		"--op", "update",
		"--name", "node.session.auth.password",
		"--value", CHAP_PASSWORD])
    output = subprocess.check_output(ISCSIADM + ["-m", "node", "--targetname",  target,
		"--login"])

    symdev = "/dev/disk/by-path/ip-" + ip + ":3260-iscsi-" + target + "-lun-0"
    c = 0
    while (not os.path.exists(symdev)) and c < 20:
	time.sleep(0.1)
	c += 1
    if not os.path.exists(symdev):
	print "Wait for device creation timout! " + symdev
	return None
    dev = os.path.realpath(symdev)
    return dev

def TargetLogout(ip):
    target = TargetGet(ip)
    print target
    msg, code = ACLRemoveFromPortal(ip)
    if code >= 400:
	return msg, code
    output = subprocess.check_output(ISCSIADM + ["-m", "node", "--targetname",  target,
		"--logout"])
    return "", 200

def RaidCreate(mddev, devices):
    cmd = MDADM + ["--create", mddev,
		"--verbose", "--run",
		"--level", "mirror",
		"--raid-devices", "2",
 		devices[0], devices[1]]
    print " ".join(cmd)
    subprocess.check_call(cmd)

def RaidDelete(mddev):
    subprocess.check_call(MDADM + ["--stop", mddev])
    if os.path.exists(mddev):
        if not os.path.exists(os.path.realpath(mdadm)):
            os.remove(mddev)
        else:
            subprocess.check_call(MDADM + ["--remove", mddev])

    for dev in devices:
        subprocess.check_call(MDADM + ["--zero-superblock", dev])

class ControllerResource(Resource):
    def get(self):
        global mddev
	return mddev, 200

    def post(self):
        args = parser.parse_args(strict = True)
        print "Controller post: " + str(args)
	return ControllerSetup(args)

    def delete(self):
	return ControllerTeardown()

def ControllerSetup(args):
    global peers
    peers = args.peers.split(",")
    if len(peers) != 2:
        return "only support 2 peers", 400

    global devices
    devices=[]
    for ip in peers:
        dev = TargetLogin(ip)
	if dev == None:
	    return "Fail to login", 400
        print dev
        devices.append(dev)

    #print "Created devices: " + " ".join(devices)
    print devices

    if len(devices) != 2:
        return "only support 2 devices, but have " + " ".join(devices), 400

    global mddev
    devname = args.device
    mddev = "/dev/md/" + devname
    #if not mddev.startswith("/dev/md"):
    #   return "device " + mddev + " must start with /dev/md", 400
    if os.path.exists(mddev):
        return "device " + mddev + " already exists", 400
    RaidCreate(mddev, devices)
    return "create complete", 200

def ControllerTeardown():
    global mddev
    RaidDelete(mddev)

    global peers
    for ip in peers:
        msg, code = TargetLogout(ip)
        if code == 400:
            return msg, code
    return "delete complete", 200

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("-p", "--peers", help="replica peers ips, separate by comma")
    parser.add_argument("-d", "--device", help="raid device name")
    parser.add_argument("-D", "--daemon", help="start daemon only",
            action="store_true")

    args = parser.parse_args()
    print args

    ip = None
    for i in range(0, 10):
        try:
            ip = urllib2.urlopen("http://rancher-metadata/2015-07-25/self/container/primary_ip").read()
        except urllib2.URLError:
            # rancher probably not ready yet, wait for it
            print "Waiting for connect to Rancher network"
            time.sleep(1)
            continue
        break
    if ip == None:
        print "Cannot get Rancher management IP"
        sys.exit(1)

    if not args.daemon:
        if args.device == None:
            print "missing required parameter --device"
            sys.exit(1)
        if args.peers == None:
            print "missing required parameter --peers"
            sys.exit(1)
        msg, code = ControllerSetup(args)
        print msg
        if code == 400:
            sys.exit(1)

    api.add_resource(ControllerResource, '/v1/controller')

    ip = urllib2.urlopen("http://rancher-metadata/2015-07-25/self/container/primary_ip").read()
    app.run(host = ip, port = 3140, debug = True, use_reloader = False)

if __name__ == "__main__":
    subprocess.check_call(["mount", "--rbind", "/host/dev", "/dev"])
    main()
