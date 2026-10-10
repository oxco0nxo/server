// Copyright 2022 The CCGO Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !ccgo.dmesg
// +build !ccgo.dmesg

package ccgo // import "modernc.org/ccgo/v4/lib"

//lint:ignore U1000 debug support
const dmesgs = false

//lint:ignore U1000 debug support
func dmesg(s string, args ...interface{}) {}
