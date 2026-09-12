//go:build darwin && cgo

package uiact

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <string.h>

static int axTrusted(void) { return AXIsProcessTrusted() ? 1 : 0; }

static AXUIElementRef axApp(pid_t pid) { return AXUIElementCreateApplication(pid); }

static CFArrayRef axCopyList(AXUIElementRef el, int windows) {
    CFTypeRef v = NULL;
    CFStringRef attr = windows ? kAXWindowsAttribute : kAXChildrenAttribute;
    if (AXUIElementCopyAttributeValue(el, attr, &v) == kAXErrorSuccess && v) {
        if (CFGetTypeID(v) == CFArrayGetTypeID()) return (CFArrayRef)v;
        CFRelease(v);
    }
    return NULL;
}

static int axCount(CFArrayRef a) { return a ? (int)CFArrayGetCount(a) : 0; }

static AXUIElementRef axAt(CFArrayRef a, int i) {
    return (AXUIElementRef)CFArrayGetValueAtIndex(a, (CFIndex)i);
}

static void axRelease(CFTypeRef r) { if (r) CFRelease(r); }

static void axText(AXUIElementRef el, int which, char *buf, int len) {
    buf[0] = 0;
    CFStringRef attr = kAXRoleAttribute;
    if (which == 1) attr = kAXTitleAttribute;
    else if (which == 2) attr = kAXDescriptionAttribute;
    else if (which == 3) attr = kAXValueAttribute;

    CFTypeRef v = NULL;
    if (AXUIElementCopyAttributeValue(el, attr, &v) == kAXErrorSuccess && v) {
        if (CFGetTypeID(v) == CFStringGetTypeID())
            CFStringGetCString((CFStringRef)v, buf, len, kCFStringEncodingUTF8);
        CFRelease(v);
    }
}

static int axFlag(AXUIElementRef el, int focused) {
    CFTypeRef v = NULL;
    CFStringRef attr = focused ? kAXFocusedAttribute : kAXEnabledAttribute;
    int out = -1;
    if (AXUIElementCopyAttributeValue(el, attr, &v) == kAXErrorSuccess && v) {
        if (CFGetTypeID(v) == CFBooleanGetTypeID())
            out = CFBooleanGetValue((CFBooleanRef)v) ? 1 : 0;
        CFRelease(v);
    }
    return out;
}

static int axRect(AXUIElementRef el, double *x, double *y, double *w, double *h) {
    CGPoint p = {0,0}; CGSize s = {0,0};
    int got = 0;
    CFTypeRef v = NULL;
    if (AXUIElementCopyAttributeValue(el, kAXPositionAttribute, &v) == kAXErrorSuccess && v) {
        if (AXValueGetValue((AXValueRef)v, kAXValueCGPointType, &p)) got |= 1;
        CFRelease(v);
    }
    v = NULL;
    if (AXUIElementCopyAttributeValue(el, kAXSizeAttribute, &v) == kAXErrorSuccess && v) {
        if (AXValueGetValue((AXValueRef)v, kAXValueCGSizeType, &s)) got |= 2;
        CFRelease(v);
    }
    *x = p.x; *y = p.y; *w = s.width; *h = s.height;
    return got;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func trusted() bool { return C.axTrusted() == 1 }

func text(el C.AXUIElementRef, which C.int) string {
	var buf [256]C.char
	C.axText(el, which, &buf[0], C.int(len(buf)))
	return C.GoString(&buf[0])
}

func rect(el C.AXUIElementRef) ([]int, []int) {
	var x, y, w, h C.double
	if C.axRect(el, &x, &y, &w, &h) != 3 {
		return nil, nil
	}
	return []int{int(x), int(y)}, []int{int(w), int(h)}
}

func walkElement(el C.AXUIElementRef, path []int, depth int, out *[]Element) {
	if len(*out) >= MaxElements || depth > MaxDepth {
		return
	}

	kids := C.axCopyList(el, 0)
	if kids == 0 {
		return
	}
	defer C.axRelease(C.CFTypeRef(kids))

	n := int(C.axCount(kids))
	for i := 0; i < n && len(*out) < MaxElements; i++ {
		child := C.axAt(kids, C.int(i))

		title := text(child, 1)
		if title == "" {
			title = text(child, 2)
		}
		position, size := rect(child)

		here := make([]int, len(path)+1)
		copy(here, path)
		here[len(path)] = i

		*out = append(*out, Element{
			Role:     text(child, 0),
			Title:    title,
			Value:    truncate(text(child, 3), 120),
			Enabled:  C.axFlag(child, 0) != 0,
			Focused:  C.axFlag(child, 1) == 1,
			Position: position,
			Size:     size,
			Path:     here,
		})

		walkElement(child, here, depth+1, out)
	}
}

func walkApp(pid int, app string) (Tree, error) {
	if !trusted() {
		return Tree{}, ErrNoAssistiveAccess
	}

	appEl := C.axApp(C.pid_t(pid))
	if appEl == 0 {
		return Tree{}, fmt.Errorf("uiact: no accessible application with pid %d", pid)
	}
	defer C.axRelease(C.CFTypeRef(appEl))

	windows := C.axCopyList(appEl, 1)
	if windows == 0 || C.axCount(windows) == 0 {
		if windows != 0 {
			C.axRelease(C.CFTypeRef(windows))
		}
		return Tree{}, ErrNoWindow
	}
	defer C.axRelease(C.CFTypeRef(windows))

	win := C.axAt(windows, 0)
	tree := Tree{App: app, Window: text(win, 1)}

	elements := make([]Element, 0, 64)
	walkElement(win, nil, 0, &elements)
	tree.Elements = elements

	return tree, nil
}

var _ = unsafe.Pointer(nil)
