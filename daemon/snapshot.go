package daemon

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/util"
	"net/http"

	. "github.com/rancher/convoy/convoydriver"
	. "github.com/rancher/convoy/logging"
)

func (s *daemon) snapshotExists(volumeUUID, snapshotName string) bool {
	volume := s.getVolume(volumeUUID)
	if volume == nil {
		return false
	}
	_, err := s.getSnapshotDriverInfo(snapshotName, volume)
	return err == nil
}

func (s *daemon) doSnapshotCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	request := &api.SnapshotCreateRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}
	volumeUUID := request.VolumeUUID
	if err := util.CheckUUID(volumeUUID); err != nil {
		return err
	}

	snapshotUUID := uuid.New()
	snapshotName := request.Name
	if snapshotName != "" {
		if err := util.CheckName(snapshotName); err != nil {
			return err
		}
		existUUID := s.NameUUIDIndex.Get(snapshotName)
		if existUUID != "" {
			return fmt.Errorf("Snapshot name %v already associated with %v", snapshotName, existUUID)
		}
	} else {
		snapshotName = "snapshot-" + snapshotUUID[:8]
		for s.NameUUIDIndex.Get(snapshotName) != "" {
			snapshotUUID = uuid.New()
			snapshotName = "snapshot-" + snapshotUUID[:8]
		}
	}

	volume := s.getVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	snapOps, err := s.getSnapshotOpsForVolume(volume)
	if err != nil {
		return err
	}

	req := Request{
		Name: snapshotName,
		Options: map[string]string{
			OPT_VOLUME_UUID: volumeUUID,
		},
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:    LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotName,
		LOG_FIELD_VOLUME:   volumeUUID,
	}).Debug()
	if err := snapOps.CreateSnapshot(req); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotName,
		LOG_FIELD_VOLUME:   volumeUUID,
	}).Debug()

	//TODO: error handling
	if err := s.SnapshotVolumeIndex.Add(snapshotName, volume.UUID); err != nil {
		return err
	}
	if err := s.NameUUIDIndex.Add(snapshotName, "exists"); err != nil {
		return err
	}
	driverInfo, err := s.getSnapshotDriverInfo(snapshotName, volume)
	if err != nil {
		return err
	}
	if request.Verbose {
		return writeResponseOutput(w, api.SnapshotResponse{
			Name:        snapshotName,
			VolumeUUID:  volume.UUID,
			CreatedTime: driverInfo[OPT_SNAPSHOT_CREATED_TIME],
			DriverInfo:  driverInfo,
		})
	}
	return writeStringResponse(w, snapshotName)
}

func (s *daemon) getSnapshotDriverInfo(snapshotName string, volume *Volume) (map[string]string, error) {
	snapOps, err := s.getSnapshotOpsForVolume(volume)
	if err != nil {
		return nil, err
	}
	req := Request{
		Name: snapshotName,
		Options: map[string]string{
			OPT_VOLUME_UUID: volume.UUID,
		},
	}
	driverInfo, err := snapOps.GetSnapshotInfo(req)
	if err != nil {
		return nil, err
	}
	driverInfo["Driver"] = snapOps.Name()
	return driverInfo, nil
}

func (s *daemon) listSnapshotDriverInfos(volume *Volume) (map[string]map[string]string, error) {
	snapOps, err := s.getSnapshotOpsForVolume(volume)
	if err != nil {
		return nil, err
	}
	opts := map[string]string{
		OPT_VOLUME_UUID: volume.UUID,
	}
	snapshots, err := snapOps.ListSnapshot(opts)
	if err != nil {
		return nil, err
	}
	return snapshots, nil
}

func (s *daemon) doSnapshotDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	request := &api.SnapshotDeleteRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}
	snapshotName := request.SnapshotName
	if err := util.CheckName(snapshotName); err != nil {
		return err
	}
	volumeUUID := s.SnapshotVolumeIndex.Get(snapshotName)
	if volumeUUID == "" {
		return fmt.Errorf("cannot find volume for snapshot %v", snapshotName)
	}

	volume := s.getVolume(volumeUUID)
	if !s.snapshotExists(volumeUUID, snapshotName) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotName, volumeUUID)
	}

	snapOps, err := s.getSnapshotOpsForVolume(volume)
	if err != nil {
		return err
	}

	req := Request{
		Name: snapshotName,
		Options: map[string]string{
			OPT_VOLUME_UUID: volumeUUID,
		},
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:    LOG_EVENT_DELETE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotName,
		LOG_FIELD_VOLUME:   volumeUUID,
	}).Debug()
	if err := snapOps.DeleteSnapshot(req); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_DELETE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotName,
		LOG_FIELD_VOLUME:   volumeUUID,
	}).Debug()

	//TODO: error handling
	if err := s.SnapshotVolumeIndex.Delete(snapshotName); err != nil {
		return err
	}
	if err := s.NameUUIDIndex.Delete(snapshotName); err != nil {
		return err
	}
	return nil
}

func (s *daemon) doSnapshotInspect(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	request := &api.SnapshotInspectRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}
	snapshotName := request.SnapshotName
	if err := util.CheckName(snapshotName); err != nil {
		return err
	}
	volumeUUID := s.SnapshotVolumeIndex.Get(snapshotName)
	if volumeUUID == "" {
		return fmt.Errorf("cannot find volume for snapshot %v", snapshotName)
	}

	volume := s.getVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("cannot find volume %v", volumeUUID)
	}

	volumeDriverInfo, err := s.getVolumeDriverInfo(volume)
	if err != nil {
		return err
	}

	snapshot, err := s.getSnapshotDriverInfo(snapshotName, volume)
	if err != nil {
		return fmt.Errorf("cannot find snapshot %v of volume %v", snapshotName, volumeUUID)
	}

	driverInfo, err := s.getSnapshotDriverInfo(snapshotName, volume)
	if err != nil {
		return err
	}

	resp := api.SnapshotResponse{
		Name:            snapshotName,
		VolumeUUID:      volume.UUID,
		VolumeName:      volumeDriverInfo[OPT_VOLUME_NAME],
		VolumeCreatedAt: volumeDriverInfo[OPT_VOLUME_CREATED_TIME],
		CreatedTime:     snapshot[OPT_SNAPSHOT_CREATED_TIME],
		DriverInfo:      driverInfo,
	}
	data, err := api.ResponseOutput(resp)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
