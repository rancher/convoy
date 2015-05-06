package blockstores

import (
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/utils"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	BLOCKSTORE_BASE        = "rancher-blockstore"
	BLOCKSTORE_CONFIG_FILE = "blockstore.cfg"
	VOLUME_DIRECTORY       = "volumes"
	VOLUME_CONFIG_FILE     = "volume.cfg"
	VOLUME_SEPARATE_LAYER1 = 2
	VOLUME_SEPARATE_LAYER2 = 4
	SNAPSHOTS_DIRECTORY    = "snapshots"
	SNAPSHOT_CONFIG_PREFIX = "snapshot_"
	BLOCKS_DIRECTORY       = "blocks"
	BLOCK_SEPARATE_LAYER1  = 2
	BLOCK_SEPARATE_LAYER2  = 4
	DEFAULT_BLOCK_SIZE     = 2097152
)

type InitFunc func(configFile string, config map[string]string) (BlockStoreDriver, error)

type BlockStoreDriver interface {
	Kind() string
	FinalizeInit(configFile, id string) error
	FileExists(path, fileName string) bool
	FileSize(path, fileName string) int64
	MkDirAll(dirName string) error
	RemoveAll(name string) error
	Read(srcPath, srcFileName string, data []byte) error
	Write(data []byte, dstPath, dstFileName string) error
	List(path string) ([]string, error)
	CopyToPath(srcFileName string, path string) error
}

type Volume struct {
	Size           int64
	Base           string
	LastSnapshotID string
}

type BlockStore struct {
	UUID      string
	Kind      string
	BlockSize int64
}

type BlockMapping struct {
	Offset        int64
	BlockChecksum string
}

type SnapshotMap struct {
	ID     string
	Blocks []BlockMapping
}

var (
	initializers map[string]InitFunc
)

func init() {
	initializers = make(map[string]InitFunc)
}

func RegisterDriver(kind string, initFunc InitFunc) error {
	if _, exists := initializers[kind]; exists {
		return fmt.Errorf("%s has already been registered", kind)
	}
	initializers[kind] = initFunc
	return nil
}

func GetBlockStoreDriver(kind, configFile string, config map[string]string) (BlockStoreDriver, error) {
	if _, exists := initializers[kind]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", kind)
	}
	return initializers[kind](configFile, config)
}

func getDriverConfigFilename(root, kind, id string) string {
	return filepath.Join(root, id+"-"+kind+".cfg")
}

func getConfigFilename(root, id string) string {
	return filepath.Join(root, id+".cfg")
}

func loadConfigInBlockStore(path, name string, driver BlockStoreDriver, v interface{}) error {
	size := driver.FileSize(path, name)
	if size < 0 {
		return fmt.Errorf("cannot find %v/%v in blockstore", path, name)
	}
	data := make([]byte, size)
	if err := driver.Read(path, name, data); err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	return nil
}

func saveConfigInBlockStore(path, name string, driver BlockStoreDriver, v interface{}) error {
	j, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := driver.Write(j, path, name); err != nil {
		return err
	}
	return nil
}

func loadVolumeConfig(volumeID string, driver BlockStoreDriver) (*Volume, error) {
	v := &Volume{}
	path := getVolumePath(volumeID)
	file := VOLUME_CONFIG_FILE
	if err := loadConfigInBlockStore(path, file, driver, v); err != nil {
		return nil, err
	}
	return v, nil
}

func saveVolumeConfig(volumeID string, driver BlockStoreDriver, v *Volume) error {
	path := getVolumePath(volumeID)
	file := VOLUME_CONFIG_FILE
	if err := saveConfigInBlockStore(path, file, driver, v); err != nil {
		return err
	}
	return nil
}

func loadBlockStoreConfig(driver BlockStoreDriver) (*BlockStore, error) {
	b := &BlockStore{}
	path := BLOCKSTORE_BASE
	file := BLOCKSTORE_CONFIG_FILE
	if err := loadConfigInBlockStore(path, file, driver, b); err != nil {
		return nil, err
	}
	return b, nil
}

func saveBlockStoreConfig(driver BlockStoreDriver, b *BlockStore) error {
	path := BLOCKSTORE_BASE
	file := BLOCKSTORE_CONFIG_FILE
	if err := saveConfigInBlockStore(path, file, driver, b); err != nil {
		return err
	}
	return nil
}

func Register(root, kind string, config map[string]string) (string, int64, error) {
	driver, err := GetBlockStoreDriver(kind, "", config)
	if err != nil {
		return "", 0, err
	}

	var id string
	bs, err := loadBlockStoreConfig(driver)
	if err == nil {
		// BlockStore has already been created
		if bs.Kind != kind {
			return "", 0, fmt.Errorf("specific kind is different from config stored in blockstore")
		}
		id = bs.UUID
		log.Debug("Loaded blockstore cfg in blockstore: ", id)
		driver.FinalizeInit(getDriverConfigFilename(root, kind, id), id)
	} else {
		log.Debug("Failed to find existed blockstore cfg in blockstore")
		id = uuid.New()
		driver.FinalizeInit(getDriverConfigFilename(root, kind, id), id)

		basePath := filepath.Join(BLOCKSTORE_BASE, VOLUME_DIRECTORY)
		err = driver.MkDirAll(basePath)
		if err != nil {
			removeDriverConfigFile(root, kind, id)
			return "", 0, err
		}
		log.Debug("Created base directory of blockstore at ", basePath)

		bs = &BlockStore{
			UUID:      id,
			Kind:      kind,
			BlockSize: DEFAULT_BLOCK_SIZE,
		}

		if err := saveBlockStoreConfig(driver, bs); err != nil {
			return "", 0, err
		}
		log.Debug("Created blockstore cfg in blockstore", bs.UUID)
	}

	configFile := getConfigFilename(root, id)
	if err := utils.SaveConfig(configFile, bs); err != nil {
		return "", 0, err
	}
	log.Debug("Created local copy of ", configFile)
	log.Debug("Registered block store ", bs.UUID)
	return bs.UUID, bs.BlockSize, nil
}

func removeDriverConfigFile(root, kind, id string) error {
	configFile := getDriverConfigFilename(root, kind, id)
	if err := exec.Command("rm", "-f", configFile).Run(); err != nil {
		return err
	}
	log.Debug("Removed ", configFile)
	return nil
}

func removeConfigFile(root, id string) error {
	configFile := getConfigFilename(root, id)
	if err := exec.Command("rm", "-f", configFile).Run(); err != nil {
		return err
	}
	log.Debug("Removed ", configFile)
	return nil
}

func Deregister(root, id string) error {
	configFile := getConfigFilename(root, id)
	b := &BlockStore{}
	err := utils.LoadConfig(configFile, b)
	if err != nil {
		return err
	}

	err = removeDriverConfigFile(root, b.Kind, id)
	if err != nil {
		return err
	}
	err = removeConfigFile(root, id)
	if err != nil {
		return err
	}
	log.Debug("Deregistered block store ", id)
	return nil
}

func AddVolume(root, id, volumeID, base string, size int64) error {
	configFile := getConfigFilename(root, id)
	b := &BlockStore{}
	err := utils.LoadConfig(configFile, b)
	if err != nil {
		return err
	}

	driverConfigFile := getDriverConfigFilename(root, b.Kind, id)
	driver, err := GetBlockStoreDriver(b.Kind, driverConfigFile, nil)
	if err != nil {
		return err
	}

	volumePath := getVolumePath(volumeID)
	volumeCfg := VOLUME_CONFIG_FILE
	if driver.FileExists(volumePath, volumeCfg) {
		return fmt.Errorf("volume %v already exists in blockstore %v", volumeID, id)
	}

	if err := driver.MkDirAll(volumePath); err != nil {
		return err
	}
	if err := driver.MkDirAll(getSnapshotsPath(volumeID)); err != nil {
		return err
	}
	if err := driver.MkDirAll(getBlocksPath(volumeID)); err != nil {
		return err
	}
	log.Debug("Created volume directory")
	volume := Volume{
		Size:           size,
		Base:           base,
		LastSnapshotID: "",
	}

	if err := saveConfigInBlockStore(volumePath, volumeCfg, driver, &volume); err != nil {
		return err
	}
	log.Debug("Created volume configuration file in blockstore: ", filepath.Join(volumePath, volumeCfg))
	log.Debug("Added blockstore volume ", volumeID)

	return nil
}

func RemoveVolume(root, id, volumeID string) error {
	configFile := getConfigFilename(root, id)
	b := &BlockStore{}
	err := utils.LoadConfig(configFile, b)
	if err != nil {
		return err
	}

	driverConfigFile := getDriverConfigFilename(root, b.Kind, id)
	driver, err := GetBlockStoreDriver(b.Kind, driverConfigFile, nil)
	if err != nil {
		return err
	}

	volumePath := getVolumePath(volumeID)
	volumeCfg := VOLUME_CONFIG_FILE
	if !driver.FileExists(volumePath, volumeCfg) {
		return fmt.Errorf("volume %v doesn't exist in blockstore %v", volumeID, id)
	}

	volumeDir := getVolumePath(volumeID)
	err = driver.RemoveAll(volumeDir)
	if err != nil {
		return err
	}
	log.Debug("Removed volume directory in blockstore: ", volumeDir)
	log.Debug("Removed blockstore volume ", volumeID)

	return nil
}

func getVolumePath(volumeID string) string {
	volumeLayer1 := volumeID[0:VOLUME_SEPARATE_LAYER1]
	volumeLayer2 := volumeID[VOLUME_SEPARATE_LAYER1:VOLUME_SEPARATE_LAYER2]
	return filepath.Join(BLOCKSTORE_BASE, VOLUME_DIRECTORY, volumeLayer1, volumeLayer2, volumeID)
}

func getSnapshotsPath(volumeID string) string {
	return filepath.Join(getVolumePath(volumeID), SNAPSHOTS_DIRECTORY)
}

func getBlocksPath(volumeID string) string {
	return filepath.Join(getVolumePath(volumeID), BLOCKS_DIRECTORY)
}

func getBlockPathAndFileName(volumeID, checksum string) (string, string) {
	blockSubDirLayer1 := checksum[0:BLOCK_SEPARATE_LAYER1]
	blockSubDirLayer2 := checksum[BLOCK_SEPARATE_LAYER1:BLOCK_SEPARATE_LAYER2]
	path := filepath.Join(getBlocksPath(volumeID), blockSubDirLayer1, blockSubDirLayer2)
	fileName := checksum + ".blk"

	return path, fileName
}

func getSnapshotConfigName(id string) string {
	return SNAPSHOT_CONFIG_PREFIX + id + ".cfg"
}

func BackupSnapshot(root, snapshotID, volumeID, blockstoreID string, sDriver drivers.Driver) error {
	configFile := getConfigFilename(root, blockstoreID)
	b := &BlockStore{}
	err := utils.LoadConfig(configFile, b)
	if err != nil {
		return err
	}
	driverConfigFile := getDriverConfigFilename(root, b.Kind, blockstoreID)
	bsDriver, err := GetBlockStoreDriver(b.Kind, driverConfigFile, nil)
	if err != nil {
		return err
	}

	volume, err := loadVolumeConfig(volumeID, bsDriver)
	if err != nil {
		return err
	}

	if snapshotExists(snapshotID, volumeID, bsDriver) {
		return fmt.Errorf("snapshot already exists in blockstore!")
	}

	lastSnapshotID := volume.LastSnapshotID

	var lastSnapshotMap *SnapshotMap
	if lastSnapshotID != "" {
		if lastSnapshotID == snapshotID {
			//Generate full snapshot if the snapshot has been backed up last time
			lastSnapshotID = ""
			log.Debug("Would create full snapshot metadata")
		} else if !sDriver.HasSnapshot(lastSnapshotID, volumeID) {
			// It's possible that the snapshot in blockstore doesn't exist
			// in local storage
			lastSnapshotID = ""
			log.Debug("Cannot find last snapshot %v in local storage, would process with full backup", lastSnapshotID)
		} else {
			log.Debug("Loading last snapshot: ", lastSnapshotID)
			lastSnapshotMap, err = loadSnapshotMap(lastSnapshotID, volumeID, bsDriver)
			if err != nil {
				return err
			}
			log.Debug("Loaded last snapshot: ", lastSnapshotID)
		}
	}

	log.Debug("Generating snapshot metadata of ", snapshotID)
	delta, err := sDriver.CompareSnapshot(snapshotID, lastSnapshotID, volumeID)
	if err != nil {
		return err
	}
	if delta.BlockSize != b.BlockSize {
		return fmt.Errorf("Currently doesn't support different block sizes between blockstore and driver")
	}
	log.Debug("Generated snapshot metadata of ", snapshotID)

	log.Debug("Creating snapshot changed blocks of ", snapshotID)
	snapshotDeltaMap := &SnapshotMap{
		Blocks: []BlockMapping{},
	}
	if err := sDriver.OpenSnapshot(snapshotID, volumeID); err != nil {
		return err
	}
	defer sDriver.CloseSnapshot(snapshotID, volumeID)
	for _, d := range delta.Mappings {
		block := make([]byte, b.BlockSize)
		for i := int64(0); i < d.Size/delta.BlockSize; i++ {
			offset := d.Offset + i*delta.BlockSize
			err := sDriver.ReadSnapshot(snapshotID, volumeID, offset, block)
			if err != nil {
				return err
			}
			checksum := utils.GetChecksum(block)
			path, fileName := getBlockPathAndFileName(volumeID, checksum)
			if bsDriver.FileSize(path, fileName) >= 0 {
				blockMapping := BlockMapping{
					Offset:        offset,
					BlockChecksum: checksum,
				}
				snapshotDeltaMap.Blocks = append(snapshotDeltaMap.Blocks, blockMapping)
				log.Debugf("Found existed block match at %v/%v", path, fileName)
				continue
			}
			log.Debugf("Creating new block file at %v/%v", path, fileName)
			if err := bsDriver.MkDirAll(path); err != nil {
				return err
			}
			if err := bsDriver.Write(block, path, fileName); err != nil {
				return err
			}
			log.Debugf("Created new block file at %v/%v", path, fileName)

			blockMapping := BlockMapping{
				Offset:        offset,
				BlockChecksum: checksum,
			}
			snapshotDeltaMap.Blocks = append(snapshotDeltaMap.Blocks, blockMapping)
		}
	}
	log.Debug("Created snapshot changed blocks of", snapshotID)

	snapshotMap := mergeSnapshotMap(snapshotID, snapshotDeltaMap, lastSnapshotMap)

	if err := saveSnapshotMap(snapshotID, volumeID, bsDriver, snapshotMap); err != nil {
		return err
	}
	log.Debug("Created snapshot config of", snapshotID)
	volume.LastSnapshotID = snapshotID
	if err := saveVolumeConfig(volumeID, bsDriver, volume); err != nil {
		return err
	}
	log.Debug("Backed up snapshot ", snapshotID)

	return nil
}

func snapshotExists(snapshotID, volumeID string, bsDriver BlockStoreDriver) bool {
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)
	return bsDriver.FileExists(path, fileName)
}

func loadSnapshotMap(snapshotID, volumeID string, bsDriver BlockStoreDriver) (*SnapshotMap, error) {
	snapshotMap := SnapshotMap{}
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)

	if err := loadConfigInBlockStore(path, fileName, bsDriver, &snapshotMap); err != nil {
		return nil, err
	}
	return &snapshotMap, nil
}

func saveSnapshotMap(snapshotID, volumeID string, bsDriver BlockStoreDriver, snapshotMap *SnapshotMap) error {
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)
	if bsDriver.FileExists(path, fileName) {
		file := filepath.Join(path, fileName)
		log.Warnf("Snapshot configuration file %v already exists, would remove it\n", file)
		if err := bsDriver.RemoveAll(file); err != nil {
			return err
		}
	}
	if err := saveConfigInBlockStore(path, fileName, bsDriver, snapshotMap); err != nil {
		return err
	}
	return nil
}

func mergeSnapshotMap(snapshotID string, deltaMap, lastMap *SnapshotMap) *SnapshotMap {
	if lastMap == nil {
		deltaMap.ID = snapshotID
		return deltaMap
	}
	sMap := &SnapshotMap{
		ID:     snapshotID,
		Blocks: []BlockMapping{},
	}
	var d, l int
	for d, l = 0, 0; d < len(deltaMap.Blocks) && l < len(lastMap.Blocks); {
		dB := deltaMap.Blocks[d]
		lB := lastMap.Blocks[l]
		if dB.Offset == lB.Offset {
			sMap.Blocks = append(sMap.Blocks, dB)
			d++
			l++
		} else if dB.Offset < lB.Offset {
			sMap.Blocks = append(sMap.Blocks, dB)
			d++
		} else {
			//dB.Offset > lB.offset
			sMap.Blocks = append(sMap.Blocks, lB)
			l++
		}
	}

	if d == len(deltaMap.Blocks) {
		sMap.Blocks = append(sMap.Blocks, lastMap.Blocks[l:]...)
	} else {
		sMap.Blocks = append(sMap.Blocks, deltaMap.Blocks[d:]...)
	}

	return sMap
}

func RestoreSnapshot(root, srcSnapshotID, srcVolumeID, dstVolumeID, blockstoreID string, sDriver drivers.Driver) error {
	configFile := getConfigFilename(root, blockstoreID)
	b := &BlockStore{}
	err := utils.LoadConfig(configFile, b)
	if err != nil {
		return err
	}
	driverConfigFile := getDriverConfigFilename(root, b.Kind, blockstoreID)
	bsDriver, err := GetBlockStoreDriver(b.Kind, driverConfigFile, nil)
	if err != nil {
		return err
	}

	if _, err := loadVolumeConfig(srcVolumeID, bsDriver); err != nil {
		return fmt.Errorf("volume %v doesn't exist in blockstore %v", srcVolumeID, blockstoreID, err)
	}

	volDevName, err := sDriver.GetVolumeDevice(dstVolumeID)
	if err != nil {
		return err
	}
	volDev, err := os.Create(volDevName)
	if err != nil {
		return err
	}
	defer volDev.Close()

	snapshotMap, err := loadSnapshotMap(srcSnapshotID, srcVolumeID, bsDriver)
	if err != nil {
		return err
	}

	for _, block := range snapshotMap.Blocks {
		data := make([]byte, b.BlockSize)
		path, file := getBlockPathAndFileName(srcVolumeID, block.BlockChecksum)
		err := bsDriver.Read(path, file, data)
		if err != nil {
			return err
		}
		if _, err := volDev.WriteAt(data, block.Offset); err != nil {
			return err
		}
	}
	log.Debugf("Restored snapshot %v of volume %v to volume %v", srcSnapshotID, srcVolumeID, dstVolumeID)

	return nil
}

func RemoveSnapshot(root, snapshotID, volumeID, blockstoreID string) error {
	configFile := getConfigFilename(root, blockstoreID)
	b := &BlockStore{}
	err := utils.LoadConfig(configFile, b)
	if err != nil {
		return err
	}
	driverConfigFile := getDriverConfigFilename(root, b.Kind, blockstoreID)
	bsDriver, err := GetBlockStoreDriver(b.Kind, driverConfigFile, nil)
	if err != nil {
		return err
	}

	v, err := loadVolumeConfig(volumeID, bsDriver)
	if err != nil {
		return fmt.Errorf("cannot find volume %v in blockstore %v", volumeID, blockstoreID, err)
	}

	snapshotMap, err := loadSnapshotMap(snapshotID, volumeID, bsDriver)
	if err != nil {
		return err
	}
	discardBlockSet := make(map[string]bool)
	for _, blk := range snapshotMap.Blocks {
		discardBlockSet[blk.BlockChecksum] = true
	}
	discardBlockCounts := len(discardBlockSet)

	snapshotPath := getSnapshotsPath(volumeID)
	snapshotFile := getSnapshotConfigName(snapshotID)
	discardFile := filepath.Join(snapshotPath, snapshotFile)
	if err := bsDriver.RemoveAll(discardFile); err != nil {
		return err
	}
	log.Debugf("Removed snapshot config file %v on blockstore", discardFile)

	if snapshotID == v.LastSnapshotID {
		v.LastSnapshotID = ""
		if err := saveVolumeConfig(volumeID, bsDriver, v); err != nil {
			return err
		}
	}

	log.Debug("GC started")
	snapshots, err := getSnapshots(volumeID, bsDriver)
	if err != nil {
		return err
	}
	for snapshotID, _ := range snapshots {
		snapshotMap, err := loadSnapshotMap(snapshotID, volumeID, bsDriver)
		if err != nil {
			return err
		}
		for _, blk := range snapshotMap.Blocks {
			if _, exists := discardBlockSet[blk.BlockChecksum]; exists {
				delete(discardBlockSet, blk.BlockChecksum)
				discardBlockCounts--
				if discardBlockCounts == 0 {
					break
				}
			}
		}
		if discardBlockCounts == 0 {
			break
		}
	}

	for blk := range discardBlockSet {
		path, file := getBlockPathAndFileName(volumeID, blk)
		if err := bsDriver.RemoveAll(filepath.Join(path, file)); err != nil {
			return err
		}
		log.Debugf("Removed unused block %v for volume %v", blk, volumeID)
	}

	log.Debug("GC completed")
	log.Debug("Removed blockstore snapshot ", snapshotID)

	return nil
}

func getSnapshots(volumeID string, driver BlockStoreDriver) (map[string]bool, error) {
	fileList, err := driver.List(getSnapshotsPath(volumeID))
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	for _, f := range fileList {
		parts := strings.Split(f, "_")
		if len(parts) != 2 {
			return nil, fmt.Errorf("incorrect filename format:", f)
		}
		parts = strings.Split(parts[1], ".")
		if len(parts) != 2 {
			return nil, fmt.Errorf("incorrect filename format:", f)
		}
		result[parts[0]] = true
	}
	return result, nil
}

func listVolume(volumeID, snapshotID string, driver BlockStoreDriver) error {
	resp := api.VolumesResponse{
		Volumes: make(map[string]api.VolumeResponse),
	}

	snapshots, err := getSnapshots(volumeID, driver)
	if err != nil {
		// Volume doesn't exist
		api.ResponseOutput(resp)
		return nil
	}

	volumeResp := api.VolumeResponse{
		UUID:      volumeID,
		Snapshots: make(map[string]api.SnapshotResponse),
	}

	if snapshotID != "" {
		if _, exists := snapshots[snapshotID]; exists {
			volumeResp.Snapshots[snapshotID] = api.SnapshotResponse{
				UUID:       snapshotID,
				VolumeUUID: volumeID,
			}
		}
	} else {
		for s, _ := range snapshots {
			volumeResp.Snapshots[s] = api.SnapshotResponse{
				UUID:       s,
				VolumeUUID: volumeID,
			}
		}
	}
	resp.Volumes[volumeID] = volumeResp
	api.ResponseOutput(resp)
	return nil
}

func List(root, blockstoreID, volumeID, snapshotID string) error {
	configFile := getConfigFilename(root, blockstoreID)
	b := &BlockStore{}
	err := utils.LoadConfig(configFile, b)
	if err != nil {
		return err
	}
	driverConfigFile := getDriverConfigFilename(root, b.Kind, blockstoreID)
	bsDriver, err := GetBlockStoreDriver(b.Kind, driverConfigFile, nil)
	if err != nil {
		return err
	}
	return listVolume(volumeID, snapshotID, bsDriver)
}
