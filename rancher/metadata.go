package rancher

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/go-rancher-metadata/metadata"
	"time"
)

const (
	RETRY_INTERVAL = 5 * time.Second
	RETRY_MAX      = 20

	RANCHER_METADATA_URL = "http://rancher-metadata/latest"
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "rancher"})
)

func GetIPsForServiceInStack(serviceName, stackName string) ([]string, error) {
	log.Debugf("Try to connect to Rancher metadata at %v", RANCHER_METADATA_URL)
	client, err := metadata.NewClientAndWait(RANCHER_METADATA_URL)
	if err != nil {
		return nil, err
	}
	log.Debugf("Got Rancher metadata at %v", RANCHER_METADATA_URL)

	containers, err := client.GetContainers()
	if err != nil {
		return nil, err
	}
	ips := []string{}
	for _, container := range containers {
		if container.StackName == stackName && container.ServiceName == serviceName {
			if container.PrimaryIp != "" {
				ips = append(ips, container.PrimaryIp)
			}
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("Cannot find containers with stack %v service %v", stackName, serviceName)
	}
	return ips, nil
}
