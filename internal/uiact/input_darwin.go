//go:build darwin && cgo

package uiact

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>

static void cgClick(double x, double y) {
    CGPoint pt = CGPointMake(x, y);
    CGEventRef move = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, pt, kCGMouseButtonLeft);
    if (move) { CGEventPost(kCGHIDEventTap, move); CFRelease(move); }
    CGEventRef down = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, pt, kCGMouseButtonLeft);
    if (down) { CGEventPost(kCGHIDEventTap, down); CFRelease(down); }
    CGEventRef up = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, pt, kCGMouseButtonLeft);
    if (up) { CGEventPost(kCGHIDEventTap, up); CFRelease(up); }
}

static void cgTypeChunk(UniChar *chars, int n) {
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
    if (down) {
        CGEventKeyboardSetUnicodeString(down, n, chars);
        CGEventPost(kCGHIDEventTap, down);
        CFRelease(down);
    }
    CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
    if (up) {
        CGEventKeyboardSetUnicodeString(up, n, chars);
        CGEventPost(kCGHIDEventTap, up);
        CFRelease(up);
    }
}

static int cgPointer(double *x, double *y) {
    CGEventRef e = CGEventCreate(NULL);
    if (!e) return 0;
    CGPoint p = CGEventGetLocation(e);
    CFRelease(e);
    *x = p.x;
    *y = p.y;
    return 1;
}

static void cgKey(int code, unsigned long long flags) {
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)code, true);
    if (down) {
        if (flags) CGEventSetFlags(down, (CGEventFlags)flags);
        CGEventPost(kCGHIDEventTap, down);
        CFRelease(down);
    }
    CGEventRef up = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)code, false);
    if (up) {
        if (flags) CGEventSetFlags(up, (CGEventFlags)flags);
        CGEventPost(kCGHIDEventTap, up);
        CFRelease(up);
    }
}
*/
import "C"

import (
	"unicode/utf16"
	"unsafe"
)

func postClick(x, y int) { C.cgClick(C.double(x), C.double(y)) }

func pointerAt() (int, int, bool) {
	var x, y C.double
	if C.cgPointer(&x, &y) != 1 {
		return 0, 0, false
	}
	return int(x), int(y), true
}

func postKey(code int, flags uint64) { C.cgKey(C.int(code), C.ulonglong(flags)) }

const typeChunk = 16

func postText(s string) {
	units := utf16.Encode([]rune(s))
	for len(units) > 0 {
		n := len(units)
		if n > typeChunk {
			n = typeChunk
		}
		part := units[:n]
		C.cgTypeChunk((*C.UniChar)(unsafe.Pointer(&part[0])), C.int(n))
		units = units[n:]
	}
}
