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

	RANCHER_METADATA_URL = "http://rancher-metadata"
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "rancher"})
)

func GetIPsForServiceInStack(serviceName, stackName string) ([]string, error) {
	handler := metadata.NewHandler(RANCHER_METADATA_URL)
	containerName := ""
	for i := 0; i < RETRY_MAX; i++ {
		log.Debugf("Try to connect to Rancher metadata at %v", RANCHER_METADATA_URL)
		container, err := handler.GetSelfContainer()
		if err == nil {
			containerName = container.Name
			break
		}
		log.Debugf("Connection error %v. Retrying", err.Error())
		time.Sleep(RETRY_INTERVAL)
	}
	if containerName == "" {
		return nil, fmt.Errorf("Rancher metadata service return empty for container name")
	}
	log.Debugf("Got Rancher metadata at %v, Convoy container name %v", RANCHER_METADATA_URL, containerName)

	containers, err := handler.GetContainers()
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
