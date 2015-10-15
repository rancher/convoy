package client

const (
	ENVIRONMENT_TYPE = "environment"
)

type Environment struct {
	Resource

	AccountId string `json:"accountId,omitempty" yaml:"account_id,omitempty"`

	Created string `json:"created,omitempty" yaml:"created,omitempty"`

	Data map[string]interface{} `json:"data,omitempty" yaml:"data,omitempty"`

	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	DockerCompose string `json:"dockerCompose,omitempty" yaml:"docker_compose,omitempty"`

	Environment map[string]interface{} `json:"environment,omitempty" yaml:"environment,omitempty"`

	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	RancherCompose string `json:"rancherCompose,omitempty" yaml:"rancher_compose,omitempty"`

	RemoveTime string `json:"removeTime,omitempty" yaml:"remove_time,omitempty"`

	Removed string `json:"removed,omitempty" yaml:"removed,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`

	Transitioning string `json:"transitioning,omitempty" yaml:"transitioning,omitempty"`

	TransitioningMessage string `json:"transitioningMessage,omitempty" yaml:"transitioning_message,omitempty"`

	TransitioningProgress int64 `json:"transitioningProgress,omitempty" yaml:"transitioning_progress,omitempty"`

	Uuid string `json:"uuid,omitempty" yaml:"uuid,omitempty"`
}

type EnvironmentCollection struct {
	Collection
	Data []Environment `json:"data,omitempty"`
}

type EnvironmentClient struct {
	rancherClient *RancherClient
}

type EnvironmentOperations interface {
	List(opts *ListOpts) (*EnvironmentCollection, error)
	Create(opts *Environment) (*Environment, error)
	Update(existing *Environment, updates interface{}) (*Environment, error)
	ById(id string) (*Environment, error)
	Delete(container *Environment) error

	ActionCreate(*Environment) (*Environment, error)

	ActionError(*Environment) (*Environment, error)

	ActionExportconfig(*Environment, *ComposeConfigInput) (*ComposeConfig, error)

	ActionRemove(*Environment) (*Environment, error)

	ActionUpdate(*Environment) (*Environment, error)

	ActionActivateServices(*Environment) (*Environment, error)
	ActionDeactivateServices(*Environment) (*Environment, error)
}

func newEnvironmentClient(rancherClient *RancherClient) *EnvironmentClient {
	return &EnvironmentClient{
		rancherClient: rancherClient,
	}
}

func (c *EnvironmentClient) Create(container *Environment) (*Environment, error) {
	resp := &Environment{}
	err := c.rancherClient.doCreate(ENVIRONMENT_TYPE, container, resp)
	return resp, err
}

func (c *EnvironmentClient) Update(existing *Environment, updates interface{}) (*Environment, error) {
	resp := &Environment{}
	err := c.rancherClient.doUpdate(ENVIRONMENT_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *EnvironmentClient) List(opts *ListOpts) (*EnvironmentCollection, error) {
	resp := &EnvironmentCollection{}
	err := c.rancherClient.doList(ENVIRONMENT_TYPE, opts, resp)
	return resp, err
}

func (c *EnvironmentClient) ById(id string) (*Environment, error) {
	resp := &Environment{}
	err := c.rancherClient.doById(ENVIRONMENT_TYPE, id, resp)
	return resp, err
}

func (c *EnvironmentClient) Delete(container *Environment) error {
	return c.rancherClient.doResourceDelete(ENVIRONMENT_TYPE, &container.Resource)
}

func (c *EnvironmentClient) ActionCreate(resource *Environment) (*Environment, error) {

	resp := &Environment{}

	err := c.rancherClient.doAction(ENVIRONMENT_TYPE, "create", &resource.Resource, nil, resp)

	return resp, err
}

func (c *EnvironmentClient) ActionError(resource *Environment) (*Environment, error) {

	resp := &Environment{}

	err := c.rancherClient.doAction(ENVIRONMENT_TYPE, "error", &resource.Resource, nil, resp)

	return resp, err
}

func (c *EnvironmentClient) ActionExportconfig(resource *Environment, input *ComposeConfigInput) (*ComposeConfig, error) {

	resp := &ComposeConfig{}

	err := c.rancherClient.doAction(ENVIRONMENT_TYPE, "exportconfig", &resource.Resource, input, resp)

	return resp, err
}

func (c *EnvironmentClient) ActionRemove(resource *Environment) (*Environment, error) {

	resp := &Environment{}

	err := c.rancherClient.doAction(ENVIRONMENT_TYPE, "remove", &resource.Resource, nil, resp)

	return resp, err
}

func (c *EnvironmentClient) ActionUpdate(resource *Environment) (*Environment, error) {

	resp := &Environment{}

	err := c.rancherClient.doAction(ENVIRONMENT_TYPE, "update", &resource.Resource, nil, resp)

	return resp, err
}

func (c *EnvironmentClient) ActionActivateServices(resource *Environment) (*Environment, error) {

	resp := &Environment{}

	err := c.rancherClient.doAction(ENVIRONMENT_TYPE, "activateservices", &resource.Resource, nil, resp)

	return resp, err
}

func (c *EnvironmentClient) ActionDeactivateServices(resource *Environment) (*Environment, error) {

	resp := &Environment{}

	err := c.rancherClient.doAction(ENVIRONMENT_TYPE, "deactivateservices", &resource.Resource, nil, resp)

	return resp, err
}
