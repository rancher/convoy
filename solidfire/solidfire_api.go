package solidfire

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Client struct {
	MVIP             string
	SVIP             string
	Login            string
	Password         string
	Endpoint         string
	DefaultAPIPort   int
	DefaultVolSize   int64
	DefaultAccountID int64
}

type QoS struct {
	MinIOPS   int64 `json:"minIOPS,omitempty"`
	MaxIOPS   int64 `json:"maxIOPS,omitempty"`
	BurstIOPS int64 `json:"burstIOPS,omitempty"`
	BurstTime int64 `json:"-"`
}

type SFVolumePair struct {
	ClusterPairID    int64  `json:"clusterPairID"`
	RemoteVolumeID   int64  `json:"remoteVolumeID"`
	RemoteSliceID    int64  `json:"remoteSliceID"`
	RemoteVolumeName string `json:"remoteVolumeName"`
	VolumePairUUID   string `json:"volumePairUUID"`
}

type SFVolume struct {
	VolumeID           int64          `json:"volumeID"`
	Name               string         `json:"name"`
	AccountID          int64          `json:"accountID"`
	CreateTime         string         `json:"createTime"`
	Status             string         `json:"status"`
	Access             string         `json:"access"`
	Enable512e         bool           `json:"enable512e"`
	Iqn                string         `json:"iqn"`
	ScsiEUIDeviceID    string         `json:"scsiEUIDeviceID"`
	ScsiNAADeviceID    string         `json:"scsiNAADeviceID"`
	Qos                QoS            `json:"qos"`
	VolumeAccessGroups []int64        `json:"volumeAccessGroups"`
	VolumePairs        []SFVolumePair `json:"volumePairs"`
	DeleteTime         string         `json:"deleteTime"`
	PurgeTime          string         `json:"purgeTime"`
	SliceCount         int64          `json:"sliceCount"`
	TotalSize          int64          `json:"totalSize"`
	BlockSize          int64          `json:"blockSize"`
	VirtualVolumeID    string         `json:"virtualVolumeID"`
	Attributes         interface{}    `json:"attributes"`
}

type SFSnapshot struct {
	SnapshotID int64       `json:"snapshotID"`
	VolumeID   int64       `json:"volumeID"`
	Name       string      `json:"name"`
	Checksum   string      `json:"checksum"`
	Status     string      `json:"status"`
	TotalSize  int64       `json:"totalSize"`
	GroupID    int64       `json:"groupID"`
	CreateTime string      `json:"createTime"`
	Attributes interface{} `json:"attributes"`
}

type CreateVolumeRequest struct {
	Name       string      `json:"name"`
	AccountID  int64       `json:"accountID"`
	TotalSize  int64       `json:"totalSize"`
	Enable512e bool        `json:"enable512e"`
	Qos        QoS         `json:"qos,omitempty"`
	Attributes interface{} `json:"attributes"`
}

type CreateVolumeResult struct {
	Id     int `json:"id"`
	Result struct {
		VolumeID int64 `json:"volumeID"`
	} `json:"result"`
}

type CloneVolumeRequest struct {
	VolumeID     int64       `json:"volumeID"`
	Name         string      `json:"name"`
	NewAccountID int64       `json:"newAccountID"`
	NewSize      int64       `json:"newSize"`
	Access       string      `json:"access"`
	SnapshotID   int64       `json:"snapshotID"`
	Attributes   interface{} `json:"attributes"`
}

type CloneVolumeResult struct {
	Id     int `json:"id"`
	Result struct {
		CloneID     int64 `json:"cloneID"`
		VolumeID    int64 `json:"volumeID"`
		AsyncHandle int64 `json:"asyncHandle"`
	} `json:"result"`
}

type CreateSnapshotRequest struct {
	VolumeID                int64       `json:"volumeID"`
	SnapshotID              int64       `json:"snapshotID"`
	Name                    string      `json:"name"`
	EnableRemoteReplication bool        `json:"enableRemoteReplication"`
	Retention               string      `json:"retention"`
	Attributes              interface{} `json:"attributes"`
}

type CreateSnapshotResult struct {
	Id     int `json:"id"`
	Result struct {
		SnapshotID int64  `json:"snapshotID"`
		Checksum   string `json:"checksum"`
	} `json:"result"`
}

type DeleteVolumeRequest struct {
	VolumeID int64 `json:"volumeID"`
}

type ISCSITarget struct {
	Ip        string
	Port      string
	Portal    string
	Iqn       string
	Lun       string
	Device    string
	Discovery string
}

type ListActiveVolumesRequest struct {
	StartVolumeID int64 `json:"startVolumeID"`
	Limit         int64 `json:"limit"`
}

type ListVolumesResult struct {
	Id     int `json:"id"`
	Result struct {
		Volumes []SFVolume `json:"volumes"`
	} `json:"result"`
}

type ListSnapshotsRequest struct {
	VolumeID int64 `json:"volumeID"`
}

type ListSnapshotsResult struct {
	Id     int `json:"id"`
	Result struct {
		Snapshots []SFSnapshot `json:"snapshots"`
	} `json:"result"`
}

type RollbackToSnapshotRequest struct {
	VolumeID         int64       `json:"volumeID"`
	SnapshotID       int64       `json:"snapshotID"`
	SaveCurrentState bool        `json:"saveCurrentState"`
	Name             string      `json:"name"`
	Attributes       interface{} `json:"attributes"`
}

type RollbackToSnapshotResult struct {
	Id     int `json:"id"`
	Result struct {
		Checksum   string `json:"checksum"`
		SnapshotID int64  `json:"snapshotID"`
	} `json:"result"`
}

type DeleteSnapshotRequest struct {
	SnapshotID int64 `json:"snapshotID"`
}

type AddVolumesToVolumeAccessGroupRequest struct {
	VolumeAccessGroupID int64   `json:"volumeAccessGroupID"`
	Volumes             []int64 `json:"volumes"`
}

type EmptyResponse struct {
	Id     int `json:"id"`
	Result struct {
	} `json:"result"`
}

type APIError struct {
	Id    int `json:"id"`
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Name    string `json:"name"`
	} `json:"error"`
}

func NewClient(endpoint string, svip string, defaultSize, defaultAccount int64) (client *Client) {
	rand.Seed(time.Now().UTC().UnixNano())
	client = &Client{Endpoint: endpoint,
		DefaultVolSize:   defaultSize,
		DefaultAccountID: defaultAccount,
		SVIP:             svip}
	return client
}

func (c *Client) Request(method string, params interface{}, id int) (response []byte, err error) {
	if c.Endpoint == "" {
		log.Debug("Endpoint is not set, unable to issue requests")
		err = errors.New("Unable to issue json-rpc requests without specifying Endpoint")
		return nil, err
	}
	data, err := json.Marshal(map[string]interface{}{
		"method": method,
		"id":     id,
		"params": params,
	})

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	Http := &http.Client{Transport: tr}
	resp, err := Http.Post(c.Endpoint,
		"json-rpc",
		strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return body, err
	}

	var prettyJson bytes.Buffer
	_ = json.Indent(&prettyJson, body, "", "  ")
	log.WithField("", prettyJson.String()).Debug("request:", id, " method:", method, " params:", params)

	errresp := APIError{}
	json.Unmarshal([]byte(body), &errresp)
	if errresp.Error.Code != 0 {
		err = errors.New("Received error response from API request")
		return body, err
	}
	return body, nil
}

func newReqID() int {
	return rand.Intn(1000-1) + 1
}

func (c *Client) CreateSnapshot(req *CreateSnapshotRequest) (snapshot SFSnapshot, err error) {
	response, err := c.Request("CreateSnapshot", req, newReqID())
	var result CreateSnapshotResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Fatal(err)
		return SFSnapshot{}, err
	}
	return c.GetSnapshot(result.Result.SnapshotID, "")
}

func (c *Client) ListSnapshots(req *ListSnapshotsRequest) (snapshots []SFSnapshot, err error) {
	response, err := c.Request("ListSnapshots", req, newReqID())
	if err != nil {
		log.Error(err)
		return nil, err
	}
	var result ListSnapshotsResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Fatal(err)
		return nil, err
	}
	snapshots = result.Result.Snapshots
	return

}

func (c *Client) RollbackToSnapshot(req *RollbackToSnapshotRequest) (newSnapID int64, err error) {
	response, err := c.Request("ListSnapshots", req, newReqID())
	if err != nil {
		log.Error(err)
		return 0, err
	}
	var result RollbackToSnapshotResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Fatal(err)
		return 0, err
	}
	newSnapID = result.Result.SnapshotID
	err = nil
	return

}

func (c *Client) DeleteSnapshot(snapshotID int64) (err error) {
	// TODO(jdg): Add options like purge=True|False, range, ALL etc
	var req DeleteSnapshotRequest
	req.SnapshotID = snapshotID
	_, err = c.Request("DeleteSnapshot", req, newReqID())
	if err != nil {
		log.Error("Failed to delete snapshot ID: ", snapshotID)
		return err
	}
	return
}
func (c *Client) CreateVolume(createReq *CreateVolumeRequest) (vol SFVolume, err error) {
	response, err := c.Request("CreateVolume", createReq, newReqID())
	var result CreateVolumeResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Fatal(err)
		return SFVolume{}, err
	}

	vol, err = c.GetVolume(result.Result.VolumeID, "")
	return
}

func (c *Client) DeleteVolume(volumeID int64) (err error) {
	// TODO(jdg): Add options like purge=True|False, range, ALL etc
	var req DeleteVolumeRequest
	req.VolumeID = volumeID
	_, err = c.Request("DeleteVolume", req, newReqID())
	if err != nil {
		log.Error("Failed to delete volume ID: ", volumeID)
		return err
	}
	return
}
func (c *Client) AddVolumesToAccessGroup(groupID int64, volIDs []int64) (err error) {
	req := &AddVolumesToVolumeAccessGroupRequest{
		VolumeAccessGroupID: groupID,
		Volumes:             volIDs,
	}
	_, err = c.Request("AddVolumesToVolumeAccessGroup", req, newReqID())
	if err != nil {
		log.Error("Failed to add volume(s) to VAG %d: ", groupID)
		return err
	}
	return err
}

func (c *Client) CloneVolume(req *CloneVolumeRequest) (vol SFVolume, err error) {
	response, err := c.Request("CloneVolume", req, newReqID())
	var result CloneVolumeResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Fatal(err)
		return SFVolume{}, err
	}
	vol, err = c.GetVolume(result.Result.VolumeID, "")
	return
}

func (c *Client) ListActiveVolumes(listVolReq *ListActiveVolumesRequest) (volumes []SFVolume, err error) {
	response, err := c.Request("ListActiveVolumes", listVolReq, newReqID())
	if err != nil {
		log.Error(err)
		return
	}
	var result ListVolumesResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		log.Fatal(err)
		return nil, err
	}
	volumes = result.Result.Volumes
	return
}

func (c *Client) GetVolume(sfID int64, sfName string) (v SFVolume, err error) {
	var listReq ListActiveVolumesRequest

	volumes, err := c.ListActiveVolumes(&listReq)
	if err != nil {
		fmt.Println("Error retrieving volumes")
		return SFVolume{}, err
	}
	for _, vol := range volumes {
		if sfID == vol.VolumeID {
			v = vol
			break
		} else if sfName != "" && sfName == v.Name {
			v = vol
			break
		}
	}
	return
}

func (c *Client) GetSnapshot(sfID int64, sfName string) (s SFSnapshot, err error) {
	var listReq ListSnapshotsRequest

	snapshots, err := c.ListSnapshots(&listReq)
	if err != nil {
		fmt.Println("Error retrieving volumes")
		return SFSnapshot{}, err
	}
	for _, snap := range snapshots {
		if sfID == snap.SnapshotID {
			s = snap
			break
		} else if sfName != "" && sfName == s.Name {
			s = snap
			break
		}
	}
	return
}

func (c *Client) AttachVolume(volumeID int64, name string) (path, device string, err error) {
	if c.SVIP == "" {
		err = errors.New("Unable to perform iSCSI actions without setting SVIP")
		return
	}
	path = "/dev/disk/by-path/ip-"
	if iscsiSupported() == false {
		err = errors.New("Unable to attach, open-iscsi tools not found on host")
		return
	}

	v, err := c.GetVolume(volumeID, name)
	if err != nil {
		err = errors.New("Failed to find volume for attach")
		return
	}

	path = path + c.SVIP + "-iscsi-" + v.Iqn + "-lun-0"
	device = getDeviceFileFromIscsiPath(path)
	if waitForPathToExist(path, 1) {
		return
	}

	targets, err := iscsiDiscovery(c.SVIP)
	if err != nil {
		log.Error("Failure encountered during iSCSI Discovery: ", err)
		log.Error("Have you setup the Volume Access Group?")
		err = errors.New("iSCSI Discovery failed")
		return
	}

	if len(targets) < 1 {
		log.Warning("Discovered zero targets at: ", c.SVIP)
		return
	}

	tgt := ISCSITarget{}
	for _, t := range targets {
		if strings.Contains(t, v.Iqn) {
			tgt.Discovery = t
		}
	}
	if tgt.Discovery == "" {
		log.Error("Failed to discover requested target: ", v.Iqn, " on: ", c.SVIP)
		return
	}
	log.Debug("Discovered target: ", tgt.Discovery)

	parsed := strings.FieldsFunc(tgt.Discovery, func(r rune) bool {
		return r == ',' || r == ' '
	})

	tgt.Ip = parsed[0]
	tgt.Iqn = parsed[2]
	err = iscsiLogin(&tgt)
	if err != nil {
		log.Error("Failed to connect to iSCSI target (", tgt.Iqn, ")")
		return
	}
	if waitForPathToExist(path, 10) == false {
		log.Error("Failed to find connection after 10 seconds")
		return
	}

	device = strings.TrimSpace(getDeviceFileFromIscsiPath(path))
	return
}

func (c *Client) DetachVolume(volumeID int64, name string) (err error) {
	if c.SVIP == "" {
		err = errors.New("Unable to perform iSCSI actions without setting SVIP")
		return
	}

	v, err := c.GetVolume(volumeID, name)
	if err != nil {
		err = errors.New("Failed to find volume for attach")
		return
	}
	tgt := &ISCSITarget{
		Ip:     c.SVIP,
		Portal: c.SVIP,
		Iqn:    v.Iqn,
	}
	err = iscsiDisableDelete(tgt)
	return
}

func (c *Client) GetIscsiDisk(identifier string) (device string) {
	devices := getAllDisksByPath(identifier)
	// First look for a part-1 with the specified identifier
	// If we don't find one, we'll use the root device
	for _, d := range devices {
		if strings.Contains(d, "lun-0-part1") {
			device = strings.Split(string(d), "../../")[1]
			return "/dev/" + device
		}
	}
	dName := strings.Split(string(devices[0]), "../../")[1]
	return "/dev/" + dName
}

func getAllDisksByPath(identifier string) []string {
	var devices []string
	out, err := exec.Command("sudo", "ls", "-la", "/dev/disk/by-path/").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if identifier == "" {
			devices = append(devices, l)
		} else if strings.Contains(l, identifier) {
			devices = append(devices, l)
		}
	}
	return devices
}

func getDeviceFileFromIscsiPath(iscsiPath string) (devFile string) {
	out, err := exec.Command("sudo", "ls", "-la", iscsiPath).Output()
	if err != nil {
		return
	}
	d := strings.Split(string(out), "../../")
	devFile = "/dev/" + d[1]
	return
}
func iscsiSupported() bool {
	_, err := exec.Command("iscsiadm", "-h").Output()
	if err != nil {
		log.Debug("iscsiadm tools not found on this host")
		return false
	}
	return true
}

func iscsiDiscovery(portal string) (targets []string, err error) {
	log.Debug("Issue sendtargets: sudo iscsiadm -m discovery -t sendtargets -p ", portal)
	out, err := exec.Command("sudo", "iscsiadm", "-m", "discovery", "-t", "sendtargets", "-p", portal).Output()
	if err != nil {
		log.Error("Error encountered in sendtargets cmd: ", out)
		return
	}
	targets = strings.Split(string(out), "\n")
	return

}

func iscsiLogin(tgt *ISCSITarget) (err error) {
	_, err = exec.Command("sudo", "iscsiadm", "-m", "node", "-p", tgt.Ip, "-T", tgt.Iqn, "--login").Output()
	return err
}

func iscsiDisableDelete(tgt *ISCSITarget) (err error) {
	_, err = exec.Command("sudo", "iscsiadm", "-m", "node", "-T", tgt.Iqn, "--portal", tgt.Ip, "-u").Output()
	if err != nil {
		return
	}
	_, err = exec.Command("sudo", "iscsiadm", "-m", "node", "-o", "delete", "-T", tgt.Iqn).Output()
	return
}

func waitForPathToExist(devicePath string, numTries int) bool {
	log.Debug("Check for presence of: ", devicePath)
	for i := 0; i < numTries; i++ {
		_, err := os.Stat(devicePath)
		if err == nil {
			return true
		}
		if err != nil && !os.IsNotExist(err) {
			return false
		}
		time.Sleep(time.Second)
	}
	return false
}
