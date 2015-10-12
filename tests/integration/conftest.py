#!/usr/bin/python

import pytest
def pytest_addoption(parser):
    parser.addoption("--s3", action="store_true",
            help="enable S3 test. You would be billed for this.")
    parser.addoption("--ebs", action="store_true",
            help="enable EBS test. Must be on EC2 instance. You would be billed for this")
    parser.addoption("--container", action="store_true",
            help="test against container instead of local binary")
