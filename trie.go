package mpt

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	db "github.com/ethereum/go-ethereum/ethdb"
)

// EmptyHash is hash of empty trie
var EmptyHash = crypto.Keccak256Hash([]byte{})

// Trie is a immutable merkle patricia tree, every change(delete or insert) will return a new trie
// with a different root and a different hash as well, the new trie maybe have pointers to subtrees
// from old trie. Field logs of Trie used to log all changes before persist to underlying db.
type Trie struct {
	db       db.KeyValueStore
	rootHash common.Hash
	log      *updateLog
}

func NewTrie(rootHash common.Hash, db db.KeyValueStore) *Trie {
	return &Trie{
		db:       db,
		rootHash: rootHash,
		log:      newUpdateLog(),
	}
}

// Get returns the values for key stored in the trie.
// Caller must not modify the result directly, if need, use Insert/Delete
func (t *Trie) Get(key []byte) []byte {
	if t.rootHash == EmptyHash {
		return nil
	}
	rootNode, err := t.resolveHash(t.rootHash)
	if err != nil {
		return nil
	}
	searchKey := bytesToNibbles(key)
	return t.tryGet(rootNode, searchKey)
}

func (t *Trie) tryGet(startNode node, searchKey []byte) []byte {
	switch n := startNode.(type) {
	case *leafNode:
		if bytes.Equal(searchKey, n.key) {
			return n.value
		}
		return nil
	case *extNode:
		if len(searchKey) < len(n.key) {
			return nil
		}
		if bytes.Equal(searchKey[0:len(n.key)], n.key) {
			return t.tryGet(n.child, searchKey[len(n.key):])
		}
		return nil
	case *branchNode:
		if len(searchKey) == 0 {
			return n.target
		}
		return t.tryGet(n.children[searchKey[0]], searchKey[1:])
	case *hashNode:
		resolved, err := t.resolveHash(n.Hash())
		if err != nil {
			return nil
		}
		return t.tryGet(resolved, searchKey)
	default:
		// this should never happen
		return nil
	}
}

// Insert insert key and value to trie, return a new trie, old trie is unchanged
func (t *Trie) Insert(key, value []byte) *Trie {
	searchKey := bytesToNibbles(key)
	var newRootNode node
	var result *insertResult
	if t.rootHash == EmptyHash {
		newRootNode = newLeafNode(searchKey, value)
		result = newInsertResult(newRootNode)
		result.insert(newRootNode)
	} else {
		rootNode, err := t.resolveHash(t.rootHash)
		if err != nil {
			panic("Insert: can't resolve rootHash")
		}
		result = t.insert(rootNode, searchKey, value)
		newRootNode = result.newNode
	}
	newTrie := NewTrie(newRootNode.Hash(), t.db)
	newTrie.log = t.log.mergeFromInsertResult(t.rootHash, result)
	return newTrie
}

func (t *Trie) insert(startNode node, searchKey, value []byte) *insertResult {
	switch n := startNode.(type) {
	case *leafNode:
		return t.insertToLeaf(n, searchKey, value)
	case *extNode:
		return t.insertToExt(n, searchKey, value)
	case *branchNode:
		return t.insertToBranch(n, searchKey, value)
	case *hashNode:
		newNode, err := t.resolveHash(n.Hash())
		if err != nil {
			panic("insert: can't resolve hashNode")
		}
		return t.insert(newNode, searchKey, value)
	default:
		// this should never happen
		return nil
	}
}

func (t *Trie) insertToLeaf(leaf *leafNode, searchKey, value []byte) *insertResult {
	ml := matchingLength(searchKey, leaf.key)
	// update current leaf node, so create new one directly
	if ml == len(searchKey) && ml == len(leaf.key) {
		newLeaf := newLeafNode(searchKey, value)
		result := newInsertResult(newLeaf)
		result.delete(leaf)
		result.insert(newLeaf)
		return result
	}
	// no common prefix, so create a new branch node first
	if ml == 0 {
		var tempBranch *branchNode
		var maybeLeaf node
		if len(leaf.key) == 0 {
			tempBranch = branchWithTarget(leaf.value)
		} else {
			maybeLeaf = newLeafNode(leaf.key[1:], leaf.value)
			tempBranch = branchWithChild(int(leaf.key[0]), maybeLeaf, nil)
		}
		result := t.insert(tempBranch, searchKey, value)
		result.delete(leaf)
		result.insert(maybeLeaf)
		return result
	}
	// have common prefix, create a new branch node which embedded in a new ext node
	var tempNode node
	if ml == len(leaf.key) {
		tempNode = branchWithTarget(leaf.value)
	} else {
		tempNode = newLeafNode(leaf.key[ml:], leaf.value)
	}
	result := t.insert(tempNode, searchKey[ml:], value)
	tempExtNode := newExtNode(leaf.key[:ml], result.newNode)
	result.newNode = tempExtNode
	result.delete(leaf)
	result.insert(tempExtNode)
	return result
}

func (t *Trie) insertToExt(ext *extNode, searchKey, value []byte) *insertResult {
	ml := matchingLength(searchKey, ext.key)
	if ml == 0 {
		// no common prefix, so we need a branch node
		var tempBranch *branchNode
		var maybeChild node
		if len(ext.key) == 1 {
			// change this node to branch directly
			tempBranch = branchWithChild(int(ext.key[0]), ext.child, nil)
		} else {
			newExt := newExtNode(ext.key[1:], ext.child)
			maybeChild = newExt
			tempBranch = branchWithChild(int(ext.key[0]), newExt, nil)
		}
		result := t.insert(tempBranch, searchKey, value)
		result.insert(maybeChild)
		result.delete(ext)
		return result
	}
	if ml == len(ext.key) {
		// matched completely, insert kv to the extNode's child
		result := t.insert(ext.child, searchKey[ml:], value)
		newExt := newExtNode(ext.key, result.newNode)
		result.newNode = newExt
		result.insert(newExt)
		result.delete(ext)
		return result
	}
	tempExt := newExtNode(ext.key[ml:], ext.child)
	result := t.insert(tempExt, searchKey[ml:], value)
	newExt := newExtNode(ext.key[:ml], result.newNode)
	result.newNode = newExt
	result.insert(newExt)
	result.delete(ext)
	return result
}

func (t *Trie) insertToBranch(branch *branchNode, searchKey, value []byte) *insertResult {
	if len(searchKey) == 0 {
		// searchKey is empty, update target value directly
		newBranch := branch.updateTarget(value)
		result := newInsertResult(newBranch)
		result.insert(newBranch)
		result.delete(branch)
		return result
	}
	pos := int(searchKey[0])
	if branch.children[pos] != nil {
		// matched to children, insert kv to children
		result := t.insert(branch.children[pos], searchKey[1:], value)
		newBranch := branch.updateChild(pos, result.newNode)
		result.newNode = newBranch
		result.insert(newBranch)
		result.delete(branch)
		return result
	}
	newLeaf := newLeafNode(searchKey[1:], value)
	newBranch := branch.updateChild(pos, newLeaf)
	result := newInsertResult(newBranch)
	result.insert(newBranch)
	result.insert(newLeaf)
	result.delete(branch)
	return result
}

// Delete delete key and value from trie, return a new trie, old trie is unchanged
func (t *Trie) Delete(key []byte) *Trie {
	if t.rootHash == EmptyHash {
		return t
	}
	searchKey := bytesToNibbles(key)
	rootNode, err := t.resolveHash(t.rootHash)
	if err != nil {
		panic("Delete: can't resolve rootHash")
	}
	result := t.delete(rootNode, searchKey)
	if !result.hasChanged {
		return t
	}
	var newRootHash common.Hash
	if result.newNode == nil {
		newRootHash = EmptyHash
	} else {
		newRootHash = result.newNode.Hash()
	}
	newTrie := NewTrie(newRootHash, t.db)
	newTrie.log = t.log.mergeFromDeleteResult(t.rootHash, result)
	return newTrie
}

func (t *Trie) delete(startNode node, searchKey []byte) *deleteResult {
	switch n := startNode.(type) {
	case *leafNode:
		return t.deleteFromLeaf(n, searchKey)
	case *extNode:
		return t.deleteFromExt(n, searchKey)
	case *branchNode:
		return t.deleteFromBranch(n, searchKey)
	case *hashNode:
		newNode, err := t.resolveHash(n.Hash())
		if err != nil {
			panic("delete: can't resolve hashNode")
		}
		return t.delete(newNode, searchKey)
	default:
		// this should never happen
		return nil
	}
}

func (t *Trie) deleteFromLeaf(leaf *leafNode, searchKey []byte) *deleteResult {
	if bytes.Equal(searchKey, leaf.key) {
		// delete this leafNode
		result := newDeleteResult(nil, true)
		result.delete(leaf)
		return result
	}
	// key is unmatched, just return
	return newDeleteResult(nil, false)
}

func (t *Trie) deleteFromExt(ext *extNode, searchKey []byte) *deleteResult {
	ml := matchingLength(ext.key, searchKey)
	if ml != len(ext.key) {
		// unmatched extension key, unchanged
		return newDeleteResult(nil, false)
	}
	result := t.delete(ext.child, searchKey[ml:])
	if !result.hasChanged {
		return result
	}
	toFixed := newExtNode(ext.key, result.newNode)
	fixedNode := t.tryFix(toFixed, result.inserted)
	result.newNode = fixedNode
	result.insert(fixedNode)
	result.delete(ext)
	return result
}

func (t *Trie) deleteFromBranch(branch *branchNode, searchKey []byte) *deleteResult {
	if len(searchKey) == 0 && branch.hasTarget() {
		// delete target value of current branch node, and try to fix that
		fixedNode := t.tryFix(branchWithChildren(branch.children), nil)
		result := newDeleteResult(fixedNode, true)
		result.insert(fixedNode)
		result.delete(branch)
		return result
	}
	if len(searchKey) == 0 && !branch.hasTarget() {
		// delete target value, but we have no target value, unchanged
		return newDeleteResult(nil, false)
	}
	childIndex := int(searchKey[0])
	if branch.children[childIndex] == nil {
		// delete from a child which is nil, unchanged
		return newDeleteResult(nil, false)
	}
	// remove from a child of current branch node
	child := branch.children[childIndex]
	result := t.delete(child, searchKey[1:])
	if !result.hasChanged {
		return result
	}
	tempBranch := branch.updateChild(childIndex, result.newNode)
	fixedNode := t.tryFix(tempBranch, result.inserted)
	result.newNode = fixedNode
	result.insert(fixedNode)
	result.delete(branch)
	return result
}

// tryFix try to fix invalid state of a trie, invalid state means:
// - branchNode have only one entry(only have single child or only have target value)
// - extNode have a child which is anything other than a branchNode
func (t *Trie) tryFix(startNode node, updates []node) node {
	switch n := startNode.(type) {
	case *branchNode:
		return t.tryFixBranch(n, updates)
	case *extNode:
		return t.tryFixExt(n, updates)
	default:
		return n
	}
}

// tryFixBranch try to fix a branch node which have only one entry
func (t *Trie) tryFixBranch(branch *branchNode, updates []node) node {
	index := branch.childrenIndex()
	// now we only have target value
	if len(index) == 0 && branch.hasTarget() {
		return newLeafNode(nil, branch.target)
	}
	// now we only have one child
	if len(index) == 1 && !branch.hasTarget() {
		idx := index[0]
		tempExtNode := newExtNode([]byte{byte(idx)}, branch.children[idx])
		return t.tryFix(tempExtNode, updates)
	}
	if len(index) == 0 && !branch.hasTarget() {
		panic("tryFixBranch: invalid branch state, no children and no target")
	}
	// otherwise, the branch have more than one entry, we don't need to fix it
	return branch
}

// tryFixExt try to fix a ext node which child is not a branch node
func (t *Trie) tryFixExt(ext *extNode, updates []node) node {
	var child node
	switch n := ext.child.(type) {
	case *hashNode:
		// first, try to find child from updates
		child = getNodeFrom(updates, n.Hash())
		if child == nil {
			var err error
			child, err = t.resolveHash(n.Hash())
			if err != nil {
				panic("tryFixExt: can't resolve child, maybe because of can't get node from updates")
			}
		}
	default:
		child = n
	}
	switch n := child.(type) {
	case *extNode:
		// the child of current ext node is a ext node, compact to a new extNode
		return newExtNode(concat(ext.key, n.key), n.child)
	case *leafNode:
		// the child of current ext node is a leaf node, compact to a new leafNode
		return newLeafNode(concat(ext.key, n.key), n.value)
	default:
		return ext
	}
}

func (t *Trie) resolveHash(hash common.Hash) (node, error) {
	if _, ok := t.log.deleted[hash]; ok {
		return nil, fmt.Errorf("trie is inconsistent, node has been deleted")
	}
	if inserted, ok := t.log.inserted[hash]; ok {
		return decodeNode(inserted)
	}
	if cached, ok := t.log.cached[hash]; ok {
		return decodeNode(cached)
	}
	return t.fetchFromDB(hash)
}

// fetch node from underlying db, and cache raw data
func (t *Trie) fetchFromDB(hash common.Hash) (node, error) {
	encoded, err := t.db.Get(hash[:])
	if err != nil || len(encoded) == 0 {
		panic("fetchFromDB: get from db failed")
	}
	n, err := decodeNode(encoded)
	if err != nil {
		panic("fetchFromDB: decodeNode failed")
	}
	t.log.cache(hash, encoded)
	return n, nil
}

func (t *Trie) CommitToBatch(batch db.Batch) {
	for k, v := range t.log.inserted {
		batch.Put(k[:], v)
	}
	for k, _ := range t.log.deleted {
		batch.Delete(k[:])
	}
}

// Persist all logs to underlying db
// TODO: it's prune mode currently, what we need is archive mode
// refer to https://blog.ethereum.org/2015/06/26/state-tree-pruning/
func (t *Trie) Persist() {
	batch := t.db.NewBatch()
	t.CommitToBatch(batch)
	batch.Write()
}

// StateRoot return the rootHash of the trie
func (t *Trie) StateRoot() common.Hash {
	return t.rootHash
}

func getNodeFrom(nodes []node, hash common.Hash) node {
	for _, n := range nodes {
		if n.Hash() == hash {
			return n
		}
	}
	return nil
}

// concat concat two byte slice to new one, without change original slice
func concat(a []byte, b []byte) []byte {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	res := make([]byte, len(a)+len(b))
	copy(res, a)
	copy(res[len(a):], b)
	return res
}