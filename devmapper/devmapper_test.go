// +build linux
package devmapper

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	dataFile     = "data.vol"
	metadataFile = "metadata.vol"
	poolName     = "test_pool"
	devRoot      = "/tmp/devmapper"
	volumeSize   = 1 << 27
	maxThin      = 10000
)

var (
	dataDev     string
	metadataDev string
)

func setUp() error {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stderr)

	if err := exec.Command("mkdir", "-p", devRoot).Run(); err != nil {
		return err
	}

	if err := exec.Command("dd", "if=/dev/zero", "of="+filepath.Join(devRoot, dataFile), "bs=4096", "count=262114").Run(); err != nil {
		return err
	}

	if err := exec.Command("dd", "if=/dev/zero", "of="+filepath.Join(devRoot, metadataFile), "bs=4096", "count=10000").Run(); err != nil {
		return err
	}

	out, err := exec.Command("losetup", "-v", "-f", filepath.Join(devRoot, dataFile)).Output()
	if err != nil {
		return err
	}
	dataDev = strings.TrimSpace(strings.SplitAfter(string(out[:]), "device is")[1])

	out, err = exec.Command("losetup", "-v", "-f", filepath.Join(devRoot, metadataFile)).Output()
	if err != nil {
		return err
	}
	metadataDev = strings.TrimSpace(strings.SplitAfter(string(out[:]), "device is")[1])

	return nil
}

func tearDown() error {
	if err := exec.Command("dmsetup", "remove", poolName).Run(); err != nil {
		return err
	}
	if err := exec.Command("losetup", "-d", dataDev, metadataDev).Run(); err != nil {
		return err
	}
	if err := exec.Command("rm", "-rf", devRoot).Run(); err != nil {
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

	err = driver.CreateVolume(volumeId, "", volumeSize)
	require.NotNil(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "Already has volume with uuid"))

	volumeId2 := uuid.New()

	wrongVolumeSize := uint64(13333333)
	err = driver.CreateVolume(volumeId2, "", wrongVolumeSize)
	require.NotNil(t, err)
	require.Equal(t, err.Error(), "Size must be multiple of block size")

	err = driver.CreateVolume(volumeId2, "", volumeSize)
	require.Nil(t, err)

	err = driver.ListVolumes()
	require.Nil(t, err)

	err = driver.DeleteVolume("123")
	require.NotNil(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "cannot find volume"))

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

	err = driver.CreateSnapshot(snapshotId, volumeId)
	require.NotNil(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "Already has snapshot with uuid"))

	snapshotId2 := uuid.New()
	err = driver.CreateSnapshot(snapshotId2, volumeId)
	require.Nil(t, err)

	err = driver.ListSnapshot("")
	require.Nil(t, err)

	err = driver.ListSnapshot(volumeId)
	require.Nil(t, err)

	err = driver.DeleteSnapshot(snapshotId, volumeId)
	require.Nil(t, err)

	err = driver.DeleteSnapshot(snapshotId, volumeId)
	require.NotNil(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "cannot find snapshot"))

	err = driver.DeleteVolume(volumeId)
	require.NotNil(t, err)
	require.True(t, strings.HasSuffix(err.Error(), "delete snapshots first"))

	err = driver.DeleteSnapshot(snapshotId2, volumeId)
	require.Nil(t, err)

	err = driver.DeleteVolume(volumeId)
	require.Nil(t, err)
}
