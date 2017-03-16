// loadbalancer_test.go
package profitbricks

import (
	"fmt"
	"testing"
)

var lbal_dcid string
var lbalid string
var lbal_srvid string
var lbal_ipid string

func TestCreateLoadbalancer(t *testing.T) {
	setupTestEnv()
	want := 202
	lbal_dcid = mkdcid("GO SDK LB DC")
	lbal_srvid = mksrvid(lbal_dcid)
	var obj = IpBlock{
		Properties: IpBlockProperties{
			Size:     1,
			Location: "us/las",
		},
	}
	resp := ReserveIpBlock(obj)
	waitTillProvisioned(resp.Headers.Get("Location"))
	lbal_ipid = resp.Id
	var request = Loadbalancer{
		Properties: LoadbalancerProperties{
			Name: "test",
			Ip:   resp.Properties.Ips[0],
			Dhcp: true,
		},
	}

	resp1 := CreateLoadbalancer(lbal_dcid, request)
	waitTillProvisioned(resp1.Headers.Get("Location"))
	lbalid = resp1.Id
	fmt.Println("Loadbalancer ID", lbalid)
	if resp1.StatusCode != want {
		t.Errorf(bad_status(want, resp1.StatusCode))
	}

}

func TestListLoadbalancers(t *testing.T) {
	want := 200
	resp := ListLoadbalancers(lbal_dcid)

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestGetLoadbalancer(t *testing.T) {
	want := 200
	fmt.Println("TestGetLoadbalancer", lbalid)

	resp := GetLoadbalancer(lbal_dcid, lbalid)

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestPatchLoadbalancer(t *testing.T) {
	want := 202

	obj := LoadbalancerProperties{Name: "Renamed Loadbalancer"}

	resp := PatchLoadbalancer(lbal_dcid, lbalid, obj)
	waitTillProvisioned(resp.Headers.Get("Location"))
	if resp.StatusCode != want {
		fmt.Println(string(resp.Response))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestAssociateNic(t *testing.T) {
	want := 202

	nicid = mknic(lbal_dcid, lbal_srvid)
	fmt.Println("AssociateNic params ", lbal_dcid, lbalid, nicid)
	resp := AssociateNic(lbal_dcid, lbalid, nicid)
	waitTillProvisioned(resp.Headers.Get("Location"))
	nicid = resp.Id
	if resp.StatusCode != want {
		t.Error(resp.Response)
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestGetBalancedNics(t *testing.T) {
	want := 200
	resp := ListBalancedNics(lbal_dcid, lbalid)

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestGetBalancedNic(t *testing.T) {
	want := 200
	resp := GetBalancedNic(lbal_dcid, lbalid, nicid)

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestDeleteBalancedNic(t *testing.T) {
	want := 202

	resp := DeleteBalancedNic(lbal_dcid, lbalid, nicid)
	waitTillProvisioned(resp.Headers.Get("Location"))

	if resp.StatusCode != want {
		t.Error(string(resp.Body))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestDeleteLoadbalancer(t *testing.T) {
	want := 202
	resp := DeleteLoadbalancer(lbal_dcid, lbalid)
	waitTillProvisioned(resp.Headers.Get("Location"))
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestLoadBalancerCleanup(t *testing.T) {
	resp := DeleteDatacenter(lbal_dcid)
	waitTillProvisioned(resp.Headers.Get("Location"))
	DeleteDatacenter(dcID)
	ReleaseIpBlock(lbal_ipid)

}
