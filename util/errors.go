package util

//error types
const (
	ErrVolumeNotFoundCode = iota
	ErrSnapshotNotFoundCode
	ErrVolumeInUseCode
	ErrVolumeExistsCode
	ErrVolumeNotAvailableCode
	ErrVolumeCreateFailureCode
	ErrVolumeDeleteFailureCode
	ErrVolumeAttachFailureCode
	ErrVolumeDetachFailureCode
	ErrDeviceFailureCode
	ErrVolumeTransitionCode
	ErrInvalidRequestCode
	ErrGenericFailureCode //prefer if no other error type suffices
)

type ConvoyDriverErr struct {
	//Original error from the backend
	Err error

	//Convoy's internal error code
	ErrorCode int
}

func (e ConvoyDriverErr) Error() string {
	return e.Err.Error()
}

//GetConvoyErr returns a ConvoyErr which contains the original error and a suitable error code
func NewConvoyDriverErr(err error, errCode int) error {
	return &ConvoyDriverErr{Err: err, ErrorCode: errCode}
}
