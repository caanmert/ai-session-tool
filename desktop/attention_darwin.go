//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications
#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>
#include <stdlib.h>

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

// Clicking a banner opens its conversation: the delegate passes the
// conversation's ID to Go (notify_click_darwin.go).
extern void aisNotificationOpened(char *chat);

@interface AISNotificationDelegate : NSObject <UNUserNotificationCenterDelegate>
@end
@implementation AISNotificationDelegate
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
    didReceiveNotificationResponse:(UNNotificationResponse *)response
             withCompletionHandler:(void (^)(void))completionHandler {
	NSString *chat = response.notification.request.content.userInfo[@"chat"];
	if ([chat isKindOfClass:[NSString class]] && chat.length > 0)
		aisNotificationOpened((char *)[chat UTF8String]);
	completionHandler();
}
@end

// The center holds its delegate weakly; this keeps it alive.
static AISNotificationDelegate *aisDelegate;

static void aisListenForClicks(void) {
	if ([[NSBundle mainBundle] bundleIdentifier] == nil) return;
	dispatch_async(dispatch_get_main_queue(), ^{
		@try {
			aisDelegate = [[AISNotificationDelegate alloc] init];
			[UNUserNotificationCenter currentNotificationCenter].delegate = aisDelegate;
		} @catch (NSException *exception) {
			// Clicks then only bring the app forward, as macOS does anyway.
		}
	});
}

// Post a notification banner. macOS asks the user for permission the first
// time; if they decline, nothing is shown and nothing else changes.
static void aisNotify(const char *ctitle, const char *cbody, const char *cchat) {
	// Notifications belong to an app bundle. A bare binary (tests, development
	// builds) has none, and asking for the notification center would fail.
	if ([[NSBundle mainBundle] bundleIdentifier] == nil) return;
	NSString *title = [NSString stringWithUTF8String:ctitle];
	NSString *body = [NSString stringWithUTF8String:cbody];
	NSString *chat = [NSString stringWithUTF8String:cchat];
	if (title == nil || body == nil || chat == nil) return;
	@try {
		UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
		[center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
		                      completionHandler:^(BOOL granted, NSError *error) {
			if (!granted) return;
			UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
			content.title = title;
			content.body = body;
			content.sound = [UNNotificationSound defaultSound];
			if (chat.length > 0) content.userInfo = @{@"chat": chat};
			UNNotificationRequest *request =
				[UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString] content:content trigger:nil];
			[center addNotificationRequest:request withCompletionHandler:nil];
		}];
	} @catch (NSException *exception) {
		// A notification is a convenience; never let it take the app down.
	}
}
*/
import "C"

import "unsafe"

func setDockAttention(count int, alert bool) {
	flag := C.int(0)
	if alert {
		flag = 1
	}
	C.aisSetAttention(C.int(count), flag)
}

func postNotification(title, body, chat string) {
	ctitle, cbody, cchat := C.CString(title), C.CString(body), C.CString(chat)
	defer C.free(unsafe.Pointer(ctitle))
	defer C.free(unsafe.Pointer(cbody))
	defer C.free(unsafe.Pointer(cchat))
	// The strings are copied before this returns, so freeing them is safe.
	C.aisNotify(ctitle, cbody, cchat)
}

// listenForNotificationClicks routes banner clicks to notificationOpened.
func listenForNotificationClicks() {
	C.aisListenForClicks()
}
