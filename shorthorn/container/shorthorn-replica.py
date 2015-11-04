#!/usr/bin/python

import argparse
import sys
import os.path
import urllib2
import time

# depends on rtslib-fb
from rtslib import FileIOStorageObject, FabricModule, Target, TPG,\
        NetworkPortal, LUN, NodeACL, MappedLUN, RTSRoot

from flask import Flask
from flask_restful import reqparse, Api, Resource

app = Flask(__name__)
api = Api(app)

CHAP_USERID = "convoy"
CHAP_PASSWORD = "shorthorn"

parser = reqparse.RequestParser()
parser.add_argument("target", help="iscsi target wwn", type=str, required=True)
parser.add_argument("file", help="file id", type=str)
parser.add_argument("dir", help="file path", type=str)
parser.add_argument("size", help="volume size", type=int)
parser.add_argument("initiator", help="iscsi initiator wwn", type=str)

def GetFileStorageObject(name):
    root = RTSRoot()
    l = list(root.storage_objects)
    for obj in l:
  	if obj.name == name:
	    return obj
    return None

def GetTarget(wwn):
    root = RTSRoot()
    l = list(root.targets)
    for obj in l:
	if obj.wwn == wwn:
	    return obj
    return None

def GetNodeACL(node_wwn):
    root = RTSRoot()
    l = list(root.node_acls)
    for obj in l:
	if obj.node_wwn == node_wwn:
	    return obj
    return None

class TargetResource(Resource):
    def post(self):
        args = parser.parse_args(strict = True)
        print "Target post: " + str(args)
	return TargetCreate(args)

    def delete(self):
        args = parser.parse_args(strict = True)
        print "Target delete: " + str(args)
	return TargetDelete(args)

class ACLResource(Resource):
    def post(self):
        args = parser.parse_args(strict = True)
        print "ACL add: " + str(args)
	return ACLAdd(args)

    def delete(self):
        args = parser.parse_args(strict = True)
        print "ACL remove " + str(args)
	return ACLRemove(args)

def TargetCreate(args):
    if (args.file == None) or (args.dir == None) or (args.size == None):
        return "missing required parameter", 400

    if not os.path.exists(args.dir):
        return "path " + args.dir + " doesn't exists", 400

    file_id = args.file
    file_path = os.path.join(args.dir, args.file + ".img")
    size = args.size

    if os.path.exists(file_path):
        print "warning: file " + file_path + " already exists"

    ip = urllib2.urlopen("http://rancher-metadata/2015-07-25/self/container/primary_ip").read()

    f = FileIOStorageObject(file_id, file_path, size)
    iscsi = FabricModule("iscsi")
    target = Target(iscsi, args.target)
    tpg = TPG(target, 1)
    portal = NetworkPortal(tpg, ip, 3260)
    lun = LUN(tpg, 0, f)

    if args.initiator != None:
        nodeacl = NodeACL(tpg, args.initiator)
        nodeacl.chap_userid = CHAP_USERID
        nodeacl.chap_password = CHAP_PASSWORD
        mlun = MappedLUN(nodeacl, 0, lun)
    tpg.enable = 1
    return args.target, 200

def TargetDelete(args):
    if args.file == None:
        return "missing require file parameter", 400
    target = GetTarget(args.target)
    if target is None:
        return "cannot find target " + args.target, 400
    f = GetFileStorageObject(args.file)
    if f is None:
        return "cannot find file storage object " + args.file, 400
    target.delete()
    f.delete()
    return "", 204

def ACLAdd(args):
    if args.initiator == None:
        return "missing required initiator name parameter", 400
    target = GetTarget(args.target)
    if target is None:
        return "cannot find target " + args.target, 400
    tpg = list(target.tpgs)[0]
    if tpg is None:
        return "cannot find tpg of target " + args.target, 400
    lun = list(tpg.luns)[0]
    if lun is None:
        return "cannot find lun of tpg of target " + args.target, 400

    nodeacl = NodeACL(tpg, args.initiator)
    nodeacl.chap_userid = CHAP_USERID
    nodeacl.chap_password = CHAP_PASSWORD
    mlun = MappedLUN(nodeacl, 0, lun)
    return "", 201

def ACLRemove(args):
    if args.initiator == None:
        return "missing required initiator name parameter", 400
    node_acl = GetNodeACL(args.initiator)
    if node_acl is None:
        return "cannot find node acl for initiator" + args.initiator, 400

    node_acl.delete()
    return "", 204

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("-t", "--target", help="iscsi target wwn",
            type=str)
    parser.add_argument("-f", "--file", help="file id", type=str)
    parser.add_argument("-d", "--dir", help="file path", type=str)
    parser.add_argument("-s", "--size", help="volume size", type=int)
    parser.add_argument("-i", "--initiator", help="iscsi initiator wwn",
            type=str)
    parser.add_argument("-D", "--daemon", help="start daemon only, don't init",
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
        if args.target == None:
            print "missing required parameter --target"
            sys.exit(1)
        msg, code = TargetCreate(args)
        print msg 
        if code == 400:
	    sys.exit(1)

    api.add_resource(TargetResource, '/v1/target')
    api.add_resource(ACLResource, '/v1/target/acl')

    app.run(host = ip, port = 3140, debug = True, use_reloader = False)

if __name__ == "__main__":
    main()
