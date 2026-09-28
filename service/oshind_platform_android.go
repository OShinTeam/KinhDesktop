//go:build android

package service

/*
#include <dlfcn.h>
#include <stdlib.h>

// OShinD 组件在 Android 上随 APK 的 jniLibs 打包，由系统解出到应用私有库目录，
// 因此这里只需库名（不带路径）即可打开。注意不能改成"运行时释放到可写目录再 dlopen"——
// Android 7 起 SELinux 与 W^X 会拒绝从可写目录加载可执行代码。
static void* oshindDlopen(const char* name) {
	return dlopen(name, RTLD_NOW | RTLD_LOCAL);
}

static void* oshindDlsym(void* handle, const char* name) {
	return dlsym(handle, name);
}

static const char* oshindDlerror(void) {
	const char* e = dlerror();
	return e ? e : "";
}

// OShinD 导出函数只有以下几种固定签名，按签名逐个包装，
// 避免在 Go 侧对不透明函数指针做类型转换。
typedef char* (*oshindFn0)(void);
typedef char* (*oshindFn1)(char*);
typedef char* (*oshindFn2)(char*, char*);
typedef int   (*oshindFn1Int)(char*);
typedef void  (*oshindFn1Void)(char*);

static char* oshindCall0(void* f)                   { return ((oshindFn0)f)(); }
static char* oshindCall1(void* f, char* a)          { return ((oshindFn1)f)(a); }
static char* oshindCall2(void* f, char* a, char* b) { return ((oshindFn2)f)(a, b); }
static int   oshindCall1Int(void* f, char* a)       { return ((oshindFn1Int)f)(a); }
static void  oshindCall1Void(void* f, char* a)      { ((oshindFn1Void)f)(a); }
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// 与 Windows 版（oshind_platform_windows.go）保持同名同形状，
// 使 oshind.go / download.go 无需任何平台分支。

type oshindLibHandle struct {
	handle unsafe.Pointer
}

type oshindProc struct {
	fn unsafe.Pointer
	// 返回类型按符号名区分：CancelTask / RemoveTask 返回 int，
	// FreeString 无返回值，其余返回 char*
	returnsInt  bool
	returnsVoid bool
}

func oshindOpenLib(name string) (*oshindLibHandle, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	h := C.oshindDlopen(cName)
	if h == nil {
		return nil, fmt.Errorf("dlopen %s 失败: %s", name, C.GoString(C.oshindDlerror()))
	}
	return &oshindLibHandle{handle: h}, nil
}

func (l *oshindLibHandle) FindProc(name string) (*oshindProc, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	fn := C.oshindDlsym(l.handle, cName)
	if fn == nil {
		return nil, fmt.Errorf("缺少符号 %s: %s", name, C.GoString(C.oshindDlerror()))
	}
	return &oshindProc{
		fn:          fn,
		returnsInt:  name == "OShinD_CancelTask" || name == "OShinD_RemoveTask",
		returnsVoid: name == "OShinD_FreeString",
	}, nil
}

// Call 与 Windows 版 *syscall.Proc.Call 同签名，供 oshind.go / download.go 共用。
//
// 入参由 strPtr 生成，指向 Go 内存中 NUL 结尾的字节串。这里先把字符串读回 Go、
// 再复制成 C 字符串后才交给组件，避免把 Go 内存指针直接递过 cgo 边界。
//
// 例外是 OShinD_FreeString：它的入参本就是组件返回的 C 指针，必须原样透传，
// 若也走一遍复制就会去释放副本、导致组件侧的原始内存泄漏。
func (p *oshindProc) Call(args ...uintptr) (uintptr, uintptr, error) {
	switch {
	case p.returnsVoid:
		// OShinD_FreeString(char*)
		C.oshindCall1Void(p.fn, (*C.char)(unsafe.Pointer(args[0])))
		return 0, 0, nil

	case len(args) == 0:
		// OShinD_Version()
		ret := C.oshindCall0(p.fn)
		return uintptr(unsafe.Pointer(ret)), 0, nil

	case len(args) == 2:
		// OShinD_Download(url, optionsJson)
		a, freeA := copyGoStringToC(args[0])
		defer freeA()
		b, freeB := copyGoStringToC(args[1])
		defer freeB()
		ret := C.oshindCall2(p.fn, a, b)
		return uintptr(unsafe.Pointer(ret)), 0, nil

	case p.returnsInt:
		// OShinD_CancelTask(taskID) / OShinD_RemoveTask(taskID)
		a, freeA := copyGoStringToC(args[0])
		defer freeA()
		return uintptr(C.oshindCall1Int(p.fn, a)), 0, nil

	default:
		// OShinD_GetTaskStatus / PauseTask / ResumeTask(taskID)
		a, freeA := copyGoStringToC(args[0])
		defer freeA()
		ret := C.oshindCall1(p.fn, a)
		return uintptr(unsafe.Pointer(ret)), 0, nil
	}
}

// copyGoStringToC 把 Go 侧字符串指针复制成独立的 C 内存，返回释放函数。
// 指针来源固定为 strPtr（见 download.go），此处只做读取与复制，不做所有权转移。
func copyGoStringToC(p uintptr) (*C.char, func()) {
	s := C.GoString((*C.char)(unsafe.Pointer(p)))
	c := C.CString(s)
	return c, func() { C.free(unsafe.Pointer(c)) }
}
