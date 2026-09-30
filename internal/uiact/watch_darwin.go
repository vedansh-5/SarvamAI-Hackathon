//go:build darwin && cgo

package uiact

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <stdlib.h>

extern void vansWindowChanged(AXObserverRef observer, AXUIElementRef element, CFStringRef notification, void *refcon);
*/
import "C"

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

var windowNotifications = []string{
	"AXFocusedUIElementChanged",
	"AXFocusedWindowChanged",
	"AXValueChanged",
	"AXUIElementDestroyed",
	"AXCreated",
	"AXLayoutChanged",
	"AXWindowCreated",
	"AXSelectedChildrenChanged",
	"AXSelectedRowsChanged",
	"AXRowCountChanged",
	"AXTitleChanged",
	"AXMenuOpened",
	"AXMenuClosed",
	"AXSelectedTextChanged",
}

var watching sync.Map

//export vansWindowChanged
func vansWindowChanged(observer C.AXObserverRef, element C.AXUIElementRef, notification C.CFStringRef, refcon unsafe.Pointer) {
	ch, ok := watching.Load(uintptr(observer))
	if !ok {
		return
	}
	select {
	case ch.(chan struct{}) <- struct{}{}:
	default:
	}
}

func cfString(s string) C.CFStringRef {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	return C.CFStringCreateWithCString(C.kCFAllocatorDefault, cs, C.kCFStringEncodingUTF8)
}

func watchWindow(ctx context.Context, pid int) (<-chan struct{}, error) {
	changed := make(chan struct{}, 1)
	ready := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		var observer C.AXObserverRef
		if rc := C.AXObserverCreate(C.pid_t(pid), C.AXObserverCallback(C.vansWindowChanged), &observer); rc != C.kAXErrorSuccess || observer == 0 {
			ready <- fmt.Errorf("uiact: cannot watch pid %d (AXError %d)", pid, int(rc))
			return
		}
		defer C.CFRelease(C.CFTypeRef(observer))

		app := C.AXUIElementCreateApplication(C.pid_t(pid))
		defer C.CFRelease(C.CFTypeRef(app))

		var names []C.CFStringRef
		for _, n := range windowNotifications {
			name := cfString(n)
			if C.AXObserverAddNotification(observer, app, name, nil) == C.kAXErrorSuccess {
				names = append(names, name)
				continue
			}
			C.CFRelease(C.CFTypeRef(name))
		}
		defer func() {
			for _, name := range names {
				C.AXObserverRemoveNotification(observer, app, name)
				C.CFRelease(C.CFTypeRef(name))
			}
		}()
		if len(names) == 0 {
			ready <- fmt.Errorf("uiact: pid %d accepts no change notifications", pid)
			return
		}

		key := uintptr(observer)
		watching.Store(key, changed)
		defer watching.Delete(key)

		loop := C.CFRunLoopGetCurrent()
		source := C.AXObserverGetRunLoopSource(observer)
		C.CFRunLoopAddSource(loop, source, C.kCFRunLoopDefaultMode)
		defer C.CFRunLoopRemoveSource(loop, source, C.kCFRunLoopDefaultMode)

		ready <- nil
		for ctx.Err() == nil {
			C.CFRunLoopRunInMode(C.kCFRunLoopDefaultMode, 0.2, 0)
		}
	}()

	if err := <-ready; err != nil {
		return nil, err
	}
	return changed, nil
}
