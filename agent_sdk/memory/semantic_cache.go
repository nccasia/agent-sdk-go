package memory

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// SemanticCache is a bounded LRU of cached answers keyed by
// (workspace, acl_cohort, embedding_model, query_embedding).
type SemanticCache struct {
	maxsize int
	ll      *list.List
	items   map[string]*list.Element
}

type cacheEntry struct {
	key   string
	value string
}

// NewSemanticCache builds an LRU cache (default maxsize 512).
func NewSemanticCache(maxsize int) *SemanticCache {
	if maxsize <= 0 {
		maxsize = 512
	}
	return &SemanticCache{maxsize: maxsize, ll: list.New(), items: map[string]*list.Element{}}
}

func makeKey(workspaceID, aclCohortHash, embeddingModelID string, queryEmbedding []byte) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%s", workspaceID, aclCohortHash, embeddingModelID, string(queryEmbedding))))
	return hex.EncodeToString(h[:])
}

// Get returns the cached answer and whether it was present.
func (c *SemanticCache) Get(workspaceID, aclCohortHash, embeddingModelID string, queryEmbedding []byte) (string, bool) {
	key := makeKey(workspaceID, aclCohortHash, embeddingModelID, queryEmbedding)
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*cacheEntry).value, true
	}
	return "", false
}

// Set stores value, evicting the least-recently-used entry past maxsize.
func (c *SemanticCache) Set(workspaceID, aclCohortHash, embeddingModelID string, queryEmbedding []byte, value string) {
	key := makeKey(workspaceID, aclCohortHash, embeddingModelID, queryEmbedding)
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		el.Value.(*cacheEntry).value = value
		return
	}
	el := c.ll.PushFront(&cacheEntry{key: key, value: value})
	c.items[key] = el
	for c.ll.Len() > c.maxsize {
		back := c.ll.Back()
		if back == nil {
			break
		}
		c.ll.Remove(back)
		delete(c.items, back.Value.(*cacheEntry).key)
	}
}
