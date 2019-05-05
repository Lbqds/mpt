package mpt

import (
	"math/rand"
	"testing"

	"bytes"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
)

// the max depth allowed when generate nested nodes
const maxDepth int = 5

var (
	source = rand.NewSource(time.Now().UnixNano())
	random = rand.New(source)
	cache  = make(map[common.Hash]node, 0)
)

func randomBytes() []byte {
	data := make([]byte, random.Intn(32))
	rand.Read(data)
	if len(data) == 0 {
		return randomBytes()
	}
	return data
}

// all node key are nibbles
func randomNibbles() []byte {
	b := randomBytes()
	nibbles := bytesToNibbles(b)
	needPad := random.Intn(2)
	if needPad == 1 {
		nibbles = append(nibbles, 3)
	}
	return nibbles
}

func generateNode(needCache bool, currentDepth int) node {
	tpe := random.Intn(3)
	var n node
	switch tpe {
	case int(leafType):
		n = generateLeafNode(needCache)
	case int(extType):
		n = generateExtNode(needCache, currentDepth)
	case int(branchType):
		n = generateBranchNode(needCache, currentDepth)
	}
	return n
}

func generateLeafNode(needCache bool) node {
	n := &leafNode{
		key:   randomNibbles(),
		value: randomBytes(),
	}
	if needCache {
		cache[n.Hash()] = n
	}
	return n
}

func generateExtNode(needCache bool, currentDepth int) node {
	var n node
	if currentDepth >= maxDepth {
		n = generateLeafNode(needCache)
	} else {
		n = &extNode{
			key:   randomNibbles(),
			child: generateNode(needCache, currentDepth+1),
		}
	}
	if needCache {
		cache[n.Hash()] = n
	}
	return n
}

func generateBranchNode(needCache bool, currentDepth int) node {
	var n node
	if currentDepth >= maxDepth {
		n = generateLeafNode(needCache)
	} else {
		var children [16]node
		for i := 0; i < 16; i++ {
			children[i] = generateNode(needCache, currentDepth+1)
		}
		n = &branchNode{
			target:   randomBytes(),
			children: children,
		}
	}
	if needCache {
		cache[n.Hash()] = n
	}
	return n
}

func checkLeafNode(original *leafNode, new node) bool {
	switch n := new.(type) {
	case *hashNode:
		return checkHashNode(original, n)
	case *leafNode:
		// use bytes.Equal instead of assert.Equal here, because:
		// assert.Equal(t, []byte(nil), []byte{}) error while
		// bytes.Equal([]byte(nil), []byte{}) not
		return bytes.Equal(original.key, n.key) && bytes.Equal(original.value, n.value)
	default:
		return false
	}
}

func checkHashNode(original node, n node) bool {
	hn := n.(*hashNode)
	hash := common.BytesToHash(hn.hash)
	cached, ok := cache[hash]
	if !ok {
		return false
	}
	return checkNode(original, cached)
}

func checkBranchNode(original *branchNode, new node) bool {
	switch n := new.(type) {
	case *hashNode:
		return checkHashNode(original, n)
	case *branchNode:
		for i := 0; i < 16; i++ {
			if !checkNode(original.children[i], n.children[i]) {
				return false
			}
		}
		return bytes.Equal(original.target, n.target)
	default:
		return false
	}
}

func checkExtNode(original *extNode, new node) bool {
	switch n := new.(type) {
	case *hashNode:
		return checkHashNode(original, n)
	case *extNode:
		return bytes.Equal(original.key, n.key) && checkNode(original.child, n.child)
	default:
		return false
	}
}

func checkNode(original node, new node) bool {
	switch n := original.(type) {
	case *leafNode:
		return checkLeafNode(n, new)
	case *extNode:
		return checkExtNode(n, new)
	case *branchNode:
		return checkBranchNode(n, new)
	default:
		return false
	}
}

func clearCache() {
	for k := range cache {
		delete(cache, k)
	}
}

func TestLeafNode(t *testing.T) {
	for i := 0; i < 10000; i++ {
		n := generateLeafNode(false)
		encoded := n.Encode()
		assert.Equal(t, leafType, encoded[len(encoded)-1]&0xf)
		decoded, err := decodeNode(encoded)
		assert.Nil(t, err)
		assert.NotNil(t, decoded)
		res := checkNode(n, decoded)
		assert.True(t, res)
	}
}

func TestExtNode(t *testing.T) {
	for i := 0; i < 100; i++ {
		clearCache()
		n := generateExtNode(true, 0)
		encoded := n.Encode()
		assert.Equal(t, extType, encoded[len(encoded)-1]&0x0f)
		ext, err := decodeNode(encoded)
		assert.Nil(t, err)
		assert.NotNil(t, ext)
		res := checkNode(n, ext)
		assert.True(t, res)
	}
}

func TestBranchNode(t *testing.T) {
	// because branchNode have 16 children, each of them can be branchNode and so forth,
	// it take a long time to generate and encode nodes, so just 10 loops
	for i := 0; i < 10; i++ {
		clearCache()
		n := generateBranchNode(true, 0)
		encoded := n.Encode()
		assert.Equal(t, branchType, encoded[len(encoded)-1])
		branch, err := decodeNode(encoded)
		assert.Nil(t, err)
		assert.NotNil(t, branch)
		res := checkNode(n, branch)
		assert.True(t, res)
	}
}

func TestNodeHash(t *testing.T) {
	for i := 0; i < 10; i++ {
		clearCache()
		n := generateNode(true, 0)
		encoded := n.Encode()
		decoded, err := decodeNode(encoded)
		assert.Nil(t, err)
		assert.NotNil(t, decoded)
		assert.Equal(t, n.Hash(), decoded.Hash())
	}
}

func TestBranchUpdateTarget(t *testing.T) {
	for i := 0; i < 10; i++ {
		clearCache()
		n := generateBranchNode(true, 0)
		branch := n.(*branchNode)
		target := branch.target
		encoded := branch.Encode()
		hash := branch.Hash()
		newBranch := branch.updateTarget(randomBytes())
		for idx := 0; idx < 16; idx++ {
			assert.True(t, checkNode(newBranch.children[idx], branch.children[idx]))
		}
		assert.Equal(t, target, branch.target)
		branch.hash = nil
		branch.encoded = nil
		assert.Equal(t, encoded, branch.Encode())
		assert.Equal(t, hash, branch.Hash())
	}
}

func TestBranchUpdateChild(t *testing.T) {
	for i := 0; i < 10; i++ {
		clearCache()
		n := generateBranchNode(true, 0)
		branch := n.(*branchNode)
		index := random.Intn(16)
		encoded := branch.Encode()
		hash := branch.Hash()
		newNode := generateNode(true, 0)
		newBranch := branch.updateChild(index, newNode)
		assert.Equal(t, branch.target, newBranch.target)
		for idx := 0; idx < 16; idx++ {
			if idx != index {
				assert.True(t, checkNode(newBranch.children[idx], branch.children[idx]))
			} else {
				assert.True(t, checkNode(newNode, newBranch.children[index]))
			}
		}
		branch.hash = nil
		branch.encoded = nil
		assert.Equal(t, encoded, branch.Encode())
		assert.Equal(t, hash, branch.Hash())
	}
}

func TestBranchChildrenIndex(t *testing.T) {
	cases := []struct {
		children [16]node
		index    []int
	}{
		{
			children: [16]node{},
			index:    []int{},
		},
		{
			children: [16]node{generateLeafNode(false), generateLeafNode(false)},
			index:    []int{0, 1},
		},
		{
			children: [16]node{nil, nil, generateLeafNode(false), generateLeafNode(false)},
			index:    []int{2, 3},
		},
	}
	for _, c := range cases {
		branch := branchWithChildren(c.children)
		assert.Equal(t, branch.childrenIndex(), c.index)
	}
}
