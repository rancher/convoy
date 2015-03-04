package devmapper

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/devicemapper"
	"github.com/yasker/volmgr/drivers"
	"github.com/yasker/volmgr/utils"
	"os"
	"path/filepath"
	"strconv"
)

const (
	DRIVER_NAME           = "devicemapper"
	DEFAULT_THINPOOL_NAME = "rancher-volume-pool"
	DEFAULT_BLOCK_SIZE    = "4096"
	DM_DIR                = "/dev/mapper/"

	DM_DATA_DEV            = "dm.datadev"
	DM_METADATA_DEV        = "dm.metadatadev"
	DM_THINPOOL_NAME       = "dm.thinpoolname"
	DM_THINPOOL_BLOCK_SIZE = "dm.thinpoolblocksize"

	// as defined in device mapper thin provisioning
	BLOCK_SIZE_MIN        = 128
	BLOCK_SIZE_MAX        = 2097152
	BLOCK_SIZE_MULTIPLIER = 128
)

type Driver struct {
	home string
	*Device
}

type Device struct {
	Root              string
	DataDevice        string
	MetadataDevice    string
	ThinpoolDevice    string
	ThinpoolSize      uint64
	ThinpoolBlockSize uint32
}

func init() {
	drivers.Register(DRIVER_NAME, Init)
}

func verifyConfig(config map[string]string) (*Device, error) {
	dv := Device{
		DataDevice:     config[DM_DATA_DEV],
		MetadataDevice: config[DM_METADATA_DEV],
	}

	if dv.DataDevice == "" || dv.MetadataDevice == "" {
		return nil, fmt.Errorf("data device or metadata device unspecified")
	}

	if _, exists := config[DM_THINPOOL_NAME]; !exists {
		config[DM_THINPOOL_NAME] = DEFAULT_THINPOOL_NAME
	}
	dv.ThinpoolDevice = DM_DIR + config[DM_THINPOOL_NAME]

	if _, exists := config[DM_THINPOOL_BLOCK_SIZE]; !exists {
		config[DM_THINPOOL_BLOCK_SIZE] = DEFAULT_BLOCK_SIZE
	}

	blockSizeString := config[DM_THINPOOL_BLOCK_SIZE]
	blockSizeTmp, err := strconv.Atoi(blockSizeString)
	if err != nil {
		return nil, fmt.Errorf("Illegal block size specified")
	}
	blockSize := uint32(blockSizeTmp)

	if blockSize < BLOCK_SIZE_MIN || blockSize > BLOCK_SIZE_MAX || blockSize%BLOCK_SIZE_MULTIPLIER != 0 {
		return nil, fmt.Errorf("Block size must between %v and %v, and must be a multiple of %v",
			BLOCK_SIZE_MIN, BLOCK_SIZE_MAX, BLOCK_SIZE_MULTIPLIER)
	}

	dv.ThinpoolBlockSize = blockSize

	return &dv, nil
}

func Init(root string, config map[string]string) (drivers.Driver, error) {
	driverConfig := root + DRIVER_NAME + ".cfg"
	if _, err := os.Stat(driverConfig); err == nil {
		dev := Device{}
		err := utils.LoadConfig(driverConfig, &dev)
		d := &Driver{}
		if err != nil {
			return d, err
		}
		d.Device = &dev
		d.home = dev.Root
		return d, nil
	}

	dev, err := verifyConfig(config)

	dev.Root = root

	dataDev, err := os.Open(dev.DataDevice)
	if err != nil {
		return nil, err
	}
	defer dataDev.Close()

	metadataDev, err := os.Open(dev.MetadataDevice)
	if err != nil {
		return nil, err
	}
	defer metadataDev.Close()

	thinpSize, err := devicemapper.GetBlockDeviceSize(dataDev)
	if err != nil {
		return nil, err
	}
	dev.ThinpoolSize = thinpSize

	err = createPool(filepath.Base(dev.ThinpoolDevice), dataDev, metadataDev, dev.ThinpoolBlockSize)
	if err != nil {
		return nil, err
	}

	err = utils.SaveConfig(driverConfig, &dev)
	if err != nil {
		return nil, err
	}
	d := &Driver{
		Device: dev,
	}
	return d, nil
}

func createPool(poolName string, dataDev, metadataDev *os.File, blockSize uint32) error {
	err := devicemapper.CreatePool(poolName, dataDev, metadataDev, blockSize)
	if err != nil {
		return err
	}
	log.Debugln("Created pool /dev/mapper/" + poolName)

	return nil
}

func (d *Driver) Name() string {
	return DRIVER_NAME
}

func (d *Driver) CreateVolume(id, baseId string) error {
	return nil
}

func (d *Driver) DeleteVolume(id string) error {
	return nil
}

func (d *Driver) CreateSnapshot(id, volumeId string) error {
	return nil
}

func (d *Driver) DeleteSnapshot(id string) error {
	return nil
}

func (d *Driver) ExportSnapshot(id, path string, blockSize uint32) error {
	return nil
}

func (d *Driver) Info() error {
	// from sector count to byte
	blockSize := d.ThinpoolBlockSize * 512

	fmt.Println("\tworking directory:", d.Root)
	fmt.Println("\tdata device:", d.DataDevice)
	fmt.Println("\tmetadata device:", d.MetadataDevice)
	fmt.Println("\tthinpool:", d.ThinpoolDevice)
	fmt.Println("\tthinpool size:", d.ThinpoolSize)
	fmt.Println("\tthinpool block size:", blockSize)

	return nil
}
