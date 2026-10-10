//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"runtime"
	"unsafe"
)

func main() {
	fmt.Printf("// %s", runtime.Version())
	fmt.Printf("\n{%q, %q}: {", runtime.GOOS, runtime.GOARCH)
	{
		type t = bool
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tBool: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = chan int
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tChan: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = complex128
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tComplex128: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = complex64
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tComplex64: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = float32
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tFloat32: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = float64
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tFloat64: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = func()
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tFunction: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = int
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tInt: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = int16
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tInt16: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = int32
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tInt32: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = int64
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tInt64: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = int8
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tInt8: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = interface{}
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tInterface: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = map[int]int
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tMap: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = *byte
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tPointer: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = []int
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tSlice: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = string
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tString: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = uint
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tUint: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = uint16
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tUint16: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = uint32
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tUint32: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = uint64
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tUint64: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = uint8
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tUint8: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = uintptr
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tUintptr: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	{
		type t = unsafe.Pointer
		var v t
		var s struct {
			_ byte
			v t
		}
		fmt.Printf("\n\tUnsafePointer: {%v, %v, %v},", unsafe.Sizeof(v), unsafe.Alignof(v), unsafe.Offsetof(s.v))
	}
	fmt.Printf("\n},\n")
}
