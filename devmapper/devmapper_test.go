// +build linux
package devmapper

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

const (
	dataDev     = "/dev/loop0"
	metadataDev = "/dev/loop1"
	poolName    = "test_pool"
	devRoot     = "/tmp/devmapper"
)

func setUp() error {
	cmd := exec.Command("mkdir", "-p", devRoot)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func tearDown() error {
	cmd := exec.Command("dmsetup", "remove", poolName)
	err := cmd.Run()
	if err != nil {
		return err
	}
	cmd = exec.Command("rm", "-rf", devRoot)
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func TestMain(m *testing.M) {
	err := setUp()
	if err != nil {
		fmt.Println("Failed to setup due to ", err)
		os.Exit(-1)
	}

	errCode := m.Run()

	err = tearDown()
	if err != nil {
		fmt.Println("Failed to tear down due to ", err)
		os.Exit(-1)
	}

	os.Exit(errCode)
}

func TestInit(t *testing.T) {
	config := make(map[string]string)

	_, err := Init(devRoot, config)
	if err == nil || err.Error() != "data device or metadata device unspecified" {
		t.Fatal("Expected error doesn't happened")
	}
	config[DM_DATA_DEV] = dataDev
	config[DM_METADATA_DEV] = metadataDev
	config[DM_THINPOOL_BLOCK_SIZE] = "100"
	_, err = Init(devRoot, config)
	if err == nil || !strings.HasPrefix(err.Error(), "Block size must") {
		t.Fatal("Expected error doesn't happened")
	}

	config[DM_THINPOOL_NAME] = "test_pool"
	delete(config, DM_THINPOOL_BLOCK_SIZE)

	driver, err := Init(devRoot, config)
	if err != nil {
		t.Fatal("Fail to init volmgr", err)
	}

	newDriver, err := Init(devRoot, config)
	if err != nil {
		t.Fatal("Fail to reload device mapper driver", err)
	}

	drv1, ok := driver.(*Driver)
	if !ok {
		t.Fatal("Fail to convert type")
	}
	drv2, ok := newDriver.(*Driver)
	if !ok {
		t.Fatal("Fail to convert type")
	}

	if !reflect.DeepEqual(*drv1, *drv2) {
		t.Fatal("Fail to verify the information from driver config")
	}

	if drv1.configFile != devRoot+"/devmapper.cfg" {
		t.Fatal("Failed driver.configFile")
	}

	if drv1.DataDevice != dataDev || drv1.MetadataDevice != metadataDev {
		t.Fatal("Failed device.DataDevice or dev.MetadataDevice check")
	}
}
