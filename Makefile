RANCHER-VOLUME_EXEC_FILE = ./bin/rancher-volume

.PHONY: all clean

all: $(RANCHER-VOLUME_EXEC_FILE) $(RANCHER-MOUNT_EXEC_FILE)

FLAGS = -tags "libdm_no_deferred_remove"
ifeq ($(STATIC_LINK), 1)
    FLAGS = -a -tags "netgo libdm_no_deferred_remove" \
	    -ldflags "-linkmode external -extldflags -static" \
	    --installsuffix netgo
endif

$(RANCHER-VOLUME_EXEC_FILE): ./main.go \
	./api/devmapper.go ./api/response.go ./api/const.go \
	./server/server.go ./server/common.go ./server/volume.go \
	./server/snapshot.go ./server/objectstore.go \
	./server/server_objectstore.go ./server/server_devmapper.go \
	./server/docker.go \
	./client/volume.go ./client/snapshot.go ./client/objectstore.go \
	./client/client.go ./client/server.go \
	./objectstore/objectstore.go ./objectstore/config.go \
	./s3/s3.go ./s3/s3_service.go \
	./vfs/vfs.go \
	./driver/driver.go \
	./devmapper/devmapper.go \
	./metadata/devmapper.go ./metadata/metadata.go \
	./util/util.go ./util/util_test.go ./util/index.go \
	./logging/logging.go
	go build $(FLAGS) -o $(RANCHER-VOLUME_EXEC_FILE)

clean:
	rm -f $(RANCHER-VOLUME_EXEC_FILE)

install:
	cp $(RANCHER-VOLUME_EXEC_FILE) /usr/local/bin/
