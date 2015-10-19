package metadata

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type Handler struct {
	url string
}

func NewHandler(url string) Handler {
	return Handler{url}
}

func (m *Handler) SendRequest(path string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", m.url+"/latest"+path, nil)
	req.Header.Add("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (m *Handler) GetVersion() (string, error) {
	resp, err := m.SendRequest("/version")
	if err != nil {
		return "", err
	}
	return string(resp[:]), nil
}

func (m *Handler) GetSelfContainer() (Container, error) {
	resp, err := m.SendRequest("/self/container")
	var container Container
	if err != nil {
		return container, err
	}

	if err = json.Unmarshal(resp, &container); err != nil {
		return container, err
	}

	return container, nil
}

func (m *Handler) GetSelfStack() (Stack, error) {
	resp, err := m.SendRequest("/self/stack")
	var stack Stack
	if err != nil {
		return stack, err
	}

	if err = json.Unmarshal(resp, &stack); err != nil {
		return stack, err
	}

	return stack, nil
}

func (m *Handler) GetServices() ([]Service, error) {
	resp, err := m.SendRequest("/services")
	var services []Service
	if err != nil {
		return services, err
	}

	if err = json.Unmarshal(resp, &services); err != nil {
		return services, err
	}
	return services, nil
}

func (m *Handler) GetContainers() ([]Container, error) {
	resp, err := m.SendRequest("/containers")
	var containers []Container
	if err != nil {
		return containers, err
	}

	if err = json.Unmarshal(resp, &containers); err != nil {
		return containers, err
	}
	return containers, nil
}

func (m *Handler) GetHosts() ([]Host, error) {
	resp, err := m.SendRequest("/hosts")
	var hosts []Host
	if err != nil {
		return hosts, err
	}

	if err = json.Unmarshal(resp, &hosts); err != nil {
		return hosts, err
	}
	return hosts, nil
}

func (m *Handler) GetHost(UUID string) (Host, error) {
	var host Host
	hosts, err := m.GetHosts()
	if err != nil {
		return host, err
	}
	for _, host := range hosts {
		if host.UUID == UUID {
			return host, nil
		}
	}

	return host, fmt.Errorf("could not find host by UUID %v", UUID)
}
