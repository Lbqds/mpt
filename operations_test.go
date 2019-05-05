package mpt

import (
	"bytes"
	"testing"

	"reflect"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
)

const maxOpNum = 16

func genOperationResult() *operationResult {
	result := newOperationResult(nil)
	insertNum := random.Intn(maxOpNum)
	deleteNum := random.Intn(maxOpNum)
	for i := 0; i < insertNum; i++ {
		result.insert(generateLeafNode(false))
	}
	for i := 0; i < deleteNum; i++ {
		result.delete(generateLeafNode(false))
	}
	return result
}

func mapCopy(m map[common.Hash][]byte) map[common.Hash][]byte {
	res := make(map[common.Hash][]byte, 0)
	for k, v := range m {
		res[k] = v
	}
	return res
}

func mapContains(a map[common.Hash][]byte, hash common.Hash, value []byte) bool {
	for k, v := range a {
		if k == hash && bytes.Equal(v, value) {
			return true
		}
	}
	return false
}

func TestMergeOperationResult(t *testing.T) {
	var oldLog *updateLog
	var newLog *updateLog
	for i := 0; i < 100; i++ {
		if i == 0 {
			oldLog = newUpdateLog()
		} else {
			oldLog = newLog
		}
		oldInserted := mapCopy(oldLog.inserted)
		oldDeleted := mapCopy(oldLog.deleted)
		result := genOperationResult()
		newLog = oldLog.copy()
		newLog.merge(EmptyHash, nil, result.deleted, result.inserted)
		assert.Equal(t, reflect.DeepEqual(oldInserted, oldLog.inserted), true)
		assert.Equal(t, reflect.DeepEqual(oldDeleted, oldLog.deleted), true)
	}
}

func TestMergeWithNewNode(t *testing.T) {
	// the encoded leaf node length less than 32
	leaf := newLeafNode([]byte{0x01, 0x02}, []byte{0x01, 0x02})
	log := newUpdateLog()
	result := newOperationResult(leaf)
	result.insert(leaf)
	log.merge(EmptyHash, leaf, result.deleted, result.inserted)
	assert.True(t, mapContains(log.inserted, leaf.Hash(), leaf.Encode()))
}

func TestMergeWithOldRootHash(t *testing.T) {
	// the encoded leaf node length less then 32
	leaf := newLeafNode([]byte{0x01, 0x02}, []byte{0x01, 0x02})
	log := newUpdateLog()
	result := newOperationResult(nil)
	result.delete(leaf)
	log.merge(leaf.Hash(), nil, result.deleted, result.inserted)
	assert.True(t, mapContains(log.deleted, leaf.Hash(), []byte{}))
}
