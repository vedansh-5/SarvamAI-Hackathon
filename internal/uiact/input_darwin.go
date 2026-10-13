//go:build darwin && cgo

package uiact

/*
#cgo LDFLAGS: -framework ApplicationServices -framework CoreFoundation
#include <ApplicationServices/ApplicationServices.h>
#include <unistd.h>

static void cgClick(double x, double y) {
    CGPoint pt = CGPointMake(x, y);
    CGEventRef move = CGEventCreateMouseEvent(NULL, kCGEventMouseMoved, pt, kCGMouseButtonLeft);
    if (move) { CGEventSetFlags(move, 0); CGEventPost(kCGHIDEventTap, move); CFRelease(move); }
    CGEventRef down = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseDown, pt, kCGMouseButtonLeft);
    if (down) { CGEventSetFlags(down, 0); CGEventPost(kCGHIDEventTap, down); CFRelease(down); }
    CGEventRef up = CGEventCreateMouseEvent(NULL, kCGEventLeftMouseUp, pt, kCGMouseButtonLeft);
    if (up) { CGEventSetFlags(up, 0); CGEventPost(kCGHIDEventTap, up); CFRelease(up); }
}

static void cgPost(CGEventType type, CGPoint pt, CGMouseButton button, int clicks) {
    CGEventRef e = CGEventCreateMouseEvent(NULL, type, pt, button);
    if (!e) return;
    if (clicks > 0) CGEventSetIntegerValueField(e, kCGMouseEventClickState, clicks);
    CGEventSetFlags(e, 0);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
}

static void cgMove(double x, double y) {
    cgPost(kCGEventMouseMoved, CGPointMake(x, y), kCGMouseButtonLeft, 0);
}

static void cgDoubleClick(double x, double y) {
    CGPoint pt = CGPointMake(x, y);
    cgPost(kCGEventMouseMoved, pt, kCGMouseButtonLeft, 0);
    cgPost(kCGEventLeftMouseDown, pt, kCGMouseButtonLeft, 1);
    cgPost(kCGEventLeftMouseUp, pt, kCGMouseButtonLeft, 1);
    cgPost(kCGEventLeftMouseDown, pt, kCGMouseButtonLeft, 2);
    cgPost(kCGEventLeftMouseUp, pt, kCGMouseButtonLeft, 2);
}

static void cgRightClick(double x, double y) {
    CGPoint pt = CGPointMake(x, y);
    cgPost(kCGEventMouseMoved, pt, kCGMouseButtonLeft, 0);
    cgPost(kCGEventRightMouseDown, pt, kCGMouseButtonRight, 1);
    cgPost(kCGEventRightMouseUp, pt, kCGMouseButtonRight, 1);
}

static void cgScroll(double x, double y, int down, int right) {
    cgPost(kCGEventMouseMoved, CGPointMake(x, y), kCGMouseButtonLeft, 0);
    usleep(40000);
    CGEventRef e = CGEventCreateScrollWheelEvent(NULL, kCGScrollEventUnitLine, 2, -down, -right);
    if (!e) return;
    CGEventSetFlags(e, 0);
    CGEventPost(kCGHIDEventTap, e);
    CFRelease(e);
}

static void cgDrag(double x1, double y1, double x2, double y2) {
    CGPoint from = CGPointMake(x1, y1);
    cgPost(kCGEventMouseMoved, from, kCGMouseButtonLeft, 0);
    usleep(60000);
    cgPost(kCGEventLeftMouseDown, from, kCGMouseButtonLeft, 1);
    usleep(120000);
    int steps = 18;
    for (int i = 1; i <= steps; i++) {
        CGPoint at = CGPointMake(x1 + (x2 - x1) * i / steps, y1 + (y2 - y1) * i / steps);
        cgPost(kCGEventLeftMouseDragged, at, kCGMouseButtonLeft, 1);
        usleep(18000);
    }
    usleep(120000);
    cgPost(kCGEventLeftMouseUp, CGPointMake(x2, y2), kCGMouseButtonLeft, 1);
}

static void cgTypeChunk(UniChar *chars, int n) {
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, 0, true);
    if (down) {
        CGEventKeyboardSetUnicodeString(down, n, chars);
        CGEventSetFlags(down, 0);
        CGEventPost(kCGHIDEventTap, down);
        CFRelease(down);
    }
    CGEventRef up = CGEventCreateKeyboardEvent(NULL, 0, false);
    if (up) {
        CGEventKeyboardSetUnicodeString(up, n, chars);
        CGEventSetFlags(up, 0);
        CGEventPost(kCGHIDEventTap, up);
        CFRelease(up);
    }
}

static void cgKey(int code, unsigned long long flags) {
    CGEventRef down = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)code, true);
    if (down) {
        CGEventSetFlags(down, (CGEventFlags)flags);
        CGEventPost(kCGHIDEventTap, down);
        CFRelease(down);
    }
    CGEventRef up = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)code, false);
    if (up) {
        CGEventSetFlags(up, (CGEventFlags)flags);
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

func postKey(code int, flags uint64) { C.cgKey(C.int(code), C.ulonglong(flags)) }

func postMove(x, y int) { C.cgMove(C.double(x), C.double(y)) }

func postDoubleClick(x, y int) { C.cgDoubleClick(C.double(x), C.double(y)) }

func postRightClick(x, y int) { C.cgRightClick(C.double(x), C.double(y)) }

func postScroll(x, y, down, right int) {
	C.cgScroll(C.double(x), C.double(y), C.int(down), C.int(right))
}

func postDrag(x1, y1, x2, y2 int) {
	C.cgDrag(C.double(x1), C.double(y1), C.double(x2), C.double(y2))
}

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
