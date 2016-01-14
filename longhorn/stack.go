package longhorn

import (
	"errors"
	"fmt"
	"time"

	rancherClient "github.com/rancher/go-rancher/client"
)

const (
	RETRY_INTERVAL = 2 * time.Second
	RETRY_MAX      = 200
)

type Stack struct {
	Client        *rancherClient.RancherClient
	ExternalId    string
	Name          string
	Environment   map[string]interface{}
	Template      string
	ContainerName string
}

func (s *Stack) Create() (*rancherClient.Environment, error) {
	env, err := s.Find()
	if err != nil {
		return nil, err
	}

	config := &rancherClient.Environment{
		Name:          s.Name,
		ExternalId:    s.ExternalId,
		Environment:   s.Environment,
		DockerCompose: s.Template,
		StartOnCreate: true,
	}

	if env == nil {
		env, err = s.Client.Environment.Create(config)
		if err != nil {
			return nil, err
		}
	}

	if err := WaitEnvironment(s.Client, env); err != nil {
		return nil, err
	}

	if err := s.waitForServices(env, "active"); err != nil {
		log.Debugf("Failed waiting services to be ready to launch. Cleaning up %v", env.Name)
		if err := s.Client.Environment.Delete(env); err != nil {
			return nil, err
		}
	}

	return env, nil
}

func (s *Stack) Delete() error {
	env, err := s.Find()
	if err != nil || env == nil {
		return err
	}

	if err := s.Client.Environment.Delete(env); err != nil {
		return err
	}

	return WaitEnvironment(s.Client, env)
}

func (s *Stack) Find() (*rancherClient.Environment, error) {
	envs, err := s.Client.Environment.List(&rancherClient.ListOpts{
		Filters: map[string]interface{}{
			"name":         s.Name,
			"externalId":   s.ExternalId,
			"removed_null": nil,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(envs.Data) == 0 {
		return nil, nil
	}
	if len(envs.Data) > 1 {
		// This really shouldn't ever happen
		return nil, fmt.Errorf("More than one stack found for %s", s.Name)
	}

	return &envs.Data[0], nil
}

func (s *Stack) confirmControllerUpgrade(env *rancherClient.Environment) (*rancherClient.Service, error) {
	services, err := s.Client.Service.List(&rancherClient.ListOpts{
		Filters: map[string]interface{}{
			"environmentId": env.Id,
			"name":          "controller",
		},
	})
	if err != nil {
		return nil, err
	}

	if len(services.Data) != 1 {
		return nil, errors.New("Failed to find controller service")
	}

	controller := &services.Data[0]
	if err := WaitService(s.Client, controller); err != nil {
		return nil, err
	}

	if controller.State == "upgraded" {
		controller, err := s.Client.Service.ActionFinishupgrade(controller)
		if err != nil {
			return nil, err
		}
		err = WaitService(s.Client, controller)
		if err != nil {
			return nil, err
		}
	}

	return controller, nil
}

func (s *Stack) MoveController() error {
	env, err := s.Find()
	if err != nil {
		return err
	}

	controller, err := s.confirmControllerUpgrade(env)
	if err != nil {
		return err
	}

	if controller.LaunchConfig.Labels[AFFINITY_LABEL] != s.ContainerName {
		newLaunchConfig := controller.LaunchConfig
		newLaunchConfig.Labels[AFFINITY_LABEL] = s.ContainerName

		log.Infof("Moving controller to next to container %s", s.ContainerName)
		controller, err = s.Client.Service.ActionUpgrade(controller, &rancherClient.ServiceUpgrade{
			InServiceStrategy: &rancherClient.InServiceUpgradeStrategy{
				LaunchConfig: newLaunchConfig,
			},
		})
		if err != nil {
			return err
		}
		if _, err := s.confirmControllerUpgrade(env); err != nil {
			return err
		}
	}

	return nil
}

func (s *Stack) waitForServices(env *rancherClient.Environment, targetState string) error {
	var serviceCollection rancherClient.ServiceCollection
	ready := false

	if err := s.Client.GetLink(env.Resource, "services", &serviceCollection); err != nil {
		return err
	}
	targetServiceCount := len(serviceCollection.Data)

	for i := 0; !ready && i < RETRY_MAX; i++ {
		log.Debugf("Waiting for %v services in %v turn to %v state", targetServiceCount, env.Name, targetState)
		time.Sleep(RETRY_INTERVAL)
		if err := s.Client.GetLink(env.Resource, "services", &serviceCollection); err != nil {
			return err
		}
		services := serviceCollection.Data
		if len(services) != targetServiceCount {
			continue
		}
		incorrectState := false
		for _, service := range services {
			if service.State != targetState {
				incorrectState = true
				break
			}
		}
		if incorrectState {
			continue
		}
		ready = true
	}
	if !ready {
		return fmt.Errorf("Failed to wait for %v services in %v turn to %v state", targetServiceCount, env.Name, targetState)
	}
	log.Debugf("Services change state to %v in %v", targetState, env.Name)
	return nil
}

func (s *Stack) waitActive(service *rancherClient.Service) (*rancherClient.Service, error) {
	err := WaitService(s.Client, service)
	if err != nil || service.State != "upgraded" {
		return service, err
	}

	if _, err := s.Client.Service.ActionFinishupgrade(service); err != nil {
		return nil, err
	}

	if err := WaitService(s.Client, service); err != nil {
		return nil, err
	}

	if service.State != "active" {
		return nil, fmt.Errorf("Service %s is not active, got %s", service.Id, service.State)
	}

	return service, nil
}
