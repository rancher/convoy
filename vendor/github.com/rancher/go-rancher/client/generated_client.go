package client

type RancherClient struct {
	RancherBaseClient

	Subscribe                                SubscribeOperations
	Publish                                  PublishOperations
	LogConfig                                LogConfigOperations
	RestartPolicy                            RestartPolicyOperations
	LoadBalancerCookieStickinessPolicy       LoadBalancerCookieStickinessPolicyOperations
	LoadBalancerAppCookieStickinessPolicy    LoadBalancerAppCookieStickinessPolicyOperations
	ExternalHandlerProcessConfig             ExternalHandlerProcessConfigOperations
	ComposeConfig                            ComposeConfigOperations
	InstanceHealthCheck                      InstanceHealthCheckOperations
	ServiceLink                              ServiceLinkOperations
	ServiceUpgrade                           ServiceUpgradeOperations
	ServiceUpgradeStrategy                   ServiceUpgradeStrategyOperations
	InServiceUpgradeStrategy                 InServiceUpgradeStrategyOperations
	ToServiceUpgradeStrategy                 ToServiceUpgradeStrategyOperations
	PublicEndpoint                           PublicEndpointOperations
	VirtualMachineDisk                       VirtualMachineDiskOperations
	HaproxyConfig                            HaproxyConfigOperations
	RollingRestartStrategy                   RollingRestartStrategyOperations
	ServiceRestart                           ServiceRestartOperations
	ServicesPortRange                        ServicesPortRangeOperations
	RecreateOnQuorumStrategyConfig           RecreateOnQuorumStrategyConfigOperations
	AddOutputsInput                          AddOutputsInputOperations
	AddRemoveClusterHostInput                AddRemoveClusterHostInputOperations
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
	SetProjectMembersInput                   SetProjectMembersInputOperations
	SetServiceLinksInput                     SetServiceLinksInputOperations
	VirtualMachine                           VirtualMachineOperations
	LoadBalancerService                      LoadBalancerServiceOperations
	ExternalService                          ExternalServiceOperations
	DnsService                               DnsServiceOperations
	LaunchConfig                             LaunchConfigOperations
	SecondaryLaunchConfig                    SecondaryLaunchConfigOperations
	AddRemoveLoadBalancerServiceLinkInput    AddRemoveLoadBalancerServiceLinkInputOperations
	SetLoadBalancerServiceLinksInput         SetLoadBalancerServiceLinksInputOperations
	LoadBalancerServiceLink                  LoadBalancerServiceLinkOperations
	PullTask                                 PullTaskOperations
	ExternalVolumeEvent                      ExternalVolumeEventOperations
	ExternalStoragePoolEvent                 ExternalStoragePoolEventOperations
	ExternalServiceEvent                     ExternalServiceEventOperations
	EnvironmentUpgrade                       EnvironmentUpgradeOperations
	ExternalDnsEvent                         ExternalDnsEventOperations
	ExternalHostEvent                        ExternalHostEventOperations
	LoadBalancerConfig                       LoadBalancerConfigOperations
	Account                                  AccountOperations
	Agent                                    AgentOperations
	AuditLog                                 AuditLogOperations
	Certificate                              CertificateOperations
	ConfigItem                               ConfigItemOperations
	ConfigItemStatus                         ConfigItemStatusOperations
	ContainerEvent                           ContainerEventOperations
	Credential                               CredentialOperations
	Databasechangelog                        DatabasechangelogOperations
	Databasechangeloglock                    DatabasechangeloglockOperations
	Environment                              EnvironmentOperations
	ExternalEvent                            ExternalEventOperations
	ExternalHandler                          ExternalHandlerOperations
	ExternalHandlerExternalHandlerProcessMap ExternalHandlerExternalHandlerProcessMapOperations
	ExternalHandlerProcess                   ExternalHandlerProcessOperations
	HealthcheckInstanceHostMap               HealthcheckInstanceHostMapOperations
	Host                                     HostOperations
	Image                                    ImageOperations
	Instance                                 InstanceOperations
	InstanceLink                             InstanceLinkOperations
	IpAddress                                IpAddressOperations
	Label                                    LabelOperations
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
	Openldapconfig                           OpenldapconfigOperations
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
	UbiquityConfig                           UbiquityConfigOperations
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
	client.LoadBalancerCookieStickinessPolicy = newLoadBalancerCookieStickinessPolicyClient(client)
	client.LoadBalancerAppCookieStickinessPolicy = newLoadBalancerAppCookieStickinessPolicyClient(client)
	client.ExternalHandlerProcessConfig = newExternalHandlerProcessConfigClient(client)
	client.ComposeConfig = newComposeConfigClient(client)
	client.InstanceHealthCheck = newInstanceHealthCheckClient(client)
	client.ServiceLink = newServiceLinkClient(client)
	client.ServiceUpgrade = newServiceUpgradeClient(client)
	client.ServiceUpgradeStrategy = newServiceUpgradeStrategyClient(client)
	client.InServiceUpgradeStrategy = newInServiceUpgradeStrategyClient(client)
	client.ToServiceUpgradeStrategy = newToServiceUpgradeStrategyClient(client)
	client.PublicEndpoint = newPublicEndpointClient(client)
	client.VirtualMachineDisk = newVirtualMachineDiskClient(client)
	client.HaproxyConfig = newHaproxyConfigClient(client)
	client.RollingRestartStrategy = newRollingRestartStrategyClient(client)
	client.ServiceRestart = newServiceRestartClient(client)
	client.ServicesPortRange = newServicesPortRangeClient(client)
	client.RecreateOnQuorumStrategyConfig = newRecreateOnQuorumStrategyConfigClient(client)
	client.AddOutputsInput = newAddOutputsInputClient(client)
	client.AddRemoveClusterHostInput = newAddRemoveClusterHostInputClient(client)
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
	client.SetProjectMembersInput = newSetProjectMembersInputClient(client)
	client.SetServiceLinksInput = newSetServiceLinksInputClient(client)
	client.VirtualMachine = newVirtualMachineClient(client)
	client.LoadBalancerService = newLoadBalancerServiceClient(client)
	client.ExternalService = newExternalServiceClient(client)
	client.DnsService = newDnsServiceClient(client)
	client.LaunchConfig = newLaunchConfigClient(client)
	client.SecondaryLaunchConfig = newSecondaryLaunchConfigClient(client)
	client.AddRemoveLoadBalancerServiceLinkInput = newAddRemoveLoadBalancerServiceLinkInputClient(client)
	client.SetLoadBalancerServiceLinksInput = newSetLoadBalancerServiceLinksInputClient(client)
	client.LoadBalancerServiceLink = newLoadBalancerServiceLinkClient(client)
	client.PullTask = newPullTaskClient(client)
	client.ExternalVolumeEvent = newExternalVolumeEventClient(client)
	client.ExternalStoragePoolEvent = newExternalStoragePoolEventClient(client)
	client.ExternalServiceEvent = newExternalServiceEventClient(client)
	client.EnvironmentUpgrade = newEnvironmentUpgradeClient(client)
	client.ExternalDnsEvent = newExternalDnsEventClient(client)
	client.ExternalHostEvent = newExternalHostEventClient(client)
	client.LoadBalancerConfig = newLoadBalancerConfigClient(client)
	client.Account = newAccountClient(client)
	client.Agent = newAgentClient(client)
	client.AuditLog = newAuditLogClient(client)
	client.Certificate = newCertificateClient(client)
	client.ConfigItem = newConfigItemClient(client)
	client.ConfigItemStatus = newConfigItemStatusClient(client)
	client.ContainerEvent = newContainerEventClient(client)
	client.Credential = newCredentialClient(client)
	client.Databasechangelog = newDatabasechangelogClient(client)
	client.Databasechangeloglock = newDatabasechangeloglockClient(client)
	client.Environment = newEnvironmentClient(client)
	client.ExternalEvent = newExternalEventClient(client)
	client.ExternalHandler = newExternalHandlerClient(client)
	client.ExternalHandlerExternalHandlerProcessMap = newExternalHandlerExternalHandlerProcessMapClient(client)
	client.ExternalHandlerProcess = newExternalHandlerProcessClient(client)
	client.HealthcheckInstanceHostMap = newHealthcheckInstanceHostMapClient(client)
	client.Host = newHostClient(client)
	client.Image = newImageClient(client)
	client.Instance = newInstanceClient(client)
	client.InstanceLink = newInstanceLinkClient(client)
	client.IpAddress = newIpAddressClient(client)
	client.Label = newLabelClient(client)
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
	client.Openldapconfig = newOpenldapconfigClient(client)
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
	client.UbiquityConfig = newUbiquityConfigClient(client)
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
