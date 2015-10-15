package client

type RancherClient struct {
	RancherBaseClient

	Subscribe                                SubscribeOperations
	Publish                                  PublishOperations
	LogConfig                                LogConfigOperations
	RestartPolicy                            RestartPolicyOperations
	LoadBalancerHealthCheck                  LoadBalancerHealthCheckOperations
	LoadBalancerCookieStickinessPolicy       LoadBalancerCookieStickinessPolicyOperations
	LoadBalancerAppCookieStickinessPolicy    LoadBalancerAppCookieStickinessPolicyOperations
	GlobalLoadBalancerPolicy                 GlobalLoadBalancerPolicyOperations
	GlobalLoadBalancerHealthCheck            GlobalLoadBalancerHealthCheckOperations
	ExternalHandlerProcessConfig             ExternalHandlerProcessConfigOperations
	ComposeConfig                            ComposeConfigOperations
	InstanceHealthCheck                      InstanceHealthCheckOperations
	ServiceLink                              ServiceLinkOperations
	ServiceUpgrade                           ServiceUpgradeOperations
	AddLoadBalancerInput                     AddLoadBalancerInputOperations
	AddRemoveClusterHostInput                AddRemoveClusterHostInputOperations
	AddRemoveLoadBalancerHostInput           AddRemoveLoadBalancerHostInputOperations
	AddRemoveLoadBalancerListenerInput       AddRemoveLoadBalancerListenerInputOperations
	AddRemoveLoadBalancerTargetInput         AddRemoveLoadBalancerTargetInputOperations
	AddRemoveServiceLinkInput                AddRemoveServiceLinkInputOperations
	ChangeSecretInput                        ChangeSecretInputOperations
	SetLabelsInput                           SetLabelsInputOperations
	ApiKey                                   ApiKeyOperations
	Cluster                                  ClusterOperations
	ComposeConfigInput                       ComposeConfigInputOperations
	Container                                ContainerOperations
	InstanceConsole                          InstanceConsoleOperations
	InstanceConsoleInput                     InstanceConsoleInputOperations
	InstanceStop                             InstanceStopOperations
	IpAddressAssociateInput                  IpAddressAssociateInputOperations
	Project                                  ProjectOperations
	Password                                 PasswordOperations
	Registry                                 RegistryOperations
	RegistryCredential                       RegistryCredentialOperations
	RemoveLoadBalancerInput                  RemoveLoadBalancerInputOperations
	SetLoadBalancerHostsInput                SetLoadBalancerHostsInputOperations
	SetLoadBalancerListenersInput            SetLoadBalancerListenersInputOperations
	SetLoadBalancerTargetsInput              SetLoadBalancerTargetsInputOperations
	SetProjectMembersInput                   SetProjectMembersInputOperations
	SetServiceLinksInput                     SetServiceLinksInputOperations
	LoadBalancerService                      LoadBalancerServiceOperations
	ExternalService                          ExternalServiceOperations
	DnsService                               DnsServiceOperations
	LaunchConfig                             LaunchConfigOperations
	SecondaryLaunchConfig                    SecondaryLaunchConfigOperations
	AddRemoveLoadBalancerServiceLinkInput    AddRemoveLoadBalancerServiceLinkInputOperations
	SetLoadBalancerServiceLinksInput         SetLoadBalancerServiceLinksInputOperations
	LoadBalancerServiceLink                  LoadBalancerServiceLinkOperations
	PullTask                                 PullTaskOperations
	Account                                  AccountOperations
	Agent                                    AgentOperations
	Certificate                              CertificateOperations
	ConfigItem                               ConfigItemOperations
	ConfigItemStatus                         ConfigItemStatusOperations
	ContainerEvent                           ContainerEventOperations
	Credential                               CredentialOperations
	Databasechangelog                        DatabasechangelogOperations
	Databasechangeloglock                    DatabasechangeloglockOperations
	Environment                              EnvironmentOperations
	ExternalHandler                          ExternalHandlerOperations
	ExternalHandlerExternalHandlerProcessMap ExternalHandlerExternalHandlerProcessMapOperations
	ExternalHandlerProcess                   ExternalHandlerProcessOperations
	GlobalLoadBalancer                       GlobalLoadBalancerOperations
	Host                                     HostOperations
	Image                                    ImageOperations
	Instance                                 InstanceOperations
	InstanceLink                             InstanceLinkOperations
	IpAddress                                IpAddressOperations
	Label                                    LabelOperations
	LoadBalancer                             LoadBalancerOperations
	LoadBalancerConfig                       LoadBalancerConfigOperations
	LoadBalancerConfigListenerMap            LoadBalancerConfigListenerMapOperations
	LoadBalancerHostMap                      LoadBalancerHostMapOperations
	LoadBalancerListener                     LoadBalancerListenerOperations
	LoadBalancerTarget                       LoadBalancerTargetOperations
	Mount                                    MountOperations
	Network                                  NetworkOperations
	PhysicalHost                             PhysicalHostOperations
	Port                                     PortOperations
	ProcessExecution                         ProcessExecutionOperations
	ProcessInstance                          ProcessInstanceOperations
	ProjectMember                            ProjectMemberOperations
	Service                                  ServiceOperations
	ServiceConsumeMap                        ServiceConsumeMapOperations
	ServiceEvent                             ServiceEventOperations
	ServiceExposeMap                         ServiceExposeMapOperations
	Setting                                  SettingOperations
	Snapshot                                 SnapshotOperations
	StoragePool                              StoragePoolOperations
	Task                                     TaskOperations
	TaskInstance                             TaskInstanceOperations
	Volume                                   VolumeOperations
	TypeDocumentation                        TypeDocumentationOperations
	FieldDocumentation                       FieldDocumentationOperations
	ContainerExec                            ContainerExecOperations
	ContainerLogs                            ContainerLogsOperations
	HostAccess                               HostAccessOperations
	DockerBuild                              DockerBuildOperations
	ActiveSetting                            ActiveSettingOperations
	ExtensionImplementation                  ExtensionImplementationOperations
	ExtensionPoint                           ExtensionPointOperations
	ProcessDefinition                        ProcessDefinitionOperations
	ResourceDefinition                       ResourceDefinitionOperations
	StateTransition                          StateTransitionOperations
	Githubconfig                             GithubconfigOperations
	Identity                                 IdentityOperations
	Ldapconfig                               LdapconfigOperations
	LocalAuthConfig                          LocalAuthConfigOperations
	StatsAccess                              StatsAccessOperations
	Amazonec2Config                          Amazonec2ConfigOperations
	AzureConfig                              AzureConfigOperations
	DigitaloceanConfig                       DigitaloceanConfigOperations
	ExoscaleConfig                           ExoscaleConfigOperations
	OpenstackConfig                          OpenstackConfigOperations
	PacketConfig                             PacketConfigOperations
	RackspaceConfig                          RackspaceConfigOperations
	SoftlayerConfig                          SoftlayerConfigOperations
	VirtualboxConfig                         VirtualboxConfigOperations
	VmwarevcloudairConfig                    VmwarevcloudairConfigOperations
	VmwarevsphereConfig                      VmwarevsphereConfigOperations
	Machine                                  MachineOperations
	HostApiProxyToken                        HostApiProxyTokenOperations
	Register                                 RegisterOperations
	RegistrationToken                        RegistrationTokenOperations
}

func constructClient() *RancherClient {
	client := &RancherClient{
		RancherBaseClient: RancherBaseClient{
			Types: map[string]Schema{},
		},
	}

	client.Subscribe = newSubscribeClient(client)
	client.Publish = newPublishClient(client)
	client.LogConfig = newLogConfigClient(client)
	client.RestartPolicy = newRestartPolicyClient(client)
	client.LoadBalancerHealthCheck = newLoadBalancerHealthCheckClient(client)
	client.LoadBalancerCookieStickinessPolicy = newLoadBalancerCookieStickinessPolicyClient(client)
	client.LoadBalancerAppCookieStickinessPolicy = newLoadBalancerAppCookieStickinessPolicyClient(client)
	client.GlobalLoadBalancerPolicy = newGlobalLoadBalancerPolicyClient(client)
	client.GlobalLoadBalancerHealthCheck = newGlobalLoadBalancerHealthCheckClient(client)
	client.ExternalHandlerProcessConfig = newExternalHandlerProcessConfigClient(client)
	client.ComposeConfig = newComposeConfigClient(client)
	client.InstanceHealthCheck = newInstanceHealthCheckClient(client)
	client.ServiceLink = newServiceLinkClient(client)
	client.ServiceUpgrade = newServiceUpgradeClient(client)
	client.AddLoadBalancerInput = newAddLoadBalancerInputClient(client)
	client.AddRemoveClusterHostInput = newAddRemoveClusterHostInputClient(client)
	client.AddRemoveLoadBalancerHostInput = newAddRemoveLoadBalancerHostInputClient(client)
	client.AddRemoveLoadBalancerListenerInput = newAddRemoveLoadBalancerListenerInputClient(client)
	client.AddRemoveLoadBalancerTargetInput = newAddRemoveLoadBalancerTargetInputClient(client)
	client.AddRemoveServiceLinkInput = newAddRemoveServiceLinkInputClient(client)
	client.ChangeSecretInput = newChangeSecretInputClient(client)
	client.SetLabelsInput = newSetLabelsInputClient(client)
	client.ApiKey = newApiKeyClient(client)
	client.Cluster = newClusterClient(client)
	client.ComposeConfigInput = newComposeConfigInputClient(client)
	client.Container = newContainerClient(client)
	client.InstanceConsole = newInstanceConsoleClient(client)
	client.InstanceConsoleInput = newInstanceConsoleInputClient(client)
	client.InstanceStop = newInstanceStopClient(client)
	client.IpAddressAssociateInput = newIpAddressAssociateInputClient(client)
	client.Project = newProjectClient(client)
	client.Password = newPasswordClient(client)
	client.Registry = newRegistryClient(client)
	client.RegistryCredential = newRegistryCredentialClient(client)
	client.RemoveLoadBalancerInput = newRemoveLoadBalancerInputClient(client)
	client.SetLoadBalancerHostsInput = newSetLoadBalancerHostsInputClient(client)
	client.SetLoadBalancerListenersInput = newSetLoadBalancerListenersInputClient(client)
	client.SetLoadBalancerTargetsInput = newSetLoadBalancerTargetsInputClient(client)
	client.SetProjectMembersInput = newSetProjectMembersInputClient(client)
	client.SetServiceLinksInput = newSetServiceLinksInputClient(client)
	client.LoadBalancerService = newLoadBalancerServiceClient(client)
	client.ExternalService = newExternalServiceClient(client)
	client.DnsService = newDnsServiceClient(client)
	client.LaunchConfig = newLaunchConfigClient(client)
	client.SecondaryLaunchConfig = newSecondaryLaunchConfigClient(client)
	client.AddRemoveLoadBalancerServiceLinkInput = newAddRemoveLoadBalancerServiceLinkInputClient(client)
	client.SetLoadBalancerServiceLinksInput = newSetLoadBalancerServiceLinksInputClient(client)
	client.LoadBalancerServiceLink = newLoadBalancerServiceLinkClient(client)
	client.PullTask = newPullTaskClient(client)
	client.Account = newAccountClient(client)
	client.Agent = newAgentClient(client)
	client.Certificate = newCertificateClient(client)
	client.ConfigItem = newConfigItemClient(client)
	client.ConfigItemStatus = newConfigItemStatusClient(client)
	client.ContainerEvent = newContainerEventClient(client)
	client.Credential = newCredentialClient(client)
	client.Databasechangelog = newDatabasechangelogClient(client)
	client.Databasechangeloglock = newDatabasechangeloglockClient(client)
	client.Environment = newEnvironmentClient(client)
	client.ExternalHandler = newExternalHandlerClient(client)
	client.ExternalHandlerExternalHandlerProcessMap = newExternalHandlerExternalHandlerProcessMapClient(client)
	client.ExternalHandlerProcess = newExternalHandlerProcessClient(client)
	client.GlobalLoadBalancer = newGlobalLoadBalancerClient(client)
	client.Host = newHostClient(client)
	client.Image = newImageClient(client)
	client.Instance = newInstanceClient(client)
	client.InstanceLink = newInstanceLinkClient(client)
	client.IpAddress = newIpAddressClient(client)
	client.Label = newLabelClient(client)
	client.LoadBalancer = newLoadBalancerClient(client)
	client.LoadBalancerConfig = newLoadBalancerConfigClient(client)
	client.LoadBalancerConfigListenerMap = newLoadBalancerConfigListenerMapClient(client)
	client.LoadBalancerHostMap = newLoadBalancerHostMapClient(client)
	client.LoadBalancerListener = newLoadBalancerListenerClient(client)
	client.LoadBalancerTarget = newLoadBalancerTargetClient(client)
	client.Mount = newMountClient(client)
	client.Network = newNetworkClient(client)
	client.PhysicalHost = newPhysicalHostClient(client)
	client.Port = newPortClient(client)
	client.ProcessExecution = newProcessExecutionClient(client)
	client.ProcessInstance = newProcessInstanceClient(client)
	client.ProjectMember = newProjectMemberClient(client)
	client.Service = newServiceClient(client)
	client.ServiceConsumeMap = newServiceConsumeMapClient(client)
	client.ServiceEvent = newServiceEventClient(client)
	client.ServiceExposeMap = newServiceExposeMapClient(client)
	client.Setting = newSettingClient(client)
	client.Snapshot = newSnapshotClient(client)
	client.StoragePool = newStoragePoolClient(client)
	client.Task = newTaskClient(client)
	client.TaskInstance = newTaskInstanceClient(client)
	client.Volume = newVolumeClient(client)
	client.TypeDocumentation = newTypeDocumentationClient(client)
	client.FieldDocumentation = newFieldDocumentationClient(client)
	client.ContainerExec = newContainerExecClient(client)
	client.ContainerLogs = newContainerLogsClient(client)
	client.HostAccess = newHostAccessClient(client)
	client.DockerBuild = newDockerBuildClient(client)
	client.ActiveSetting = newActiveSettingClient(client)
	client.ExtensionImplementation = newExtensionImplementationClient(client)
	client.ExtensionPoint = newExtensionPointClient(client)
	client.ProcessDefinition = newProcessDefinitionClient(client)
	client.ResourceDefinition = newResourceDefinitionClient(client)
	client.StateTransition = newStateTransitionClient(client)
	client.Githubconfig = newGithubconfigClient(client)
	client.Identity = newIdentityClient(client)
	client.Ldapconfig = newLdapconfigClient(client)
	client.LocalAuthConfig = newLocalAuthConfigClient(client)
	client.StatsAccess = newStatsAccessClient(client)
	client.Amazonec2Config = newAmazonec2ConfigClient(client)
	client.AzureConfig = newAzureConfigClient(client)
	client.DigitaloceanConfig = newDigitaloceanConfigClient(client)
	client.ExoscaleConfig = newExoscaleConfigClient(client)
	client.OpenstackConfig = newOpenstackConfigClient(client)
	client.PacketConfig = newPacketConfigClient(client)
	client.RackspaceConfig = newRackspaceConfigClient(client)
	client.SoftlayerConfig = newSoftlayerConfigClient(client)
	client.VirtualboxConfig = newVirtualboxConfigClient(client)
	client.VmwarevcloudairConfig = newVmwarevcloudairConfigClient(client)
	client.VmwarevsphereConfig = newVmwarevsphereConfigClient(client)
	client.Machine = newMachineClient(client)
	client.HostApiProxyToken = newHostApiProxyTokenClient(client)
	client.Register = newRegisterClient(client)
	client.RegistrationToken = newRegistrationTokenClient(client)

	return client
}

func NewRancherClient(opts *ClientOpts) (*RancherClient, error) {
	client := constructClient()

	err := setupRancherBaseClient(&client.RancherBaseClient, opts)
	if err != nil {
		return nil, err
	}

	return client, nil
}
