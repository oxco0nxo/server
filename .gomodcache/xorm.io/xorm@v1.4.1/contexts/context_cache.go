// Copyright 2018 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package contexts

// ContextCache is the interface that operates the cache data.
type ContextCache interface {
	// Put puts value into cache with key.
	Put(key string, val any)
	// Get gets cached value by given key.
	Get(key string) any
}

type memoryContextCache map[string]any

// NewMemoryContextCache return memoryContextCache
func NewMemoryContextCache() memoryContextCache {
	return make(map[string]any)
}

// Put puts value into cache with key.
func (m memoryContextCache) Put(key string, val any) {
	m[key] = val
}

// Get gets cached value by given key.
func (m memoryContextCache) Get(key string) any {
	return m[key]
}
