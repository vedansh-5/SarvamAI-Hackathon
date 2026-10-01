//go:build darwin && cgo

package uiact

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <stdlib.h>
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

// Every application holding an ordinary window, newline separated.
//
// kCGWindowListOptionAll rather than OnScreenOnly, which means literally
// displayed: a window on another Space, or behind a full-screen application, is
// not on screen and its application is still open. The question here is what a
// person could switch to, so the narrower list answered it wrongly — measured on
// 2026-09-23, OnScreenOnly reported two applications on a Mac running eight.
//
// The window list needs no permission, so this costs nothing and never prompts.
static int cgAppsOnScreen(char *out, int len) {
    out[0] = 0;
    CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionAll, kCGNullWindowID);
    if (!list) return 0;
    int used = 0;
    CFIndex n = CFArrayGetCount(list);
    for (CFIndex i = 0; i < n; i++) {
        CFDictionaryRef d = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
        CFNumberRef v;
        int layer = 0;
        if ((v = (CFNumberRef)CFDictionaryGetValue(d, kCGWindowLayer)))
            CFNumberGetValue(v, kCFNumberIntType, &layer);
        if (layer != 0) continue;
        CFStringRef on = (CFStringRef)CFDictionaryGetValue(d, kCGWindowOwnerName);
        if (!on) continue;
        char name[256];
        if (!CFStringGetCString(on, name, sizeof(name), kCFStringEncodingUTF8)) continue;
        if (!name[0]) continue;
        int need = (int)strlen(name) + 1;
        if (used + need >= len) break;
        if (used > 0) out[used++] = '\n';
        memcpy(out + used, name, strlen(name));
        used += (int)strlen(name);
        out[used] = 0;
    }
    CFRelease(list);
    return used > 0;
}

static int cgFrontmostApp(char *name, int len) {
    name[0] = 0;
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
        CFStringRef on = (CFStringRef)CFDictionaryGetValue(d, kCGWindowOwnerName);
        if (on && CFStringGetCString(on, name, len, kCFStringEncodingUTF8) && name[0]) found = 1;
    }
    CFRelease(list);
    return found;
}

static int cgPidForOwner(const char *want, int *pid) {
    *pid = 0;
    CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID);
    if (!list) return 0;
    int found = 0;
    char name[256];
    CFIndex n = CFArrayGetCount(list);
    for (CFIndex i = 0; i < n && !found; i++) {
        CFDictionaryRef d = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
        CFStringRef on = (CFStringRef)CFDictionaryGetValue(d, kCGWindowOwnerName);
        if (!on) continue;
        if (!CFStringGetCString(on, name, sizeof(name), kCFStringEncodingUTF8)) continue;
        if (strcasecmp(name, want) != 0) continue;
        CFNumberRef v = (CFNumberRef)CFDictionaryGetValue(d, kCGWindowOwnerPID);
        if (!v) continue;
        CFNumberGetValue(v, kCFNumberIntType, pid);
        found = *pid > 0;
    }
    CFRelease(list);
    return found;
}

// Walks down from the application's first window by the index chain a captured
// element carries.
static AXUIElementRef axMenuBar(AXUIElementRef app) {
    CFTypeRef v = NULL;
    if (AXUIElementCopyAttributeValue(app, kAXMenuBarAttribute, &v) != kAXErrorSuccess || !v) return NULL;
    return (AXUIElementRef)v;
}

static AXUIElementRef axAtPath(int pid, const int *path, int n) {
    AXUIElementRef app = AXUIElementCreateApplication((pid_t)pid);
    if (!app) return NULL;

    if (n > 0 && path[0] < 0) {
        AXUIElementRef el = axMenuBar(app);
        CFRelease(app);
        if (!el) return NULL;
        for (int i = 1; i < n; i++) {
            CFArrayRef kids = axCopyList(el, 0);
            if (!kids || path[i] < 0 || path[i] >= CFArrayGetCount(kids)) {
                if (kids) CFRelease(kids);
                CFRelease(el);
                return NULL;
            }
            AXUIElementRef next = (AXUIElementRef)CFRetain(axAt(kids, path[i]));
            CFRelease(kids);
            CFRelease(el);
            el = next;
        }
        return el;
    }

    CFArrayRef windows = axCopyList(app, 1);
    CFRelease(app);
    if (!windows || CFArrayGetCount(windows) == 0) {
        if (windows) CFRelease(windows);
        return NULL;
    }

    AXUIElementRef el = (AXUIElementRef)CFRetain(axAt(windows, 0));
    CFRelease(windows);

    for (int i = 0; i < n; i++) {
        CFArrayRef kids = axCopyList(el, 0);
        if (!kids || path[i] < 0 || path[i] >= CFArrayGetCount(kids)) {
            if (kids) CFRelease(kids);
            CFRelease(el);
            return NULL;
        }
        AXUIElementRef next = (AXUIElementRef)CFRetain(axAt(kids, path[i]));
        CFRelease(kids);
        CFRelease(el);
        el = next;
    }
    return el;
}

static int axPublishesPress(AXUIElementRef el) {
    CFArrayRef names = NULL;
    if (AXUIElementCopyActionNames(el, &names) != kAXErrorSuccess || !names) return 0;

    int found = 0;
    CFIndex n = CFArrayGetCount(names);
    for (CFIndex i = 0; i < n && !found; i++) {
        CFStringRef s = (CFStringRef)CFArrayGetValueAtIndex(names, i);
        if (s && CFStringCompare(s, kAXPressAction, 0) == kCFCompareEqualTo) found = 1;
    }
    CFRelease(names);
    return found;
}

// 1 pressed, 0 the control publishes no press, negative an accessibility error.
static int axPress(int pid, const int *path, int n) {
    AXUIElementRef el = axAtPath(pid, path, n);
    if (!el) return -1;

    if (!axPublishesPress(el)) {
        CFRelease(el);
        return 0;
    }

    AXError e = AXUIElementPerformAction(el, kAXPressAction);
    CFRelease(el);
    return e == kAXErrorSuccess ? 1 : -2;
}

// Gives a control the keyboard without clicking it.
static int axFocus(int pid, const int *path, int n) {
    AXUIElementRef el = axAtPath(pid, path, n);
    if (!el) return -1;

    AXError e = AXUIElementSetAttributeValue(el, kAXFocusedAttribute, kCFBooleanTrue);
    CFRelease(el);
    return e == kAXErrorSuccess ? 1 : -2;
}

static int axScrollToVisible(int pid, const int *path, int n) {
    AXUIElementRef el = axAtPath(pid, path, n);
    if (!el) return -1;

    AXError e = AXUIElementPerformAction(el, CFSTR("AXScrollToVisible"));
    CFRelease(el);
    return e == kAXErrorSuccess ? 1 : -2;
}

static int axValueAt(int pid, const int *path, int n, char *buf, int len) {
    buf[0] = 0;
    AXUIElementRef el = axAtPath(pid, path, n);
    if (!el) return -1;
    CFTypeRef v = NULL;
    int ok = 0;
    if (AXUIElementCopyAttributeValue(el, kAXValueAttribute, &v) == kAXErrorSuccess && v) {
        if (CFGetTypeID(v) == CFStringGetTypeID()) {
            CFStringGetCString((CFStringRef)v, buf, len, kCFStringEncodingUTF8);
            ok = 1;
        }
        CFRelease(v);
    }
    CFRelease(el);
    return ok;
}

static int cgWindowOf(int pid, int *wid, double *x, double *y, double *w, double *h) {
    CFArrayRef list = CGWindowListCopyWindowInfo(
        kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
    if (!list) return 0;
    int found = 0;
    CFIndex n = CFArrayGetCount(list);
    for (CFIndex i = 0; i < n && !found; i++) {
        CFDictionaryRef d = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
        int owner = 0, layer = -1, number = 0;
        CFNumberRef v = (CFNumberRef)CFDictionaryGetValue(d, kCGWindowOwnerPID);
        if (v) CFNumberGetValue(v, kCFNumberIntType, &owner);
        if (owner != pid) continue;
        v = (CFNumberRef)CFDictionaryGetValue(d, kCGWindowLayer);
        if (v) CFNumberGetValue(v, kCFNumberIntType, &layer);
        if (layer != 0) continue;
        CGRect r;
        CFDictionaryRef b = (CFDictionaryRef)CFDictionaryGetValue(d, kCGWindowBounds);
        if (!b || !CGRectMakeWithDictionaryRepresentation(b, &r)) continue;
        if (r.size.width < 120 || r.size.height < 80) continue;
        v = (CFNumberRef)CFDictionaryGetValue(d, kCGWindowNumber);
        if (v) CFNumberGetValue(v, kCFNumberIntType, &number);
        if (number <= 0) continue;
        *wid = number; *x = r.origin.x; *y = r.origin.y; *w = r.size.width; *h = r.size.height;
        found = 1;
    }
    CFRelease(list);
    return found;
}

static int axSelectedText(int pid, char *buf, int len) {
    buf[0] = 0;
    AXUIElementRef app = AXUIElementCreateApplication((pid_t)pid);
    if (!app) return 0;
    CFTypeRef focused = NULL;
    int ok = 0;
    if (AXUIElementCopyAttributeValue(app, kAXFocusedUIElementAttribute, &focused) == kAXErrorSuccess && focused) {
        CFTypeRef v = NULL;
        if (AXUIElementCopyAttributeValue((AXUIElementRef)focused, kAXSelectedTextAttribute, &v) == kAXErrorSuccess && v) {
            if (CFGetTypeID(v) == CFStringGetTypeID()) {
                CFStringGetCString((CFStringRef)v, buf, len, kCFStringEncodingUTF8);
                ok = buf[0] != 0;
            }
            CFRelease(v);
        }
        CFRelease(focused);
    }
    CFRelease(app);
    return ok;
}

static int axWake(int pid) {
    AXUIElementRef app = AXUIElementCreateApplication((pid_t)pid);
    if (!app) return -1;
    AXError e = AXUIElementSetAttributeValue(app, CFSTR("AXManualAccessibility"), kCFBooleanTrue);
    CFRelease(app);
    return (int)e;
}

static int axRaise(int pid) {
    AXUIElementRef app = AXUIElementCreateApplication((pid_t)pid);
    if (!app) return -1;
    AXError e = AXUIElementSetAttributeValue(app, kAXFrontmostAttribute, kCFBooleanTrue);
    CFRelease(app);
    return (int)e;
}*/
import "C"

import (
	"fmt"
	"github.com/vedansh/vans/internal/names"
	"sort"
	"strings"
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
	return C.GoString(&r[0]), names.Strip(C.GoString(&t[0])), rc == 0, nil
}

func text(el C.AXUIElementRef, which C.int) string {
	var buf [256]C.char
	C.axText(el, which, &buf[0], C.int(len(buf)))
	return names.Strip(C.GoString(&buf[0]))
}

func rect(el C.AXUIElementRef) ([]int, []int) {
	var x, y, w, h C.double
	if C.axRect(el, &x, &y, &w, &h) != 3 {
		return nil, nil
	}
	return []int{int(x), int(y)}, []int{int(w), int(h)}
}

// pressElement presses a control through accessibility, and says whether it
// could.
//
// False and no error means the control publishes no press — a plain view, a cell
// in some tables — and the caller moves the mouse instead. That is asked before
// pressing rather than inferred from a failure, because AXPress returning success
// on a control that ignores it would leave a step that never happened looking like
// one that did.
func pressElement(pid int, path []int) (bool, error) {
	if !trusted() {
		return false, ErrNoAssistiveAccess
	}

	indices := cPath(path)
	switch rc := int(C.axPress(C.int(pid), &indices[0], C.int(len(path)))); rc {
	case 1:
		return true, nil
	case 0:
		return false, nil
	case -1:
		return false, ErrNoWindow
	default:
		return false, fmt.Errorf("uiact: the control refused to be pressed (accessibility error %d)", rc)
	}
}

// focusElement gives a control the keyboard without clicking it.
func focusElement(pid int, path []int) error {
	if !trusted() {
		return ErrNoAssistiveAccess
	}

	indices := cPath(path)
	switch rc := int(C.axFocus(C.int(pid), &indices[0], C.int(len(path)))); rc {
	case 1:
		return nil
	case -1:
		return ErrNoWindow
	default:
		return fmt.Errorf("uiact: the control would not take the keyboard (accessibility error %d)", rc)
	}
}

func elementValue(pid int, path []int) (string, bool) {
	if !trusted() {
		return "", false
	}
	indices := cPath(path)
	buf := make([]C.char, 16384)
	if C.axValueAt(C.int(pid), &indices[0], C.int(len(path)), &buf[0], C.int(len(buf))) != 1 {
		return "", false
	}
	return names.Strip(C.GoString(&buf[0])), true
}

func selectedText(pid int) (string, bool) {
	if !trusted() {
		return "", false
	}
	buf := make([]C.char, 8192)
	if C.axSelectedText(C.int(pid), &buf[0], C.int(len(buf))) != 1 {
		return "", false
	}
	return names.Strip(C.GoString(&buf[0])), true
}

func wakeApp(pid int) bool {
	return trusted() && int(C.axWake(C.int(pid))) == 0
}

func windowOf(pid int) (id, x, y, w, h int, ok bool) {
	var wid C.int
	var cx, cy, cw, ch C.double
	if C.cgWindowOf(C.int(pid), &wid, &cx, &cy, &cw, &ch) != 1 {
		return 0, 0, 0, 0, 0, false
	}
	return int(wid), int(cx), int(cy), int(cw), int(ch), true
}

func scrollElement(pid int, path []int) error {
	if !trusted() {
		return ErrNoAssistiveAccess
	}

	indices := cPath(path)
	switch rc := int(C.axScrollToVisible(C.int(pid), &indices[0], C.int(len(path)))); rc {
	case 1:
		return nil
	case -1:
		return ErrNoWindow
	default:
		return fmt.Errorf("uiact: the control could not be scrolled into view (accessibility error %d)", rc)
	}
}

// One longer than the path, so &indices[0] is an address even for a path that
// names the window itself.
func cPath(path []int) []C.int {
	indices := make([]C.int, len(path)+1)
	for i, step := range path {
		indices[i] = C.int(step)
	}
	return indices
}

func raiseApp(pid int) error {
	if rc := int(C.axRaise(C.int(pid))); rc != 0 {
		return fmt.Errorf("uiact: could not raise the application (accessibility error %d)", rc)
	}
	return nil
}

// The pid of the application publishing a window under this name.
//
// The window list knows every application with something on screen and needs no
// permission VANS does not already hold. System Events knows it too and costs an
// Automation grant, which is why this is asked first.
func pidByWindowList(app string) (int, bool) {
	for _, owner := range ownersOnScreen() {
		if owner == "" || !names.Same(owner, app) {
			continue
		}
		name := C.CString(owner)
		var pid C.int
		found := C.cgPidForOwner(name, &pid) == 1
		C.free(unsafe.Pointer(name))
		if found {
			return int(pid), true
		}
	}
	return 0, false
}

// appsOnScreen is every application showing a window, deduplicated. One
// application usually owns several windows, and the answer wanted here is which
// applications a person could switch to.
func ownersOnScreen() []string {
	var buf [8192]C.char
	if C.cgAppsOnScreen(&buf[0], C.int(len(buf))) != 1 {
		return nil
	}
	return strings.Split(C.GoString(&buf[0]), "\n")
}

func appsOnScreen() []string {
	seen := map[string]bool{}
	var out []string
	for _, name := range ownersOnScreen() {
		name = names.Clean(name)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func frontmostApp() (string, bool) {
	var buf [256]C.char
	if C.cgFrontmostApp(&buf[0], C.int(len(buf))) != 1 {
		return "", false
	}
	return names.Clean(C.GoString(&buf[0])), true
}

func windowOnScreen(pid int) bool {
	app := C.axApp(C.pid_t(pid))
	if app == 0 {
		return false
	}
	defer C.axRelease(C.CFTypeRef(app))

	windows := C.axCopyList(app, 1)
	if windows == 0 {
		return false
	}
	defer C.axRelease(C.CFTypeRef(windows))

	return C.axCount(windows) > 0
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

const (
	MenuBarRoot  = -1
	mostMenuRows = 60
)

func walkMenuBar(app C.AXUIElementRef, out *[]Element) {
	bar := C.axMenuBar(app)
	if bar == 0 {
		return
	}
	defer C.axRelease(C.CFTypeRef(bar))

	tops := C.axCopyList(bar, 0)
	if tops == 0 {
		return
	}
	defer C.axRelease(C.CFTypeRef(tops))

	for i := 0; i < int(C.axCount(tops)); i++ {
		top := C.axAt(tops, C.int(i))
		title := text(top, 1)
		position, size := rect(top)
		if title == "" || size == nil {
			continue
		}
		*out = append(*out, Element{
			Role: "AXMenuBarItem", Title: title, Enabled: C.axFlag(top, 0) != 0,
			Position: position, Size: size, Path: []int{MenuBarRoot, i},
		})

		menus := C.axCopyList(top, 0)
		if menus == 0 {
			continue
		}
		if C.axCount(menus) > 0 {
			rows := C.axCopyList(C.axAt(menus, 0), 0)
			if rows != 0 {
				var open []Element
				for j := 0; j < int(C.axCount(rows)) && j < mostMenuRows; j++ {
					row := C.axAt(rows, C.int(j))
					name := text(row, 1)
					p, s := rect(row)
					if name == "" || s == nil || s[0] <= 0 || s[1] <= 0 {
						continue
					}
					open = append(open, Element{
						Role: "AXMenuItem", Title: name, Enabled: C.axFlag(row, 0) != 0,
						Position: p, Size: s, Path: []int{MenuBarRoot, i, 0, j},
					})
				}
				C.axRelease(C.CFTypeRef(rows))
				if menuIsOpen(open) {
					*out = append(*out, open...)
				}
			}
		}
		C.axRelease(C.CFTypeRef(menus))
	}
}

type limits struct{ depth, most int }

func walkElement(el C.AXUIElementRef, path []int, depth int, out *[]Element, lim limits) {
	if len(*out) >= lim.most || depth > lim.depth {
		return
	}

	kids := C.axCopyList(el, 0)
	if kids == 0 {
		return
	}
	defer C.axRelease(C.CFTypeRef(kids))

	n := int(C.axCount(kids))
	for i := 0; i < n && len(*out) < lim.most; i++ {
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

		walkElement(child, here, depth+1, out, lim)
	}
}

func walkApp(pid int, app string) (Tree, error) {
	return walkAppWithin(pid, app, limits{depth: MaxDepth, most: MaxElements})
}

func walkAppDeep(pid int, app string) (Tree, error) {
	tree, err := walkAppWithin(pid, app, limits{depth: DeepDepth, most: DeepElements})
	if err != nil {
		return tree, err
	}
	tree.Elements = Slim(tree.Elements, MaxDeepKept)
	return tree, nil
}

func walkAppWithin(pid int, app string, lim limits) (Tree, error) {
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
	walkElement(win, nil, 0, &elements, lim)
	NameByContents(elements)
	walkMenuBar(appEl, &elements)
	tree.Elements = elements

	return tree, nil
}

var _ = unsafe.Pointer(nil)
