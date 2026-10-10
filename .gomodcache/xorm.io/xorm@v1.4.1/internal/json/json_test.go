// Copyright 2026 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package json

import "testing"

type samplePayload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestStdJSONMarshalUnmarshal(t *testing.T) {
	input := samplePayload{Name: "demo", Age: 18}
	handler := StdJSON{}

	data, err := handler.Marshal(input)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var output samplePayload
	if err := handler.Unmarshal(data, &output); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if output != input {
		t.Fatalf("expected %+v, got %+v", input, output)
	}
}

func TestDefaultJSONHandler(t *testing.T) {
	data, err := DefaultJSONHandler.Marshal(map[string]int{"value": 1})
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var output map[string]int
	if err := DefaultJSONHandler.Unmarshal(data, &output); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if output["value"] != 1 {
		t.Fatalf("expected value=1, got %v", output["value"])
	}
}
