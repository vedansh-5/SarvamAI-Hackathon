//go:build darwin && cgo

package uiact

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework Vision -framework CoreGraphics -framework ImageIO
#include <stdlib.h>
#import <Foundation/Foundation.h>
#import <Vision/Vision.h>
#import <ImageIO/ImageIO.h>

static char *readWords(const void *bytes, int length) {
	@autoreleasepool {
		NSData *data = [NSData dataWithBytes:bytes length:length];
		CGImageSourceRef source = CGImageSourceCreateWithData((__bridge CFDataRef)data, NULL);
		if (!source) return NULL;
		CGImageRef image = CGImageSourceCreateImageAtIndex(source, 0, NULL);
		CFRelease(source);
		if (!image) return NULL;
		size_t width = CGImageGetWidth(image), height = CGImageGetHeight(image);

		VNRecognizeTextRequest *request = [[VNRecognizeTextRequest alloc] init];
		request.recognitionLevel = VNRequestTextRecognitionLevelAccurate;
		request.usesLanguageCorrection = NO;
		request.recognitionLanguages = @[@"en-US"];
		VNImageRequestHandler *handler = [[VNImageRequestHandler alloc] initWithCGImage:image options:@{}];
		NSError *error = nil;
		BOOL done = [handler performRequests:@[request] error:&error];
		CGImageRelease(image);
		if (!done) return NULL;

		NSMutableArray *lines = [NSMutableArray array];
		for (VNRecognizedTextObservation *seen in request.results) {
			VNRecognizedText *best = [[seen topCandidates:1] firstObject];
			if (!best) continue;
			NSString *text = best.string;
			NSMutableArray *words = [NSMutableArray array];
			[text enumerateSubstringsInRange:NSMakeRange(0, text.length)
			                         options:NSStringEnumerationByWords
			                      usingBlock:^(NSString *word, NSRange range, NSRange enclosing, BOOL *stop) {
				VNRectangleObservation *box = [best boundingBoxForRange:range error:nil];
				if (!box) return;
				CGRect b = box.boundingBox;
				[words addObject:@{@"at": @(range.location), @"len": @(range.length),
				                   @"x": @(b.origin.x), @"y": @(1.0 - b.origin.y - b.size.height),
				                   @"w": @(b.size.width), @"h": @(b.size.height)}];
			}];
			[lines addObject:@{@"t": text, @"c": @(best.confidence), @"words": words}];
		}
		NSDictionary *page = @{@"width": @(width), @"height": @(height), @"lines": lines};
		NSData *json = [NSJSONSerialization dataWithJSONObject:page options:0 error:nil];
		if (!json) return NULL;
		char *out = malloc(json.length + 1);
		memcpy(out, json.bytes, json.length);
		out[json.length] = 0;
		return out;
	}
}
*/
import "C"

import (
	"encoding/json"
	"unsafe"
)

func ReadWords(picture []byte) ([]Word, error) {
	if len(picture) == 0 {
		return nil, ErrCannotReadWords
	}
	raw := C.readWords(unsafe.Pointer(&picture[0]), C.int(len(picture)))
	if raw == nil {
		return nil, ErrCannotReadWords
	}
	defer C.free(unsafe.Pointer(raw))
	var page seenPage
	if err := json.Unmarshal([]byte(C.GoString(raw)), &page); err != nil {
		return nil, ErrCannotReadWords
	}
	return pieces(page), nil
}
