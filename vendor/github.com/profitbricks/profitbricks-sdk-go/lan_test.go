// lan_test.go
package profitbricks

import (
	"testing"
)

var lan_dcid string
var lanid string

func TestCreateLan(t *testing.T) {
	setupTestEnv()
	lan_dcid = mkdcid("GO SDK LAN DC")
	want := 202
	var request = Lan{
		Properties: LanProperties{
			Public: true,
			Name:   "Lan Test",
		},
	}
	lan := CreateLan(lan_dcid, request)
	waitTillProvisioned(lan.Headers.Get("Location"))
	lanid = lan.Id
	if lan.StatusCode != want {
		t.Errorf(bad_status(want, lan.StatusCode))
	}
}

func TestListLans(t *testing.T) {
	want := 200
	lans := ListLans(lan_dcid)

	if lans.StatusCode != want {
		t.Errorf(bad_status(want, lans.StatusCode))
	}
}

func TestGetLan(t *testing.T) {
	want := 200
	lan := GetLan(lan_dcid, lanid)

	if lan.StatusCode != want {
		t.Errorf(bad_status(want, lan.StatusCode))
	}
}

func TestPatchLan(t *testing.T) {
	want := 202
	obj := LanProperties{Public: false}

	lan := PatchLan(lan_dcid, lanid, obj)
	if lan.StatusCode != want {
		t.Errorf(bad_status(want, lan.StatusCode))
	}
}

func TestDeleteLan(t *testing.T) {
	want := 202
	resp := DeleteLan(lan_dcid, lanid)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestLanCleanup(t *testing.T) {
	DeleteLan(lan_dcid, lanid)
	DeleteDatacenter(lan_dcid)
}
