package longhorn

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/util"
	"github.com/rancher/go-rancher-metadata/metadata"

	. "github.com/rancher/convoy/convoydriver"
	rancherClient "github.com/rancher/go-rancher/client"
)

const (
	DRIVER_NAME        = "longhorn"
	DRIVER_CONFIG_FILE = "longhorn.cfg"
	MOUNTS_DIR         = "mounts"
	DEV_DIR            = "/dev/longhorn/%s"

	VOLUME_CFG_PREFIX   = "volume_"
	LONGHORN_CFG_PREFIX = DRIVER_NAME + "_"
	CFG_POSTFIX         = ".json"

	SNAPSHOT_PATH = "snapshots"

	DEFAULT_VOLUME_SIZE = "10G"

	RANCHER_METADATA_URL = "http://rancher-metadata/2015-12-19"

	LH_RANCHER_URL         = "lh.rancherurl"
	LH_RANCHER_ACCESS_KEY  = "lh.rancheraccesskey"
	LH_RANCHER_SECRET_KEY  = "lh.ranchersecretkey"
	LH_DEFAULT_VOLUME_SIZE = "lh.defaultvolumesize"
	LH_CONTAINER_NAME      = "lh.containername"

	COMPOSE_VOLUME_NAME = "VOLUME_NAME"
	COMPOSE_VOLUME_SIZE = "VOLUME_SIZE"
	COMPOSE_SLAB_SIZE   = "SLAB_SIZE"
	COMPOSE_CONVOY      = "CONVOY_CONTAINER"

	AFFINITY_LABEL = "io.rancher.scheduler.affinity:container"
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "longhorn"})
)

type Driver struct {
	mutex         *sync.RWMutex
	client        *rancherClient.RancherClient
	mdClient      *metadata.Client
	containerName string
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
	Size         int64
	Name         string
	MountPoint   string
	PrepareForVM bool
	CreatedTime  string

	configPath string
}

func (v *Volume) Stack(driver *Driver) *Stack {
	sizeString := strconv.FormatInt(v.Size, 10)
	env := map[string]interface{}{
		COMPOSE_SLAB_SIZE:   sizeString,
		COMPOSE_VOLUME_NAME: v.Name,
		COMPOSE_VOLUME_SIZE: sizeString,
		COMPOSE_CONVOY:      driver.containerName,
	}

	return &Stack{
		Client:        driver.client,
		Name:          "longhorn-vol-" + strings.Replace(v.Name, "_", "-", -1),
		ExternalId:    "system://longhorn?name=" + v.Name,
		Template:      DockerComposeTemplate,
		Environment:   env,
		ContainerName: driver.containerName,
	}
}

func (v *Volume) ConfigFile() (string, error) {
	if v.Name == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume name")
	}
	if v.configPath == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume config path")
	}
	return filepath.Join(v.configPath, LONGHORN_CFG_PREFIX+VOLUME_CFG_PREFIX+v.Name+CFG_POSTFIX), nil
}

func (v *Volume) GetDevice() (string, error) {
	return fmt.Sprintf(DEV_DIR, v.Name), nil
}

func (v *Volume) GetMountOpts() []string {
	return []string{}
}

func (v *Volume) GenerateDefaultMountPoint() string {
	return filepath.Join(v.configPath, MOUNTS_DIR, v.Name)
}

func (d *Driver) blankVolume(name string) *Volume {
	return &Volume{
		configPath: d.Root,
		Name:       name,
	}
}

func init() {
	Register(DRIVER_NAME, Init)
}

func override(existing, newValue string) string {
	if newValue != "" {
		return newValue
	}
	return existing
}

func Init(root string, config map[string]string) (ConvoyDriver, error) {
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
		dev.RancherURL = override(dev.RancherURL, config[LH_RANCHER_URL])
		dev.RancherAccessKey = override(dev.RancherAccessKey, config[LH_RANCHER_ACCESS_KEY])
		dev.RancherSecretKey = override(dev.RancherSecretKey, config[LH_RANCHER_SECRET_KEY])
	} else {
		if err := util.MkdirIfNotExists(root); err != nil {
			return nil, err
		}

		url := config[LH_RANCHER_URL]
		accessKey := config[LH_RANCHER_ACCESS_KEY]
		secretKey := config[LH_RANCHER_SECRET_KEY]

		if url == "" || accessKey == "" || secretKey == "" {
			return nil, fmt.Errorf("Missing required parameter. lh.rancherurl or lh.rancheraccesskey or lh.ranchersecretkey")
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

	mdClient := metadata.NewClient(RANCHER_METADATA_URL)
	containerName := config[LH_CONTAINER_NAME]
	if containerName == "" {
		container, err := mdClient.GetSelfContainer()
		if err != nil {
			return nil, err
		}
		containerName = container.UUID
	}

	log.Debugf("Try to connect to Rancher server at %s [%s:%s]", dev.RancherURL, dev.RancherAccessKey, dev.RancherSecretKey)
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
		client:        client,
		mdClient:      mdClient,
		containerName: containerName,
		Device:        *dev,
		mutex:         &sync.RWMutex{},
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

func (d *Driver) VolumeOps() (VolumeOperations, error) {
	return d, nil
}

func (d *Driver) CreateVolume(req Request) error {
	id := req.Name
	opts := req.Options

	size, err := util.ParseSize(opts[OPT_SIZE])
	if err != nil {
		return err
	}
	if size == 0 {
		size = d.DefaultVolumeSize
	}

	volume := d.blankVolume(id)
	volume.Size = size
	volume.Name = opts[OPT_VOLUME_NAME]
	volume.PrepareForVM, err = strconv.ParseBool(opts[OPT_PREPARE_FOR_VM])
	volume.CreatedTime = util.Now()
	if err != nil {
		return err
	}

	stack := volume.Stack(d)

	if err := d.doCreateVolume(volume, stack, id, opts); err != nil {
		stack.Delete()
		return err
	}

	return nil
}

func (d *Driver) doCreateVolume(volume *Volume, stack *Stack, id string, opts map[string]string) error {
	// Doing find just to see if we are creating versus using an existing stack
	env, err := stack.Find()
	if err != nil {
		return err
	}

	// Always run create because it also ensures that things are active
	if _, err := stack.Create(); err != nil {
		return err
	}

	// If env was nil then we created stack so we need to format
	if env == nil {
		dev, _ := volume.GetDevice()
		if err := waitForDevice(dev); err != nil {
			return err
		}

		log.Infof("Formatting %s", dev)
		if _, err := util.Execute("mkfs.ext4", []string{dev}); err != nil {
			return err
		}
	}

	if err := volume.Stack(d).MoveController(); err != nil {
		log.Errorf("Failed to move controller to %s", d.containerName)
		return err
	}

	return util.ObjectSave(volume)
}

func (d *Driver) DeleteVolume(req Request) error {
	id := req.Name
	opts := req.Options

	volume := d.blankVolume(id)

	if err := util.ObjectLoad(volume); err != nil {
		return err
	}

	if volume.MountPoint != "" {
		return fmt.Errorf("Cannot delete volume %v. It is still mounted", id)
	}

	referenceOnly, _ := strconv.ParseBool(opts[OPT_REFERENCE_ONLY])
	if !referenceOnly {
		log.Debugf("Deleting stack for volume %v", id)
		if err := volume.Stack(d).Delete(); err != nil {
			return err
		}
	}

	return util.ObjectDelete(volume)
}

func waitForDevice(dev string) error {
	err := Backoff(5*time.Minute, fmt.Sprintf("Failed to find %s", dev), func() (bool, error) {
		if _, err := os.Stat(dev); err == nil {
			return true, nil
		}
		return false, nil
	})
	return err
}

func (d *Driver) MountVolume(req Request) (string, error) {
	id := req.Name
	opts := req.Options

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return "", err
	}

	dev, _ := volume.GetDevice()
	if err := waitForDevice(dev); err != nil {
		return "", err
	}

	mountPoint, err := util.VolumeMount(volume, opts[OPT_MOUNT_POINT], false)
	if err != nil {
		return "", err
	}

	if volume.PrepareForVM {
		if err := util.MountPointPrepareImageFile(mountPoint, volume.Size); err != nil {
			return "", err
		}
	}

	if err := util.ObjectSave(volume); err != nil {
		return "", err
	}

	return mountPoint, nil
}

func (d *Driver) UmountVolume(req Request) error {
	id := req.Name

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return err
	}

	if err := util.VolumeUmount(volume); err != nil {
		return err
	}

	return util.ObjectSave(volume)
}

func (d *Driver) MountPoint(req Request) (string, error) {
	volume := d.blankVolume(req.Name)
	return volume.GenerateDefaultMountPoint(), nil
}

func (d *Driver) GetVolumeInfo(id string) (map[string]string, error) {
	return d.getVolumeInfo(id, true)
}

func (d *Driver) getVolumeInfo(id string, filter bool) (map[string]string, error) {
	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return nil, err
	}

	vol := map[string]string{
		"Size":                  strconv.FormatInt(volume.Size, 10),
		OPT_PREPARE_FOR_VM:      strconv.FormatBool(volume.PrepareForVM),
		OPT_VOLUME_CREATED_TIME: volume.CreatedTime,
		OPT_VOLUME_NAME:         volume.Name,
	}

	if filter {
		forFilter := map[string]map[string]string{id: vol}
		d.filterThroughRancher(forFilter)
		if len(forFilter) == 0 {
			return nil, nil
		}
	}
	return vol, nil
}

func (d *Driver) ListVolume(opts map[string]string) (map[string]map[string]string, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	volumeIDs, err := d.listVolumeIDs()
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]string{}
	for _, id := range volumeIDs {
		volInfo, err := d.getVolumeInfo(id, false)
		if err != nil {
			continue
		}
		if volInfo != nil {
			result[id] = volInfo
		}
	}
	d.filterThroughRancher(result)
	return result, nil
}

func (device *Device) listVolumeIDs() ([]string, error) {
	return util.ListConfigIDs(device.Root, LONGHORN_CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_POSTFIX)
}

func (d *Driver) filterThroughRancher(inputVols map[string]map[string]string) error {
	stacks, err := d.mdClient.GetStacks()
	if err != nil {
		return err
	}

	con, err := d.mdClient.GetSelfContainer()
	if err != nil {
		return err
	}
	hostUUID := con.HostUUID
	fromRancher := map[string]bool{}
	for _, stack := range stacks {
		if strings.HasPrefix(stack.Name, "longhorn-vol") {
			onThisHost := false
			for _, service := range stack.Services {
				if service.Name == "controller" {
					for _, container := range service.Containers {
						if container.HostUUID == hostUUID {
							onThisHost = true
							break
						}
					}
				}
				if onThisHost {
					if lhmd := service.Metadata["longhorn"]; lhmd != nil {
						if m, ok := lhmd.(map[string]interface{}); ok {
							if n := m["volume_name"]; n != nil {
								if name, ok := n.(string); ok && name != "" {
									fromRancher[name] = true
								}
							}
						}
					}
				}
			}
		}
	}

	for k := range inputVols {
		if _, ok := fromRancher[k]; !ok {
			delete(inputVols, k)
		}
	}

	return nil
}

func (d *Driver) SnapshotOps() (SnapshotOperations, error) {
	return nil, fmt.Errorf("Longhorn doesn't support snapshot ops")
}

func (d *Driver) BackupOps() (BackupOperations, error) {
	return nil, fmt.Errorf("Longhorn doesn't support backup ops")
}
