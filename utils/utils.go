package utils

import (
	"reflect"
	"unsafe"
)

// byte to string without allocation. Use with care and make sure not to modify
// the byte buffer while using the resulting string
func ByteToStr(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// String to byte without allocation. Use with care and make sure not to modify
// the provided string while using the resulting byte slice
func StrToByte(s string) (b []byte) {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	bh.Data = sh.Data
	bh.Cap = sh.Len
	bh.Len = sh.Len
	return b
}
