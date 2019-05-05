package mpt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncoding(t *testing.T) {
	for i := 0; i < 10000; i++ {
		bytes := randomBytes()
		nibbles := bytesToNibbles(bytes)
		recovered := nibblesToBytes(nibbles)
		assert.Equal(t, bytes, recovered)
	}
}

func TestEncDecLeafKey(t *testing.T) {
	keyNibbles := randomNibbles()
	keyBytes, flag := encodeKey(keyNibbles, leafType)
	if len(keyNibbles)%2 == 0 {
		assert.Equal(t, leafType, flag)
	} else {
		assert.Equal(t, leafWithPad, flag)
	}
	nibbles, nodeType := decodeKey(flag, keyBytes)
	assert.Equal(t, leafType, nodeType)
	assert.Equal(t, keyNibbles, nibbles)
}

func TestEncDecExtKey(t *testing.T) {
	keyNibbles := randomNibbles()
	keyBytes, flag := encodeKey(keyNibbles, extType)
	if len(keyNibbles)%2 == 0 {
		assert.Equal(t, extType, flag)
	} else {
		assert.Equal(t, extWithPad, flag)
	}
	nibbles, nodeType := decodeKey(flag, keyBytes)
	assert.Equal(t, extType, nodeType)
	assert.Equal(t, keyNibbles, nibbles)
}

func TestMatchingLength(t *testing.T) {
	cases := []struct {
		a            []byte
		b            []byte
		commonLength int
	}{
		{
			a:            []byte{0x01, 0x02, 0x03},
			b:            []byte{0x01, 0x02, 0x03},
			commonLength: 3,
		},
		{
			a:            []byte{0x01, 0x02, 0x03, 0x04},
			b:            []byte{0x01, 0x02, 0x03},
			commonLength: 3,
		},
		{
			a:            []byte{0x01, 0x02, 0x03},
			b:            []byte{0x01, 0x02, 0x03, 0x04},
			commonLength: 3,
		},
		{
			a:            []byte{},
			b:            []byte{0x01, 0x02, 0x03},
			commonLength: 0,
		},
	}
	for _, c := range cases {
		ml := matchingLength(c.a, c.b)
		assert.Equal(t, ml, c.commonLength)
	}
}
