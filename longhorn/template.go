package longhorn

const (
	DockerComposeTemplate = `
replica:
  restart: always
  labels:
    io.rancher.scheduler.affinity:container_label_ne: io.rancher.stack_service.name=replica
    io.rancher.container.pull_image: always
  entrypoint:
  - /bin/bash
  - -c
  command:
  - /usr/local/bin/replica --name [VOLUME_NAME] --create --size [VOLUME_SIZE] --slab [SLAB_SIZE] | tee storage_replica.log
  working_dir: /storage
  volumes:
  - /storage
  image: yasker/infra
controller:
  restart: always
  labels:
    io.rancher.scheduler.affinity:container: [CONVOY_CONTAINER]
    io.rancher.container.pull_image: always
  entrypoint:
  - /bin/bash
  - -c
  command:
  - /storage/start_controller.sh replica | tee storage_controller.log
  image: yasker/infra
  working_dir: /storage
  volumes:
  - /storage
  links:
  - replica:replica
`
	RancherComposeTemplate = `
replica:
  scale: 2
controller:
  scale: 1
`
)
