package metadata

type Mapping struct {
	Offset uint64
	Size   uint64
}

type Mappings struct {
	Mappings  []Mapping
	BlockSize uint32
}
