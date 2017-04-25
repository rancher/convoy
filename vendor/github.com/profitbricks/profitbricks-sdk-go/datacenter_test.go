package profitbricks

import (
	"testing"
)

var dcID string

func TestListDatacenters(t *testing.T) {
	setupTestEnv()
	want := 200

	resp := ListDatacenters()

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestCreateDatacenter(t *testing.T) {
	want := 202
	var obj = Datacenter{
		Properties: DatacenterProperties{
			Name:        "GO SDK",
			Description: "description",
			Location:    location,
		},
	}
	resp := CompositeCreateDatacenter(obj)
	dcID = resp.Id

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}
func TestGetDatacenter(t *testing.T) {
	want := 200
	resp := GetDatacenter(dcID)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestPatchDatacenter(t *testing.T) {
	want := 202
	newName := "Renamed DC"
	obj := DatacenterProperties{Name: newName} //map[string]string{"name": "Renamed DC"}

	resp := PatchDatacenter(dcID, obj)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
	if resp.Properties.Name != newName {
		t.Errorf("Not updated")
	}

}

func TestDeleteDatacenter(t *testing.T) {
	want := 202
	resp := DeleteDatacenter(dcID)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}
