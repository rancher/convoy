VOLMGR_EXEC_FILE = ./bin/volmgr
VOLMGR_MOUNT_EXEC_FILE = ./bin/volmgr_mount

.PHONY: all clean

all: $(VOLMGR_EXEC_FILE) $(VOLMGR_MOUNT_EXEC_FILE)

$(VOLMGR_MOUNT_EXEC_FILE): ./tools/volmgr_mount.c
	gcc -o ./bin/volmgr_mount ./tools/volmgr_mount.c

$(VOLMGR_EXEC_FILE): ./api/devmapper.go ./api/response.go \
	./blockstore/blockstore.go ./blockstore/config.go \
	./s3blockstore/s3blockstore.go ./s3blockstore/s3_service.go \
	./vfsblockstore/vfsblockstore.go \
	./devmapper/devmapper.go \
 	./drivers/drivers.go \
	./metadata/devmapper.go ./metadata/metadata.go \
	./util/util.go ./util/util_test.go \
	./logging/logging.go \
	./volume_cmds.go ./snapshot_cmds.go ./blockstore_cmds.go \
	./server.go ./client.go \
	./commands.go ./main.go ./main_blockstore.go ./main_devmapper.go
	go build -o ./bin/volmgr

clean:
	rm -f ./bin/volmgr*
