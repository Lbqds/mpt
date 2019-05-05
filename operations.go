package mpt

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
)

// updateLog record all operations for immutable trie:
// - cached: cache key value from underlying db
// - inserted: record all inserted key value
// - deleted: record all deleted key value
// all inserted and deleted key value will be flushed to
// underlying db when execute trie.persist
type updateLog struct {
	cached   map[common.Hash][]byte
	inserted map[common.Hash][]byte
	deleted  map[common.Hash][]byte
}

func newUpdateLog() *updateLog {
	return &updateLog{
		cached:   make(map[common.Hash][]byte, 0),
		inserted: make(map[common.Hash][]byte, 0),
		deleted:  make(map[common.Hash][]byte, 0),
	}
}

func (log *updateLog) cache(key common.Hash, value []byte) {
	log.cached[key] = value
}

func (log *updateLog) insert(key common.Hash, value []byte) {
	delete(log.deleted, key)
	delete(log.cached, key)
	log.inserted[key] = value
}

func (log *updateLog) delete(key common.Hash) {
	delete(log.inserted, key)
	delete(log.cached, key)
	log.deleted[key] = []byte{}
}

func (log *updateLog) copy() *updateLog {
	newLog := newUpdateLog()
	for k, v := range log.cached {
		newLog.cached[k] = v
	}
	for k, v := range log.inserted {
		newLog.inserted[k] = v
	}
	for k, _ := range log.deleted {
		newLog.deleted[k] = []byte{}
	}
	return newLog
}

// merge return a new log which include current log and new inserted result
func (log *updateLog) mergeFromInsertResult(oldRootHash common.Hash, result *insertResult) *updateLog {
	if result == nil {
		return log
	}
	newLog := log.copy()
	newLog.merge(oldRootHash, result.newNode, result.deleted, result.inserted)
	return newLog
}

// merge return a new log which include current log and new deleted result
func (log *updateLog) mergeFromDeleteResult(oldRootHash common.Hash, result *deleteResult) *updateLog {
	if result == nil {
		return log
	}
	newLog := log.copy()
	newLog.merge(oldRootHash, result.newNode, result.deleted, result.inserted)
	return newLog
}

func (log *updateLog) merge(oldRootHash common.Hash, newRootNode node, deleted []node, inserted []node) {
	for _, n := range deleted {
		capped := n.Capped()
		vh := n.Hash()
		// if encoded old root node less than 32 bytes, we must delete
		// the old root node from underlying db
		if len(capped) == common.HashLength || vh == oldRootHash {
			log.delete(vh)
		}
	}
	for _, n := range inserted {
		capped := n.Capped()
		var newRootCapped []byte
		if newRootNode == nil {
			newRootCapped = EmptyHash[:]
		} else {
			newRootCapped = newRootNode.Capped()
		}
		// if the encoded root node less than 32 bytes, we must persist the root
		// node to underlying db, and the hash of the root node is state hash
		if len(capped) == common.HashLength || bytes.Equal(capped, newRootCapped) {
			log.insert(n.Hash(), n.Encode())
		}
	}
}

type operationResult struct {
	newNode  node
	inserted []node
	deleted  []node
}

func newOperationResult(newNode node) *operationResult {
	return &operationResult{
		newNode:  newNode,
		inserted: make([]node, 0),
		deleted:  make([]node, 0),
	}
}

func (res *operationResult) delete(n node) {
	if n != nil {
		res.deleted = append(res.deleted, n)
	}
}

func (res *operationResult) insert(n node) {
	if n != nil {
		res.inserted = append(res.inserted, n)
	}
}

// insertResult record all deleted kv and inserted kv after trie.Insert
type insertResult struct {
	*operationResult
}

func newInsertResult(newNode node) *insertResult {
	return &insertResult{
		operationResult: newOperationResult(newNode),
	}
}

// deleteResult record all deleted kv and inserted kv after trie.Delete
// hasChanged is false indicated that the deleted kv is not in current trie
type deleteResult struct {
	*operationResult
	hasChanged bool
}

func newDeleteResult(newNode node, hasChanged bool) *deleteResult {
	return &deleteResult{
		operationResult: newOperationResult(newNode),
		hasChanged:      hasChanged,
	}
}
