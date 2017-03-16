// server_test.go
package profitbricks

import (
	"fmt"
	"sync"
	"testing"
)

var (
	once_dc sync.Once
	once_srv sync.Once
	once_volume sync.Once
	srv_dc_id string
	srv_srvid string
	srv_vol string
	imageId string
)

func setupDataCenter() {
	setupTestEnv()
	srv_dc_id = mkdcid("GO SDK SERVER DC 02")
	fmt.Println("Datacenter id: ", srv_dc_id)
	if len(srv_dc_id) == 0 {
		fmt.Errorf("DataCenter not created %s", srv_dc_id)
	}
}

func setupServer() {
	srv_srvid = setupCreateServer(srv_dc_id)
	fmt.Println("Server id: ", srv_srvid)
	if len(srv_srvid) == 0 {
		fmt.Errorf("Server not created %s", srv_srvid)
	}
}

func setupVolume() {

	vol := Volume{
		Properties: VolumeProperties{
			Type:          "HDD",
			Image:         image,
			Size:          5,
			ImagePassword: "test1234",
		},
	}
	resp := CreateVolume(srv_dc_id, vol)
	srv_vol = resp.Id

	waitTillProvisioned(resp.Headers.Get("Location"))
	if len(srv_vol) == 0 {
		fmt.Errorf("Volume not created %s", 1)
	}

}

func setupCreateServer(srv_dc_id string) string {

	var req = Server{
		Properties: ServerProperties{
			Name:  "test",
			Ram:   1024,
			Cores: 2,
		},
	}
	fmt.Println("Creating server....")
	srv := CreateServer(srv_dc_id, req)
	// wait for server to be running
	waitTillProvisioned(srv.Headers.Get("Location"))
	srvid := srv.Id
	return srvid
}

func TestCreateServer(t *testing.T) {
	once_dc.Do(setupDataCenter)

	want := 202

	var req = Server{
		Properties: ServerProperties{
			Name:  "go01",
			Ram:   1024,
			Cores: 2,
		},
	}
	t.Logf("Creating server in DC: %s", srv_dc_id)
	srv := CreateServer(srv_dc_id, req)
	waitTillProvisioned(srv.Headers.Get("Location"))
	srv_srvid = srv.Id
	if srv.StatusCode != want {
		t.Errorf(bad_status(want, srv.StatusCode))
	}
}

func TestGetServer(t *testing.T) {
	want := 200
	resp := GetServer(srv_dc_id, srv_srvid)

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestListServers(t *testing.T) {
	if srv_dc_id == "" {
		once_dc.Do(setupDataCenter)
		once_srv.Do(setupServer)
	}

	want := 200

	resp := ListServers(srv_dc_id)

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}

}

func TestPatchServer(t *testing.T) {
	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	want := 202
	req := ServerProperties{
		Name:  "go01renamed",
		Cores: 1,
	}
	fmt.Println("SERVER ID : ", srv_srvid)
	resp := PatchServer(srv_dc_id, srv_srvid, req)
	if resp.StatusCode != want {
		t.Error("resp: ", resp.Response)
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestStopServer(t *testing.T) {
	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	want := 202
	resp := StopServer(srv_dc_id, srv_srvid)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}

}

func TestStartServer(t *testing.T) {
	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	want := 202
	resp := StartServer(srv_dc_id, srv_srvid)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}

}

func TestRebootServer(t *testing.T) {
	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	want := 202
	resp := RebootServer(srv_dc_id, srv_srvid)
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}

}

func TestAttachImage(t *testing.T) {
	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)
	once_volume.Do(setupVolume)

	want := 202

	resp := AttachVolume(srv_dc_id, srv_srvid, srv_vol)
	waitTillProvisioned(resp.Headers.Get("Location"))

	if resp.StatusCode != want {
		t.Error(string(resp.Response))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestListAttachedVolumes(t *testing.T) {
	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	want := 200

	resp := ListAttachedVolumes(srv_dc_id, srv_srvid)

	if resp.StatusCode != want {
		t.Error(string(resp.Response))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestGetAttachedVolume(t *testing.T) {
	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	want := 200

	resp := GetAttachedVolume(srv_dc_id, srv_srvid, srv_vol)
	fmt.Println(resp)

	if resp.StatusCode != want {
		t.Error(string(resp.Response))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestDetachVolume(t *testing.T) {
	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)
	once_volume.Do(setupVolume)

	want := 202
	fmt.Println(srv_dc_id, srv_srvid, srv_vol)
	resp := DetachVolume(srv_dc_id, srv_srvid, srv_vol)
	if resp.StatusCode != want {
		t.Error(string(resp.Body))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestAttachCdrom(t *testing.T) {
	want := 202
	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	images := ListImages()
	for _, image := range images.Items {
		if image.Properties.ImageType == "CDROM" && image.Properties.Location == "us/las" {
			imageId = image.Id
			break
		}
	}

	resp := AttachCdrom(srv_dc_id, srv_srvid, imageId)

	waitTillProvisioned(resp.Headers.Get("Location"))

	if resp.StatusCode != want {
		t.Error(string(resp.Response))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestListAttachedCdroms(t *testing.T) {
	want := 200

	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	resp := ListAttachedCdroms(srv_dc_id, srv_srvid)
	if resp.StatusCode != want {
		t.Error(string(resp.Response))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestGetAttachedCdrom(t *testing.T) {
	want := 200

	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	resp := GetAttachedCdrom(srv_dc_id, srv_srvid, imageId)
	if resp.StatusCode != want {
		t.Error(string(resp.Response))
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestDetachCdrom(t *testing.T) {
	want := 202
	once_dc.Do(setupDataCenter)
	once_srv.Do(setupServer)

	resp := DetachCdrom(srv_dc_id, srv_srvid, imageId)

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}
}

func TestDeleteServer(t *testing.T) {
	once_dc.Do(setupDataCenter)
	once_dc.Do(setupServer)

	want := 202

	resp := DeleteServer(srv_dc_id, srv_srvid)

	if resp.StatusCode != want {
		t.Error(string(resp.Body))
		t.Errorf(bad_status(want, resp.StatusCode))
	}

	fmt.Println("Removed everything")
}

func TestCreateCompositeServer(t *testing.T) {
	once_dc.Do(setupDataCenter)

	want := 202

	var req = Server{
		Properties: ServerProperties{
			Name:  "server1",
			Ram:   2048,
			Cores: 1,
		},
		Entities: &ServerEntities{
			Volumes: &Volumes{
				Items: []Volume{
					{
						Properties: VolumeProperties{
							Type:          "HDD",
							Size:          5,
							Name:          "volume1",
							Image:         image,
							ImagePassword: "test1234",
						},
					},
				},
			},
			Nics: &Nics{
				Items: []Nic{
					{
						Properties: NicProperties{
							Name: "nic",
							Lan:  1,
						},
					},
				},
			},
		},
	}

	t.Logf("Creating server in DC: %s", srv_dc_id)
	srv := CreateServer(srv_dc_id, req)

	if srv.StatusCode != want {
		fmt.Println(srv.Response)
		t.Errorf(bad_status(want, srv.StatusCode))
	}
	waitTillProvisioned(srv.Headers.Get("Location"))

	resp := DeleteDatacenter(srv_dc_id)

	if resp.StatusCode != want {
		fmt.Println(srv.Response)
		t.Errorf(bad_status(want, resp.StatusCode))

	}
}
