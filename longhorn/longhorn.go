package longhorn

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/convoydriver"
	"github.com/rancher/convoy/util"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	rancherClient "github.com/rancher/go-rancher/client"
)

const (
	DRIVER_NAME        = "longhorn"
	DRIVER_CONFIG_FILE = "longhorn.cfg"

	VOLUME_CFG_PREFIX   = "volume_"
	LONGHORN_CFG_PREFIX = DRIVER_NAME + "_"
	CFG_POSTFIX         = ".json"

	SNAPSHOT_PATH = "snapshots"

	DEFAULT_VOLUME_SIZE = "10G"

	LH_RANCHER_URL         = "lh.rancherurl"
	LH_RANCHER_ACCESS_KEY  = "lh.rancheraccesskey"
	LH_RANCHER_SECRET_KEY  = "lh.ranchersecretkey"
	LH_DEFAULT_VOLUME_SIZE = "lh.defaultvolumesize"

	COMPOSE_VOLUME_NAME = "[VOLUME_NAME]"
	COMPOSE_VOLUME_SIZE = "[VOLUME_SIZE]"
	COMPOSE_SLAB_SIZE   = "[SLAB_SIZE]"
	COMPOSE_CONVOY      = "[CONVOY_CONTAINER]"
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "longhorn"})
)

type Driver struct {
	mutex  *sync.RWMutex
	client *rancherClient.RancherClient
	Device
}

type Device struct {
	Root              string
	DefaultVolumeSize int64
	RancherURL        string
	RancherAccessKey  string
	RancherSecretKey  string
}

func (dev *Device) ConfigFile() (string, error) {
	if dev.Root == "" {
		return "", fmt.Errorf("BUG: Invalid empty device config path")
	}
	return filepath.Join(dev.Root, DRIVER_CONFIG_FILE), nil
}

type Volume struct {
	UUID       string
	Size       int64
	MountPoint string
	StackID    string
	StackName  string

	configPath string
}

func (v *Volume) ConfigFile() (string, error) {
	if v.UUID == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume UUID")
	}
	if v.configPath == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume config path")
	}
	return filepath.Join(v.configPath, LONGHORN_CFG_PREFIX+VOLUME_CFG_PREFIX+v.UUID+CFG_POSTFIX), nil
}

func (d *Driver) blankVolume(id string) *Volume {
	return &Volume{
		configPath: d.Root,
		UUID:       id,
	}
}

func init() {
	convoydriver.Register(DRIVER_NAME, Init)
}

func Init(root string, config map[string]string) (convoydriver.ConvoyDriver, error) {
	dev := &Device{
		Root: root,
	}
	exists, err := util.ObjectExists(dev)
	if err != nil {
		return nil, err
	}
	if exists {
		if err := util.ObjectLoad(dev); err != nil {
			return nil, err
		}
	} else {
		if err := util.MkdirIfNotExists(root); err != nil {
			return nil, err
		}

		url := config[LH_RANCHER_URL]
		accessKey := config[LH_RANCHER_ACCESS_KEY]
		secretKey := config[LH_RANCHER_SECRET_KEY]

		if url == "" || accessKey == "" || secretKey == "" {
			return nil, fmt.Errorf("Missing required parameter. longhorn.rancher_url or longhorn.rancher_access_key or longhorn.rancher_secret_key")
		}

		if _, exists := config[LH_DEFAULT_VOLUME_SIZE]; !exists {
			config[LH_DEFAULT_VOLUME_SIZE] = DEFAULT_VOLUME_SIZE
		}
		volumeSize, err := util.ParseSize(config[LH_DEFAULT_VOLUME_SIZE])
		if err != nil || volumeSize == 0 {
			return nil, fmt.Errorf("Illegal default volume size specified")
		}

		dev = &Device{
			Root:              root,
			RancherURL:        url,
			RancherAccessKey:  accessKey,
			RancherSecretKey:  secretKey,
			DefaultVolumeSize: volumeSize,
		}
	}

	log.Debugf("Try to connect to Rancher server at %v", dev.RancherURL)
	client, err := rancherClient.NewRancherClient(&rancherClient.ClientOpts{
		Url:       dev.RancherURL,
		AccessKey: dev.RancherAccessKey,
		SecretKey: dev.RancherSecretKey,
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to establish connection to Rancher server")
	}

	if err := util.ObjectSave(dev); err != nil {
		return nil, err
	}
	d := &Driver{
		mutex:  &sync.RWMutex{},
		client: client,
		Device: *dev,
	}

	return d, nil
}

func (d *Driver) Name() string {
	return DRIVER_NAME
}

func (d *Driver) Info() (map[string]string, error) {
	return map[string]string{
		"Root":             d.Root,
		"RancherURL":       d.RancherURL,
		"RancherAccessKey": d.RancherAccessKey,
		"RancherSecretKey": d.RancherSecretKey,
	}, nil
}

func (d *Driver) VolumeOps() (convoydriver.VolumeOperations, error) {
	return d, nil
}

func getStackName(name string) string {
	return "Longhorn_" + name
}

func (d *Driver) CreateVolume(id string, opts map[string]string) error {
	size, err := util.ParseSize(opts[convoydriver.OPT_SIZE])
	if err != nil {
		return err
	}
	if size == 0 {
		size = d.DefaultVolumeSize
	}

	volume := d.blankVolume(id)
	volume.Size = size
	volume.StackName = "teststack"

	sizeString := strconv.FormatInt(size, 10)
	dockerCompose := DockerComposeTemplate
	dockerCompose = strings.Replace(dockerCompose, COMPOSE_VOLUME_NAME, id, -1)
	dockerCompose = strings.Replace(dockerCompose, COMPOSE_VOLUME_SIZE, sizeString, -1)
	dockerCompose = strings.Replace(dockerCompose, COMPOSE_SLAB_SIZE, sizeString, -1)
	dockerCompose = strings.Replace(dockerCompose, COMPOSE_CONVOY, "testcon", -1)
	rancherCompose := RancherComposeTemplate

	config := &rancherClient.Environment{
		Name:           volume.StackName,
		DockerCompose:  dockerCompose,
		RancherCompose: rancherCompose,
	}
	resp, err := d.client.Environment.Create(config)
	if err != nil {
		return err
	}
	volume.StackID = resp.Id
	return util.ObjectSave(volume)
}

func (d *Driver) DeleteVolume(id string, opts map[string]string) error {
	volume := d.blankVolume(id)

	if err := util.ObjectLoad(volume); err != nil {
		return err
	}

	if volume.MountPoint != "" {
		return fmt.Errorf("Cannot delete volume %v. It is still mounted", id)
	}

	env, err := d.client.Environment.ById(volume.StackID)
	if err != nil {
		return err
	}
	if err := d.client.Environment.Delete(env); err != nil {
		return err
	}
	return util.ObjectDelete(volume)
}

func (d *Driver) MountVolume(id string, opts map[string]string) (string, error) {
	return "", nil
}

func (d *Driver) UmountVolume(id string) error {
	return nil
}

func (d *Driver) MountPoint(id string) (string, error) {
	return "", nil
}

func (d *Driver) GetVolumeInfo(id string) (map[string]string, error) {
	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return nil, err
	}
	return map[string]string{
		"Size":      strconv.FormatInt(volume.Size, 10),
		"StackName": volume.StackName,
		"StackID":   volume.StackID,
	}, nil
}

func (d *Driver) ListVolume(opts map[string]string) (map[string]map[string]string, error) {
	return nil, nil
}

func (d *Driver) SnapshotOps() (convoydriver.SnapshotOperations, error) {
	return nil, fmt.Errorf("Longhorn doesn't support snapshot ops")
}

func (d *Driver) BackupOps() (convoydriver.BackupOperations, error) {
	return nil, fmt.Errorf("Longhorn doesn't support backup ops")
}
