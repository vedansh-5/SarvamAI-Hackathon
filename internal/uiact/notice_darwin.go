//go:build darwin && cgo

package uiact

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <string.h>

static void noticeText(AXUIElementRef el, CFStringRef attr, char *buf, int len) {
    buf[0] = 0;
    CFTypeRef v = NULL;
    if (AXUIElementCopyAttributeValue(el, attr, &v) == kAXErrorSuccess && v) {
        if (CFGetTypeID(v) == CFStringGetTypeID())
            CFStringGetCString((CFStringRef)v, buf, len, kCFStringEncodingUTF8);
        CFRelease(v);
    }
}

static int noticeAt(double x, double y, char *roles, char *titles, int each, int *pid, int *inweb) {
    *pid = 0; *inweb = 0;
    AXUIElementRef sys = AXUIElementCreateSystemWide();
    if (!sys) return -1;
    AXUIElementRef el = NULL;
    AXError e = AXUIElementCopyElementAtPosition(sys, (float)x, (float)y, &el);
    CFRelease(sys);
    if (e != kAXErrorSuccess || !el) return -1;
    pid_t owner = 0;
    AXUIElementGetPid(el, &owner);
    *pid = (int)owner;

    int layers = 0;
    char role[128];
    for (int depth = 0; el && depth < 40; depth++) {
        noticeText(el, kAXRoleAttribute, role, sizeof role);
        if (strcmp(role, "AXWebArea") == 0) *inweb = 1;
        if (depth < 3) {
            strncpy(roles + depth * each, role, each - 1);
            roles[depth * each + each - 1] = 0;
            char *title = titles + depth * each;
            noticeText(el, kAXTitleAttribute, title, each);
            if (!title[0]) noticeText(el, kAXDescriptionAttribute, title, each);
            layers = depth + 1;
        }
        CFTypeRef parent = NULL;
        if (AXUIElementCopyAttributeValue(el, kAXParentAttribute, &parent) != kAXErrorSuccess) parent = NULL;
        CFRelease(el);
        el = (AXUIElementRef)parent;
    }
    if (el) CFRelease(el);
    return layers;
}
*/
import "C"

import (
	"unsafe"
)

const noticeEach = 256

func NoticeAt(x, y int) (Noticed, bool) {
	var roles, titles [3 * noticeEach]C.char
	var pid, inWeb C.int
	n := int(C.noticeAt(C.double(x), C.double(y), &roles[0], &titles[0], noticeEach, &pid, &inWeb))
	if n <= 0 {
		return Noticed{}, false
	}
	layers := make([]Layer, n)
	for i := range layers {
		layers[i] = Layer{
			Role:  C.GoString((*C.char)(unsafe.Pointer(&roles[i*noticeEach]))),
			Title: C.GoString((*C.char)(unsafe.Pointer(&titles[i*noticeEach]))),
		}
	}
	app := ""
	if owner, name, ok := frontWindowAt(x, y); ok && owner == int(pid) {
		app = name
	} else if front, ok := frontmostApp(); ok {
		app = front
	}
	return Pick(app, layers, inWeb == 1)
}
