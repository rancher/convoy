package longhorn

import (
	"errors"
	"fmt"
	"time"

	rancherClient "github.com/rancher/go-rancher/client"
)

func Backoff(maxDuration time.Duration, timeoutMessage string, f func() (bool, error)) error {
	startTime := time.Now()
	waitTime := 150 * time.Millisecond
	maxWaitTime := 2 * time.Second
	for {
		if time.Now().Sub(startTime) > maxDuration {
			return errors.New(timeoutMessage)
		}

		if done, err := f(); err != nil {
			return err
		} else if done {
			return nil
		}

		time.Sleep(waitTime)

		waitTime *= 2
		if waitTime > maxWaitTime {
			waitTime = maxWaitTime
		}
	}
}

func WaitFor(client *rancherClient.RancherClient, resource *rancherClient.Resource, output interface{}, transitioning func() string) error {
	return Backoff(5*time.Minute, fmt.Sprintf("Failed waiting for %s:%s", resource.Type, resource.Id), func() (bool, error) {
		err := client.Reload(resource, output)
		if err != nil {
			return false, err
		}
		if transitioning() != "yes" {
			return true, nil
		}
		return false, nil
	})
}

func WaitService(client *rancherClient.RancherClient, service *rancherClient.Service) error {
	return WaitFor(client, &service.Resource, service, func() string {
		return service.Transitioning
	})
}

func WaitEnvironment(client *rancherClient.RancherClient, env *rancherClient.Environment) error {
	return WaitFor(client, &env.Resource, env, func() string {
		return env.Transitioning
	})
}
