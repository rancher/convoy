# Using Convoy With Docker

As a Docker plugin, Convoy works with Docker flawlessly to provide a great experience for users.

## Register Convoy plugin to Docker
Please make sure Docker v1.8+ is available. Then follow the following steps to register Convoy plugin to Docker.
```
sudo mkdir -p /etc/docker/plugins/
sudo bash -c 'echo "unix:///var/run/convoy/convoy.sock" > /etc/docker/plugins/convoy.spec'
```

## Docker commands
Any existing Convoy volume would be refered by it's name in Docker.

### Create Container
Docker can specify volumes to be associate with when creating a container, represented by its name. If the specified volume name doesn't exist in Convoy, it would create a volume using that name, with [default driver and options](https://github.com/rancher/convoy/blob/master/docs/cli_reference.md#daemon), then hand it to Docker. So after:
```
sudo docker run -it -v new_volume:/vol1 --volume-driver=convoy ubuntu
```
Docker would have a volume named `new_volume` mounted inside container at `/vol1`. And you can see the details of that volume by running:
```
sudo convoy inspect new_volume
```

So if user want a volume different than default driver and options, user can create the volume beforehand, like:
```
sudo convoy create new_volume --driver ebs --size 10G --type io1 --iops 200
sudo docker run -it -v new_volume:/vol1 --volume-driver=convoy ubuntu
```
Or create a volume from a backup:
```
sudo convoy create restored_volume --backup s3://convoy-backup@us-west-2/?backup=f98f9ea1-dd6e-4490-8212-6d50df1982ea\u0026volume=e0d386c5-6a24-446c-8111-1077d10356b0
sudo docker run -it -v restored_volume:/vol1 --volume-driver=convoy ubuntu
```

### Delete Container
By default, Docker doesn't delete volume associated with container when container got deleted. Means after:
```
sudo docker run -name db_container -v db_vol:/var/lib/mysql/ --volume-driver=convoy mariadb
sudo docker rm db_container
sudo convoy inspect db_vol
```
You can still see `db_vol` details.

In order to delete volume associate with Docker, you would need `--volume/-v` parameter of `docker rm`:
```
sudo docker run -name db_container -v db_vol:/var/lib/mysql/ --volume-driver=convoy mariadb
sudo docker rm -v db_container
sudo convoy inspect db_vol
```
Convoy would error out because it cannot find volume with name `db_vol` this time.

Also, if you use `--rm` with `docker run`, all the volumes associated with the container would be deleted in the same way as executing `docker rm -v` when exit. See [Docker run reference](https://docs.docker.com/engine/reference/run/) for details.

Notice the behavior of `docker rm -v` would be treated as `convoy delete` with  `-r/--reference` in Convoy, means for VFS/NFS or EBS, Convoy won't delete the real content of the volume, in case user want to reuse it in the future. You won't able to see volume in Convoy anymore, but the contents are still available on [local directory/NFS server](https://github.com/rancher/convoy/blob/master/docs/vfs.md#delete) or [EBS](https://github.com/rancher/convoy/blob/master/docs/ebs.md#delete). And you can recreate Convoy volumes to associate with the volume directory in [VFS/NFS](https://github.com/rancher/convoy/blob/master/docs/vfs.md#create) or EBS volume in the case of [EBS](https://github.com/rancher/convoy/blob/master/docs/ebs.md#create).

### Docker volume subcommand
Docker v1.9 would introduce a series of command focused on manage volumes.

#### Create Volume
```docker volume create``` can accept driver specific options, and it's supported by Convoy. So:
```
sudo docker volume create --name new_volume --volume-driver=convoy --opt driver=ebs --opt size=10G --opt type=io1 --opt iops=200
```
Equals to:
```
sudo convoy create new_volume --driver ebs --size 10G --type io1 --iops 200
```

#### Delete Volume
`docker volume rm` would be treated as `convoy delete` with `-r/--reference` in the same case as delete container mentioned above. So:
```
sudo docker volume rm new_volume
```
Equals to
```
sudo convoy delete -r new_volume
```

#### List And Inspect Volume
Currently `docker volume ls` and `docker volume inspect` haven't involved volume plugin yet, so the commands' behavior won't be affected by Convoy.
