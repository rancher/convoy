RANCHER-VOLUME_EXEC_FILE = ./bin/rancher-volume
RANCHER-MOUNT_EXEC_FILE = ./bin/rancher-mount

.PHONY: all clean

all: $(RANCHER-VOLUME_EXEC_FILE) $(RANCHER-MOUNT_EXEC_FILE)

$(RANCHER-MOUNT_EXEC_FILE): ./tools/rancher_mount.c
	gcc -o $(RANCHER-MOUNT_EXEC_FILE) ./tools/rancher_mount.c

$(RANCHER-VOLUME_EXEC_FILE): ./api/devmapper.go ./api/response.go \
	./objectstore/objectstore.go ./objectstore/config.go \
	./s3/s3.go ./s3/s3_service.go \
	./vfs/vfs.go \
	./devmapper/devmapper.go \
 	./drivers/drivers.go \
	./metadata/devmapper.go ./metadata/metadata.go \
	./util/util.go ./util/util_test.go ./util/index.go \
	./logging/logging.go \
	./volume_cmds.go ./snapshot_cmds.go ./objectstore_cmds.go \
	./server.go ./client.go ./docker.go \
	./commands.go ./main.go ./main_objectstore.go ./main_devmapper.go
	go build -a -tags "netgo libdm_no_deferred_remove" \
	    -ldflags '-linkmode external -extldflags "-static"' \
	    --installsuffix netgo -o $(RANCHER-VOLUME_EXEC_FILE)

clean:
	rm -f $(RANCHER-VOLUME_EXEC_FILE) $(RANCHER-MOUNT_EXEC_FILE)

install:
	cp $(RANCHER-VOLUME_EXEC_FILE) /usr/local/bin/
	cp $(RANCHER-MOUNT_EXEC_FILE) /usr/local/bin/
