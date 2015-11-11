package metadata

type Stack struct {
	EnvironmentName string   `json:"environment_name"`
	Name            string   `json:"name"`
	Services        []string `json:"services"`
}

type Service struct {
	Name        string            `json:"name"`
	StackName   string            `json:"stack_name"`
	Kind        string            `json:"kind"`
	Hostname    string            `json:"hostname"`
	Vip         string            `json:"vip"`
	CreateIndex string            `json:"create_index"`
	ExternalIps []string          `json:"external_ips"`
	Sidekicks   []string          `json:"sidekicks"`
	Containers  []string          `json:"containers"`
	Ports       []string          `json:"ports"`
	Labels      map[string]string `json:"labels"`
	Links       map[string]string `json:"links"`
	Metadata    map[string]string `json:"metadata"`
}

type Container struct {
	Name        string            `json:"name"`
	PrimaryIp   string            `json:"primary_ip"`
	Ips         []string          `json:"ips"`
	Ports       []string          `json:"ports"`
	ServiceName string            `json:"service_name"`
	StackName   string            `json:"stack_name"`
	Labels      map[string]string `json:"labels"`
	CreateIndex int               `json:"create_index"`
	HostUUID    string            `json:"host_uuid"`
}

type Host struct {
	Name    string            `json:"name"`
	AgentIP string            `json:"agent_ip"`
	HostId  int               `json:"host_id"`
	Labels  map[string]string `json:"labels"`
	UUID    string            `json:"uuid"`
}
