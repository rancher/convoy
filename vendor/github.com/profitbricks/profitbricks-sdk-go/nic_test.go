package profitbricks

import (
	"testing"
)

var nic_dcid string
var nic_srvid string
var nicid string

func TestCreateNic(t *testing.T) {
	setupTestEnv()
	nic_dcid = mkdcid("GO SDK NIC DC")
	nic_srvid = mksrvid(nic_dcid)

	want := 202
	var request = Nic{
		Properties: NicProperties{
			Lan:  1,
			Name: "Test NIC",
			Nat:  false,
		},
	}

	resp := CreateNic(nic_dcid, nic_srvid, request)
	waitTillProvisioned(resp.Headers.Get("Location"))
	nicid = resp.Id
	if resp.StatusCode != want {
		t.Error(resp.Response)
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestListNics(t *testing.T) {
	//t.Parallel()
	want := 200
	resp := ListNics(nic_dcid, nic_srvid)

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestGetNic(t *testing.T) {
	want := 200
	resp := GetNic(nic_dcid, nic_srvid, nicid)

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}
func TestPatchNic(t *testing.T) {
	want := 202
	obj := NicProperties{Name: "Renamed Nic", Lan: 1}

	resp := PatchNic(nic_dcid, nic_srvid, nicid, obj)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}
func TestDeleteNic(t *testing.T) {
	want := 202
	resp := DeleteNic(nic_dcid, nic_srvid, nicid)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestNicCleanup(t *testing.T) {
	DeleteServer(nic_dcid, nic_srvid)
	DeleteDatacenter(nic_dcid)
}
