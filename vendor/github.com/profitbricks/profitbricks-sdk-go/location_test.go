package profitbricks

import (
	"testing"
	"fmt"
)

var locid string

func TestListLocations(t *testing.T) {
	setupTestEnv()
	want := 200
	resp := ListLocations()
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
	locid = resp.Items[0].Id
}

func TestGetLocation(t *testing.T) {
	//t.Parallel()
	want := 200
	resp := GetLocation("us/las")

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}
