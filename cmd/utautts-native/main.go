package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"runtime/cgo"
	"sync"
	"unsafe"

	"utautts/internal/native"
)

type response struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// lastCreateErrorは直近のUtauTTSCreate失敗を保持し、
// 呼び出し側がハンドルなしでUtauTTSLastErrorを通じて取得できるようにする
// Qtバックエンドが唯一の利用者で、initialize()中にメインスレッド上で
// Createに続けてLastErrorを同期的に呼ぶから単一スロットで十分
// 成功したcreateは意図的にスロットをクリアしない: 並行するcreateが、
// 他スレッドが読もうとしている失敗を消してはならない。
var lastCreateError struct {
	sync.Mutex
	message string
}

//export UtauTTSCreate
func UtauTTSCreate(configJSON *C.char) C.uintptr_t {
	var data []byte
	if configJSON != nil {
		data = []byte(C.GoString(configJSON))
	}
	engine, err := native.NewJSON(data)
	if err != nil {
		lastCreateError.Lock()
		lastCreateError.message = err.Error()
		lastCreateError.Unlock()
		return 0
	}
	return C.uintptr_t(cgo.NewHandle(engine))
}

//export UtauTTSLastError
func UtauTTSLastError() *C.char {
	lastCreateError.Lock()
	defer lastCreateError.Unlock()
	return C.CString(lastCreateError.message)
}

//export UtauTTSCall
func UtauTTSCall(rawHandle C.uintptr_t, method, requestJSON *C.char) *C.char {
	answer := response{}
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				answer.Error = "invalid native engine handle"
			}
		}()
		engine, ok := cgo.Handle(rawHandle).Value().(*native.Engine)
		if !ok {
			answer.Error = "invalid native engine"
			return
		}
		var request []byte
		if requestJSON != nil {
			request = []byte(C.GoString(requestJSON))
		}
		result, err := engine.Call(C.GoString(method), request)
		if err != nil {
			answer.Error = err.Error()
			return
		}
		answer.OK = true
		answer.Result = result
	}()
	data, _ := json.Marshal(answer)
	return C.CString(string(data))
}

//export UtauTTSDestroy
func UtauTTSDestroy(rawHandle C.uintptr_t) {
	defer func() { _ = recover() }()
	cgo.Handle(rawHandle).Delete()
}

//export UtauTTSFree
func UtauTTSFree(value *C.char) { C.free(unsafe.Pointer(value)) }

func main() {}
