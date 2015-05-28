package logging

const (
	LOG_FIELD_VOLUME     = "volume"
	LOG_FIELD_SNAPSHOT   = "snapshot"
	LOG_FIELD_BLOCKSTORE = "blockstore"
	LOG_FIELD_MOUNTPOINT = "mountpoint"
	LOG_FIELD_NAMESPACE  = "namespace"
	LOG_FIELD_CFG        = "config_file"
	LOG_FIELD_IMAGE      = "image"
	LOG_FIELD_IMAGE_DEV  = "image_dev"

	LOG_FIELD_EVENT    = "event"
	LOG_EVENT_MOUNT    = "mount"
	LOG_EVENT_UMOUNT   = "umount"
	LOG_EVENT_ACTIVATE = "activate"
	LOG_EVENT_CREATE   = "create"
	LOG_EVENT_REMOVE   = "remove"
	LOG_EVENT_LOAD_CFG = "load_cfg"

	LOG_FIELD_REASON    = "reason"
	LOG_REASON_START    = "start"
	LOG_REASON_COMPLETE = "complete"
	LOG_REASON_FAILURE  = "failure"
	LOG_REASON_ROLLBACK = "rollback"
)
