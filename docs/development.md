# Development guide

You should have a Linux box for Convoy development.

## Build Convoy

1. Environment: Ensure a Go environment, Docker, `git`, and the `libdevmapper-dev` packages are installed.
2. Download [latest](https://github.com/rancher/thin-provisioning-tools/releases) `convoy-pdata_tools` and put the binary in your `$PATH`.
3. Build:
```bash
go get github.com/rancher/convoy
cd $GOPATH/src/github.com/rancher/convoy
make
```

## Integration tests
1. Environment: Ensure python, [pytest](https://docs.pytest.org/en/latest/getting-started.html) and [start-stop-daemon](http://www.man7.org/linux/man-pages/man8/start-stop-daemon.8.html) are installed.
2. Run the tests
```bash
cd $GOPATH/src/github.com/rancher/convoy/tests/integration
sudo python -m pytest
```

#### Integration tests with AWS
__WARNING: You will be billed if you run the EBS tests or set the S3 environment variables to an AWS S3 object store.__

##### S3 and EBS credentials
By default, the S3 tests will use a [Minio](https://github.com/minio/minio) container with pre-configured credentials.
However, if you set any of the `CONVOY_TEST_...` environment variables, then it will use your existing S3 object store.

Valid S3 credentials are required for all S3 and EBS tests. To configure this, add the following lines to the root user's credential file `/root/.aws/credentials`:
```config
[default]
aws_access_key_id = <your_aws_access_key_id>
aws_secret_access_key = <your_secret_access_key>
```
See [here](https://github.com/aws/aws-sdk-go#configuring-credentials) for more details.

##### Custom S3 Backup Destination
In order to run S3 tests on your own S3 bucket, you need to set following environment variables. It's recommended to put them in your ~/.profile.
```bash
export CONVOY_TEST_AWS_REGION=us-west-2
export CONVOY_TEST_AWS_BUCKET=convoytest
```
If you want to use a custom S3 endpoint, set this too:
```bash
export CONVOY_TEST_S3_ENDPOINT=http://s3.example.com:9000/
```
And add the following line to `visudo` to enable sudo to keep the environment variable:
```
Defaults        env_keep += "CONVOY_TEST_*"
```

**Note: Make sure the bucket is empty before you run S3 tests.**

Then run all tests with the chosen S3 object store as the backup destination:
```bash
sudo python -m pytest
```
Or just the S3 test
```bash
sudo python -m pytest test_main.py::test_s3_objectstore
```

##### EBS test:
In order to run the EBS test, you must run your development box on EC2. Otherwise, it will error out.

You can run all tests, including EBS, with the following:
```bash
sudo python -m pytest --ebs
```

#### Test informations
All the test information will be stored in `/tmp/convoy_test/` and the log is stored in `/tmp/convoy_test/convoy.log`.
