package mpt

import (
	"fmt"
	"io"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/golang/protobuf/proto"
)

type node interface {
	Encode() []byte
	Hash() common.Hash
	Capped() []byte
	Cache([]byte)
}

// we use proto rather than rlp, so we need to append one flag byte to the
// encoded node, which indicate that the length of key nibbles is odd/even
// and the type of node, the low 4 bits represent the type of node, the
// high 4 bits represent if we need to remove the first padded nibble of the key
const (
	leafType    byte = 0x00
	leafWithPad byte = 0x10
	extType     byte = 0x01
	extWithPad  byte = 0x11
	// branch node have no key, so pad is unnecessary
	branchType byte = 0x02
)

type (
	extNode struct {
		key     []byte
		child   node
		encoded []byte
		hash    []byte
	}
	branchNode struct {
		children [16]node
		target   []byte
		encoded  []byte
		hash     []byte
	}
	leafNode struct {
		key     []byte
		value   []byte
		encoded []byte
		hash    []byte
	}
	// use struct instead of `hashNode []byte` here because of we use pointer implement node interface
	hashNode struct {
		hash []byte
	}
)

func branchWithTarget(target []byte) *branchNode {
	return &branchNode{
		target: target,
	}
}

func branchWithChild(pos int, n node, target []byte) *branchNode {
	b := &branchNode{
		target: target,
	}
	b.children[pos] = n
	return b
}

func branchWithChildren(children [16]node) *branchNode {
	return &branchNode{
		children: children,
	}
}

func (n *branchNode) updateTarget(target []byte) *branchNode {
	b := &branchNode{
		children: n.children,
		target:   target,
	}
	return b
}

func (n *branchNode) updateChild(pos int, child node) *branchNode {
	b := &branchNode{
		children: n.children,
		target:   n.target,
	}
	b.children[pos] = child
	return b
}

// return the index of children
func (n *branchNode) childrenIndex() []int {
	index := make([]int, 0)
	for i, child := range n.children {
		if child != nil {
			index = append(index, i)
		}
	}
	return index
}

func (n *branchNode) hasTarget() bool {
	return n.target != nil
}

func (n *branchNode) Encode() []byte {
	if n.encoded != nil {
		return n.encoded
	}
	var rawNode BranchNode
	for _, n := range n.children {
		if n == nil {
			rawNode.Children = append(rawNode.Children, nil)
		} else {
			rawNode.Children = append(rawNode.Children, n.Capped())
		}
	}
	rawNode.Target = n.target
	encoded, _ := proto.Marshal(&rawNode)
	encoded = append(encoded, branchType)
	n.encoded = encoded
	return encoded
}

func (n *branchNode) Hash() common.Hash {
	if n.hash != nil {
		return common.BytesToHash(n.hash)
	}
	hash := crypto.Keccak256Hash(n.Encode())
	n.hash = hash[:]
	return hash
}

func (n *branchNode) Capped() []byte {
	encoded := n.Encode()
	if len(encoded) < common.HashLength {
		return encoded
	} else {
		hash := n.Hash()
		return hash[:]
	}
}

func (n *branchNode) Cache(bytes []byte) {
	n.encoded = bytes
}

func newExtNode(key []byte, child node) *extNode {
	return &extNode{
		key:   key,
		child: child,
	}
}

func (n *extNode) Encode() []byte {
	if n.encoded != nil {
		return n.encoded
	}
	capped := n.child.Capped()
	keyBytes, flag := encodeKey(n.key, extType)
	rawNode := &ExtNode{
		Key:  keyBytes,
		Node: capped,
	}
	encoded, _ := proto.Marshal(rawNode)
	encoded = append(encoded, flag)
	n.encoded = encoded
	return encoded
}

func (n *extNode) Hash() common.Hash {
	if n.hash != nil {
		return common.BytesToHash(n.hash)
	}
	hash := crypto.Keccak256Hash(n.Encode())
	n.hash = hash[:]
	return hash
}

func (n *extNode) Cache(bytes []byte) {
	n.encoded = bytes
}

func (n *extNode) Capped() []byte {
	encoded := n.Encode()
	if len(encoded) < common.HashLength {
		return encoded
	} else {
		hash := n.Hash()
		return hash[:]
	}
}

func newLeafNode(key, value []byte) *leafNode {
	return &leafNode{
		key:   key,
		value: value,
	}
}

func (n *leafNode) Encode() []byte {
	if n.encoded != nil {
		return n.encoded
	}
	keyBytes, flag := encodeKey(n.key, leafType)
	rawNode := &LeafNode{
		Key:   keyBytes,
		Value: n.value,
	}
	encoded, _ := proto.Marshal(rawNode)
	encoded = append(encoded, flag)
	n.encoded = encoded
	return encoded
}

func (n *leafNode) Hash() common.Hash {
	if n.hash != nil {
		return common.BytesToHash(n.hash)
	}
	hash := crypto.Keccak256Hash(n.Encode())
	n.hash = hash[:]
	return hash
}

func (n *leafNode) Capped() []byte {
	encoded := n.Encode()
	if len(encoded) < common.HashLength {
		return encoded
	} else {
		hash := n.Hash()
		return hash[:]
	}
}

func (n *leafNode) Cache(bytes []byte) {
	n.encoded = bytes
}

func (n *hashNode) Encode() []byte {
	return n.hash
}

func (n *hashNode) Hash() common.Hash {
	return common.BytesToHash(n.hash)
}

func (n *hashNode) Capped() []byte {
	return n.hash
}

func (n *hashNode) Cache(bytes []byte) {
}

func decodeNode(bytes []byte) (node, error) {
	if len(bytes) <= 1 {
		return nil, io.ErrUnexpectedEOF
	}
	flag := bytes[len(bytes)-1]
	raw := bytes[0 : len(bytes)-1]
	switch flag & 0x0f {
	case leafType:
		return decodeLeafNode(raw, flag)
	case extType:
		return decodeExtNode(raw, flag)
	case branchType:
		return decodeBranchNode(raw)
	default:
		// this should never happen
		return nil, fmt.Errorf("unknown node type: %v", flag)
	}
}

func decodeLeafNode(bytes []byte, flag byte) (node, error) {
	var rawNode LeafNode
	err := proto.Unmarshal(bytes, &rawNode)
	if err != nil {
		return nil, err
	}
	keyNibbles, _ := decodeKey(flag, rawNode.Key)
	n := &leafNode{
		key:   keyNibbles,
		value: rawNode.Value,
	}
	return n, nil
}

func decodeExtNode(bytes []byte, flag byte) (node, error) {
	var rawNode ExtNode
	err := proto.Unmarshal(bytes, &rawNode)
	if err != nil {
		return nil, err
	}
	var n extNode
	keyNibbles, _ := decodeKey(flag, rawNode.Key)
	n.key = keyNibbles
	if len(rawNode.Node) == common.HashLength {
		n.child = &hashNode{rawNode.Node}
	} else {
		n.child, err = decodeNode(rawNode.Node)
	}
	return &n, err
}

func decodeBranchNode(bytes []byte) (node, error) {
	var rawNode BranchNode
	err := proto.Unmarshal(bytes, &rawNode)
	if err != nil {
		return nil, err
	}
	var n branchNode
	n.target = rawNode.Target
	for i, child := range rawNode.Children {
		if len(child) == 0 {
			n.children[i] = nil
		} else if len(child) == common.HashLength {
			n.children[i] = &hashNode{child}
		} else {
			n.children[i], err = decodeNode(child)
		}
	}
	return &n, err
}
