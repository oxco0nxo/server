// Copyright 2020 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package caches

import (
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"io"
)

// Md5 return md5 hash string
func Md5(str string) string {
	m := md5.New()
	_, _ = io.WriteString(m, str)
	return hex.EncodeToString(m.Sum(nil))
}

// Encode Encode data
func Encode(data any) ([]byte, error) {
	// return JsonEncode(data)
	return GobEncode(data)
}

// Decode decode data
func Decode(data []byte, to any) error {
	// return JsonDecode(data, to)
	return GobDecode(data, to)
}

// GobEncode encode data with gob
func GobEncode(data any) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(&data)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// GobDecode decode data with gob
func GobDecode(data []byte, to any) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(to)
}

// JsonEncode encode data with json
func JsonEncode(data any) ([]byte, error) {
	val, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return val, nil
}

// JsonDecode decode data with json
func JsonDecode(data []byte, to any) error {
	return json.Unmarshal(data, to)
}
