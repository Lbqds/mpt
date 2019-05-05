package mpt

func nibblesToBytes(nibbles []byte) []byte {
	// assert(len(nibbles)/2 == 0)
	bytes := make([]byte, len(nibbles)/2)
	for bi, ni := 0, 0; ni < len(nibbles); bi, ni = bi+1, ni+2 {
		bytes[bi] = nibbles[ni]<<4 | nibbles[ni+1]
	}
	return bytes
}

func bytesToNibbles(bytes []byte) []byte {
	nibbles := make([]byte, len(bytes)*2)
	for i, b := range bytes {
		nibbles[i*2] = b / 16
		nibbles[i*2+1] = b % 16
	}
	return nibbles
}

// get flag byte and stored key bytes according to key nibbles and node type
func encodeKey(nibbles []byte, nodeType byte) ([]byte, byte) {
	needPad := byte(0)
	padded := make([]byte, len(nibbles))
	copy(padded, nibbles)
	if len(nibbles)%2 != 0 {
		needPad = 1
		padded = append(padded, 0)
	}
	flag := needPad << 4
	flag = flag | nodeType
	return nibblesToBytes(padded), flag
}

// recover key nibbles and node type from flag and key bytes
func decodeKey(flag byte, bytes []byte) ([]byte, byte) {
	nibbles := bytesToNibbles(bytes)
	if flag>>4 == 1 {
		nibbles = nibbles[0 : len(nibbles)-1]
	}
	return nibbles, flag & 0x0f
}

// matchingLength return common prefix length of two bytes
func matchingLength(a []byte, b []byte) int {
	i := 0
	for ; i < len(a) && i < len(b) && a[i] == b[i]; i++ {
	}
	return i
}
