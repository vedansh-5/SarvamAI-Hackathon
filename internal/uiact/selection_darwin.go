//go:build darwin && cgo

package uiact

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices -framework CoreFoundation
#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>

static CFTypeRef selCopy(AXUIElementRef el, CFStringRef name) {
    CFTypeRef v = NULL;
    if (AXUIElementCopyAttributeValue(el, name, &v) != kAXErrorSuccess) return NULL;
    return v;
}

static int selWalk(AXUIElementRef el, int depth, int *seen, char *buf, int len) {
    if (depth > 60 || ++(*seen) > 4000) return 0;
    CFTypeRef v = selCopy(el, kAXSelectedTextAttribute);
    if (v) {
        int ok = 0;
        if (CFGetTypeID(v) == CFStringGetTypeID() && CFStringGetLength((CFStringRef)v) > 0) {
            ok = CFStringGetCString((CFStringRef)v, buf, len, kCFStringEncodingUTF8) && buf[0] != 0;
        }
        CFRelease(v);
        if (ok) return 1;
    }
    CFTypeRef kids = selCopy(el, kAXChildrenAttribute);
    if (!kids) return 0;
    int found = 0;
    if (CFGetTypeID(kids) == CFArrayGetTypeID()) {
        CFIndex n = CFArrayGetCount((CFArrayRef)kids);
        for (CFIndex i = 0; i < n && !found; i++) {
            found = selWalk((AXUIElementRef)CFArrayGetValueAtIndex((CFArrayRef)kids, i), depth + 1, seen, buf, len);
        }
    }
    CFRelease(kids);
    return found;
}

static int selAnywhere(int pid, char *buf, int len) {
    buf[0] = 0;
    AXUIElementRef app = AXUIElementCreateApplication((pid_t)pid);
    if (!app) return 0;
    CFTypeRef win = selCopy(app, kAXFocusedWindowAttribute);
    CFRelease(app);
    if (!win) return 0;
    int seen = 0;
    int ok = selWalk((AXUIElementRef)win, 0, &seen, buf, len);
    CFRelease(win);
    return ok;
}

static AXUIElementRef selCopyItem(AXUIElementRef app) {
    CFTypeRef bar = selCopy(app, kAXMenuBarAttribute);
    if (!bar) return NULL;
    AXUIElementRef found = NULL;
    CFTypeRef tops = selCopy((AXUIElementRef)bar, kAXChildrenAttribute);
    if (tops && CFGetTypeID(tops) == CFArrayGetTypeID()) {
        for (CFIndex i = 0; i < CFArrayGetCount((CFArrayRef)tops) && !found; i++) {
            CFTypeRef menus = selCopy((AXUIElementRef)CFArrayGetValueAtIndex((CFArrayRef)tops, i), kAXChildrenAttribute);
            if (!menus) continue;
            if (CFGetTypeID(menus) == CFArrayGetTypeID() && CFArrayGetCount((CFArrayRef)menus) > 0) {
                CFTypeRef items = selCopy((AXUIElementRef)CFArrayGetValueAtIndex((CFArrayRef)menus, 0), kAXChildrenAttribute);
                if (items && CFGetTypeID(items) == CFArrayGetTypeID()) {
                    for (CFIndex j = 0; j < CFArrayGetCount((CFArrayRef)items) && !found; j++) {
                        AXUIElementRef item = (AXUIElementRef)CFArrayGetValueAtIndex((CFArrayRef)items, j);
                        CFTypeRef key = selCopy(item, kAXMenuItemCmdCharAttribute);
                        CFTypeRef mods = selCopy(item, kAXMenuItemCmdModifiersAttribute);
                        int plain = 0;
                        if (mods && CFGetTypeID(mods) == CFNumberGetTypeID()) {
                            int m = -1;
                            CFNumberGetValue((CFNumberRef)mods, kCFNumberIntType, &m);
                            plain = m == kAXMenuItemModifierNone;
                        }
                        if (plain && key && CFGetTypeID(key) == CFStringGetTypeID() &&
                            CFStringCompare((CFStringRef)key, CFSTR("C"), kCFCompareCaseInsensitive) == kCFCompareEqualTo) {
                            found = (AXUIElementRef)CFRetain(item);
                        }
                        if (key) CFRelease(key);
                        if (mods) CFRelease(mods);
                    }
                }
                if (items) CFRelease(items);
            }
            CFRelease(menus);
        }
    }
    if (tops) CFRelease(tops);
    CFRelease(bar);
    return found;
}

// 1 copied; 0 Copy is off or copied nothing, so nothing is selected; -1 no Copy to ask.
static int selByCopying(int pid, char *buf, int len) {
    buf[0] = 0;
    AXUIElementRef app = AXUIElementCreateApplication((pid_t)pid);
    if (!app) return -1;
    AXUIElementRef item = selCopyItem(app);
    CFRelease(app);
    if (!item) return -1;

    CFTypeRef enabled = selCopy(item, kAXEnabledAttribute);
    int on = enabled && CFGetTypeID(enabled) == CFBooleanGetTypeID() && CFBooleanGetValue((CFBooleanRef)enabled);
    if (enabled) CFRelease(enabled);
    if (!on) {
        CFRelease(item);
        return 0;
    }

    int result = 0;
    @autoreleasepool {
        NSPasteboard *pb = [NSPasteboard generalPasteboard];
        NSMutableArray<NSPasteboardItem *> *kept = [NSMutableArray array];
        for (NSPasteboardItem *was in pb.pasteboardItems) {
            NSPasteboardItem *copy = [[NSPasteboardItem alloc] init];
            for (NSPasteboardType type in was.types) {
                NSData *data = [was dataForType:type];
                if (data) [copy setData:data forType:type];
            }
            [kept addObject:copy];
        }
        NSInteger before = pb.changeCount;
        AXError pressed = AXUIElementPerformAction(item, kAXPressAction);
        for (int i = 0; i < 60 && pressed == kAXErrorSuccess && pb.changeCount == before; i++) usleep(10000);
        if (pb.changeCount != before) {
            NSString *text = [pb stringForType:NSPasteboardTypeString];
            while (text.length > 0 && ![text getCString:buf maxLength:len encoding:NSUTF8StringEncoding]) {
                text = [text substringToIndex:text.length / 2];
            }
            if (text.length > 0) result = 1;
            [pb clearContents];
            if (kept.count > 0) [pb writeObjects:kept];
        }
    }
    CFRelease(item);
    return result;
}
*/
import "C"

import (
	"sync"
	"time"

	"github.com/vedansh/vans/internal/names"
)

const keepSelection = 2 * time.Second

var lastSelection struct {
	sync.Mutex
	pid  int
	at   time.Time
	text string
	ok   bool
}

func selectedText(pid int) (string, bool) {
	lastSelection.Lock()
	defer lastSelection.Unlock()
	if lastSelection.pid == pid && time.Since(lastSelection.at) < keepSelection {
		return lastSelection.text, lastSelection.ok
	}
	text, ok := readSelection(pid)
	lastSelection.pid, lastSelection.at, lastSelection.text, lastSelection.ok = pid, time.Now(), text, ok
	return text, ok
}

func readSelection(pid int) (string, bool) {
	if text, ok := focusedSelectedText(pid); ok && text != "" {
		return text, true
	}
	if !trusted() {
		return "", false
	}
	buf := make([]C.char, 64<<10)
	switch C.selByCopying(C.int(pid), &buf[0], C.int(len(buf))) {
	case 1:
		return names.Strip(C.GoString(&buf[0])), true
	case 0:
		return "", false
	}
	if C.selAnywhere(C.int(pid), &buf[0], C.int(len(buf))) == 1 {
		return names.Strip(C.GoString(&buf[0])), true
	}
	return "", false
}
