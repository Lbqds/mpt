## mpt

the implementation of [merkle patricia trie](https://github.com/ethereum/wiki/wiki/Patricia-Tree)

## what's the difference

the differences with the implementation of go-ethereum:

* use protobuf rather than RLP.
* immutable, every update(insert or delete) will get a new trie. This make it easier to implement 
  transaction parallel execution like [khipu](https://github.com/khipu-io/khipu).
