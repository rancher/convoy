package profitbricks

import (
	"fmt"
	"testing"
)

var snapshotId string

func createVolume() {
	setupTestEnv()
	want := 202
	var request = Volume{
		Properties: VolumeProperties{
			Size:          5,
			Name:          "Volume Test",
			Image:         image,
			Type:          "HDD",
			ImagePassword: "test1234",
		},
	}

	dcID = mkdcid("GO SDK snapshot DC")
	resp := CreateVolume(dcID, request)

	waitTillProvisioned(resp.Headers.Get("Location"))
	volumeId = resp.Id

	if resp.StatusCode != want {
		fmt.Println(string(resp.Response))
	}
}

func createSnapshot() {
	resp := CreateSnapshot(dcID, volumeId, "testSnapshot")
	waitTillProvisioned(resp.Headers.Get("Location"))
	snapshotId = resp.Id

}

func TestGetSnapshot(t *testing.T) {
	createVolume()
	createSnapshot()

	want := 200

	resp := GetSnapshot(snapshotId)

	if resp.StatusCode != want {
		fmt.Println(string(resp.Response))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestListSnapshot(t *testing.T) {
	want := 200

	resp := ListSnapshots()

	if resp.StatusCode != want {
		fmt.Println(string(resp.Response))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestUpdateSnapshot(t *testing.T) {
	want := 202
	newValue := "whatever"
	resp := UpdateSnapshot(snapshotId, SnapshotProperties{Name: newValue})

	if resp.StatusCode != want {
		fmt.Println(string(resp.Response))
		t.Errorf(bad_status(want, resp.StatusCode))
	}

	if newValue != resp.Properties.Name {
		t.Errorf("Snapshot wasn't updated.")
	}
}

func TestDeleteSnapshot(t *testing.T) {
	want := 202

	resp := DeleteSnapshot(snapshotId)

	if resp.StatusCode != want {
		fmt.Println(string(resp.Body))
		t.Errorf(bad_status(want, resp.StatusCode))
	}

	resp = DeleteDatacenter(dcID)

	if resp.StatusCode != want {
		fmt.Println(string(resp.Body))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}
