package client

const (
	LOAD_BALANCER_CONFIG_TYPE = "loadBalancerConfig"
)

type LoadBalancerConfig struct {
	Resource

	AccountId string `json:"accountId,omitempty" yaml:"account_id,omitempty"`

	AppCookieStickinessPolicy *LoadBalancerAppCookieStickinessPolicy `json:"appCookieStickinessPolicy,omitempty" yaml:"app_cookie_stickiness_policy,omitempty"`

	Created string `json:"created,omitempty" yaml:"created,omitempty"`

	Data map[string]interface{} `json:"data,omitempty" yaml:"data,omitempty"`

	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	HealthCheck *LoadBalancerHealthCheck `json:"healthCheck,omitempty" yaml:"health_check,omitempty"`

	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`

	LbCookieStickinessPolicy *LoadBalancerCookieStickinessPolicy `json:"lbCookieStickinessPolicy,omitempty" yaml:"lb_cookie_stickiness_policy,omitempty"`

	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	RemoveTime string `json:"removeTime,omitempty" yaml:"remove_time,omitempty"`

	Removed string `json:"removed,omitempty" yaml:"removed,omitempty"`

	ServiceId string `json:"serviceId,omitempty" yaml:"service_id,omitempty"`

	State string `json:"state,omitempty" yaml:"state,omitempty"`

	Transitioning string `json:"transitioning,omitempty" yaml:"transitioning,omitempty"`

	TransitioningMessage string `json:"transitioningMessage,omitempty" yaml:"transitioning_message,omitempty"`

	TransitioningProgress int64 `json:"transitioningProgress,omitempty" yaml:"transitioning_progress,omitempty"`

	Uuid string `json:"uuid,omitempty" yaml:"uuid,omitempty"`
}

type LoadBalancerConfigCollection struct {
	Collection
	Data []LoadBalancerConfig `json:"data,omitempty"`
}

type LoadBalancerConfigClient struct {
	rancherClient *RancherClient
}

type LoadBalancerConfigOperations interface {
	List(opts *ListOpts) (*LoadBalancerConfigCollection, error)
	Create(opts *LoadBalancerConfig) (*LoadBalancerConfig, error)
	Update(existing *LoadBalancerConfig, updates interface{}) (*LoadBalancerConfig, error)
	ById(id string) (*LoadBalancerConfig, error)
	Delete(container *LoadBalancerConfig) error

	ActionAddlistener(*LoadBalancerConfig, *AddRemoveLoadBalancerListenerInput) (*LoadBalancerConfig, error)

	ActionCreate(*LoadBalancerConfig) (*LoadBalancerConfig, error)

	ActionRemove(*LoadBalancerConfig) (*LoadBalancerConfig, error)

	ActionRemovelistener(*LoadBalancerConfig, *AddRemoveLoadBalancerListenerInput) (*LoadBalancerConfig, error)

	ActionSetlisteners(*LoadBalancerConfig, *SetLoadBalancerListenersInput) (*LoadBalancerConfig, error)

	ActionUpdate(*LoadBalancerConfig) (*LoadBalancerConfig, error)
}

func newLoadBalancerConfigClient(rancherClient *RancherClient) *LoadBalancerConfigClient {
	return &LoadBalancerConfigClient{
		rancherClient: rancherClient,
	}
}

func (c *LoadBalancerConfigClient) Create(container *LoadBalancerConfig) (*LoadBalancerConfig, error) {
	resp := &LoadBalancerConfig{}
	err := c.rancherClient.doCreate(LOAD_BALANCER_CONFIG_TYPE, container, resp)
	return resp, err
}

func (c *LoadBalancerConfigClient) Update(existing *LoadBalancerConfig, updates interface{}) (*LoadBalancerConfig, error) {
	resp := &LoadBalancerConfig{}
	err := c.rancherClient.doUpdate(LOAD_BALANCER_CONFIG_TYPE, &existing.Resource, updates, resp)
	return resp, err
}

func (c *LoadBalancerConfigClient) List(opts *ListOpts) (*LoadBalancerConfigCollection, error) {
	resp := &LoadBalancerConfigCollection{}
	err := c.rancherClient.doList(LOAD_BALANCER_CONFIG_TYPE, opts, resp)
	return resp, err
}

func (c *LoadBalancerConfigClient) ById(id string) (*LoadBalancerConfig, error) {
	resp := &LoadBalancerConfig{}
	err := c.rancherClient.doById(LOAD_BALANCER_CONFIG_TYPE, id, resp)
	return resp, err
}

func (c *LoadBalancerConfigClient) Delete(container *LoadBalancerConfig) error {
	return c.rancherClient.doResourceDelete(LOAD_BALANCER_CONFIG_TYPE, &container.Resource)
}

func (c *LoadBalancerConfigClient) ActionAddlistener(resource *LoadBalancerConfig, input *AddRemoveLoadBalancerListenerInput) (*LoadBalancerConfig, error) {

	resp := &LoadBalancerConfig{}

	err := c.rancherClient.doAction(LOAD_BALANCER_CONFIG_TYPE, "addlistener", &resource.Resource, input, resp)

	return resp, err
}

func (c *LoadBalancerConfigClient) ActionCreate(resource *LoadBalancerConfig) (*LoadBalancerConfig, error) {

	resp := &LoadBalancerConfig{}

	err := c.rancherClient.doAction(LOAD_BALANCER_CONFIG_TYPE, "create", &resource.Resource, nil, resp)

	return resp, err
}

func (c *LoadBalancerConfigClient) ActionRemove(resource *LoadBalancerConfig) (*LoadBalancerConfig, error) {

	resp := &LoadBalancerConfig{}

	err := c.rancherClient.doAction(LOAD_BALANCER_CONFIG_TYPE, "remove", &resource.Resource, nil, resp)

	return resp, err
}

func (c *LoadBalancerConfigClient) ActionRemovelistener(resource *LoadBalancerConfig, input *AddRemoveLoadBalancerListenerInput) (*LoadBalancerConfig, error) {

	resp := &LoadBalancerConfig{}

	err := c.rancherClient.doAction(LOAD_BALANCER_CONFIG_TYPE, "removelistener", &resource.Resource, input, resp)

	return resp, err
}

func (c *LoadBalancerConfigClient) ActionSetlisteners(resource *LoadBalancerConfig, input *SetLoadBalancerListenersInput) (*LoadBalancerConfig, error) {

	resp := &LoadBalancerConfig{}

	err := c.rancherClient.doAction(LOAD_BALANCER_CONFIG_TYPE, "setlisteners", &resource.Resource, input, resp)

	return resp, err
}

func (c *LoadBalancerConfigClient) ActionUpdate(resource *LoadBalancerConfig) (*LoadBalancerConfig, error) {

	resp := &LoadBalancerConfig{}

	err := c.rancherClient.doAction(LOAD_BALANCER_CONFIG_TYPE, "update", &resource.Resource, nil, resp)

	return resp, err
}
