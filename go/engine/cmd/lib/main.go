package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"unsafe"

	"harbour-freesailwallet/engine/pass"
	"harbour-freesailwallet/engine/wallet"
)

var current *wallet.Store

func ret(v any) *C.char {
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(map[string]string{"error": err.Error()})
	}
	return C.CString(string(b))
}

func fail(err error) *C.char {
	return ret(map[string]string{"error": err.Error()})
}

func store() (*wallet.Store, *C.char) {
	if current == nil {
		return nil, ret(map[string]string{"error": "wallet is not open"})
	}
	return current, nil
}

func bytesArg(data *C.uchar, n C.int) []byte {
	if data == nil || n <= 0 {
		return nil
	}
	return C.GoBytes(unsafe.Pointer(data), n)
}

//export PKENGINE_PassFree
func PKENGINE_PassFree(p *C.char) {
	C.free(unsafe.Pointer(p))
}

//export PKENGINE_PassInspect
func PKENGINE_PassInspect(data *C.uchar, n C.int) *C.char {
	b := bytesArg(data, n)
	if len(b) == 0 {
		return ret(map[string]string{"error": "empty pass"})
	}
	got, err := pass.Inspect(b, pass.Options{})
	if err != nil {
		return fail(err)
	}
	return ret(got)
}

//export PKENGINE_WalletOpen
func PKENGINE_WalletOpen(root *C.char) *C.char {
	st, err := wallet.Open(C.GoString(root))
	if err != nil {
		return fail(err)
	}
	current = st
	return ret(map[string]bool{"ok": true})
}

//export PKENGINE_WalletSetDisplay
func PKENGINE_WalletSetDisplay(locale *C.char, scale C.double) *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	st.SetDisplay(C.GoString(locale), float64(scale))
	return ret(map[string]bool{"ok": true})
}

//export PKENGINE_WalletImport
func PKENGINE_WalletImport(data *C.uchar, n C.int) *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	b := bytesArg(data, n)
	if len(b) == 0 {
		return ret(map[string]string{"error": "empty pass"})
	}
	got, err := st.Import(b)
	if err != nil {
		return fail(err)
	}
	return ret(got)
}

//export PKENGINE_WalletList
func PKENGINE_WalletList() *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	got, err := st.List()
	if err != nil {
		return fail(err)
	}
	return ret(got)
}

//export PKENGINE_WalletGet
func PKENGINE_WalletGet(id *C.char) *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	got, err := st.Get(C.GoString(id))
	if err != nil {
		return fail(err)
	}
	return ret(got)
}

//export PKENGINE_WalletBarcodePNG
func PKENGINE_WalletBarcodePNG(id *C.char, size C.int) *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	got, err := st.BarcodePNG(C.GoString(id), int(size))
	if err != nil {
		return fail(err)
	}
	return ret(got)
}

//export PKENGINE_WalletArchive
func PKENGINE_WalletArchive(id *C.char) *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	if err := st.Archive(C.GoString(id)); err != nil {
		return fail(err)
	}
	return ret(map[string]bool{"ok": true})
}

//export PKENGINE_WalletUnarchive
func PKENGINE_WalletUnarchive(id *C.char) *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	if err := st.Unarchive(C.GoString(id)); err != nil {
		return fail(err)
	}
	return ret(map[string]bool{"ok": true})
}

//export PKENGINE_WalletMove
func PKENGINE_WalletMove(id *C.char, place *C.char) *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	if err := st.Move(C.GoString(id), C.GoString(place)); err != nil {
		return fail(err)
	}
	return ret(map[string]bool{"ok": true})
}

//export PKENGINE_WalletHistory
func PKENGINE_WalletHistory(id *C.char) *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	got, err := st.History(C.GoString(id))
	if err != nil {
		return fail(err)
	}
	return ret(got)
}

//export PKENGINE_WalletGetRevision
func PKENGINE_WalletGetRevision(id *C.char, revision *C.char) *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	got, err := st.GetRevision(C.GoString(id), C.GoString(revision))
	if err != nil {
		return fail(err)
	}
	return ret(got)
}

//export PKENGINE_WalletPoll
func PKENGINE_WalletPoll() *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	got, err := st.Poll()
	if err != nil {
		return fail(err)
	}
	return ret(got)
}

//export PKENGINE_WalletPollPass
func PKENGINE_WalletPollPass(id *C.char) *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	got, err := st.PollPass(C.GoString(id))
	if err != nil {
		return fail(err)
	}
	return ret(got)
}

//export PKENGINE_WalletExport
func PKENGINE_WalletExport() *C.char {
	st, errc := store()
	if errc != nil {
		return errc
	}
	got, err := st.Export()
	if err != nil {
		return fail(err)
	}
	return ret(got)
}

func main() {}
