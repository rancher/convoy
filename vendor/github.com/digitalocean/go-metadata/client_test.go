package metadata

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestDropletID(t *testing.T) {
	var (
		resp = "4567"
		want = 4567
	)
	withServer(t, "/metadata/v1/id", resp, func(client *Client) {
		got, err := client.DropletID()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("want=%#v", want)
			t.Errorf(" got=%#v", got)
		}
	})
}

func TestHostname(t *testing.T) {
	var (
		resp = "localhost"
		want = "localhost"
	)
	withServer(t, "/metadata/v1/hostname", resp, func(client *Client) {
		got, err := client.Hostname()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("want=%#v", want)
			t.Errorf(" got=%#v", got)
		}
	})
}

func TestUserData(t *testing.T) {
	var (
		resp = "#!/bin/sh\necho 'hello world'"
		want = "#!/bin/sh\necho 'hello world'"
	)
	withServer(t, "/metadata/v1/user-data", resp, func(client *Client) {
		got, err := client.UserData()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("want=%#v", want)
			t.Errorf(" got=%#v", got)
		}
	})
}

func TestVendorData(t *testing.T) {
	var (
		resp = "#!/bin/sh\necho 'hello world'"
		want = "#!/bin/sh\necho 'hello world'"
	)
	withServer(t, "/metadata/v1/vendor-data", resp, func(client *Client) {
		got, err := client.VendorData()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("want=%#v", want)
			t.Errorf(" got=%#v", got)
		}
	})
}

func TestRegion(t *testing.T) {
	var (
		resp = "nyc3"
		want = "nyc3"
	)
	withServer(t, "/metadata/v1/region", resp, func(client *Client) {
		got, err := client.Region()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("want=%#v", want)
			t.Errorf(" got=%#v", got)
		}
	})
}

func TestPublicKeys(t *testing.T) {
	var (
		resp = "ssh-rsa sshkeysshkeysshkey1 user@workstation2\nssh-rsa sshkeysshkeysshkey2 user@workstation2"
		want = []string{"ssh-rsa sshkeysshkeysshkey1 user@workstation2", "ssh-rsa sshkeysshkeysshkey2 user@workstation2"}
	)
	withServer(t, "/metadata/v1/public-keys", resp, func(client *Client) {
		got, err := client.PublicKeys()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("want=%#v", want)
			t.Errorf(" got=%#v", got)
		}
	})
}

func TestNameservers(t *testing.T) {
	var (
		resp = "8.8.8.8\n8.8.4.4"
		want = []string{"8.8.8.8", "8.8.4.4"}
	)
	withServer(t, "/metadata/v1/dns/nameservers", resp, func(client *Client) {
		got, err := client.Nameservers()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("want=%#v", want)
			t.Errorf(" got=%#v", got)
		}
	})
}

func TestFloatingIPv4Active(t *testing.T) {
	tests := []struct {
		resp string
		want bool
	}{
		{
			resp: "true",
			want: true,
		},
		{
			resp: "false",
			want: false,
		},
		{
			resp: "",
			want: false,
		},
		{
			resp: "something stange",
			want: false,
		},
	}

	for _, test := range tests {
		withServer(t, "/metadata/v1/floating_ip/ipv4/active", test.resp, func(client *Client) {
			got, err := client.FloatingIPv4Active()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(test.want, got) {
				t.Errorf("want=%#v", test.want)
				t.Errorf(" got=%#v", got)
			}
		})
	}
}

func TestMetadata(t *testing.T) {
	resp := `{
  "droplet_id": 7473395,
  "hostname": "workstation.nyc3",
  "user_data": "#cloud-config\ndisable_root: false\nmanage_etc_hosts: true\n\n# The modules that run in the 'init' stage\ncloud_init_modules:\n - migrator\n - ubuntu-init-switch\n - seed_random\n - bootcmd\n - write-files\n - growpart\n - resizefs\n - set_hostname\n - update_hostname\n - [ update_etc_hosts, once-per-instance ]\n - ca-certs\n - rsyslog\n - users-groups\n - ssh\n\n# The modules that run in the 'config' stage\ncloud_config_modules:\n - disk_setup\n - mounts\n - ssh-import-id\n - locale\n - set-passwords\n - grub-dpkg\n - apt-pipelining\n - apt-configure\n - package-update-upgrade-install\n - landscape\n - timezone\n - puppet\n - chef\n - salt-minion\n - mcollective\n - disable-ec2-metadata\n - runcmd\n - byobu\n\n# The modules that run in the 'final' stage\ncloud_final_modules:\n - rightscale_userdata\n - scripts-vendor\n - scripts-per-once\n - scripts-per-boot\n - scripts-per-instance\n - scripts-user\n - ssh-authkey-fingerprints\n - keys-to-console\n - phone-home\n - final-message\n - power-state-change\n",
  "vendor_data": "#cloud-config\ndisable_root: false\nmanage_etc_hosts: true\n\n# The modules that run in the 'init' stage\ncloud_init_modules:\n - migrator\n - ubuntu-init-switch\n - seed_random\n - bootcmd\n - write-files\n - growpart\n - resizefs\n - set_hostname\n - update_hostname\n - [ update_etc_hosts, once-per-instance ]\n - ca-certs\n - rsyslog\n - users-groups\n - ssh\n\n# The modules that run in the 'config' stage\ncloud_config_modules:\n - disk_setup\n - mounts\n - ssh-import-id\n - locale\n - set-passwords\n - grub-dpkg\n - apt-pipelining\n - apt-configure\n - package-update-upgrade-install\n - landscape\n - timezone\n - puppet\n - chef\n - salt-minion\n - mcollective\n - disable-ec2-metadata\n - runcmd\n - byobu\n\n# The modules that run in the 'final' stage\ncloud_final_modules:\n - rightscale_userdata\n - scripts-vendor\n - scripts-per-once\n - scripts-per-boot\n - scripts-per-instance\n - scripts-user\n - ssh-authkey-fingerprints\n - keys-to-console\n - phone-home\n - final-message\n - power-state-change\n",
  "public_keys": [
    "ssh-rsa sshkeysshkeysshkey1 user@workstation2",
    "ssh-rsa sshkeysshkeysshkey2 user@workstation2"
  ],
  "region": "nyc3",
  "interfaces": {
    "public": [
      {
        "anchor_ipv4": {
          "gateway": "192.168.0.1",
          "netmask": "255.255.0.0",
          "ip_address": "192.168.0.100"
        },
        "ipv4": {
          "ip_address": "192.168.0.100",
          "netmask": "255.255.240.0",
          "gateway": "192.168.0.1"
        },
        "ipv6": {
          "ip_address": "192.168.0.100",
          "cidr": 16,
          "gateway": "192.168.0.1"
        },
        "mac": "DE:AD:BE:EF:DE:AD",
        "type": "public"
      }
    ]
  },
  "dns": {
    "nameservers": [
      "8.8.8.8",
      "8.8.4.4"
    ]
  },
  "floating_ip": {
    "ipv4": {
      "ip_address": "192.168.0.100",
      "active": true
    }
  }
}`

	var (
		want = new(Metadata)
		got  *Metadata
	)

	if err := json.NewDecoder(strings.NewReader(resp)).Decode(want); err != nil {
		t.Fatalf("%#v", err)
	}

	withServer(t, "/metadata/v1.json", resp, func(client *Client) {
		var err error
		got, err = client.Metadata()
		if err != nil {
			t.Fatal(err)
		}
	})
	if !reflect.DeepEqual(*want, *got) {
		t.Errorf("want=%#v", *want)
		t.Errorf(" got=%#v", *got)
	}
}

func withServer(t testing.TB, path, response string, test func(*Client)) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != path {
			http.Error(rw, "bad path", http.StatusBadRequest)
			t.Errorf("bad URL sent to server: %v", req.URL.String())
			return
		}
		rw.Write([]byte(response))
	}))
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		panic(err)
	}
	test(NewClient(WithBaseURL(u)))
}
