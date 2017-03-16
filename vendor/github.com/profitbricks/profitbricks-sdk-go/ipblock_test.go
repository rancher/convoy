// ipblock_test.go
package profitbricks

import "testing"

var ipblkid string

func TestListIpBlocks(t *testing.T) {
	setupTestEnv()
	want := 200
	resp := ListIpBlocks()
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestReserveIpBlock(t *testing.T) {
	want := 202
	var obj = IpBlock{
		Properties: IpBlockProperties{
			Size:     1,
			Location: "us/las",
		},
	}

	resp := ReserveIpBlock(obj)
	ipblkid = resp.Id
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestGetIpBlock(t *testing.T) {
	want := 200
	resp := GetIpBlock(ipblkid)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestReleaseIpBlock(t *testing.T) {
	want := 202
	resp := ReleaseIpBlock(ipblkid)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}
