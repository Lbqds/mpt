package mpt

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/stretchr/testify/assert"
)

const iterateTimes = 1000

// TestTrieInsertCase1, insert kv to trie, persist and get the state root,
// then reload from the underlying db with new state root, get key and check
// if we can get expected result(YES, WE CAN), check if the old trie changed,
// and check if we can get value from old trie
func TestTrieInsertCase1(t *testing.T) {
	memDB := memorydb.New()
	trie := NewTrie(EmptyHash, memDB)
	for i := 0; i < iterateTimes; i++ {
		randomKey := randomBytes()
		randomValue := randomBytes()
		// does the old trie have randomKey?
		existBeforeInsert := len(trie.Get(randomKey)) != 0
		oldLog := trie.log.copy()
		oldStateRoot := trie.StateRoot()
		newTrie := trie.Insert(randomKey, randomValue)
		newTrie.Persist()
		existAfterInsert := len(trie.Get(randomKey)) != 0
		assert.Equal(t, existBeforeInsert, existAfterInsert)
		assert.Equal(t, oldStateRoot, trie.StateRoot())
		if !reflect.DeepEqual(oldLog, trie.log) {
			t.Fatal("trie has changed")
		}
		stateRoot := newTrie.StateRoot()
		// reload from the underlying db
		newLoadedTrie := NewTrie(stateRoot, memDB)
		value := newLoadedTrie.Get(randomKey)
		if !reflect.DeepEqual(value, randomValue) {
			t.Fatalf("unmatched value, expected: %v, have: %v", value, randomValue)
		}
		trie = newLoadedTrie
	}
}

// TestTrieInsertCase2, insert kv to trie, DO NOT persist, check if
// we can get expected value(YES, WE CAN), check if we can get value
// from lod trie(NO, WE CAN'T), then check if the trie changed
// NOTE: because all node store at memory, it takes a long time to
// execute when recursive depth too large
func TestTrieInsertCase2(t *testing.T) {
	memDB := memorydb.New()
	trie := NewTrie(EmptyHash, memDB)
	for i := 0; i < iterateTimes; i++ {
		randomKey := randomBytes()
		randomValue := randomBytes()
		existBeforInsert := len(trie.Get(randomKey)) != 0
		oldLogs := trie.log.copy()
		oldStateRoot := trie.StateRoot()
		newTrie := trie.Insert(randomKey, randomValue)
		existAfterInsert := len(trie.Get(randomKey)) != 0
		assert.Equal(t, existBeforInsert, existAfterInsert)
		assert.Equal(t, oldStateRoot, trie.StateRoot())
		value := newTrie.Get(randomKey)
		if !reflect.DeepEqual(oldLogs, trie.log) {
			t.Fatalf("trie has changed")
		}
		if !bytes.Equal(value, randomValue) {
			t.Fatal()
		}
		trie = newTrie
	}
}

// TestTrieInsertCase3, insert kv to trie, persist, then reload with
// the old state root, get key and check if we can get expected
// result(NO, WE CAN'T)
func TestTrieInsertCase3(t *testing.T) {
	memDB := memorydb.New()
	trie := NewTrie(EmptyHash, memDB)
	for i := 0; i < iterateTimes; i++ {
		randomKey := randomBytes()
		randomValue := randomBytes()
		existBeforeInsert := len(trie.Get(randomKey)) != 0
		newTrie := trie.Insert(randomKey, randomValue)
		newTrie.Persist()
		// reload from underlying db with old state root
		trie = NewTrie(trie.StateRoot(), memDB)
		existAfterInsert := len(trie.Get(randomKey)) != 0
		assert.Equal(t, existBeforeInsert, existAfterInsert)
	}
}

type kv struct {
	k []byte
	v []byte
}

func newKV() kv {
	return kv{
		k: randomBytes(),
		v: randomBytes(),
	}
}

// TestTrieDeleteCase1, insert kvs to trie, then delete these kvs,
// check if we can get expected value from old trie(YES, WE CAN)
// and new trie(NO, WE CAN'T), and check if the old trie changed
func TestTrieDeleteCase1(t *testing.T) {
	memDB := memorydb.New()
	kvs := make([]kv, 0)
	trie := NewTrie(EmptyHash, memDB)
	for i := 0; i < iterateTimes; i++ {
		elem := newKV()
		trie = trie.Insert(elem.k, elem.v)
		kvs = append(kvs, elem)
	}
	for _, elem := range kvs {
		existInOldTrie := len(trie.Get(elem.k)) != 0
		oldStateRoot := trie.StateRoot()
		oldLogs := trie.log.copy()
		newTrie := trie.Delete(elem.k)
		stillExistInOldTrie := len(trie.Get(elem.k)) != 0
		assert.Equal(t, existInOldTrie, stillExistInOldTrie)
		assert.Equal(t, oldStateRoot, trie.StateRoot())
		if !reflect.DeepEqual(oldLogs, trie.log) {
			t.Fatal("trie has changed")
		}
		existInNewTrie := len(newTrie.Get(elem.k)) != 0
		assert.False(t, existInNewTrie)
		trie = newTrie
	}
	stateRoot := trie.StateRoot()
	assert.Equal(t, stateRoot, EmptyHash, "delete all keys, but state root is: %v", stateRoot)
}

// TestTrieDeleteCase2, insert kvs to trie, persist, reload from the
// underlying db with the new state root, then delete these kvs, check
// if we can get expected value from old trie and new trie, persist again,
// reload again, then check again
func TestTrieDeleteCase2(t *testing.T) {
	memDB := memorydb.New()
	kvs := make([]kv, 0)
	trie := NewTrie(EmptyHash, memDB)
	for i := 0; i < iterateTimes; i++ {
		elem := newKV()
		trie = trie.Insert(elem.k, elem.v)
		kvs = append(kvs, elem)
	}
	trie.Persist()
	for _, elem := range kvs {
		existInOldTrie := len(trie.Get(elem.k)) != 0
		newTrie := NewTrie(trie.StateRoot(), memDB)
		newTrie = newTrie.Delete(elem.k)
		stillExistInOldTrie := len(trie.Get(elem.k)) != 0
		assert.Equal(t, existInOldTrie, stillExistInOldTrie)
		existInNewTrie := len(newTrie.Get(elem.k)) != 0
		assert.False(t, existInNewTrie)
		newTrie.Persist()
		reloadTrie := NewTrie(newTrie.StateRoot(), memDB)
		checkExist := len(reloadTrie.Get(elem.k)) != 0
		assert.False(t, checkExist)
		trie = newTrie
	}
}

func TestDeleteThenInsert(t *testing.T) {
	memDB := memorydb.New()
	kvs := make([]kv, 0)
	trie := NewTrie(EmptyHash, memDB)
	for i := 0; i < 3; i++ {
		elem := newKV()
		trie = trie.Insert(elem.k, elem.v)
		kvs = append(kvs, elem)
	}
	stateRoot := trie.StateRoot()
	trie.Persist()
	trie = NewTrie(stateRoot, memDB)
	trie = trie.Delete(kvs[2].k)
	trie = trie.Insert(kvs[2].k, kvs[2].v)
	stateRoot = trie.StateRoot()
	trie.Persist()
	trie = NewTrie(stateRoot, memDB)
	value := trie.Get(kvs[2].k)
	assert.Equal(t, value, kvs[2].v)
}
