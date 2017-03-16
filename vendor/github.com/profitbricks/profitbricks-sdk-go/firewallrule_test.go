package profitbricks

import (
	"fmt"
	"testing"
)

var fwId string

func setup() {
	datacenter := Datacenter{
		Properties: DatacenterProperties{
			Name:     "composite test",
			Location: location,
		},
		Entities: DatacenterEntities{
			Servers: &Servers{
				Items: []Server{
					Server{
						Properties: ServerProperties{
							Name:  "server1",
							Ram:   2048,
							Cores: 1,
						},
						Entities: &ServerEntities{
							Nics: &Nics{
								Items: []Nic{
									Nic{
										Properties: NicProperties{
											Name: "nic",
											Lan:  1,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	datacenter = CompositeCreateDatacenter(datacenter)
	waitTillProvisioned(datacenter.Headers.Get("Location"))

	dcID = datacenter.Id
	srv_srvid = datacenter.Entities.Servers.Items[0].Id
	nicid = datacenter.Entities.Servers.Items[0].Entities.Nics.Items[0].Id
}
func TestCreateFirewallRule(t *testing.T) {
	setupTestEnv()
	want := 202
	setup()

	fw := FirewallRule{
		Properties: FirewallruleProperties{
			Name:     "firewallrule",
			Protocol: "TCP",
		},
	}

	fw = CreateFirewallRule(dcID, srv_srvid, nicid, fw)

	waitTillProvisioned(fw.Headers.Get("Location"))

	if fw.StatusCode != want {
		t.Error(fw.Response)
		t.Errorf(bad_status(want, fw.StatusCode))
	}
	fwId = fw.Id
}

func TestGetFirewallRule(t *testing.T) {
	want := 200

	fw := GetFirewallRule(dcID, srv_srvid, nicid, fwId)
	if fw.StatusCode != want {
		t.Error(fw.Response)
		t.Errorf(bad_status(want, fw.StatusCode))
	}
}

func TestListFirewallRules(t *testing.T) {
	want := 200
	fws := ListFirewallRules(dcID, srv_srvid, nicid)
	if fws.StatusCode != want {
		t.Error(fws.Response)
		t.Errorf(bad_status(want, fws.StatusCode))
	}
}

func TestPatchFirewallRule(t *testing.T) {
	want := 202
	props := FirewallruleProperties{
		Name: "updated",
	}
	fw := PatchFirewallRule(dcID, srv_srvid, nicid, fwId, props)
	if fw.StatusCode != want {
		t.Error(fw.Response)
		t.Errorf(bad_status(want, fw.StatusCode))
	}

	if fw.Properties.Name == "updated" {
		fmt.Println("Test succeeded")
	}
}

func TestDeleteFirewallRule(t *testing.T) {
	want := 202
	resp := DeleteFirewallRule(dcID, srv_srvid, nicid, fwId)

	if resp.StatusCode != want {
		t.Error(string(resp.Body))
		t.Errorf(bad_status(want, resp.StatusCode))
	}

	DeleteDatacenter(dcID)
}
