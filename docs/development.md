# Development guide

You should have a Linux box, for Convoy development.

## Build Convoy

1. Environment: Ensure Go environment, mercurial and `libdevmapper-dev` package are installed.
2. Download [latest](https://github.com/rancher/thin-provisioning-tools/releases) convoy-pdata_tools and put the binary in your $PATH.
3. Build:
```
go get github.com/rancher/convoy
cd $GOPATH/src/github.com/rancher/convoy
make
```

## Integration tests
1. Environment: Ensure python and pytest are installed.
2. Run the tests
```
cd $GOPATH/src/github.com/rancher/convoy/tests/integration
sudo py.test
```

#### Integration tests with AWS
__WARNING: You would be billed for all AWS related tests, including S3 and EBS.__

##### AWS credentials
A valid AWS crdential is needed for all AWS related tests. Add following lines to `~/.aws/credentials`
```
[default]
aws_access_key_id = <your_aws_access_key_id>
aws_secret_access_key = <your_secret_access_key>
```
See [here](https://github.com/aws/aws-sdk-go#configuring-credentials) for more details.

##### S3 test:
In order to run S3 tests, you need to set following environment variables. It's recommended to put them in your ~/.profile
```
export CONVOY_TEST_AWS_REGION=us-west-2
export CONVOY_TEST_AWS_BUCKET=convoytest
```
And add following line to `visudo` to enable sudo to keep the environment variable:
```
Defaults        env_keep += "CONVOY_TEST_AWS*"
```

Make sure the bucket is empty before you run S3 tests.

Then run all the tests includes S3 objectstore as backup destination:
```
sudo py.test --s3
```
Or just S3 test
```
sudo py.test --s3 test_main.py::test_s3_objectstore
```

##### EBS test:
In order to run EBS test, you got to run your development box on EC2. Otherwise it would error out.

You can run all the tests including EBS as:
```
sudo py.test --ebs
```

#### Test informations
All the test information would be stored at `/tmp/convoy_test/`, and log is at `/tmp/convoy_test/convoy.log`.
