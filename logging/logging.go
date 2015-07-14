package logging

import (
	"fmt"
	"github.com/Sirupsen/logrus"
)

const (
	LOG_FIELD_DRIVER        = "driver"
	LOG_FIELD_VOLUME        = "volume"
	LOG_FIELD_VOLUME_NAME   = "volume_name"
	LOG_FIELD_ORIN_VOLUME   = "original_volume"
	LOG_FIELD_SNAPSHOT      = "snapshot"
	LOG_FIELD_LAST_SNAPSHOT = "last_snapshot"
	LOG_FIELD_OBJECTSTORE   = "objectstore"
	LOG_FIELD_MOUNTPOINT    = "mountpoint"
	LOG_FIELD_NAMESPACE     = "namespace"
	LOG_FIELD_CFG           = "config_file"
	LOG_FIELD_SIZE          = "size"
	LOG_FIELD_FILESYSTEM    = "filesystem"
	LOG_FIELD_OPTION        = "option"
	LOG_FIELD_NEED_FORMAT   = "need_format"
	LOG_FIELD_BLOCKSIZE     = "blocksize"
	LOG_FIELD_KIND          = "kind"
	LOG_FIELD_FILEPATH      = "filepath"
	LOG_FIELD_CONTEXT       = "context"

	LOG_FIELD_EVENT      = "event"
	LOG_EVENT_INIT       = "init"
	LOG_EVENT_CREATE     = "create"
	LOG_EVENT_DELETE     = "delete"
	LOG_EVENT_FORMAT     = "format"
	LOG_EVENT_LIST       = "list"
	LOG_EVENT_MOUNT      = "mount"
	LOG_EVENT_UMOUNT     = "umount"
	LOG_EVENT_ACTIVATE   = "activate"
	LOG_EVENT_DEACTIVATE = "deactivate"
	LOG_EVENT_REGISTER   = "register"
	LOG_EVENT_DEREGISTER = "deregister"
	LOG_EVENT_ADD        = "add"
	LOG_EVENT_REMOVE     = "remove"
	LOG_EVENT_BACKUP     = "backup"
	LOG_EVENT_RESTORE    = "restore"
	LOG_EVENT_LOAD       = "load"
	LOG_EVENT_SAVE       = "save"
	LOG_EVENT_COMPARE    = "compare"
	LOG_EVENT_UPLOAD     = "upload"
	LOG_EVENT_DOWNLOAD   = "download"

	LOG_FIELD_REASON    = "reason"
	LOG_REASON_PREPARE  = "prepare"
	LOG_REASON_START    = "start"
	LOG_REASON_COMPLETE = "complete"
	LOG_REASON_FAILURE  = "failure"
	LOG_REASON_ROLLBACK = "rollback"
	LOG_REASON_FALLBACK = "fallback"

	LOG_FIELD_OBJECT       = "object"
	LOG_OBJECT_DRIVER      = "driver"
	LOG_OBJECT_VOLUME      = "volume"
	LOG_OBJECT_SNAPSHOT    = "snapshot"
	LOG_OBJECT_OBJECTSTORE = "objectstore"
	LOG_OBJECT_CONFIG      = "config"
)

type LoggingError struct {
	entry *logrus.Entry
	error
}

func ErrorWithFields(pkg string, fields logrus.Fields, format string, v ...interface{}) LoggingError {
	fields["pkg"] = pkg
	entry := logrus.WithFields(fields)
	entry.Message = fmt.Sprintf(format, v...)

	return LoggingError{entry, fmt.Errorf(format, v...)}
}
