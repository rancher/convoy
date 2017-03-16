package profitbricks

import(
  "os"
  "fmt"
  "errors"
  "time"
  "strings"

  "github.com/Sirupsen/logrus"
  "github.com/dselans/dmidecode"
  sdk "github.com/profitbricks/profitbricks-sdk-go"
)

const(
  DEFAULT_DEPTH = "5"
  DEFAULT_LICENSE_TYPE = "OTHER"
  TIMEOUT = 120
)

var(
  log = logrus.WithFields(logrus.Fields{"pkg": "profitbricks"})
  datacenterId string
  serverId string
)

type Client struct {}

type Volume struct {
  Name string
  Id string
  Device string
  MountPoint string
  Size int
  configPath string
  Type string
  State string
  AvailabilityZone string
  CreationTime string
  DeviceNumber int
  Snapshots  map[string]Snapshot
}

type Snapshot struct {
  Name string
  Id string
  Description string
  Size int
  State string
  Location string
  CreationTime string
}

type CreateVolumeParams struct {
  Id string
  Name string
  Size int
  Type string
  SnapshotId string
}

func InitClient() (Client, error) {
  user, password, err := getCredentials()
  if err != nil {
    return Client{}, err
  }
  sdk.SetAuth(user, password)
  sdk.SetDepth(DEFAULT_DEPTH)

  datacenterId, err = getDatacenterId()
  if err != nil {
    return Client{}, err
  }
  serverId, err = getServerId()
  if err != nil {
    return Client{}, err
  }
  return Client{}, nil
}

func getCredentials() (string, string, error) {
	user := os.Getenv("PROFITBRICKS_USERNAME")
  password := os.Getenv("PROFITBRICKS_PASSWORD")
	if user == "" {
		return "", "", errors.New("No user found. Please set the PROFITBRICKS_USERNAME environment variable.")
	}
  if password == "" {
		return "", "", errors.New("No password found. Please set the PROFITBRICKS_PASSWORD environment variable.")
	}
	return user, password, nil
}

func getDatacenterId() (string, error) {
  datacenterId := os.Getenv("DATACENTER_ID")
  if datacenterId == "" {
		return "", errors.New("No datacenter ID found. Please set the DATACENTER_ID environment variable.")
	}
  return datacenterId, nil
}

func getServerId() (string, error) {
  dmi := dmidecode.New()
  if err := dmi.Run(); err != nil {
      return "", fmt.Errorf("Unable to get dmidecode information. Error: %v\n", err)
  }

  systemInfo, err := dmi.SearchByName("System Information")
  if err != nil {
    return "", fmt.Errorf("Unable to get System Information. Error: %v\n", err)
  }

	uuid := string(systemInfo["UUID"])
  return strings.ToLower(uuid), nil
}

func checkResponse(statusCode int, response, action string) (error) {
  if statusCode > 299 {
    logError(statusCode, response, action)
    return errors.New("Bad response from ProfitBricks API.")
  }
  return nil
}

func logError(statusCode int, response, action string) {
  log.WithFields(logrus.Fields{
    "status_code": statusCode,
    "response": response,
  }).Errorf("%s call failed.", action)
}

func waitFor(path string) (bool, error) {
  counter := 0
  for counter <= TIMEOUT {
    request, err := getRequestStatus(path)
    if err != nil {
      return false, err
    }
    if request.Metadata.Status == "DONE" {
      return true, nil
    }
    time.Sleep(time.Second * 1)
    counter++
  }
  return false, fmt.Errorf("Timed out checking the status of request %s", path)
}

func getRequestStatus(path string) (sdk.RequestStatus, error) {
  status := sdk.GetRequestStatus(path)
  err := checkResponse(status.StatusCode, status.Response, "Get Request status")
  if err != nil {
    return sdk.RequestStatus{}, err
  }
  return status, nil
}

func (c *Client) GetDeviceSuffix(deviceNumber int) (string) {
  if deviceNumber == 0 {
    return ""
  } else {
    return string('a' - 1 + deviceNumber)
  }
}

func (c *Client) CreateVolume(params CreateVolumeParams) (Volume, error) {
  if params.Id == "" && params.SnapshotId == "" {
    volume := sdk.CreateVolume(datacenterId, sdk.Volume{
      Properties: sdk.VolumeProperties{
        Name: params.Name,
        Size: params.Size,
        Type: params.Type,
        LicenceType: DEFAULT_LICENSE_TYPE,
      },
    })
    err := checkResponse(volume.StatusCode, volume.Response, "Create volume")
    if err != nil {
      return Volume{}, errors.New(fmt.Sprintf("Creation of volume: %s failed.", params.Name))
    }
    if success, err := waitFor(volume.Headers.Get("Location")); success {
      return toVolume(volume), nil
    } else {
      return Volume{}, errors.New(fmt.Sprintf("Creation of volume: %s failed. Reason: %s", params.Name, err))
    }
  } else if params.Id != "" && params.SnapshotId == "" {
    volume, err := getVolume(params.Id)
    if err != nil {
      return Volume{}, err
    }

    volumeSize := volume.Size
    if params.Size > volumeSize {
      volumeSize = params.Size
    }

    updatedVolume := sdk.PatchVolume(datacenterId, params.Id, sdk.VolumeProperties{
      Name: params.Name,
      Size: volumeSize,
    })
    err = checkResponse(updatedVolume.StatusCode, updatedVolume.Response, "Patch volume")
    if err != nil {
      return Volume{}, fmt.Errorf("Failed to PATCH volume with UUID %s. Reason: %s", params.Id, err)
    }
    if success, err := waitFor(updatedVolume.Headers.Get("Location")); success {
      return toVolume(updatedVolume), nil
    } else {
      return Volume{}, err
    }
  } else if params.Id == "" && params.SnapshotId != "" {
    snapshot, err := getSnapshot(params.SnapshotId)
    if err != nil {
      return Volume{}, errors.New(fmt.Sprintf("Failed to retrieve snapshot: ", params.SnapshotId))
    }
    size := snapshot.Size
    if params.Size > size {
      size = params.Size
    }
    volume := sdk.CreateVolume(datacenterId, sdk.Volume{
      Properties: sdk.VolumeProperties{
        Name: params.Name,
        Size: size,
        Type: params.Type,
        Image: params.SnapshotId,
      },
    })
    err = checkResponse(volume.StatusCode, volume.Response, "Create volume")
    if err != nil {
      return Volume{}, errors.New(fmt.Sprintf("Creation of volume: %s failed.", params.Name))
    }
    if success, err := waitFor(volume.Headers.Get("Location")); success {
      return toVolume(volume), nil
    } else {
      return Volume{}, errors.New(fmt.Sprintf("Creation of volume: %s failed. Reason: %s", params.Name, err))
    }
  } else if params.Id != "" && params.SnapshotId != "" {
    return Volume{}, errors.New("Cannot restore snapshot to existing volume. You can either add an existing volume to Convoy, or create a new volume from snapshot.")
  } else {
    return Volume{}, errors.New("Bad argument combination for the CreateVolume request.")
  }
}

func (c *Client) DeleteVolume(volumeId string) (error) {
  response := sdk.DeleteVolume(datacenterId, volumeId)
  err := checkResponse(response.StatusCode, "", "Delete volume")
  if err != nil {
    return errors.New(fmt.Sprint("Failed to permanently delete volume: ", volumeId))
  }
  return nil
}

func (c *Client) AttachVolume(volumeId string) (Volume, error) {
  response := sdk.AttachVolume(datacenterId, serverId, volumeId)
  err := checkResponse(response.StatusCode, response.Response, "Attach volume")
  if err != nil {
    return Volume{}, err
  }
  if success, err := waitFor(response.Headers.Get("Location")); success {
    volume, err := getVolume(volumeId)
    if err != nil {
      return Volume{}, err
    }
    return volume, nil
  } else {
    return Volume{}, err
  }
}

func (c *Client) GetVolume(volumeId string) (Volume, error) {
  return getVolume(volumeId)
}

func getVolume(volumeId string) (Volume, error) {
  volume := sdk.GetVolume(datacenterId, volumeId)
  err := checkResponse(volume.StatusCode, volume.Response, "Get volume")
  if err != nil {
    return Volume{}, err
  }
  return toVolume(volume), nil
}

func (c *Client) CreateSnapshot(volumeId, name string) (Snapshot, error) {
  snapshot := sdk.CreateSnapshot(datacenterId, volumeId, name)
  err := checkResponse(snapshot.StatusCode, snapshot.Response, "Create snapshot")
  if err != nil {
    return Snapshot{}, errors.New(fmt.Sprint("Failed to create snapshot: ", name))
  }
  if success, err := waitFor(snapshot.Headers.Get("Location")); success {
    provisionedSnapshot, err := getSnapshot(snapshot.Id) // Refresh snapshot info
    if err != nil {
      return Snapshot{}, err
    }
    return provisionedSnapshot, nil
  } else {
    return Snapshot{}, err
  }
}

func (c *Client) DeleteSnapshot(snapshotId string) (error) {
  response := sdk.DeleteSnapshot(snapshotId)
  err := checkResponse(response.StatusCode, "", "Delete snapshot")
  if err != nil {
    return errors.New(fmt.Sprint("Failed to permanently delete snapshot: ", snapshotId))
  }
  return nil
}

func (c *Client) GetSnapshot(snapshotId string) (Snapshot, error) {
  return getSnapshot(snapshotId)
}

func getSnapshot(snapshotId string) (Snapshot, error) {
  snapshot := sdk.GetSnapshot(snapshotId)
  err := checkResponse(snapshot.StatusCode, snapshot.Response, "Get snapshot")
  if err != nil {
    return Snapshot{}, err
  }
  return toSnapshot(snapshot), nil
}

func toVolume(volume sdk.Volume) (Volume) {
  return Volume{
    Name: volume.Properties.Name,
    Id: volume.Id,
    Type: volume.Properties.Type,
    Size: volume.Properties.Size,
    State: volume.Metadata.State,
    AvailabilityZone: volume.Properties.AvailabilityZone,
    CreationTime: volume.Metadata.CreatedDate.String(),
    DeviceNumber: int(volume.Properties.DeviceNumber),
  }
}

func toSnapshot(snapshot sdk.Snapshot) (Snapshot) {
  return Snapshot{
    Name: snapshot.Properties.Name,
    Id: snapshot.Id,
    Description: snapshot.Properties.Description,
    Size: snapshot.Properties.Size,
    State: snapshot.Metadata.State,
    Location: snapshot.Properties.Location,
    CreationTime: snapshot.Metadata.CreatedDate.String(),
  }
}
