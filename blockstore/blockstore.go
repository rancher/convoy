package blockstore

import (
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/utils"
	"os"
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
	HASH_LEVEL             = 2
)

type InitFunc func(root, cfgName string, config map[string]string) (BlockStoreDriver, error)

type BlockStoreDriver interface {
	Kind() string
	FinalizeInit(root, cfgName, id string) error
	FileExists(filePath string) bool
	FileSize(filePath string) int64
	MkDirAll(dirName string) error
	Remove(name string) error //Would return error if it's not empty
	RemoveAll(name string) error
	Read(src string, data []byte) error
	Write(data []byte, dst string) error
	List(path string) ([]string, error)
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

func GetBlockStoreDriver(kind, root, cfgName string, config map[string]string) (BlockStoreDriver, error) {
	if _, exists := initializers[kind]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", kind)
	}
	return initializers[kind](root, cfgName, config)
}

func getDriverCfgName(kind, id string) string {
	return "blockstore_" + id + "_" + kind + ".cfg"
}

func getCfgName(id string) string {
	return "blockstore_" + id + ".cfg"
}

func loadConfigInBlockStore(filePath string, driver BlockStoreDriver, v interface{}) error {
	size := driver.FileSize(filePath)
	if size < 0 {
		return fmt.Errorf("cannot find %v in blockstore", filePath)
	}
	data := make([]byte, size)
	if err := driver.Read(filePath, data); err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	return nil
}

func saveConfigInBlockStore(filePath string, driver BlockStoreDriver, v interface{}) error {
	j, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := driver.Write(j, filePath); err != nil {
		return err
	}
	return nil
}

func loadVolumeConfig(volumeID string, driver BlockStoreDriver) (*Volume, error) {
	v := &Volume{}
	path := getVolumePath(volumeID)
	file := VOLUME_CONFIG_FILE
	if err := loadConfigInBlockStore(filepath.Join(path, file), driver, v); err != nil {
		return nil, err
	}
	return v, nil
}

func saveVolumeConfig(volumeID string, driver BlockStoreDriver, v *Volume) error {
	path := getVolumePath(volumeID)
	file := VOLUME_CONFIG_FILE
	if err := saveConfigInBlockStore(filepath.Join(path, file), driver, v); err != nil {
		return err
	}
	return nil
}

func loadRemoteBlockStoreConfig(driver BlockStoreDriver) (*BlockStore, error) {
	b := &BlockStore{}
	path := BLOCKSTORE_BASE
	file := BLOCKSTORE_CONFIG_FILE
	if err := loadConfigInBlockStore(filepath.Join(path, file), driver, b); err != nil {
		return nil, err
	}
	return b, nil
}

func saveRemoteBlockStoreConfig(driver BlockStoreDriver, b *BlockStore) error {
	path := BLOCKSTORE_BASE
	file := BLOCKSTORE_CONFIG_FILE
	if err := saveConfigInBlockStore(filepath.Join(path, file), driver, b); err != nil {
		return err
	}
	return nil
}

func Register(root, kind string, config map[string]string) (string, int64, error) {
	driver, err := GetBlockStoreDriver(kind, root, "", config)
	if err != nil {
		return "", 0, err
	}

	var id string
	bs, err := loadRemoteBlockStoreConfig(driver)
	if err == nil {
		// BlockStore has already been created
		if bs.Kind != kind {
			return "", 0, fmt.Errorf("specific kind is different from config stored in blockstore")
		}
		id = bs.UUID
		log.Debug("Loaded blockstore cfg in blockstore: ", id)
		driver.FinalizeInit(root, getDriverCfgName(kind, id), id)
	} else {
		log.Debug("Failed to find existed blockstore cfg in blockstore")
		id = uuid.New()
		driver.FinalizeInit(root, getDriverCfgName(kind, id), id)

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

		if err := saveRemoteBlockStoreConfig(driver, bs); err != nil {
			return "", 0, err
		}
		log.Debug("Created blockstore cfg in blockstore", bs.UUID)
	}

	if err := utils.SaveConfig(root, getCfgName(id), bs); err != nil {
		return "", 0, err
	}
	log.Debug("Created local copy of ", getCfgName(id))
	log.Debug("Registered block store ", bs.UUID)
	return bs.UUID, bs.BlockSize, nil
}

func removeDriverConfigFile(root, kind, id string) error {
	cfgName := getDriverCfgName(kind, id)
	if err := utils.RemoveConfig(root, cfgName); err != nil {
		return err
	}
	log.Debug("Removed ", cfgName)
	return nil
}

func removeConfigFile(root, id string) error {
	cfgName := getCfgName(id)
	if err := utils.RemoveConfig(root, cfgName); err != nil {
		return err
	}
	log.Debug("Removed ", cfgName)
	return nil
}

func Deregister(root, id string) error {
	b := &BlockStore{}
	err := utils.LoadConfig(root, getCfgName(id), b)
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
	b := &BlockStore{}
	err := utils.LoadConfig(root, getCfgName(id), b)
	if err != nil {
		return err
	}

	driver, err := GetBlockStoreDriver(b.Kind, root, getDriverCfgName(b.Kind, id), nil)
	if err != nil {
		return err
	}

	volumePath := getVolumePath(volumeID)
	volumeCfg := VOLUME_CONFIG_FILE
	volumeFile := filepath.Join(volumePath, volumeCfg)
	if driver.FileExists(volumeFile) {
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

	if err := saveConfigInBlockStore(volumeFile, driver, &volume); err != nil {
		return err
	}
	log.Debug("Created volume configuration file in blockstore: ", volumeFile)
	log.Debug("Added blockstore volume ", volumeID)

	return nil
}

// Used for cleanup remaining hashed directories
func removeAndCleanup(path string, driver BlockStoreDriver) error {
	if err := driver.RemoveAll(path); err != nil {
		return err
	}
	dir := path
	for i := 0; i < HASH_LEVEL; i++ {
		dir = filepath.Dir(dir)
		// If directory is not empty, then we don't need to continue
		if err := driver.Remove(dir); err != nil {
			break
		}
	}
	return nil
}

func RemoveVolume(root, id, volumeID string) error {
	b := &BlockStore{}
	err := utils.LoadConfig(root, getCfgName(id), b)
	if err != nil {
		return err
	}

	driver, err := GetBlockStoreDriver(b.Kind, root, getDriverCfgName(b.Kind, id), nil)
	if err != nil {
		return err
	}

	volumePath := getVolumePath(volumeID)
	volumeCfg := VOLUME_CONFIG_FILE
	volumeFile := filepath.Join(volumePath, volumeCfg)
	if !driver.FileExists(volumeFile) {
		return fmt.Errorf("volume %v doesn't exist in blockstore %v", volumeID, id)
	}

	volumeDir := getVolumePath(volumeID)
	if err := removeAndCleanup(volumeDir, driver); err != nil {
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

func getBlockFilePath(volumeID, checksum string) string {
	blockSubDirLayer1 := checksum[0:BLOCK_SEPARATE_LAYER1]
	blockSubDirLayer2 := checksum[BLOCK_SEPARATE_LAYER1:BLOCK_SEPARATE_LAYER2]
	path := filepath.Join(getBlocksPath(volumeID), blockSubDirLayer1, blockSubDirLayer2)
	fileName := checksum + ".blk"

	return filepath.Join(path, fileName)
}

func getSnapshotConfigName(id string) string {
	return SNAPSHOT_CONFIG_PREFIX + id + ".cfg"
}

func BackupSnapshot(root, snapshotID, volumeID, blockstoreID string, sDriver drivers.Driver) error {
	b := &BlockStore{}
	err := utils.LoadConfig(root, getCfgName(blockstoreID), b)
	if err != nil {
		return err
	}
	bsDriver, err := GetBlockStoreDriver(b.Kind, root, getDriverCfgName(b.Kind, blockstoreID), nil)
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
			blkFile := getBlockFilePath(volumeID, checksum)
			if bsDriver.FileSize(blkFile) >= 0 {
				blockMapping := BlockMapping{
					Offset:        offset,
					BlockChecksum: checksum,
				}
				snapshotDeltaMap.Blocks = append(snapshotDeltaMap.Blocks, blockMapping)
				log.Debugf("Found existed block match at %v", blkFile)
				continue
			}
			log.Debugf("Creating new block file at %v", blkFile)
			if err := bsDriver.MkDirAll(filepath.Dir(blkFile)); err != nil {
				return err
			}
			if err := bsDriver.Write(block, blkFile); err != nil {
				return err
			}
			log.Debugf("Created new block file at %v", blkFile)

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
	return bsDriver.FileExists(filepath.Join(path, fileName))
}

func loadSnapshotMap(snapshotID, volumeID string, bsDriver BlockStoreDriver) (*SnapshotMap, error) {
	snapshotMap := SnapshotMap{}
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)

	if err := loadConfigInBlockStore(filepath.Join(path, fileName), bsDriver, &snapshotMap); err != nil {
		return nil, err
	}
	return &snapshotMap, nil
}

func saveSnapshotMap(snapshotID, volumeID string, bsDriver BlockStoreDriver, snapshotMap *SnapshotMap) error {
	path := getSnapshotsPath(volumeID)
	fileName := getSnapshotConfigName(snapshotID)
	filePath := filepath.Join(path, fileName)
	if bsDriver.FileExists(filePath) {
		log.Warnf("Snapshot configuration file %v already exists, would remove it\n", filePath)
		if err := bsDriver.RemoveAll(filePath); err != nil {
			return err
		}
	}
	if err := saveConfigInBlockStore(filePath, bsDriver, snapshotMap); err != nil {
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
	b := &BlockStore{}
	err := utils.LoadConfig(root, getCfgName(blockstoreID), b)
	if err != nil {
		return err
	}
	bsDriver, err := GetBlockStoreDriver(b.Kind, root, getDriverCfgName(b.Kind, blockstoreID), nil)
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
		blkFile := getBlockFilePath(srcVolumeID, block.BlockChecksum)
		err := bsDriver.Read(blkFile, data)
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
	b := &BlockStore{}
	err := utils.LoadConfig(root, getCfgName(blockstoreID), b)
	if err != nil {
		return err
	}
	bsDriver, err := GetBlockStoreDriver(b.Kind, root, getDriverCfgName(b.Kind, blockstoreID), nil)
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
	for snapshotID := range snapshots {
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
		blkFile := getBlockFilePath(volumeID, blk)
		if err := removeAndCleanup(blkFile, bsDriver); err != nil {
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

	v, err := loadVolumeConfig(volumeID, driver)
	if err != nil {
		// Volume doesn't exist
		api.ResponseOutput(resp)
		return nil
	}

	snapshots, err := getSnapshots(volumeID, driver)
	if err != nil {
		// Volume doesn't exist
		api.ResponseOutput(resp)
		return nil
	}

	volumeResp := api.VolumeResponse{
		UUID:      volumeID,
		Base:      v.Base,
		Size:      v.Size,
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
		for s := range snapshots {
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

func ListVolume(root, blockstoreID, volumeID, snapshotID string) error {
	b := &BlockStore{}
	err := utils.LoadConfig(root, getCfgName(blockstoreID), b)
	if err != nil {
		return err
	}
	bsDriver, err := GetBlockStoreDriver(b.Kind, root, getDriverCfgName(b.Kind, blockstoreID), nil)
	if err != nil {
		return err
	}
	return listVolume(volumeID, snapshotID, bsDriver)
}
