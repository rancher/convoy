package api

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
)

type ErrorResponse struct {
	Error string
}

type VolumesResponse struct {
	Volumes map[string]VolumeResponse
}

type VolumeResponse struct {
	UUID      string
	Base      string
	Size      int64
	Snapshots map[string]SnapshotResponse
}

type SnapshotResponse struct {
	UUID       string
	VolumeUUID string
}

type BlockStoreResponse struct {
	UUID      string
	Kind      string
	BlockSize int64
}

func ResponseError(format string, a ...interface{}) {
	response := ErrorResponse{Error: fmt.Sprintf(format, a...)}
	j, err := json.MarshalIndent(&response, "", "\t")
	if err != nil {
		panic(fmt.Sprintf("Failed to generate response for error:", err))
	}
	fmt.Println(string(j[:]))
}

func ResponseLogAndError(format string, a ...interface{}) {
	log.Errorf(format, a...)
	ResponseError(format, a...)
}

func ResponseOutput(v interface{}) {
	j, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		panic(fmt.Sprintf("Failed to generate response due to error:", err))
	}
	fmt.Println(string(j[:]))
}
