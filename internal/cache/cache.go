package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Cache interface defines the caching operations
type Cache interface {
	Get(url string) (data []byte, contentType string, found bool, err error)
	Set(url string, data []byte, contentType string) error
	Delete(url string) error
	Close() error
}

// CachedResponse stores the response data and metadata
type CachedResponse struct {
	Data        []byte `json:"data"`
	ContentType string `json:"content_type"`
}

// BadgerCache implements Cache using BadgerDB
type BadgerCache struct {
	db  *badger.DB
	ttl time.Duration
}

// NewBadgerCache creates a new BadgerDB-backed cache
func NewBadgerCache(path string, ttl time.Duration) (*BadgerCache, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil // Disable BadgerDB logging

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	return &BadgerCache{
		db:  db,
		ttl: ttl,
	}, nil
}

// GenerateCacheKey creates a deterministic key from a URL
func GenerateCacheKey(url string) string {
	hash := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hash[:])
}

// Get retrieves a cached response
func (c *BadgerCache) Get(url string) ([]byte, string, bool, error) {
	key := GenerateCacheKey(url)

	var response CachedResponse
	err := c.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &response)
		})
	})

	if err == badger.ErrKeyNotFound {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}

	return response.Data, response.ContentType, true, nil
}

// Set stores a response in the cache with TTL
func (c *BadgerCache) Set(url string, data []byte, contentType string) error {
	key := GenerateCacheKey(url)

	response := CachedResponse{
		Data:        data,
		ContentType: contentType,
	}

	value, err := json.Marshal(response)
	if err != nil {
		return err
	}

	return c.db.Update(func(txn *badger.Txn) error {
		entry := badger.NewEntry([]byte(key), value).WithTTL(c.ttl)
		return txn.SetEntry(entry)
	})
}

// Delete removes a cached response
func (c *BadgerCache) Delete(url string) error {
	key := GenerateCacheKey(url)

	return c.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

// Close closes the database
func (c *BadgerCache) Close() error {
	return c.db.Close()
}
