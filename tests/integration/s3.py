#!/usr/bin/env python

import subprocess


MINIO_VERSION = "RELEASE.2018-11-30T03-56-59Z"


class S3Server:
    def __init__(self, access_key, secret_key, name="convoy_minio", port=9000):
        self.access_key = access_key
        self.secret_key = secret_key
        self.name = name
        self.port = port

    def start(self):
        cmd = [
            "docker", "run", "--detach",
            "--name", self.name,
            "--publish", "%d:9000" % self.port,
            "--env", "MINIO_ACCESS_KEY=" + self.access_key,
            "--env", "MINIO_SECRET_KEY=" + self.secret_key,
            "minio/minio:" + MINIO_VERSION, "server", "/data",
        ]
        return subprocess.check_call(cmd)

    def make_bucket(self, bucket):
        return subprocess.check_call(["docker", "exec", self.name,
                                     "sh", "-c", "mkdir -p /data/" + bucket])

    def stop(self):
        return subprocess.check_call(["docker", "rm", "--force",
                                     "--volumes", self.name])

    def connection_str(self):
        return "http://localhost:%d/" % self.port
