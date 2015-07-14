package objectstore

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/drivers"
	"github.com/rancher/rancher-volume/util"
	"io"
	"os"
	"path/filepath"
	"strings"

	. "github.com/rancher/rancher-volume/logging"
)

const (
	DEFAULT_BLOCK_SIZE = 2097152
)

type InitFunc func(root, cfgName string, config map[string]string) (ObjectStoreDriver, error)

type ObjectStoreDriver interface {
	Kind() string
	FinalizeInit(root, cfgName, id string) error
	FileExists(filePath string) bool
	FileSize(filePath string) int64
	Remove(names ...string) error
	Read(src string) (io.ReadCloser, error) // Caller needs to close
	Write(dst string, rs io.ReadSeeker) error
	List(path string) ([]string, error)
	Upload(src, dst string) error
	Download(src, dst string) error
}

type Volume struct {
	UUID           string
	Name           string
	Size           int64
	LastSnapshotID string
}

type ObjectStore struct {
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

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "objectstore"})
)

func generateError(fields logrus.Fields, format string, v ...interface{}) error {
	return ErrorWithFields("objectstore", fields, format, v)
}

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

func GetObjectStoreDriver(kind, root, cfgName string, config map[string]string) (ObjectStoreDriver, error) {
	if _, exists := initializers[kind]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", kind)
	}
	return initializers[kind](root, cfgName, config)
}

func Register(root, kind string, config map[string]string) (*ObjectStore, error) {
	driver, err := GetObjectStoreDriver(kind, root, "", config)
	if err != nil {
		return nil, err
	}

	var id string
	bs, err := loadRemoteObjectStoreConfig(driver)
	if err == nil {
		// ObjectStore has already been created
		if bs.Kind != kind {
			return nil, generateError(logrus.Fields{
				LOG_FIELD_OBJECTSTORE: bs.UUID,
				LOG_FIELD_KIND:        bs.Kind,
			}, "Specific kind is different from config stored in objectstore")
		}
		id = bs.UUID
		log.Debug("Loaded objectstore cfg in objectstore: ", id)
		driver.FinalizeInit(root, getDriverCfgName(kind, id), id)
	} else {
		log.Debug("Cannot load existed objectstore cfg in objectstore, create a new one: ", err.Error())
		id = uuid.New()
		driver.FinalizeInit(root, getDriverCfgName(kind, id), id)

		bs = &ObjectStore{
			UUID:      id,
			Kind:      kind,
			BlockSize: DEFAULT_BLOCK_SIZE,
		}

		if err := saveRemoteObjectStoreConfig(driver, bs); err != nil {
			return nil, err
		}
		log.Debug("Created objectstore cfg in objectstore", bs.UUID)
	}

	if err := util.SaveConfig(root, getCfgName(id), bs); err != nil {
		return nil, err
	}
	log.Debug("Created local copy of ", getCfgName(id))
	log.Debug("Registered block store ", bs.UUID)
	return bs, nil
}

func Deregister(root, id string) error {
	b := &ObjectStore{}
	err := util.LoadConfig(root, getCfgName(id), b)
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

func loadObjectStoreConfig(root, objectstoreUUID string) (*ObjectStore, error) {
	b := &ObjectStore{}
	err := util.LoadConfig(root, getCfgName(objectstoreUUID), b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func getObjectStoreCfgAndDriver(root, objectstoreUUID string) (*ObjectStore, ObjectStoreDriver, error) {
	b, err := loadObjectStoreConfig(root, objectstoreUUID)
	if err != nil {
		return nil, nil, err
	}

	driver, err := GetObjectStoreDriver(b.Kind, root, getDriverCfgName(b.Kind, objectstoreUUID), nil)
	if err != nil {
		return nil, nil, err
	}
	log.Debug("Loaded configure for objectstore ", objectstoreUUID)
	return b, driver, nil
}

func VolumeExists(root, volumeUUID, objectstoreUUID string) bool {
	_, driver, err := getObjectStoreCfgAndDriver(root, objectstoreUUID)
	if err != nil {
		return false
	}

	return driver.FileExists(getVolumeFilePath(volumeUUID))
}

func AddVolume(root, id, volumeID, volumeName string, size int64) error {
	_, driver, err := getObjectStoreCfgAndDriver(root, id)
	if err != nil {
		return err
	}

	volumeFile := getVolumeFilePath(volumeID)
	if driver.FileExists(volumeFile) {
		log.Debugf("Volume %v already exists in objectstore %v, ignore the command", volumeID, id)
		return nil
	}

	volume := &Volume{
		UUID:           volumeID,
		Name:           volumeName,
		Size:           size,
		LastSnapshotID: "",
	}

	if err := saveVolumeConfig(volumeID, driver, volume); err != nil {
		log.Error("Fail add volume ", volumeID)
		return err
	}
	log.Debug("Added objectstore volume ", volumeID)

	return nil
}

func RemoveVolume(root, id, volumeID string) error {
	_, driver, err := getObjectStoreCfgAndDriver(root, id)
	if err != nil {
		return err
	}

	volumePath := getVolumePath(volumeID)
	volumeCfg := VOLUME_CONFIG_FILE
	volumeFile := filepath.Join(volumePath, volumeCfg)
	if !driver.FileExists(volumeFile) {
		return fmt.Errorf("Volume %v doesn't exist in objectstore %v", volumeID, id)
	}

	volumeDir := getVolumePath(volumeID)
	if err := driver.Remove(volumeDir); err != nil {
		return err
	}
	log.Debug("Removed volume directory in objectstore: ", volumeDir)
	log.Debug("Removed objectstore volume ", volumeID)

	return nil
}

func BackupSnapshot(root, snapshotID, volumeID, objectstoreID string, sDriver drivers.Driver) error {
	b, bsDriver, err := getObjectStoreCfgAndDriver(root, objectstoreID)
	if err != nil {
		return err
	}

	volume, err := loadVolumeConfig(volumeID, bsDriver)
	if err != nil {
		return err
	}

	if snapshotExists(snapshotID, volumeID, bsDriver) {
		return generateError(logrus.Fields{
			LOG_FIELD_SNAPSHOT:    snapshotID,
			LOG_FIELD_VOLUME:      volumeID,
			LOG_FIELD_OBJECTSTORE: objectstoreID,
		}, "Snapshot already exists in objectstore!")
	}

	lastSnapshotID := volume.LastSnapshotID

	var lastSnapshotMap *SnapshotMap
	if lastSnapshotID != "" {
		if lastSnapshotID == snapshotID {
			//Generate full snapshot if the snapshot has been backed up last time
			lastSnapshotID = ""
			log.Debug("Would create full snapshot metadata")
		} else if !sDriver.HasSnapshot(lastSnapshotID, volumeID) {
			// It's possible that the snapshot in objectstore doesn't exist
			// in local storage
			lastSnapshotID = ""
			log.WithFields(logrus.Fields{
				LOG_FIELD_REASON:   LOG_REASON_FALLBACK,
				LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
				LOG_FIELD_SNAPSHOT: lastSnapshotID,
				LOG_FIELD_VOLUME:   volumeID,
			}).Debug("Cannot find last snapshot in local storage, would process with full backup")
		} else {
			log.WithFields(logrus.Fields{
				LOG_FIELD_REASON:   LOG_REASON_START,
				LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
				LOG_FIELD_EVENT:    LOG_EVENT_LOAD,
				LOG_FIELD_SNAPSHOT: lastSnapshotID,
			}).Debug("Loading last snapshot")
			lastSnapshotMap, err = loadSnapshotMap(lastSnapshotID, volumeID, bsDriver)
			if err != nil {
				return err
			}
			log.WithFields(logrus.Fields{
				LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
				LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
				LOG_FIELD_EVENT:    LOG_EVENT_LOAD,
				LOG_FIELD_SNAPSHOT: lastSnapshotID,
			}).Debug("Loaded last snapshot")
		}
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:        LOG_REASON_START,
		LOG_FIELD_OBJECT:        LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_EVENT:         LOG_EVENT_COMPARE,
		LOG_FIELD_SNAPSHOT:      snapshotID,
		LOG_FIELD_LAST_SNAPSHOT: lastSnapshotID,
	}).Debug("Generating snapshot changed blocks metadata")
	delta, err := sDriver.CompareSnapshot(snapshotID, lastSnapshotID, volumeID)
	if err != nil {
		return err
	}
	if delta.BlockSize != b.BlockSize {
		return fmt.Errorf("Currently doesn't support different block sizes between objectstore and driver")
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:        LOG_REASON_COMPLETE,
		LOG_FIELD_OBJECT:        LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_EVENT:         LOG_EVENT_COMPARE,
		LOG_FIELD_SNAPSHOT:      snapshotID,
		LOG_FIELD_LAST_SNAPSHOT: lastSnapshotID,
	}).Debug("Generated snapshot changed blocks metadata")

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotID,
	}).Debug("Creating snapshot changed blocks")
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
			checksum := util.GetChecksum(block)
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
			if err := bsDriver.Write(blkFile, bytes.NewReader(block)); err != nil {
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

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotID,
	}).Debug("Created snapshot changed blocks")
	snapshotMap := mergeSnapshotMap(snapshotID, snapshotDeltaMap, lastSnapshotMap)

	if err := saveSnapshotMap(snapshotID, volumeID, bsDriver, snapshotMap); err != nil {
		return err
	}

	volume.LastSnapshotID = snapshotID
	if err := saveVolumeConfig(volumeID, bsDriver, volume); err != nil {
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

func RestoreSnapshot(root, srcSnapshotID, srcVolumeID, dstVolumeID, objectstoreID string, sDriver drivers.Driver) error {
	b, bsDriver, err := getObjectStoreCfgAndDriver(root, objectstoreID)
	if err != nil {
		return err
	}

	if _, err := loadVolumeConfig(srcVolumeID, bsDriver); err != nil {
		return generateError(logrus.Fields{
			LOG_FIELD_VOLUME:      srcVolumeID,
			LOG_FIELD_OBJECTSTORE: objectstoreID,
		}, "Volume doesn't exist in objectstore: %v", err)
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

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_START,
		LOG_FIELD_EVENT:       LOG_EVENT_RESTORE,
		LOG_FIELD_OBJECT:      LOG_FIELD_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    srcSnapshotID,
		LOG_FIELD_ORIN_VOLUME: srcVolumeID,
		LOG_FIELD_VOLUME:      dstVolumeID,
		LOG_FIELD_OBJECTSTORE: objectstoreID,
	}).Debug()
	for _, block := range snapshotMap.Blocks {
		blkFile := getBlockFilePath(srcVolumeID, block.BlockChecksum)
		rc, err := bsDriver.Read(blkFile)
		if err != nil {
			return err
		}
		if _, err := volDev.Seek(block.Offset, 0); err != nil {
			rc.Close()
			return err
		}
		if _, err := io.CopyN(volDev, rc, b.BlockSize); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}

	return nil
}

func RemoveSnapshot(root, snapshotID, volumeID, objectstoreID string) error {
	_, bsDriver, err := getObjectStoreCfgAndDriver(root, objectstoreID)
	if err != nil {
		return err
	}

	v, err := loadVolumeConfig(volumeID, bsDriver)
	if err != nil {
		return fmt.Errorf("Cannot find volume %v in objectstore %v", volumeID, objectstoreID, err)
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
	if err := bsDriver.Remove(discardFile); err != nil {
		return err
	}
	log.Debugf("Removed snapshot config file %v on objectstore", discardFile)

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

	var blkFileList []string
	for blk := range discardBlockSet {
		blkFileList = append(blkFileList, getBlockFilePath(volumeID, blk))
		log.Debugf("Found unused blocks %v for volume %v", blk, volumeID)
	}
	if err := bsDriver.Remove(blkFileList...); err != nil {
		return err
	}
	log.Debug("Removed unused blocks for volume ", volumeID)

	log.Debug("GC completed")
	log.Debug("Removed objectstore snapshot ", snapshotID)

	return nil
}

func listVolume(volumeID, snapshotID string, driver ObjectStoreDriver) ([]byte, error) {
	log.WithFields(logrus.Fields{
		LOG_FIELD_VOLUME:   volumeID,
		LOG_FIELD_SNAPSHOT: snapshotID,
	}).Debug("Listing objectstore for volume and snapshot")
	resp := api.VolumesResponse{
		Volumes: make(map[string]api.VolumeResponse),
	}

	v, err := loadVolumeConfig(volumeID, driver)
	if err != nil {
		// Volume doesn't exist
		return api.ResponseOutput(resp)
	}

	snapshots, err := getSnapshots(volumeID, driver)
	if err != nil {
		return nil, err
	}

	volumeResp := api.VolumeResponse{
		UUID:      volumeID,
		Name:      v.Name,
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
	return api.ResponseOutput(resp)
}

func ListVolume(root, objectstoreID, volumeID, snapshotID string) ([]byte, error) {
	_, bsDriver, err := getObjectStoreCfgAndDriver(root, objectstoreID)
	if err != nil {
		return nil, err
	}
	return listVolume(volumeID, snapshotID, bsDriver)
}

func listObjectStoreIDs(root string) []string {
	ids := []string{}
	outputs := util.ListConfigIDs(root, OBJECTSTORE_CFG_PREFIX, CFG_POSTFIX)
	for _, i := range outputs {
		// Remove driver specific config
		if strings.Contains(i, "_") {
			continue
		}
		ids = append(ids, i)
	}
	return ids
}

func List(root, objectstoreUUID string) ([]byte, error) {
	var objectstoreIDs []string

	resp := &api.ObjectStoresResponse{
		ObjectStores: make(map[string]api.ObjectStoreResponse),
	}
	if objectstoreUUID != "" {
		objectstoreIDs = []string{objectstoreUUID}
	} else {
		objectstoreIDs = listObjectStoreIDs(root)
	}
	for _, id := range objectstoreIDs {
		b, err := loadObjectStoreConfig(root, id)
		if err != nil {
			return nil, generateError(logrus.Fields{
				LOG_FIELD_VOLUME: id,
			}, "Objectstore %v doesn't exist", err.Error())
		}
		store := api.ObjectStoreResponse{
			UUID:      b.UUID,
			Kind:      b.Kind,
			BlockSize: b.BlockSize,
		}
		resp.ObjectStores[b.UUID] = store
	}
	return api.ResponseOutput(resp)
}
