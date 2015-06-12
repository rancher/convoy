package blockstore

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/util"
	"io"
	"os"
	"path/filepath"

	. "github.com/rancherio/volmgr/logging"
)

const (
	DEFAULT_BLOCK_SIZE = 2097152
)

type InitFunc func(root, cfgName string, config map[string]string) (BlockStoreDriver, error)

type BlockStoreDriver interface {
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

type Image struct {
	UUID        string
	Name        string
	Size        int64
	Checksum    string
	RawChecksum string
}

var (
	initializers map[string]InitFunc
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "blockstore"})
)

func generateError(fields logrus.Fields, format string, v ...interface{}) error {
	return ErrorWithFields("blockstore", fields, format, v)
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

func GetBlockStoreDriver(kind, root, cfgName string, config map[string]string) (BlockStoreDriver, error) {
	if _, exists := initializers[kind]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", kind)
	}
	return initializers[kind](root, cfgName, config)
}

func Register(root, kind string, config map[string]string) (*BlockStore, error) {
	driver, err := GetBlockStoreDriver(kind, root, "", config)
	if err != nil {
		return nil, err
	}

	var id string
	bs, err := loadRemoteBlockStoreConfig(driver)
	if err == nil {
		// BlockStore has already been created
		if bs.Kind != kind {
			return nil, generateError(logrus.Fields{
				LOG_FIELD_BLOCKSTORE: bs.UUID,
				LOG_FIELD_KIND:       bs.Kind,
			}, "Specific kind is different from config stored in blockstore")
		}
		id = bs.UUID
		log.Debug("Loaded blockstore cfg in blockstore: ", id)
		driver.FinalizeInit(root, getDriverCfgName(kind, id), id)
	} else {
		log.Debug("Cannot find existed blockstore cfg in blockstore, create a new one")
		id = uuid.New()
		driver.FinalizeInit(root, getDriverCfgName(kind, id), id)

		bs = &BlockStore{
			UUID:      id,
			Kind:      kind,
			BlockSize: DEFAULT_BLOCK_SIZE,
		}

		if err := saveRemoteBlockStoreConfig(driver, bs); err != nil {
			return nil, err
		}
		log.Debug("Created blockstore cfg in blockstore", bs.UUID)
	}

	if err := util.SaveConfig(root, getCfgName(id), bs); err != nil {
		return nil, err
	}
	log.Debug("Created local copy of ", getCfgName(id))
	log.Debug("Registered block store ", bs.UUID)
	return bs, nil
}

func Deregister(root, id string) error {
	b := &BlockStore{}
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

func getBlockstoreCfgAndDriver(root, blockstoreUUID string) (*BlockStore, BlockStoreDriver, error) {
	b := &BlockStore{}
	err := util.LoadConfig(root, getCfgName(blockstoreUUID), b)
	if err != nil {
		return nil, nil, err
	}

	driver, err := GetBlockStoreDriver(b.Kind, root, getDriverCfgName(b.Kind, blockstoreUUID), nil)
	if err != nil {
		return nil, nil, err
	}
	log.Debug("Loaded configure for blockstore ", blockstoreUUID)
	return b, driver, nil
}

func AddVolume(root, id, volumeID, volumeName, base string, size int64) error {
	_, driver, err := getBlockstoreCfgAndDriver(root, id)
	if err != nil {
		return err
	}

	if base != "" {
		_, err := loadImageConfig(base, driver)
		if err != nil {
			return err
		}
	}

	volumePath := getVolumePath(volumeID)
	volumeCfg := VOLUME_CONFIG_FILE
	volumeFile := filepath.Join(volumePath, volumeCfg)
	if driver.FileExists(volumeFile) {
		return fmt.Errorf("Volume %v already exists in blockstore %v", volumeID, id)
	}

	volume := Volume{
		UUID:           volumeID,
		Name:           volumeName,
		Size:           size,
		Base:           base,
		LastSnapshotID: "",
	}

	if err := saveConfigInBlockStore(volumeFile, driver, &volume); err != nil {
		log.Error("Fail add volume ", volumeID)
		return err
	}
	log.Debug("Created volume configuration file in blockstore: ", volumeFile)
	log.Debug("Added blockstore volume ", volumeID)

	return nil
}

func RemoveVolume(root, id, volumeID string) error {
	_, driver, err := getBlockstoreCfgAndDriver(root, id)
	if err != nil {
		return err
	}

	volumePath := getVolumePath(volumeID)
	volumeCfg := VOLUME_CONFIG_FILE
	volumeFile := filepath.Join(volumePath, volumeCfg)
	if !driver.FileExists(volumeFile) {
		return fmt.Errorf("Volume %v doesn't exist in blockstore %v", volumeID, id)
	}

	volumeDir := getVolumePath(volumeID)
	if err := driver.Remove(volumeDir); err != nil {
		return err
	}
	log.Debug("Removed volume directory in blockstore: ", volumeDir)
	log.Debug("Removed blockstore volume ", volumeID)

	return nil
}

func BackupSnapshot(root, snapshotID, volumeID, blockstoreID string, sDriver drivers.Driver) error {
	b, bsDriver, err := getBlockstoreCfgAndDriver(root, blockstoreID)
	if err != nil {
		return err
	}

	volume, err := loadVolumeConfig(volumeID, bsDriver)
	if err != nil {
		return err
	}

	if snapshotExists(snapshotID, volumeID, bsDriver) {
		return generateError(logrus.Fields{
			LOG_FIELD_SNAPSHOT:   snapshotID,
			LOG_FIELD_VOLUME:     volumeID,
			LOG_FIELD_BLOCKSTORE: blockstoreID,
		}, "Snapshot already exists in blockstore!")
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
		return fmt.Errorf("Currently doesn't support different block sizes between blockstore and driver")
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

func RestoreSnapshot(root, srcSnapshotID, srcVolumeID, dstVolumeID, blockstoreID string, sDriver drivers.Driver) error {
	b, bsDriver, err := getBlockstoreCfgAndDriver(root, blockstoreID)
	if err != nil {
		return err
	}

	if _, err := loadVolumeConfig(srcVolumeID, bsDriver); err != nil {
		return generateError(logrus.Fields{
			LOG_FIELD_VOLUME:     srcVolumeID,
			LOG_FIELD_BLOCKSTORE: blockstoreID,
		}, "Volume doesn't exist in blockstore: %v", err)
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
		LOG_FIELD_BLOCKSTORE:  blockstoreID,
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

func RemoveSnapshot(root, snapshotID, volumeID, blockstoreID string) error {
	_, bsDriver, err := getBlockstoreCfgAndDriver(root, blockstoreID)
	if err != nil {
		return err
	}

	v, err := loadVolumeConfig(volumeID, bsDriver)
	if err != nil {
		return fmt.Errorf("Cannot find volume %v in blockstore %v", volumeID, blockstoreID, err)
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
	log.Debug("Removed blockstore snapshot ", snapshotID)

	return nil
}

func listVolume(volumeID, snapshotID string, driver BlockStoreDriver) ([]byte, error) {
	log.WithFields(logrus.Fields{
		LOG_FIELD_VOLUME:   volumeID,
		LOG_FIELD_SNAPSHOT: snapshotID,
	}).Debug("Listing blockstore for volume and snapshot")
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
	return api.ResponseOutput(resp)
}

func ListVolume(root, blockstoreID, volumeID, snapshotID string) ([]byte, error) {
	_, bsDriver, err := getBlockstoreCfgAndDriver(root, blockstoreID)
	if err != nil {
		return nil, err
	}
	return listVolume(volumeID, snapshotID, bsDriver)
}

func AddImage(root, imageDir, imageUUID, imageName, imageFilePath, blockstoreUUID string) ([]byte, error) {
	imageStat, err := os.Stat(imageFilePath)
	if os.IsNotExist(err) || imageStat.IsDir() {
		return nil, fmt.Errorf("Invalid image file")
	}
	imageLocalStorePath := GetImageLocalStorePath(imageDir, imageUUID)
	if _, err := os.Stat(imageLocalStorePath); err == nil {
		return nil, generateError(logrus.Fields{
			LOG_FIELD_IMAGE: imageUUID,
		}, "UUID is already used by another image")
	}

	_, bsDriver, err := getBlockstoreCfgAndDriver(root, blockstoreUUID)
	if err != nil {
		return nil, err
	}

	imageBlockStorePath := getImageBlockStorePath(imageUUID)
	imageCfgBlockStorePath := getImageCfgBlockStorePath(imageUUID)

	imageExists := bsDriver.FileExists(imageBlockStorePath)
	imageCfgExists := bsDriver.FileExists(imageCfgBlockStorePath)
	if imageExists && imageCfgExists {
		return nil, generateError(logrus.Fields{
			LOG_FIELD_IMAGE: imageUUID,
		}, "The image already existed in blockstore")
	} else if imageExists != imageCfgExists {
		return nil, generateError(logrus.Fields{
			LOG_FIELD_IMAGE: imageUUID,
		}, "The image state is inconsistent in blockstore")
	}

	if imageStat.Size()%DEFAULT_BLOCK_SIZE != 0 {
		return nil, fmt.Errorf("The image size must be multiplier of %v", DEFAULT_BLOCK_SIZE)
	}

	image := &Image{}
	image.UUID = imageUUID
	image.Name = imageName
	image.Size = imageStat.Size()

	log.Debugf("Copying image %v to local store %v", imageFilePath, imageLocalStorePath)
	if err := util.Copy(imageFilePath, imageLocalStorePath); err != nil {
		log.Debugf("Copying image failed")
		return nil, err
	}
	log.Debug("Copied image to local store")

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_START,
		LOG_FIELD_EVENT:      LOG_EVENT_UPLOAD,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_IMAGE_FILE: imageLocalStorePath,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug("Uploading image to blockstore")
	if err := uploadImage(imageLocalStorePath, bsDriver, image); err != nil {
		log.Debugf("Uploading image failed")
		return nil, err
	}

	if err := saveImageConfig(imageUUID, bsDriver, image); err != nil {
		return nil, err
	}

	imageResp := api.ImageResponse{
		UUID:        image.UUID,
		Name:        image.Name,
		Size:        image.Size,
		Checksum:    image.Checksum,
		RawChecksum: image.RawChecksum,
	}
	return api.ResponseOutput(imageResp)
}

func uploadImage(imageLocalStorePath string, bsDriver BlockStoreDriver, image *Image) error {
	log.Debug("Calculating checksum for raw image")
	rawChecksum, err := util.GetFileChecksum(imageLocalStorePath)
	if err != nil {
		log.Debug("Calculation failed")
		return err
	}
	log.Debug("Calculation done, raw checksum: ", rawChecksum)
	image.RawChecksum = rawChecksum

	log.Debug("Compressing raw image")
	if err := util.CompressFile(imageLocalStorePath); err != nil {
		log.Debug("Compressing failed ")
		return err
	}
	compressedLocalPath := imageLocalStorePath + ".gz"
	log.Debug("Compressed raw image to ", compressedLocalPath)

	log.Debug("Calculating checksum for compressed image")
	if image.Checksum, err = util.GetFileChecksum(compressedLocalPath); err != nil {
		log.Debug("Calculation failed")
		return err
	}
	log.Debug("Calculation done, checksum: ", image.Checksum)

	imageBlockStorePath := getImageBlockStorePath(image.UUID)
	log.Debug("Uploading image to blockstore path: ", imageBlockStorePath)
	if err := bsDriver.Upload(compressedLocalPath, imageBlockStorePath); err != nil {
		log.Debugf("Uploading failed")
		return err
	}
	log.Debugf("Uploading done")
	return nil
}

func removeImage(bsDriver BlockStoreDriver, image *Image) error {
	if err := removeImageConfig(image, bsDriver); err != nil {
		return err
	}
	log.Debugf("Removed image %v's config from blockstore", image.UUID)
	imageBlockStorePath := getImageBlockStorePath(image.UUID)
	if err := bsDriver.Remove(imageBlockStorePath); err != nil {
		return err
	}
	log.Debug("Removed image at ", imageBlockStorePath)
	return nil
}

func RemoveImage(root, imageDir, imageUUID, blockstoreUUID string) error {
	_, driver, err := getBlockstoreCfgAndDriver(root, blockstoreUUID)
	if err != nil {
		return err
	}

	image, err := loadImageConfig(imageUUID, driver)
	if err != nil {
		return err
	}

	imageLocalStorePath := GetImageLocalStorePath(imageDir, imageUUID)
	if _, err := os.Stat(imageLocalStorePath); err == nil {
		return fmt.Errorf("Image %v is still activated", imageUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_START,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	if err := removeImage(driver, image); err != nil {
		return err
	}

	return nil
}

func ActivateImage(root, imageDir, imageUUID, blockstoreUUID string) error {
	_, driver, err := getBlockstoreCfgAndDriver(root, blockstoreUUID)
	if err != nil {
		return err
	}

	image, err := loadImageConfig(imageUUID, driver)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_START,
		LOG_FIELD_EVENT:      LOG_EVENT_ACTIVATE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	if err := downloadImage(imageDir, driver, image); err != nil {
		return err
	}
	return nil
}

func loadImageCache(fileName string, compressed bool, image *Image) (bool, error) {
	if st, err := os.Stat(fileName); err == nil && !st.IsDir() {
		log.Debug("Found local image cache at ", fileName)
		log.Debug("Calculating checksum for local image cache")
		checksum, err := util.GetFileChecksum(fileName)
		if err != nil {
			return false, err
		}
		log.Debug("Calculation done, checksum ", checksum)
		if compressed && checksum == image.Checksum {
			log.Debugf("Found image %v in local images directory, and checksum matched, no need to re-download\n", image.UUID)
			return true, nil
		} else if !compressed && checksum == image.RawChecksum {
			log.Debugf("Found image %v in local images directory, and checksum matched, no need to re-download\n", image.UUID)
			return true, nil
		} else {
			log.Debugf("Found image %v in local images directory, but checksum doesn't match record, would re-download\n", image.UUID)
			if err := os.RemoveAll(fileName); err != nil {
				return false, err
			}
			log.Debug("Removed local image cache at ", fileName)
		}
	}
	return false, nil
}

func uncompressImage(fileName string) error {
	log.Debugf("Uncompressing image %v ", fileName)
	if err := util.UncompressFile(fileName); err != nil {
		return err
	}
	log.Debug("Image uncompressed")
	return nil
}

func downloadImage(imagesDir string, driver BlockStoreDriver, image *Image) error {
	imageLocalStorePath := GetImageLocalStorePath(imagesDir, image.UUID)
	found, err := loadImageCache(imageLocalStorePath, false, image)
	if found || err != nil {
		return err
	}

	compressedLocalPath := imageLocalStorePath + ".gz"
	found, err = loadImageCache(compressedLocalPath, true, image)
	if err != nil {
		return err
	}
	if found {
		return uncompressImage(compressedLocalPath)
	}

	imageBlockStorePath := getImageBlockStorePath(image.UUID)
	log.Debugf("Downloading image from blockstore %v to %v", imageBlockStorePath, compressedLocalPath)
	if err := driver.Download(imageBlockStorePath, compressedLocalPath); err != nil {
		return err
	}
	log.Debug("Download complete")

	if err := uncompressImage(compressedLocalPath); err != nil {
		return err
	}

	log.Debug("Calculating checksum for local image")
	rawChecksum, err := util.GetFileChecksum(imageLocalStorePath)
	if err != nil {
		return err
	}
	log.Debug("Calculation done, raw checksum ", rawChecksum)
	if rawChecksum != image.RawChecksum {
		return fmt.Errorf("Image %v checksum verification failed!", image.UUID)
	}
	return nil
}

func DeactivateImage(root, imageDir, imageUUID, blockstoreUUID string) error {
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_START,
		LOG_FIELD_EVENT:      LOG_EVENT_DEACTIVATE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	imageLocalStorePath := GetImageLocalStorePath(imageDir, imageUUID)
	if st, err := os.Stat(imageLocalStorePath); err == nil && !st.IsDir() {
		if err := os.RemoveAll(imageLocalStorePath); err != nil {
			return err
		}
		log.Debug("Removed local image cache at ", imageLocalStorePath)
	}
	return nil
}
