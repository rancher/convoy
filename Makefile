VOLMGR_EXEC_FILE = ./bin/rancher-volume
VOLMGR_MOUNT_EXEC_FILE = ./bin/rancher-mount

.PHONY: all clean

all: $(VOLMGR_EXEC_FILE) $(VOLMGR_MOUNT_EXEC_FILE)

$(VOLMGR_MOUNT_EXEC_FILE): ./tools/volmgr_mount.c
	gcc -o ./bin/rancher-mount ./tools/volmgr_mount.c

$(VOLMGR_EXEC_FILE): ./api/devmapper.go ./api/response.go \
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
