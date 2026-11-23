package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"unsafe"

	"harbour-freesailwallet/sail"
)

//export WalletCall
func WalletCall(method, args *C.char) *C.char {
	return C.CString(sail.Call(C.GoString(method), C.GoString(args)))
}

//export WalletFree
func WalletFree(p unsafe.Pointer) {
	C.free(p)
}

//export WalletImage
func WalletImage(key, name *C.char, w, h C.int, outLen *C.int) unsafe.Pointer {
	b := sail.ImagePNG(C.GoString(key), C.GoString(name), int(w), int(h))
	if len(b) == 0 {
		*outLen = 0
		return nil
	}
	*outLen = C.int(len(b))
	return C.CBytes(b)
}

func main() {}
