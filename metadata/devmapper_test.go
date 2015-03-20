package metadata

import (
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

const (
	thinDeltaOutputSame = `<same>
	  	<range begin="0" data_begin="0" length="1"/>
	    	<range begin="4" data_begin="1" length="1"/>
	    	</same>`
	thinDeltaOutputDiff = `<different>
  		<range begin="0" left_data_begin="0" right_data_begin="5" length="1"/>
    		<range begin="4" left_data_begin="1" right_data_begin="4" length="1"/>
    		</different>`
	thinDeltaOutputMix = `<same>
	  	<range begin="0" data_begin="0" length="1"/>
	  	</same>
	  
	  	<different>
	    	<range begin="4" left_data_begin="1" right_data_begin="3" length="1"/>
	    	</different>`
	blockSize = 2097152
)

func TestThinDelta(t *testing.T) {
	mSame := Mappings{
		Mappings: []Mapping{
			{Offset: 0, Size: 1 * blockSize},
			{Offset: 4 * blockSize, Size: 1 * blockSize},
		},
		BlockSize: blockSize,
	}
	mDiff := Mappings{
		Mappings: []Mapping{
			{Offset: 0, Size: 1 * blockSize},
			{Offset: 4 * blockSize, Size: 1 * blockSize},
		},
		BlockSize: blockSize,
	}
	mMix := Mappings{
		Mappings: []Mapping{
			{Offset: 4 * blockSize, Size: 1 * blockSize},
		},
		BlockSize: blockSize,
	}

	m := &Mappings{}
	err := DeviceMapperThinDeltaParser([]byte(thinDeltaOutputMix), blockSize, m)
	require.Nil(t, err)
	if !reflect.DeepEqual(*m, mMix) {
		t.Fatal("Fail to get expect result from `mix` check")
	}

	m = &Mappings{}
	err = DeviceMapperThinDeltaParser([]byte(thinDeltaOutputSame), blockSize, m)
	require.Nil(t, err)
	if !reflect.DeepEqual(*m, mSame) {
		t.Fatal("Fail to get expect result from `same` check")
	}

	m = &Mappings{}
	err = DeviceMapperThinDeltaParser([]byte(thinDeltaOutputDiff), blockSize, m)
	require.Nil(t, err)
	if !reflect.DeepEqual(*m, mDiff) {
		t.Fatal("Fail to get expect result from `diff` check")
	}
}
