package utils

import "unsafe"

// byte to string without allocation. Use with care and make sure to not modify
// the byte buffer while using the resulting string
func ByteToStr(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
