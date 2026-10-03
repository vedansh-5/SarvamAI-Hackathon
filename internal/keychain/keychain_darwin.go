//go:build darwin && cgo

package keychain

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

static CFStringRef vansString(const char *s) {
	return CFStringCreateWithCString(kCFAllocatorDefault, s, kCFStringEncodingUTF8);
}

static CFMutableDictionaryRef vansQuery(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = vansString(service);
	CFStringRef a = vansString(account);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	CFRelease(s);
	CFRelease(a);
	return q;
}

static int vansGet(const char *service, const char *account, char **out, int *length) {
	CFMutableDictionaryRef q = vansQuery(service, account);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef found = NULL;
	OSStatus status = SecItemCopyMatching(q, &found);
	CFRelease(q);
	if (status != errSecSuccess) {
		return (int)status;
	}
	CFDataRef data = (CFDataRef)found;
	CFIndex n = CFDataGetLength(data);
	char *buf = malloc(n + 1);
	memcpy(buf, CFDataGetBytePtr(data), n);
	buf[n] = 0;
	*out = buf;
	*length = (int)n;
	CFRelease(found);
	return 0;
}

static int vansSet(const char *service, const char *account, const char *secret, int length) {
	CFDataRef data = CFDataCreate(kCFAllocatorDefault, (const UInt8 *)secret, length);
	CFMutableDictionaryRef q = vansQuery(service, account);

	CFMutableDictionaryRef change = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(change, kSecValueData, data);
	OSStatus status = SecItemUpdate(q, change);
	CFRelease(change);

	if (status == errSecItemNotFound) {
		CFDictionarySetValue(q, kSecValueData, data);
		status = SecItemAdd(q, NULL);
	}
	CFRelease(q);
	CFRelease(data);
	return (int)status;
}

static int vansDelete(const char *service, const char *account) {
	CFMutableDictionaryRef q = vansQuery(service, account);
	OSStatus status = SecItemDelete(q);
	CFRelease(q);
	return (int)status;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const notFound = -25300

func get(service, account string) (string, error) {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))

	var out *C.char
	var n C.int
	switch status := int(C.vansGet(s, a, &out, &n)); status {
	case 0:
		defer C.free(unsafe.Pointer(out))
		return C.GoStringN(out, n), nil
	case notFound:
		return "", ErrNotFound
	default:
		return "", fmt.Errorf("keychain: could not read (status %d)", status)
	}
}

func set(service, account, secret string) error {
	s, a, v := C.CString(service), C.CString(account), C.CString(secret)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(v))

	if status := int(C.vansSet(s, a, v, C.int(len(secret)))); status != 0 {
		return fmt.Errorf("keychain: could not save (status %d)", status)
	}
	return nil
}

func remove(service, account string) error {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))

	switch status := int(C.vansDelete(s, a)); status {
	case 0, notFound:
		return nil
	default:
		return fmt.Errorf("keychain: could not remove (status %d)", status)
	}
}
