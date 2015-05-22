package blockstore

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/util"
	"os"
	"path/filepath"
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
	Remove(name string) error //Would return error if it's not empty
	RemoveAll(name string) error
	Read(src string, data []byte) error
	Write(data []byte, dst string) error
	List(path string) ([]string, error)
	Upload(src, dst string) error
	Download(src, dst string) error
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
			return nil, fmt.Errorf("specific kind is different from config stored in blockstore")
		}
		id = bs.UUID
		log.Debug("Loaded blockstore cfg in blockstore: ", id)
		driver.FinalizeInit(root, getDriverCfgName(kind, id), id)
	} else {
		log.Debug("Failed to find existed blockstore cfg in blockstore")
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
	log.Debug("blockstore: loaded configure for blockstore ", blockstoreUUID)
	return b, driver, nil
}

func AddVolume(root, id, volumeID, base string, size int64) error {
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
		return fmt.Errorf("volume %v already exists in blockstore %v", volumeID, id)
	}

	volume := Volume{
		Size:           size,
		Base:           base,
		LastSnapshotID: "",
	}

	if err := saveConfigInBlockStore(volumeFile, driver, &volume); err != nil {
		log.Error("fail add volume ", volumeID)
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
	_, bsDriver, err := getBlockstoreCfgAndDriver(root, blockstoreID)
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

func listVolume(volumeID, snapshotID string, driver BlockStoreDriver) error {
	log.Debugf("blockstore: listing blockstore for volume %v snapshot %v", volumeID, snapshotID)
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
		return err
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
	log.Error("5")
	return nil
}

func ListVolume(root, blockstoreID, volumeID, snapshotID string) error {
	_, bsDriver, err := getBlockstoreCfgAndDriver(root, blockstoreID)
	if err != nil {
		return err
	}
	return listVolume(volumeID, snapshotID, bsDriver)
}

func AddImage(root, imageDir, imageUUID, imageName, imageFilePath, blockstoreUUID string) error {
	imageStat, err := os.Stat(imageFilePath)
	if os.IsNotExist(err) || imageStat.IsDir() {
		return fmt.Errorf("Invalid image file")
	}
	imageLocalStorePath := GetImageLocalStorePath(imageDir, imageUUID)
	if _, err := os.Stat(imageLocalStorePath); err == nil {
		return fmt.Errorf("Image already stored with UUID %v", imageUUID)
	}

	_, bsDriver, err := getBlockstoreCfgAndDriver(root, blockstoreUUID)
	if err != nil {
		return err
	}

	imageBlockStorePath := getImageBlockStorePath(imageUUID)
	imageCfgBlockStorePath := getImageCfgBlockStorePath(imageUUID)

	imageExists := bsDriver.FileExists(imageBlockStorePath)
	imageCfgExists := bsDriver.FileExists(imageCfgBlockStorePath)
	if imageExists && imageCfgExists {
		return fmt.Errorf("The image with uuid %v already existed in blockstore", imageUUID)
	} else if imageExists != imageCfgExists {
		return fmt.Errorf("The image with uuid %v state is inconsistent in blockstore", imageUUID)
	}

	if imageStat.Size()%DEFAULT_BLOCK_SIZE != 0 {
		return fmt.Errorf("The image size must be multiplier of %v", DEFAULT_BLOCK_SIZE)
	}

	image := &Image{}
	image.UUID = imageUUID
	image.Name = imageName
	image.Size = imageStat.Size()

	log.Debugf("blockstore: copying image %v to local store %v", imageFilePath, imageLocalStorePath)
	if err := util.Copy(imageFilePath, imageLocalStorePath); err != nil {
		log.Debugf("blockstore: copying image failed")
		return err
	}
	log.Debug("blockstore: copied image to local store")

	log.Debugf("blockstore: prepare uploading image to blockstore ")
	if err := uploadImage(imageLocalStorePath, bsDriver, image); err != nil {
		log.Debugf("blockstore: uploading image failed")
		return err
	}
	log.Debug("blockstore: uploaded image to blockstore")

	if err := saveImageConfig(imageUUID, bsDriver, image); err != nil {
		return err
	}
	log.Debug("blockstore: save image config to blockstore done")

	imageResp := api.ImageResponse{
		UUID:        image.UUID,
		Name:        image.Name,
		Size:        image.Size,
		Checksum:    image.Checksum,
		RawChecksum: image.RawChecksum,
	}
	api.ResponseOutput(imageResp)
	return nil
}

func uploadImage(imageLocalStorePath string, bsDriver BlockStoreDriver, image *Image) error {
	log.Debug("blockstore: calculating checksum for raw image")
	rawChecksum, err := util.GetFileChecksum(imageLocalStorePath)
	if err != nil {
		log.Debug("blockstore: calculation failed")
		return err
	}
	log.Debug("blockstore: calculation done, raw checksum: ", rawChecksum)
	image.RawChecksum = rawChecksum

	log.Debug("blockstore: compressing raw image")
	if err := util.CompressFile(imageLocalStorePath); err != nil {
		log.Debug("blockstore: compressing failed ")
		return err
	}
	compressedLocalPath := imageLocalStorePath + ".gz"
	log.Debug("blockstore: compressed raw image to ", compressedLocalPath)

	log.Debug("blockstore: calculating checksum for compressed image")
	if image.Checksum, err = util.GetFileChecksum(compressedLocalPath); err != nil {
		log.Debug("blockstore: calculation failed")
		return err
	}
	log.Debug("blockstore: calculation done, checksum: ", image.Checksum)

	imageBlockStorePath := getImageBlockStorePath(image.UUID)
	log.Debug("blockstore: uploading image to blockstore path: ", imageBlockStorePath)
	if err := bsDriver.Upload(compressedLocalPath, imageBlockStorePath); err != nil {
		log.Debugf("blockstore: uploading failed")
		return err
	}
	log.Debugf("blockstore: uploading done")
	return nil
}

func removeImage(bsDriver BlockStoreDriver, image *Image) error {
	if err := removeImageConfig(image, bsDriver); err != nil {
		return err
	}
	log.Debugf("blockstore: removed image %v's config from blockstore", image.UUID)
	imageBlockStorePath := getImageBlockStorePath(image.UUID)
	if err := bsDriver.RemoveAll(imageBlockStorePath); err != nil {
		return err
	}
	log.Debug("blockstore: removed image at ", imageBlockStorePath)
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

	if err := removeImage(driver, image); err != nil {
		return err
	}
	log.Debugf("blockstore: image %v removed", imageUUID)

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

	if err := downloadImage(imageDir, driver, image); err != nil {
		return err
	}
	return nil
}

func loadImageCache(fileName string, compressed bool, image *Image) (bool, error) {
	if st, err := os.Stat(fileName); err == nil && !st.IsDir() {
		log.Debug("blockstore: found local image cache at ", fileName)
		log.Debug("blockstore: calculating checksum for local image cache")
		checksum, err := util.GetFileChecksum(fileName)
		if err != nil {
			return false, err
		}
		log.Debug("blockstore: calculation done, checksum ", checksum)
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
			log.Debug("blockstore: removed local image cache at ", fileName)
		}
	}
	return false, nil
}

func uncompressImage(fileName string) error {
	log.Debugf("blockstore: uncompressing image %v ", fileName)
	if err := util.UncompressFile(fileName); err != nil {
		return err
	}
	log.Debug("blockstore: image uncompressed")
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
	log.Debugf("blockstore: downloading image from blockstore %v to %v", imageBlockStorePath, compressedLocalPath)
	if err := driver.Download(imageBlockStorePath, compressedLocalPath); err != nil {
		return err
	}
	log.Debug("blockstore: download complete")

	if err := uncompressImage(compressedLocalPath); err != nil {
		return err
	}

	log.Debug("blockstore: calculating checksum for local image")
	rawChecksum, err := util.GetFileChecksum(imageLocalStorePath)
	if err != nil {
		return err
	}
	log.Debug("blockstore: calculation done, raw checksum ", rawChecksum)
	if rawChecksum != image.RawChecksum {
		return fmt.Errorf("Image %v checksum verification failed!", image.UUID)
	}
	return nil
}

func DeactivateImage(root, imageDir, imageUUID, blockstoreUUID string) error {
	imageLocalStorePath := GetImageLocalStorePath(imageDir, imageUUID)
	if st, err := os.Stat(imageLocalStorePath); err == nil && !st.IsDir() {
		if err := os.RemoveAll(imageLocalStorePath); err != nil {
			return err
		}
		log.Debug("blockstore: removed local image cache at ", imageLocalStorePath)
	}
	log.Debug("blockstore: deactivated image ", imageUUID)
	return nil
}
