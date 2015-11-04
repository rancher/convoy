package shorthorn

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
  - /usr/local/bin/shorthorn-replica.py -f [VOLUME_UUID] --size [VOLUME_SIZE] | tee shorthorn_replica.log
  working_dir: /storage
  volumes:
  - /storage
  - /sys/kernel/config:/sys/kernel/config
  image: yasker/sh
  privileged: true
controller:
  restart: always
  labels:
    io.rancher.scheduler.affinity:container: [CONVOY_CONTAINER]
    io.rancher.container.pull_image: always
  entrypoint:
  - /bin/bash
  - -c
  command:
  - /usr/local/bin/shorthorn-controller.py -s replica -d [VOLUME_NAME] | tee shorthorn_controller.log
  image: yasker/sh
  working_dir: /storage
  volumes:
  - /storage
  - /dev:/host/dev
  - /etc/iscsi:/etc/iscsi
  - /proc:/host/proc
  links:
  - replica:replica
  privileged: true
`
	RancherComposeTemplate = `
replica:
  scale: 2
controller:
  scale: 1
`
)
