package metadata

import (
	"encoding/xml"
)

func DeviceMapperThinDeltaParser(data []byte, blockSize int64, includeSame bool) (*Mappings, error) {
	type Entry struct {
		XMLName xml.Name
		Begin   int64 `xml:"begin,attr"`
		Length  int64 `xml:"length,attr"`
	}

	type Diff struct {
		Entries []Entry `xml:",any"`
	}

	type Superblock struct {
		Diff Diff `xml:"diff"`
	}

	superblock := &Superblock{}
	if err := xml.Unmarshal(data, superblock); err != nil {
		return nil, err
	}

	mapping := &Mappings{}
	for _, d := range superblock.Diff.Entries {
		if !includeSame && d.XMLName.Local == "same" {
			continue
		}
		var m Mapping
		m.Offset = d.Begin * blockSize
		m.Size = d.Length * blockSize
		mapping.Mappings = append(mapping.Mappings, m)
	}
	mapping.BlockSize = blockSize

	return mapping, nil
}
