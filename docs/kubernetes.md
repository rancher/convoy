# Using Kubernetes Flex Volume Plugin

As a Kubernetes flex volume plugin, k8s driver binary works with Convoy daemon to provide volume management tasks for uses using Kubenetes.

## Register Flex Volume plugin to Kubernetes

Please make sure Kubernetes v1.3.4+ are installed. Create a folder(vendor~driver) on the Kubelet volume plugin path: /usr/libexec/kubernetes/kubelet-plugins/volume/exec
```
sudo mkdir -p /usr/libexec/kubernetes/kubelet-plugins/volume/exec/rancher.io~k8s
```
Here we have a driver called k8s and vendor name is rancher.io. Then copy k8s driver binary to above created folder
```
sudo cp k8s /usr/libexec/kubernetes/kubelet-plugins/volume/exec/rancher.io~k8s
```

## Initialization

When Kubelet starts, it will call all the volume plugin drivers in the folder /usr/libexec/kubernetes/kubelet-plugins/volume/exec
with "init" cmd as the argument to the binary(k8s) to let the drivers initialize themselves

## Flex Volume plugin integration with Kubelet

When Kubernetes deploys a pod on the node, Kubelet will find the volume plugin driver according to pod spec in Kubelet volume plugin path, then Kubelet
will invoke the plugin drivers several times with different tasks to eventually mount a persistent volume for docker container to use inside the container.

Kubelet invokes volume plugin driver with 4 main commands: Attach, Detach, Mount, Unmount, besides Init

### Here is Flex Volume driver invocation model by Kubenetes:

Init:
```
driver  init
```

Attach:
```
driver  attach json_options
``` 

Detach:
```
driver  detach  mounted_device
```

Mount:
```
driver  mount  mountpoint optional_mounted_device json_options
``` 
Unmount:
```
driver  unmount  mountpoint
```

The json_options are the options in the pod spec under Flex Volume section, to pass vendor driver's specific options

### Driver output:

Flex Volume expects the driver to reply with the status of each invocation in the following format.

```
{
    "status": "Success/Failure"
    "message": "Reason for success/failure"
    "device": "Path to the device attached. This field is valid only for attach calls. Also it is optional."
}
```

### Rancher Flex Volume Spec options -- json_options

Rancher Flex Volume driver supports different options according to different Convoy drivers loaded in the Convoy daemon.
The first options is

```
"convoyDriver" : "ebs/nfs/efs",   which Convoy daemon driver to use to manage the volumes
```

#### if convoyDriver = ebs

User is responsible to setup aws ec2 access to instances and ebs volumes on the Kubelet host before using the driver

```
 "volumeId" : "existing EBS volume id"
 "volumeType" : "create a new volume using this EBS volume type"
 "size" : "create a new volume using this size"
 "iops" : "create a new volume using this iops"
```

#### if convoyDriver = nfs or convoyDriver = efs

User is responsible to mount a remote file system to a path(localPath) and invoke Convoy with vfs.Path=localPath specified

```
 "name" : "Convoy volume name, actually a folder name representing the NFS/EFS volume'
```

#### Kubenetes built in options

In addition to the flags specified by the user in the Options, the following flags are also passed to the executable, that
Rancher's k8s driver will make use.

```
"kubernetes.io/fsType" : "FS type"
"kubernetes.io/readwrite" : "rw/ro"
```