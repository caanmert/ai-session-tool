//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// Show how many conversations need the user on the Dock icon, and bounce the
// icon once when asked. AppKit must be used from the main thread.
static void aisSetAttention(int count, int alert) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSString *label = count > 0 ? [NSString stringWithFormat:@"%d", count] : nil;
		[[NSApp dockTile] setBadgeLabel:label];
		// requestUserAttention does nothing while the app is active.
		if (alert) [NSApp requestUserAttention:NSInformationalRequest];
	});
}
*/
import "C"

func setDockAttention(count int, alert bool) {
	flag := C.int(0)
	if alert {
		flag = 1
	}
	C.aisSetAttention(C.int(count), flag)
}
