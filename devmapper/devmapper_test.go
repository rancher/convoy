// +build linux,devmapper

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
	devCfg       = "driver_devicemapper.cfg"
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

	_, err := Init(devRoot, devCfg, config)
	require.NotNil(t, err)
	require.Equal(t, err.Error(), "data device or metadata device unspecified")

	config[DM_DATA_DEV] = dataDev
	config[DM_METADATA_DEV] = metadataDev
	config[DM_THINPOOL_BLOCK_SIZE] = "100"
	_, err = Init(devRoot, devCfg, config)
	require.NotNil(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "Block size must"))

	config[DM_THINPOOL_NAME] = "test_pool"
	delete(config, DM_THINPOOL_BLOCK_SIZE)

	driver, err := Init(devRoot, devCfg, config)
	require.Nil(t, err)

	newDriver, err := Init(devRoot, devCfg, config)
	require.Nil(t, err)

	drv1, ok := driver.(*Driver)
	require.True(t, ok)
	drv2, ok := newDriver.(*Driver)
	require.True(t, ok)

	if !reflect.DeepEqual(*drv1, *drv2) {
		t.Fatal("Fail to verify the information from driver config")
	}

	require.Equal(t, drv1.configFile, filepath.Join(devRoot, devCfg))

	require.Equal(t, drv1.DataDevice, dataDev)
	require.Equal(t, drv1.MetadataDevice, metadataDev)
}

func TestVolume(t *testing.T) {
	driver, err := Init(devRoot, devCfg, nil)
	require.Nil(t, err)

	drv := driver.(*Driver)
	lastDevID := drv.LastDevID
	volumeID := uuid.New()

	err = driver.CreateVolume(volumeID, "", volumeSize)
	require.Nil(t, err)

	require.Equal(t, drv.LastDevID, lastDevID+1)

	err = driver.CreateVolume(volumeID, "", volumeSize)
	require.NotNil(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "Already has volume with uuid"))

	volumeID2 := uuid.New()

	wrongVolumeSize := int64(13333333)
	err = driver.CreateVolume(volumeID2, "", wrongVolumeSize)
	require.NotNil(t, err)
	require.Equal(t, err.Error(), "Size must be multiple of block size")

	err = driver.CreateVolume(volumeID2, "", volumeSize)
	require.Nil(t, err)

	err = driver.ListVolume("", "")
	require.Nil(t, err)

	err = driver.ListVolume(volumeID, "")
	require.Nil(t, err)

	err = driver.DeleteVolume("123")
	require.NotNil(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "cannot find volume"))

	err = driver.DeleteVolume(volumeID2)
	require.Nil(t, err)

	err = driver.DeleteVolume(volumeID)
	require.Nil(t, err)
}

func TestSnapshot(t *testing.T) {
	driver, err := Init(devRoot, devCfg, nil)
	require.Nil(t, err)

	volumeID := uuid.New()
	err = driver.CreateVolume(volumeID, "", volumeSize)
	require.Nil(t, err)

	snapshotID := uuid.New()
	err = driver.CreateSnapshot(snapshotID, volumeID)
	require.Nil(t, err)

	err = driver.CreateSnapshot(snapshotID, volumeID)
	require.NotNil(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "Already has snapshot with uuid"))

	snapshotID2 := uuid.New()
	err = driver.CreateSnapshot(snapshotID2, volumeID)
	require.Nil(t, err)

	err = driver.DeleteSnapshot(snapshotID, volumeID)
	require.Nil(t, err)

	err = driver.DeleteSnapshot(snapshotID, volumeID)
	require.NotNil(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "cannot find snapshot"))

	err = driver.DeleteVolume(volumeID)
	require.NotNil(t, err)
	require.True(t, strings.HasSuffix(err.Error(), "delete snapshots first"))

	err = driver.DeleteSnapshot(snapshotID2, volumeID)
	require.Nil(t, err)

	err = driver.DeleteVolume(volumeID)
	require.Nil(t, err)
}
