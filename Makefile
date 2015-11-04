CONVOY_EXEC_FILE = ./bin/convoy

.PHONY: all clean

all: $(CONVOY_EXEC_FILE)

FLAGS = -tags "libdm_no_deferred_remove"
ifeq ($(STATIC_LINK), 1)
    FLAGS = -a -tags "netgo libdm_no_deferred_remove" \
	    -ldflags "-linkmode external -extldflags -static" \
	    --installsuffix netgo
endif

$(CONVOY_EXEC_FILE): ./main.go ./api/request.go \
	./api/response.go ./api/const.go \
	./daemon/daemon.go ./daemon/common.go ./daemon/volume.go \
	./daemon/snapshot.go ./daemon/objectstore.go \
	./daemon/import_objectstore.go ./daemon/import_devmapper.go \
	./daemon/import_ebs.go \
	./daemon/docker.go \
	./client/volume.go ./client/snapshot.go ./client/objectstore.go \
	./client/client.go ./client/daemon.go \
	./objectstore/objectstore.go ./objectstore/driver.go \
	./objectstore/config.go \
	./objectstore/deltablock.go ./objectstore/singlefile.go \
	./s3/s3.go ./s3/s3_service.go \
	./ebs/ebs.go ./ebs/ebs_service.go \
	./vfs/vfs_objectstore.go ./vfs/vfs_storage.go \
	./convoydriver/convoydriver.go \
	./devmapper/devmapper.go ./devmapper/backup.go \
	./shorthorn/shorthorn.go ./shorthorn/template.go \
	./metadata/devmapper.go ./metadata/metadata.go \
	./util/util.go ./util/config.go \
	./util/volume.go ./util/index.go \
	./logging/logging.go
	go build $(FLAGS) -o $(CONVOY_EXEC_FILE)

clean:
	rm -f $(CONVOY_EXEC_FILE)

install:
	cp $(CONVOY_EXEC_FILE) /usr/local/bin/

test:
	go test -tags "libdm_no_deferred_remove" ./...
