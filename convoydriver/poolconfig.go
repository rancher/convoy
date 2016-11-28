package convoydriver

type ThinPoolConfig struct {
    Driver             string	`json:"driver", omitempty`
    StorageType        string	`json:"storageType", omitempty`
    ThinPoolMetaDevice string	`json:"thinPoolMetaDevice", omitempty`
    ThinPoolDataDevice string	`json:"thinPoolDataDevice", omitempty`
    BlockSize          string	`json:"blockSize", omitempty`
    VolumeSize         string	`json:"volumeSize", omitempty`
    FileSystem         string	`json:"fileSystem", omitempty`
}

