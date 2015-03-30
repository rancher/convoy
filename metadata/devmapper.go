package metadata

import (
	"bytes"
	"encoding/xml"
)

func DeviceMapperThinDeltaParser(data []byte, blockSize int64, includeSame bool) (*Mappings, error) {
	type Entry struct {
		XMLName xml.Name
		Begin   int64 `xml:"begin,attr"`
		Length  int64 `xml:"length,attr"`
	}

	type Delta struct {
		Entries []Entry `xml:",any"`
	}

	wrapStart := []byte("<wrap>")
	wrapEnd := []byte("</wrap>")
	wrapData := bytes.Join([][]byte{wrapStart, data, wrapEnd}, []byte(" "))
	delta := &Delta{}
	if err := xml.Unmarshal(wrapData, delta); err != nil {
		return nil, err
	}

	mapping := &Mappings{}
	for _, d := range delta.Entries {
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
