RANCHER-VOLUME_EXEC_FILE = ./bin/rancher-volume
RANCHER-VOLUME_MOUNT_EXEC_FILE = ./bin/rancher-mount

.PHONY: all clean

all: $(RANCHER-VOLUME_EXEC_FILE) $(RANCHER-VOLUME_MOUNT_EXEC_FILE)

$(RANCHER-VOLUME_MOUNT_EXEC_FILE): ./tools/rancher_mount.c
	gcc -o ./bin/rancher-mount ./tools/rancher_mount.c

$(RANCHER-VOLUME_EXEC_FILE): ./api/devmapper.go ./api/response.go \
	./objectstore/objectstore.go ./objectstore/config.go \
	./s3/s3.go ./s3/s3_service.go \
	./vfs/vfs.go \
	./devmapper/devmapper.go \
 	./drivers/drivers.go \
	./metadata/devmapper.go ./metadata/metadata.go \
	./util/util.go ./util/util_test.go \
	./logging/logging.go \
	./volume_cmds.go ./snapshot_cmds.go ./objectstore_cmds.go \
	./server.go ./client.go ./docker.go \
	./commands.go ./main.go ./main_objectstore.go ./main_devmapper.go
	go build -tags libdm_no_deferred_remove -o ./bin/rancher-volume

clean:
	rm -f ./bin/rancher-*

install:
	cp ./bin/rancher-* /usr/local/bin/
