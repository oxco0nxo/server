// Copyright 2019 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package json

import "encoding/json"

// Interface represents an interface to handle json data
type Interface interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// DefaultJSONHandler default json handler
var DefaultJSONHandler Interface = StdJSON{}

// StdJSON implements JSONInterface via encoding/json
type StdJSON struct{}

// Marshal implements JSONInterface
func (StdJSON) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal implements JSONInterface
func (StdJSON) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
