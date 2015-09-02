#!/usr/bin/python

import pytest
def pytest_addoption(parser):
    parser.addoption("--s3", action="store_true",
            help="enable S3 test")
