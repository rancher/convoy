package objectstore

import (
	"bytes"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/drivers"
	"github.com/rancher/rancher-volume/util"
	"io"
	"net/url"
	"os"
	"path/filepath"

	. "github.com/rancher/rancher-volume/logging"
)

const (
	DEFAULT_BLOCK_SIZE = 2097152
)

type InitFunc func(destURL string) (ObjectStoreDriver, error)

type ObjectStoreDriver interface {
	Kind() string
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
	FileSystem     string
	CreatedTime    string
	LastSnapshotID string
}

type ObjectStore struct {
	UUID string
	Kind string
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

func getObjectStoreDriver(destURL string) (ObjectStoreDriver, error) {
	u, err := url.Parse(destURL)
	if err != nil {
		return nil, err
	}
	if _, exists := initializers[u.Scheme]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", u.Scheme)
	}
	return initializers[u.Scheme](destURL)
}

func VolumeExists(volumeUUID, destURL string) bool {
	driver, err := getObjectStoreDriver(destURL)
	if err != nil {
		return false
	}

	return driver.FileExists(getVolumeFilePath(volumeUUID))
}

func addVolume(volume *Volume, driver ObjectStoreDriver) error {
	if volumeExists(volume.UUID, driver) {
		return nil
	}

	if err := saveVolumeConfig(volume, driver); err != nil {
		log.Error("Fail add volume ", volume.UUID)
		return err
	}
	log.Debug("Added objectstore volume ", volume.UUID)

	return nil
}

func removeVolume(volumeUUID string, driver ObjectStoreDriver) error {
	if !volumeExists(volumeUUID, driver) {
		return fmt.Errorf("Volume %v doesn't exist in objectstore", volumeUUID)
	}

	volumeDir := getVolumePath(volumeUUID)
	if err := driver.Remove(volumeDir); err != nil {
		return err
	}
	log.Debug("Removed volume directory in objectstore: ", volumeDir)
	log.Debug("Removed objectstore volume ", volumeUUID)

	return nil
}

func BackupSnapshot(volumeDesc *Volume, snapshotID, destURL string, sDriver drivers.Driver) (string, error) {
	bsDriver, err := getObjectStoreDriver(destURL)
	if err != nil {
		return "", err
	}

	if err := addVolume(volumeDesc, bsDriver); err != nil {
		return "", err
	}

	volume, err := loadVolumeConfig(volumeDesc.UUID, bsDriver)
	if err != nil {
		return "", err
	}

	if snapshotExists(snapshotID, volume.UUID, bsDriver) {
		return "", generateError(logrus.Fields{
			LOG_FIELD_SNAPSHOT: snapshotID,
			LOG_FIELD_VOLUME:   volume.UUID,
			LOG_FIELD_DEST_URL: destURL,
		}, "Snapshot already exists in objectstore!")
	}

	lastSnapshotID := volume.LastSnapshotID

	var lastSnapshotMap *SnapshotMap
	if lastSnapshotID != "" {
		if lastSnapshotID == snapshotID {
			//Generate full snapshot if the snapshot has been backed up last time
			lastSnapshotID = ""
			log.Debug("Would create full snapshot metadata")
		} else if !sDriver.HasSnapshot(lastSnapshotID, volume.UUID) {
			// It's possible that the snapshot in objectstore doesn't exist
			// in local storage
			lastSnapshotID = ""
			log.WithFields(logrus.Fields{
				LOG_FIELD_REASON:   LOG_REASON_FALLBACK,
				LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
				LOG_FIELD_SNAPSHOT: lastSnapshotID,
				LOG_FIELD_VOLUME:   volume.UUID,
			}).Debug("Cannot find last snapshot in local storage, would process with full backup")
		} else {
			log.WithFields(logrus.Fields{
				LOG_FIELD_REASON:   LOG_REASON_START,
				LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
				LOG_FIELD_EVENT:    LOG_EVENT_LOAD,
				LOG_FIELD_SNAPSHOT: lastSnapshotID,
			}).Debug("Loading last snapshot")
			lastSnapshotMap, err = loadSnapshotMap(lastSnapshotID, volume.UUID, bsDriver)
			if err != nil {
				return "", err
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
	delta, err := sDriver.CompareSnapshot(snapshotID, lastSnapshotID, volume.UUID)
	if err != nil {
		return "", err
	}
	if delta.BlockSize != DEFAULT_BLOCK_SIZE {
		return "", fmt.Errorf("Currently doesn't support different block sizes driver other than %v", DEFAULT_BLOCK_SIZE)
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
	if err := sDriver.OpenSnapshot(snapshotID, volume.UUID); err != nil {
		return "", err
	}
	defer sDriver.CloseSnapshot(snapshotID, volume.UUID)
	for _, d := range delta.Mappings {
		block := make([]byte, DEFAULT_BLOCK_SIZE)
		for i := int64(0); i < d.Size/delta.BlockSize; i++ {
			offset := d.Offset + i*delta.BlockSize
			err := sDriver.ReadSnapshot(snapshotID, volume.UUID, offset, block)
			if err != nil {
				return "", err
			}
			checksum := util.GetChecksum(block)
			blkFile := getBlockFilePath(volume.UUID, checksum)
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
				return "", err
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

	if err := saveSnapshotMap(snapshotID, volume.UUID, bsDriver, snapshotMap); err != nil {
		return "", err
	}

	volume.LastSnapshotID = snapshotID
	if err := saveVolumeConfig(volume, bsDriver); err != nil {
		return "", err
	}

	return encodeBackupURL(destURL, volume.UUID, snapshotID), nil
}

func encodeBackupURL(destURL, volumeUUID, snapshotUUID string) string {
	v := url.Values{}
	v.Add("volume", volumeUUID)
	v.Add("snapshot", snapshotUUID)
	return destURL + "?" + v.Encode()
}

func decodeBackupURL(backupURL string) (string, string, error) {
	u, err := url.Parse(backupURL)
	if err != nil {
		return "", "", err
	}
	v := u.Query()
	volumeUUID := v.Get("volume")
	snapshotUUID := v.Get("snapshot")
	if !util.ValidateUUID(volumeUUID) || !util.ValidateUUID(snapshotUUID) {
		return "", "", fmt.Errorf("Invalid UUID parsed")
	}
	return volumeUUID, snapshotUUID, nil
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

func RestoreSnapshot(backupURL, dstVolumeID string, sDriver drivers.Driver) error {
	bsDriver, err := getObjectStoreDriver(backupURL)
	if err != nil {
		return err
	}

	srcVolumeID, srcSnapshotID, err := decodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	if _, err := loadVolumeConfig(srcVolumeID, bsDriver); err != nil {
		return generateError(logrus.Fields{
			LOG_FIELD_VOLUME:     srcVolumeID,
			LOG_FIELD_BACKUP_URL: backupURL,
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
		//LOG_FIELD_OBJECTSTORE: objectstoreID,
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
		if _, err := io.CopyN(volDev, rc, DEFAULT_BLOCK_SIZE); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}

	return nil
}

func RemoveSnapshot(backupURL string) error {
	bsDriver, err := getObjectStoreDriver(backupURL)
	if err != nil {
		return err
	}

	volumeID, snapshotID, err := decodeBackupURL(backupURL)
	if err != nil {
		return err
	}

	v, err := loadVolumeConfig(volumeID, bsDriver)
	if err != nil {
		return fmt.Errorf("Cannot find volume %v in objectstore", volumeID, err)
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
		if err := saveVolumeConfig(v, bsDriver); err != nil {
			return err
		}
	}

	snapshots, err := getSnapshots(volumeID, bsDriver)
	if err != nil {
		return err
	}
	if len(snapshots) == 0 {
		log.Debugf("No snapshot existed for the volume %v, removing volume", volumeID)
		if err := removeVolume(volumeID, bsDriver); err != nil {
			log.Warningf("Failed to remove volume %v due to: %v", volumeID, err.Error())
		}
		return nil
	}

	log.Debug("GC started")
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

func ListVolume(destURL, volumeID, snapshotID string) ([]byte, error) {
	bsDriver, err := getObjectStoreDriver(destURL)
	if err != nil {
		return nil, err
	}
	return listVolume(volumeID, snapshotID, bsDriver)
}
