// +build linux
package devmapper

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/require"
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
	volumeSize  = 1 << 27
)

func setUp() error {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stderr)

	cmd := exec.Command("mkdir", "-p", devRoot)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func tearDown() error {
	cmd := exec.Command("rm", "-rf", devRoot)
	err := cmd.Run()
	if err != nil {
		return err
	}
	cmd = exec.Command("dmsetup", "remove", poolName)
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
	require.NotNil(t, err)
	require.Equal(t, err.Error(), "data device or metadata device unspecified")

	config[DM_DATA_DEV] = dataDev
	config[DM_METADATA_DEV] = metadataDev
	config[DM_THINPOOL_BLOCK_SIZE] = "100"
	_, err = Init(devRoot, config)
	require.NotNil(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "Block size must"))

	config[DM_THINPOOL_NAME] = "test_pool"
	delete(config, DM_THINPOOL_BLOCK_SIZE)

	driver, err := Init(devRoot, config)
	require.Nil(t, err)

	newDriver, err := Init(devRoot, config)
	require.Nil(t, err)

	drv1, ok := driver.(*Driver)
	require.True(t, ok)
	drv2, ok := newDriver.(*Driver)
	require.True(t, ok)

	if !reflect.DeepEqual(*drv1, *drv2) {
		t.Fatal("Fail to verify the information from driver config")
	}

	require.Equal(t, drv1.configFile, devRoot+"/devicemapper.cfg")

	require.Equal(t, drv1.DataDevice, dataDev)
	require.Equal(t, drv1.MetadataDevice, metadataDev)
}

func TestVolume(t *testing.T) {
	driver, err := Init(devRoot, nil)
	require.Nil(t, err)

	drv := driver.(*Driver)
	lastDevId := drv.LastDevId
	volumeId := uuid.New()
	err = driver.CreateVolume(volumeId, "", volumeSize)
	require.Nil(t, err)

	require.Equal(t, drv.LastDevId, lastDevId+1)

	volumeId2 := uuid.New()

	err = driver.CreateVolume(volumeId2, "", volumeSize)
	require.Nil(t, err)

	err = driver.ListVolumes()
	require.Nil(t, err)

	err = driver.DeleteVolume(volumeId2)
	require.Nil(t, err)

	err = driver.DeleteVolume(volumeId)
	require.Nil(t, err)
}

func TestSnapshot(t *testing.T) {
	driver, err := Init(devRoot, nil)
	require.Nil(t, err)

	volumeId := uuid.New()
	err = driver.CreateVolume(volumeId, "", volumeSize)
	require.Nil(t, err)

	snapshotId := uuid.New()
	err = driver.CreateSnapshot(snapshotId, volumeId)
	require.Nil(t, err)

	snapshotId2 := uuid.New()
	err = driver.CreateSnapshot(snapshotId2, volumeId)
	require.Nil(t, err)

	err = driver.ListSnapshot("")
	require.Nil(t, err)

	err = driver.ListSnapshot(volumeId)
	require.Nil(t, err)

	err = driver.DeleteSnapshot(snapshotId, volumeId)
	require.Nil(t, err)

	err = driver.DeleteVolume(volumeId)
	require.NotNil(t, err)
	require.True(t, strings.HasSuffix(err.Error(), "delete snapshots first"))

	err = driver.DeleteSnapshot(snapshotId2, volumeId)
	require.Nil(t, err)

	err = driver.DeleteVolume(volumeId)
	require.Nil(t, err)

}
