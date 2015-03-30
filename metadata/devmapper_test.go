package metadata

import (
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

const (
	thinDeltaOutputSame = `<same begin="0" length="1"/>
			<same begin="1" length="1"/>
			<same begin="4" length="3"/>
			<same begin="8" length="1"/>
			<same begin="12" length="1"/>`
	thinDeltaOutputDiff = `<different begin="0" length="1"/>
		<different begin="4" length="1"/>`
	thinDeltaOutputMix = `<same begin="0" length="1"/>
		<left begin="2" length="1"/>
		<different begin="4" length="1"/>
		<different begin="6" length="1"/>
		<right begin="8" length="2"/>`
	blockSize = 2097152
)

func TestThinDelta(t *testing.T) {
	mSame := Mappings{
		Mappings: []Mapping{
			{Offset: 0, Size: 1 * blockSize},
			{Offset: 1 * blockSize, Size: 1 * blockSize},
			{Offset: 4 * blockSize, Size: 3 * blockSize},
			{Offset: 8 * blockSize, Size: 1 * blockSize},
			{Offset: 12 * blockSize, Size: 1 * blockSize},
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
			{Offset: 2 * blockSize, Size: 1 * blockSize},
			{Offset: 4 * blockSize, Size: 1 * blockSize},
			{Offset: 6 * blockSize, Size: 1 * blockSize},
			{Offset: 8 * blockSize, Size: 2 * blockSize},
		},
		BlockSize: blockSize,
	}

	var (
		m   *Mappings
		err error
	)

	m, err = DeviceMapperThinDeltaParser([]byte(thinDeltaOutputSame), blockSize, true)
	require.Nil(t, err)
	if !reflect.DeepEqual(*m, mSame) {
		t.Fatal("Fail to get expect result from `same` check")
	}

	m, err = DeviceMapperThinDeltaParser([]byte(thinDeltaOutputMix), blockSize, false)
	require.Nil(t, err)
	if !reflect.DeepEqual(*m, mMix) {
		t.Fatal("Fail to get expect result from `mix` check")
	}

	m, err = DeviceMapperThinDeltaParser([]byte(thinDeltaOutputDiff), blockSize, false)
	require.Nil(t, err)
	if !reflect.DeepEqual(*m, mDiff) {
		t.Fatal("Fail to get expect result from `diff` check")
	}
}
