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

static int axElementAt(pid_t want, double x, double y,
                       char *role, int rlen, char *title, int tlen) {
    role[0] = 0; title[0] = 0;
    AXUIElementRef sys = AXUIElementCreateSystemWide();
    if (!sys) return -1;
    AXUIElementRef el = NULL;
    AXError e = AXUIElementCopyElementAtPosition(sys, (float)x, (float)y, &el);
    CFRelease(sys);
    if (e != kAXErrorSuccess || !el) return -2;

    pid_t owner = 0;
    AXUIElementGetPid(el, &owner);
    axText(el, 0, role, rlen);
    axText(el, 1, title, tlen);
    if (!title[0]) axText(el, 2, title, tlen);
    CFRelease(el);
    return (owner == want) ? 0 : 1;
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
static int axDisplayBounds(double *x, double *y, double *w, double *h) {
    uint32_t count = 0;
    if (CGGetActiveDisplayList(0, NULL, &count) != kCGErrorSuccess || count == 0) return 0;
    if (count > 16) count = 16;
    CGDirectDisplayID ids[16];
    if (CGGetActiveDisplayList(count, ids, &count) != kCGErrorSuccess || count == 0) return 0;
    CGRect u = CGDisplayBounds(ids[0]);
    for (uint32_t i = 1; i < count; i++) u = CGRectUnion(u, CGDisplayBounds(ids[i]));
    *x = u.origin.x; *y = u.origin.y; *w = u.size.width; *h = u.size.height;
    return 1;
}
static int cgFrontOwnerAt(double x, double y, int *owner, char *name, int nlen) {
    *owner = -1; name[0] = 0;
    CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID);
    if (!list) return 0;
    int found = 0;
    CFIndex n = CFArrayGetCount(list);
    for (CFIndex i = 0; i < n && !found; i++) {
        CFDictionaryRef d = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
        CFNumberRef v;
        int layer = 0;
        if ((v = (CFNumberRef)CFDictionaryGetValue(d, kCGWindowLayer)))
            CFNumberGetValue(v, kCFNumberIntType, &layer);
        if (layer != 0) continue;
        double alpha = 1.0;
        if ((v = (CFNumberRef)CFDictionaryGetValue(d, kCGWindowAlpha)))
            CFNumberGetValue(v, kCFNumberDoubleType, &alpha);
        if (alpha <= 0.01) continue;
        CFDictionaryRef bd = (CFDictionaryRef)CFDictionaryGetValue(d, kCGWindowBounds);
        if (!bd) continue;
        CGRect r;
        if (!CGRectMakeWithDictionaryRepresentation(bd, &r)) continue;
        if (!CGRectContainsPoint(r, CGPointMake(x, y))) continue;
        int pid = 0;
        if ((v = (CFNumberRef)CFDictionaryGetValue(d, kCGWindowOwnerPID)))
            CFNumberGetValue(v, kCFNumberIntType, &pid);
        CFStringRef on = (CFStringRef)CFDictionaryGetValue(d, kCGWindowOwnerName);
        if (on) CFStringGetCString(on, name, nlen, kCFStringEncodingUTF8);
        *owner = pid;
        found = 1;
    }
    CFRelease(list);
    return found;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func trusted() bool { return C.axTrusted() == 1 }

func Trusted() bool { return trusted() }

func elementAt(pid, x, y int) (role, title string, owned bool, err error) {
	var r, t [256]C.char
	rc := C.axElementAt(C.pid_t(pid), C.double(x), C.double(y),
		&r[0], C.int(len(r)), &t[0], C.int(len(t)))
	switch rc {
	case -1:
		return "", "", false, fmt.Errorf("uiact: the accessibility API is not answering")
	case -2:
		return "", "", false, fmt.Errorf("uiact: nothing is on screen at (%d,%d)", x, y)
	}
	return C.GoString(&r[0]), C.GoString(&t[0]), rc == 0, nil
}

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

func frontWindowAt(x, y int) (pid int, name string, ok bool) {
	var owner C.int
	var buf [256]C.char
	if C.cgFrontOwnerAt(C.double(x), C.double(y), &owner, &buf[0], C.int(len(buf))) != 1 {
		return 0, "", false
	}
	return int(owner), C.GoString(&buf[0]), true
}

func displayBounds() (box, bool) {
	var x, y, w, h C.double
	if C.axDisplayBounds(&x, &y, &w, &h) != 1 {
		return box{}, false
	}
	return box{int(x), int(y), int(x + w), int(y + h)}, true
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
