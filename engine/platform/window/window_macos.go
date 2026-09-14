//go:build darwin

// Package window provides a Cocoa NSWindow-based window with a Skia raster
// backend surface. It is an alternative to the GLFW backend (window_glfw.go)
// for macOS systems where a pure Cocoa backend is preferred.
//
// The window creates an NSApplication + NSWindow + NSView, uses a Skia CPU
// raster surface via graphics.Canvas for rendering, and presents via
// CGImage on the view's layer. This avoids any dependency on GLFW.

package window

/*
#cgo LDFLAGS: -framework Cocoa -framework CoreGraphics
#include <stdlib.h>
#include <objc/runtime.h>
#include <objc/message.h>

// --- Event queue (thread-safe, lock-free single-producer single-consumer) ---
#define MAX_EVENTS 256
typedef struct {
    int type;     // 0=mouseBtn 1=cursorMove 2=key 3=resize 4=scroll 5=touch 6=drop
    double x, y;
    int button, action, key, mods, width, height;
    double scrollY;
    char text[256]; // for drop file paths / text
} NativeEvent;

typedef struct {
    NativeEvent buf[MAX_EVENTS];
    volatile int head, tail;
} EventQueue;

static void eq_push(EventQueue* q, NativeEvent* e) {
    int next = (q->head + 1) % MAX_EVENTS;
    if (next == q->tail) return; // full
    q->buf[q->head] = *e;
    q->head = next;
    __sync_synchronize();
}

static int eq_pop(EventQueue* q, NativeEvent* e) {
    if (q->tail == q->head) return 0;
    *e = q->buf[q->tail];
    q->tail = (q->tail + 1) % MAX_EVENTS;
    __sync_synchronize();
    return 1;
}

// --- IME event queue (for NSTextInputClient composition events) ---
#define MAX_IME_EVENTS 64
#define MAX_IME_TEXT 512

typedef struct {
    int kind;       // 0=composition update, 1=char commit, 2=composition end
    char text[MAX_IME_TEXT];
    int cursor_pos;
} IMEEvent;

typedef struct {
    IMEEvent buf[MAX_IME_EVENTS];
    volatile int head, tail;
} IMEEventQueue;

static void ime_eq_init(IMEEventQueue* q) { q->head = 0; q->tail = 0; }

static void ime_eq_push(IMEEventQueue* q, IMEEvent* e) {
    int next = (q->head + 1) % MAX_IME_EVENTS;
    if (next == q->tail) return;
    q->buf[q->head] = *e;
    q->head = next;
    __sync_synchronize();
}

static int ime_eq_pop(IMEEventQueue* q, IMEEvent* e) {
    if (q->tail == q->head) return 0;
    *e = q->buf[q->tail];
    q->tail = (q->tail + 1) % MAX_IME_EVENTS;
    __sync_synchronize();
    return 1;
}

// --- CocoaWindow handle ---
typedef struct {
    void* win;    // NSWindow*
    void* view;   // GoCocoaView*
    void* layer;  // CALayer*
    EventQueue eq;
    int width, height;
    double scale;
    int shouldClose;
} CocoaWindow;

// Forward declarations for the Go-callable view event notifier
// (unused in this port; events flow through the NativeEvent queue instead).
// extern void goCocoaViewEvent(void* ctx, int type, double x, double y, int button, int action, int key, double scrollY);

// --- View subclass (created at runtime via ObjC runtime) ---
static Class viewClass = NULL;

static void ensureViewClass(void) {
    if (viewClass) return;
    viewClass = objc_allocateClassPair((Class)objc_getClass("NSView"), "GoCocoaView", 0);
    if (!viewClass) { viewClass = objc_getClass("GoCocoaView"); return; }

    // Add context pointer (iVar) to store CocoaWindow*
    class_addIvar(viewClass, "ctxPtr", sizeof(void*), rint(log2(sizeof(void*))), "^v");
    // Add IME event queue pointer (iVar) for NSTextInputClient
    class_addIvar(viewClass, "imeQueuePtr", sizeof(void*), rint(log2(sizeof(void*))), "^v");

    // mouseDown:
    SEL mouseDownSEL = sel_registerName("mouseDown:");
    IMP mouseDownIMP = imp_implementationWithBlock(^(id self, id event) {
        CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
        if (!cw) return;
        id loc = ((id(*)(id, SEL))objc_msgSend)(event, sel_registerName("locationInWindow"));
        double x = (double)((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("x"));
        double y = (double)((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("y"));
        // Flip y: Cocoa origin is bottom-left, we want top-left
        id view = ((id(*)(id, SEL))objc_msgSend)(self, sel_registerName("superview"));
        if (view) {
            id ws = ((id(*)(id, SEL))objc_msgSend)(view, sel_registerName("window"));
            if (ws) {
                id contentView = ((id(*)(id, SEL))objc_msgSend)(ws, sel_registerName("contentView"));
                if (contentView) {
                    CGFloat ch = ((CGFloat(*)(id, SEL))objc_msgSend)(contentView, sel_registerName("bounds"));
                    y = ch - y;
                }
            }
        }
        NativeEvent ne; ne.type=0; ne.x=x; ne.y=y; ne.button=0; ne.action=1;
        eq_push(&cw->eq, &ne);
    });
    class_addMethod(viewClass, mouseDownSEL, mouseDownIMP, "v@:@");

    // mouseUp:
    SEL mouseUpSEL = sel_registerName("mouseUp:");
    IMP mouseUpIMP = imp_implementationWithBlock(^(id self, id event) {
        CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
        if (!cw) return;
        id loc = ((id(*)(id, SEL))objc_msgSend)(event, sel_registerName("locationInWindow"));
        double x = (double)((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("x"));
        double y = (double)((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("y"));
        NativeEvent ne; ne.type=0; ne.x=x; ne.y=y; ne.button=0; ne.action=0;
        eq_push(&cw->eq, &ne);
    });
    class_addMethod(viewClass, mouseUpSEL, mouseUpIMP, "v@:@");

    // mouseDragged:
    SEL mouseDragSEL = sel_registerName("mouseDragged:");
    IMP mouseDragIMP = imp_implementationWithBlock(^(id self, id event) {
        CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
        if (!cw) return;
        id loc = ((id(*)(id, SEL))objc_msgSend)(event, sel_registerName("locationInWindow"));
        double x = (double)((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("x"));
        double y = (double)((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("y"));
        NativeEvent ne; ne.type=1; ne.x=x; ne.y=y;
        eq_push(&cw->eq, &ne);
    });
    class_addMethod(viewClass, mouseDragSEL, mouseDragIMP, "v@:@");

    // rightMouseDown:
    SEL rightMouseDownSEL = sel_registerName("rightMouseDown:");
    IMP rightMouseDownIMP = imp_implementationWithBlock(^(id self, id event) {
        CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
        if (!cw) return;
        id loc = ((id(*)(id, SEL))objc_msgSend)(event, sel_registerName("locationInWindow"));
        double x = (double)((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("x"));
        double y = (double)((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("y"));
        NativeEvent ne; ne.type=0; ne.x=x; ne.y=y; ne.button=1; ne.action=1;
        eq_push(&cw->eq, &ne);
    });
    class_addMethod(viewClass, rightMouseDownSEL, rightMouseDownIMP, "v@:@");

    // rightMouseUp:
    SEL rightMouseUpSEL = sel_registerName("rightMouseUp:");
    IMP rightMouseUpIMP = imp_implementationWithBlock(^(id self, id event) {
        CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
        if (!cw) return;
        id loc = ((id(*)(id, SEL))objc_msgSend)(event, sel_registerName("locationInWindow"));
        double x = (double)((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("x"));
        double y = (double)((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("y"));
        NativeEvent ne; ne.type=0; ne.x=x; ne.y=y; ne.button=1; ne.action=0;
        eq_push(&cw->eq, &ne);
    });
    class_addMethod(viewClass, rightMouseUpSEL, rightMouseUpIMP, "v@:@");

    // scrollWheel:
    SEL scrollSEL = sel_registerName("scrollWheel:");
    IMP scrollIMP = imp_implementationWithBlock(^(id self, id event) {
        CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
        if (!cw) return;
        double dy = (double)((CGFloat(*)(id, SEL))objc_msgSend)(event, sel_registerName("deltaY"));
        NativeEvent ne; ne.type=4; ne.scrollY=dy;
        eq_push(&cw->eq, &ne);
    });
    class_addMethod(viewClass, scrollSEL, scrollIMP, "v@:@");

    // keyDown:
    SEL keyDownSEL = sel_registerName("keyDown:");
    IMP keyDownIMP = imp_implementationWithBlock(^(id self, id event) {
        CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
        if (!cw) return;
        unsigned short keyCode = ((unsigned short(*)(id, SEL))objc_msgSend)(event, sel_registerName("keyCode"));
        NativeEvent ne; ne.type=2; ne.key=(int)keyCode; ne.action=1;
        eq_push(&cw->eq, &ne);
    });
    class_addMethod(viewClass, keyDownSEL, keyDownIMP, "v@:@");

    // keyUp:
    SEL keyUpSEL = sel_registerName("keyUp:");
    IMP keyUpIMP = imp_implementationWithBlock(^(id self, id event) {
        CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
        if (!cw) return;
        unsigned short keyCode = ((unsigned short(*)(id, SEL))objc_msgSend)(event, sel_registerName("keyCode"));
        NativeEvent ne; ne.type=2; ne.key=(int)keyCode; ne.action=0;
        eq_push(&cw->eq, &ne);
    });
    class_addMethod(viewClass, keyUpSEL, keyUpIMP, "v@:@");

    // viewDidEndLiveResize — notify resize completion
    SEL resizeSEL = sel_registerName("viewDidEndLiveResize:");
    IMP resizeIMP = imp_implementationWithBlock(^(id self, id note) {
        CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
        if (!cw) return;
        id ws = ((id(*)(id, SEL))objc_msgSend)(self, sel_registerName("window"));
        if (ws) {
            id cv = ((id(*)(id, SEL))objc_msgSend)(ws, sel_registerName("contentView"));
            if (cv) {
                id bounds = ((id(*)(id, SEL))objc_msgSend)(cv, sel_registerName("bounds"));
                CGFloat w = ((CGFloat(*)(id, SEL))objc_msgSend)(bounds, sel_registerName("size.width"));
                CGFloat h = ((CGFloat(*)(id, SEL))objc_msgSend)(bounds, sel_registerName("size.height"));
                NativeEvent ne; ne.type=3; ne.width=(int)w; ne.height=(int)h;
                eq_push(&cw->eq, &ne);
            }
        }
    });
    class_addMethod(viewClass, resizeSEL, resizeIMP, "v@:@");

    // ---- NSTextInputClient protocol methods ----

    // Helper: extract UTF-8 string from NSString (id) into a C buffer
    // Returns the length of the string written.
    // (Defined as a static helper outside ensureViewClass, it is in scope here)

    // setMarkedText:selectedRange:replacementRange:
    // Called when IME composition text changes.
    {
        SEL sel = sel_registerName("setMarkedText:selectedRange:replacementRange:");
        IMP imp = imp_implementationWithBlock(^(id self, id string, NSRange selRange, NSRange replRange) {
            (void)replRange;
            IMEEventQueue* q = *(IMEEventQueue**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "imeQueuePtr")));
            if (!q) return;
            IMEEvent ev;
            memset(&ev, 0, sizeof(ev));
            ev.kind = 0; // composition update
            ev.cursor_pos = (int)selRange.location;
            if (string) {
                const char* utf8 = ((const char* (*)(id, SEL))objc_msgSend)(string, sel_registerName("UTF8String"));
                if (utf8) {
                    size_t len = strlen(utf8);
                    if (len > MAX_IME_TEXT - 1) len = MAX_IME_TEXT - 1;
                    memcpy(ev.text, utf8, len);
                    ev.text[len] = '\0';
                }
            }
            ime_eq_push(q, &ev);
        });
        class_addMethod(viewClass, sel, imp, "v@:@");
    }

    // insertText:replacementRange:
    // Called when IME commits text (or plain text input).
    {
        SEL sel = sel_registerName("insertText:replacementRange:");
        IMP imp = imp_implementationWithBlock(^(id self, id string, NSRange range) {
            (void)range;
            IMEEventQueue* q = *(IMEEventQueue**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "imeQueuePtr")));
            if (!q) return;
            IMEEvent ev;
            memset(&ev, 0, sizeof(ev));
            ev.kind = 1; // char commit
            if (string) {
                const char* utf8 = ((const char* (*)(id, SEL))objc_msgSend)(string, sel_registerName("UTF8String"));
                if (utf8) {
                    size_t len = strlen(utf8);
                    if (len > MAX_IME_TEXT - 1) len = MAX_IME_TEXT - 1;
                    memcpy(ev.text, utf8, len);
                    ev.text[len] = '\0';
                }
            }
            ime_eq_push(q, &ev);
        });
        class_addMethod(viewClass, sel, imp, "v@:@");
    }

    // unmarkText
    // Called when IME composition ends.
    {
        SEL sel = sel_registerName("unmarkText");
        IMP imp = imp_implementationWithBlock(^(id self) {
            IMEEventQueue* q = *(IMEEventQueue**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "imeQueuePtr")));
            if (!q) return;
            IMEEvent ev;
            memset(&ev, 0, sizeof(ev));
            ev.kind = 2; // composition end
            ime_eq_push(q, &ev);
        });
        class_addMethod(viewClass, sel, imp, "v@:");
    }

    // selectedRange
    {
        SEL sel = sel_registerName("selectedRange");
        IMP imp = imp_implementationWithBlock(^NSRange(id self) {
            (void)self;
            return NSMakeRange(NSNotFound, 0);
        });
        class_addMethod(viewClass, sel, imp, "{_NSRange=QQ}@:");
    }

    // markedRange
    {
        SEL sel = sel_registerName("markedRange");
        IMP imp = imp_implementationWithBlock(^NSRange(id self) {
            (void)self;
            return NSMakeRange(NSNotFound, 0);
        });
        class_addMethod(viewClass, sel, imp, "{_NSRange=QQ}@:");
    }

    // attributedSubstringForProposedRange:actualRange:
    {
        SEL sel = sel_registerName("attributedSubstringForProposedRange:actualRange:");
        IMP imp = imp_implementationWithBlock(^id(id self, NSRange range, NSRangePointer actual) {
            (void)self; (void)range; (void)actual;
            return nil;
        });
        class_addMethod(viewClass, sel, imp, "@@:{_NSRange=QQ}^v");
    }

    // validAttributesForMarkedText
    {
        SEL sel = sel_registerName("validAttributesForMarkedText");
        IMP imp = imp_implementationWithBlock(^id(id self) {
            (void)self;
            return [NSArray array];
        });
        class_addMethod(viewClass, sel, imp, "@@:");
    }

    // firstRectForCharacterRange:actualRange:
    // Returns the screen rect for the given character range (used for
    // candidate window positioning). Returns the window's frame origin
    // as a fallback when the caller has not set composition position.
    {
        SEL sel = sel_registerName("firstRectForCharacterRange:actualRange:");
        IMP imp = imp_implementationWithBlock(^NSRect(id self, NSRange range, NSRangePointer actual) {
            (void)range; (void)actual;
            id ws = ((id(*)(id, SEL))objc_msgSend)(self, sel_registerName("window"));
            if (ws) {
                id wf = ((id(*)(id, SEL))objc_msgSend)(ws, sel_registerName("frame"));
                CGFloat x = ((CGFloat(*)(id, SEL))objc_msgSend)(wf, sel_registerName("origin.x"));
                CGFloat y = ((CGFloat(*)(id, SEL))objc_msgSend)(wf, sel_registerName("origin.y"));
                // Return a rect at the window origin (caller should
                // update via SetCompositionPos).
                return NSMakeRect(x, y + 40, 100, 20);
            }
            return NSMakeRect(0, 0, 100, 20);
        });
        class_addMethod(viewClass, sel, imp, "{_NSRect={_NSPoint=dd}{_NSSize=dd}}@:{_NSRange=QQ}^v");
    }

    // characterIndexForPoint:
    {
        SEL sel = sel_registerName("characterIndexForPoint:");
        IMP imp = imp_implementationWithBlock(^NSUInteger(id self, NSPoint point) {
            (void)self; (void)point;
            return 0;
        });
        class_addMethod(viewClass, sel, imp, "Q@:{_NSPoint=dd}");
    }

    // inputContext — lazy NSTextInputContext creation
    {
        SEL sel = sel_registerName("inputContext");
        IMP imp = imp_implementationWithBlock(^id(id self) {
            // Check if the view already has an associated input context
            id ctx = ((id(*)(id, SEL))objc_msgSend)(self, sel_registerName("associatedInputContext"));
            if (!ctx) {
                ctx = ((id(*)(id, SEL))objc_msgSend)(
                    (id)objc_getClass("NSTextInputContext"),
                    sel_registerName("alloc")
                );
                ctx = ((id(*)(id, SEL, id))objc_msgSend)(
                    ctx, sel_registerName("initWithClient:"), self
                );
                // Store as associated object
                ((void(*)(id, SEL, id))objc_msgSend)(
                    self, sel_registerName("setAssociatedInputContext:"), ctx
                );
                [ctx release];
            }
            return ctx;
        });
        class_addMethod(viewClass, sel, imp, "@@:");
    }

    // Associated object storage for inputContext
    {
        SEL sel = sel_registerName("associatedInputContext");
        IMP imp = imp_implementationWithBlock(^id(id self) {
            return objc_getAssociatedObject(self, sel);
        });
        class_addMethod(viewClass, sel, imp, "@@:");
    }
    {
        SEL sel = sel_registerName("setAssociatedInputContext:");
        IMP imp = imp_implementationWithBlock(^(id self, id ctx) {
            objc_setAssociatedObject(self, sel, ctx, OBJC_ASSOCIATION_RETAIN);
        });
        class_addMethod(viewClass, sel, imp, "v@:@");
    }

    // ---- Touch event methods (NSTouch) ----
    // touchesBeganWithEvent:
    {
        SEL sel = sel_registerName("touchesBeganWithEvent:");
        IMP imp = imp_implementationWithBlock(^(id self, id event) {
            CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
            if (!cw) return;
            id touches = ((id(*)(id, SEL, id))objc_msgSend)(event, sel_registerName("touchesMatchingPhase:inView:"), 1, self); // NSTouchPhaseBegan
            id enumerator = ((id(*)(id, SEL))objc_msgSend)(touches, sel_registerName("objectEnumerator"));
            id touch;
            while ((touch = ((id(*)(id, SEL))objc_msgSend)(enumerator, sel_registerName("nextObject")))) {
                id loc = ((id(*)(id, SEL))objc_msgSend)(touch, sel_registerName("locationInView:"), self);
                CGFloat x = ((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("x"));
                CGFloat y = ((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("y"));
                // Flip y coordinate
                id bounds = ((id(*)(id, SEL))objc_msgSend)(self, sel_registerName("bounds"));
                CGFloat h = ((CGFloat(*)(id, SEL))objc_msgSend)(bounds, sel_registerName("size.height"));
                NativeEvent ne; ne.type=5; ne.x=x; ne.y=h-y; ne.action=1;
                eq_push(&cw->eq, &ne);
            }
        });
        class_addMethod(viewClass, sel, imp, "v@:@");
    }
    // touchesMovedWithEvent:
    {
        SEL sel = sel_registerName("touchesMovedWithEvent:");
        IMP imp = imp_implementationWithBlock(^(id self, id event) {
            CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
            if (!cw) return;
            id touches = ((id(*)(id, SEL, id))objc_msgSend)(event, sel_registerName("touchesMatchingPhase:inView:"), 2, self); // NSTouchPhaseMoved
            id enumerator = ((id(*)(id, SEL))objc_msgSend)(touches, sel_registerName("objectEnumerator"));
            id touch;
            while ((touch = ((id(*)(id, SEL))objc_msgSend)(enumerator, sel_registerName("nextObject")))) {
                id loc = ((id(*)(id, SEL))objc_msgSend)(touch, sel_registerName("locationInView:"), self);
                CGFloat x = ((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("x"));
                CGFloat y = ((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("y"));
                id bounds = ((id(*)(id, SEL))objc_msgSend)(self, sel_registerName("bounds"));
                CGFloat h = ((CGFloat(*)(id, SEL))objc_msgSend)(bounds, sel_registerName("size.height"));
                NativeEvent ne; ne.type=5; ne.x=x; ne.y=h-y; ne.action=2;
                eq_push(&cw->eq, &ne);
            }
        });
        class_addMethod(viewClass, sel, imp, "v@:@");
    }
    // touchesEndedWithEvent:
    {
        SEL sel = sel_registerName("touchesEndedWithEvent:");
        IMP imp = imp_implementationWithBlock(^(id self, id event) {
            CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
            if (!cw) return;
            id touches = ((id(*)(id, SEL, id))objc_msgSend)(event, sel_registerName("touchesMatchingPhase:inView:"), 4, self); // NSTouchPhaseEnded
            id enumerator = ((id(*)(id, SEL))objc_msgSend)(touches, sel_registerName("objectEnumerator"));
            id touch;
            while ((touch = ((id(*)(id, SEL))objc_msgSend)(enumerator, sel_registerName("nextObject")))) {
                id loc = ((id(*)(id, SEL))objc_msgSend)(touch, sel_registerName("locationInView:"), self);
                CGFloat x = ((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("x"));
                CGFloat y = ((CGFloat(*)(id, SEL))objc_msgSend)(loc, sel_registerName("y"));
                id bounds = ((id(*)(id, SEL))objc_msgSend)(self, sel_registerName("bounds"));
                CGFloat h = ((CGFloat(*)(id, SEL))objc_msgSend)(bounds, sel_registerName("size.height"));
                NativeEvent ne; ne.type=5; ne.x=x; ne.y=h-y; ne.action=0;
                eq_push(&cw->eq, &ne);
            }
        });
        class_addMethod(viewClass, sel, imp, "v@:@");
    }

    // ---- Drag & Drop support ----
    // draggingEntered:
    {
        SEL sel = sel_registerName("draggingEntered:");
        IMP imp = imp_implementationWithBlock(^NSDragOperation(id self, id sender) {
            return NSDragOperationCopy;
        });
        class_addMethod(viewClass, sel, imp, "q@:@");
    }
    // draggingUpdated:
    {
        SEL sel = sel_registerName("draggingUpdated:");
        IMP imp = imp_implementationWithBlock(^NSDragOperation(id self, id sender) {
            return NSDragOperationCopy;
        });
        class_addMethod(viewClass, sel, imp, "q@:@");
    }
    // draggingExited:
    {
        SEL sel = sel_registerName("draggingExited:");
        IMP imp = imp_implementationWithBlock(^(id self, id sender) {
            (void)self; (void)sender;
        });
        class_addMethod(viewClass, sel, imp, "v@:@");
    }
    // performDragOperation:
    {
        SEL sel = sel_registerName("performDragOperation:");
        IMP imp = imp_implementationWithBlock(^(id self, id sender) {
            CocoaWindow* cw = *(CocoaWindow**)((char*)self + ivar_getOffset(class_getInstanceVariable(viewClass, "ctxPtr")));
            if (!cw) return NO;
            id pb = ((id(*)(id, SEL))objc_msgSend)(sender, sel_registerName("draggingPasteboard"));
            // Try file URLs first, then plain text
            id classes = ((id(*)(id, SEL, id))objc_msgSend)(
                (id)objc_getClass("NSArray"), sel_registerName("arrayWithObject:"),
                (id)objc_getClass("NSURL")
            );
            id options = [NSDictionary dictionary];
            id urls = ((id(*)(id, SEL, id, id, id*))objc_msgSend)(
                pb, sel_registerName("readObjectsForClasses:options:"), classes, options, nil
            );
            if (urls && ((id(*)(id, SEL))objc_msgSend)(urls, sel_registerName("count")) > 0) {
                id firstURL = ((id(*)(id, SEL, NSUInteger))objc_msgSend)(urls, sel_registerName("objectAtIndex:"), 0);
                id path = ((id(*)(id, SEL))objc_msgSend)(firstURL, sel_registerName("path"));
                if (path) {
                    const char* utf8 = ((const char* (*)(id, SEL))objc_msgSend)(path, sel_registerName("UTF8String"));
                    if (utf8) {
                        NativeEvent ne; ne.type=6; // EventDrop
                        strncpy(ne.text, utf8, sizeof(ne.text)-1);
                        ne.text[sizeof(ne.text)-1] = '\0';
                        eq_push(&cw->eq, &ne);
                    }
                }
            } else {
                // Try plain text
                id textClasses = ((id(*)(id, SEL, id))objc_msgSend)(
                    (id)objc_getClass("NSArray"), sel_registerName("arrayWithObject:"),
                    (id)objc_getClass("NSString")
                );
                id strings = ((id(*)(id, SEL, id, id, id*))objc_msgSend)(
                    pb, sel_registerName("readObjectsForClasses:options:"), textClasses, options, nil
                );
                if (strings && ((id(*)(id, SEL))objc_msgSend)(strings, sel_registerName("count")) > 0) {
                    id firstStr = ((id(*)(id, SEL, NSUInteger))objc_msgSend)(strings, sel_registerName("objectAtIndex:"), 0);
                    const char* utf8 = ((const char* (*)(id, SEL))objc_msgSend)(firstStr, sel_registerName("UTF8String"));
                    if (utf8) {
                        NativeEvent ne; ne.type=6; // EventDrop (text)
                        strncpy(ne.text, utf8, sizeof(ne.text)-1);
                        ne.text[sizeof(ne.text)-1] = '\0';
                        eq_push(&cw->eq, &ne);
                    }
                }
            }
            return YES;
        });
        class_addMethod(viewClass, sel, imp, "c@:@");
    }

    objc_registerClassPair(viewClass);
}

// --- Cocoa window creation ---
static CocoaWindow* cocoa_create_window(int width, int height, const char* ctitle) {
    ensureViewClass();

    // NSApplication sharedApplication
    id app = ((id(*)(id, SEL))objc_msgSend)((id)objc_getClass("NSApplication"), sel_registerName("sharedApplication"));
    [app setActivationPolicy:NSApplicationActivationPolicyRegular];

    // Allocate CocoaWindow
    CocoaWindow* cw = (CocoaWindow*)calloc(1, sizeof(CocoaWindow));
    cw->width = width;
    cw->height = height;
    cw->scale = 1.0;
    cw->head = 0; cw->tail = 0;

    // Create content rect
    CGRect contentRect = CGRectMake(0, 0, width, height);
    id nsRect = ((id(*)(id, SEL, CGRect))objc_msgSend)((id)objc_getClass("NSValue"), sel_registerName("valueWithRect:"), contentRect);
    // Use NSValue's rectValue as NSRect for NSWindow init

    // NSWindow* — use simple style mask
    id win = ((id(*)(id, SEL, CGRect, int, int, int))objc_msgSend)(
        (id)objc_getClass("NSWindow"),
        sel_registerName("alloc")
    );
    unsigned long styleMask = 7; // titled + closable + miniaturizable + resizable
    win = ((id(*)(id, SEL, CGRect, int, int, BOOL))objc_msgSend)(
        win, sel_registerName("initWithContentRect:styleMask:backing:defer:"),
        contentRect, styleMask, 2, 0 // 2 = NSBackingStoreBuffered
    );
    cw->win = (void*)win;

    // Set title
    id titleStr = ((id(*)(id, SEL, const char*))objc_msgSend)(
        (id)objc_getClass("NSString"), sel_registerName("stringWithUTF8String:"), ctitle
    );
    ((void(*)(id, SEL, id))objc_msgSend)(win, sel_registerName("setTitle:"), titleStr);

    // Create GoCocoaView as contentView
    id view = ((id(*)(id, SEL, CGRect))objc_msgSend)((id)viewClass, sel_registerName("alloc"));
    view = ((id(*)(id, SEL, CGRect))objc_msgSend)(view, sel_registerName("initWithFrame:"), contentRect);
    cw->view = (void*)view;

    // Store CocoaWindow* in the view's ctxPtr iVar
    object_setInstanceVariable(view, "ctxPtr", cw);

    // Set wantsLayer = YES for layer-backed rendering
    ((void(*)(id, SEL, BOOL))objc_msgSend)(view, sel_registerName("setWantsLayer:"), 1);

    // Get the layer
    id layer = ((id(*)(id, SEL))objc_msgSend)(view, sel_registerName("layer"));
    cw->layer = (void*)layer;

    // Set view as contentView
    ((void(*)(id, SEL, id))objc_msgSend)(win, sel_registerName("setContentView:"), view);

    // Make key and order front
    ((void(*)(id, SEL))objc_msgSend)(win, sel_registerName("makeKeyAndOrderFront:"), win);

    // Get backing scale factor (Retina support)
    id screen = ((id(*)(id, SEL))objc_msgSend)(win, sel_registerName("screen"));
    if (screen) {
        CGFloat scale = ((CGFloat(*)(id, SEL))objc_msgSend)(screen, sel_registerName("backingScaleFactor"));
        if (scale < 1.0) scale = 1.0;
        cw->scale = (double)scale;
        // Set layer contents scale for Retina
        ((void(*)(id, SEL, CGFloat))objc_msgSend)(layer, sel_registerName("setContentsScale:"), scale);
    }

    // Activate app
    [app activateIgnoringOtherApps:YES];

    return cw;
}

// --- Poll events: pump the event loop once, then drain the event queue ---
static int cocoa_poll_events(CocoaWindow* cw, NativeEvent* out, int max) {
    if (!cw) return 0;
    // Pump NSApp event loop once (non-blocking)
    id app = ((id(*)(id, SEL))objc_msgSend)((id)objc_getClass("NSApplication"), sel_registerName("sharedApplication"));
    // Process one event if available (distantPast = non-blocking)
    id distantPast = ((id(*)(id, SEL))objc_msgSend)((id)objc_getClass("NSDate"), sel_registerName("distantPast"));
    id event = ((id(*)(id, SEL, unsigned long, id, id, BOOL))objc_msgSend)(
        app, sel_registerName("nextEventMatchingMask:untilDate:inMode:dequeue:"),
        0xFFFFFFFF, distantPast, ((id(*)(id, SEL))objc_msgSend)((id)objc_getClass("NSString"), sel_registerName("stringWithUTF8String:"), "kCFRunLoopDefaultMode"), 1
    );
    if (event) {
        ((void(*)(id, SEL, id))objc_msgSend)(app, sel_registerName("sendEvent:"), event);
    }

    // Drain event queue
    int count = 0;
    NativeEvent ne;
    while (count < max && eq_pop(&cw->eq, &ne)) {
        out[count++] = ne;
    }
    return count;
}

// --- Present: create CGImage from pixel data and set as layer contents ---
static void cocoa_present(CocoaWindow* cw, void* pixels, int w, int h) {
    if (!cw || !pixels || w <= 0 || h <= 0) return;

    // Create a CGImage from the RGBA pixel data using CGBitmapContext
    CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
    if (!cs) return;

    CGContextRef ctx = CGBitmapContextCreate(
        pixels, w, h, 8, w * 4, cs,
        kCGBitmapByteOrder32Big | kCGImageAlphaPremultipliedLast
    );
    CGColorSpaceRelease(cs);
    if (!ctx) return;

    CGImageRef cgImg = CGBitmapContextCreateImage(ctx);
    CGContextRelease(ctx);
    if (!cgImg) return;

    // Set as layer contents on the main thread
    id layer = (id)cw->layer;
    if (layer) {
        // dispatch_async to main queue
        dispatch_async(dispatch_get_main_queue(), ^{
            id contents = (id)cgImg;
            ((void(*)(id, SEL, id))objc_msgSend)(layer, sel_registerName("setContents:"), contents);
            CGImageRelease(cgImg);
        });
    } else {
        CGImageRelease(cgImg);
    }
}

// --- Close window ---
static void cocoa_close(CocoaWindow* cw) {
    if (!cw) return;
    if (cw->win) {
        id win = (id)cw->win;
        ((void(*)(id, SEL))objc_msgSend)(win, sel_registerName("close"));
        ((void(*)(id, SEL))objc_msgSend)(win, sel_registerName("release"));
    }
    cw->shouldClose = 1;
    free(cw);
}

// --- Focus / raise window ---
static void cocoa_focus(CocoaWindow* cw) {
    if (!cw || !cw->win) return;
    id win = (id)cw->win;
    ((void(*)(id, SEL))objc_msgSend)(win, sel_registerName("makeKeyAndOrderFront:"));
    ((void(*)(id, SEL))objc_msgSend)(win, sel_registerName("orderFront:"));
}
*/
import "C"

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/hoonfeng/goskia/skia"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/platform/ime"
)

// EventType identifies the kind of input event.
type EventType int

const (
	EventMouseButton EventType = iota
	EventChar
	EventCursorMove
	EventCursorLeave // 鼠标移出窗口（清除 hover/光标残留）
	EventKey
	EventResize
	EventScroll
	EventDrop
	EventTouch
)

// Event represents a single input event collected from the window.
type Event struct {
	Type    EventType
	X, Y    float64
	Button  int // 0=left, 1=right
	Action  int // 0=release, 1=press
	Key     int // macOS keyCode
	Mods    int
	Width   int // for EventResize
	Height  int
	ScrollY float64
	ScrollX float64
	DropFiles []string // for EventDrop
	TouchX, TouchY float64
	TouchID        int
	TouchAction    int // 0=end, 1=begin, 2=move
}

// Window is a Cocoa NSWindow with a Skia raster backend surface.
type Window struct {
	handle *C.CocoaWindow

	width, height        int
	fbWidth, fbHeight    int
	contentScaleX, contentScaleY float64

	canvas *graphics.Canvas

	ime ime.Handler

	events   []Event
	eventsMu sync.Mutex

	closeCallback func()
	dropCallback  func([]string)
	shouldClose   bool
}

// NewWindow creates a Cocoa NSWindow with the given dimensions and title.
func NewWindow(width, height int, title string) (*Window, error) {
	ctitle := C.CString(title)
	defer C.free(unsafe.Pointer(ctitle))

	cw := C.cocoa_create_window(C.int(width), C.int(height), ctitle)
	if cw == nil {
		return nil, fmt.Errorf("window cocoa: failed to create window")
	}

	scale := float64(cw.scale)
	physW := int(float64(width) * scale)
	physH := int(float64(height) * scale)

	canvas := graphics.NewCanvas(physW, physH)

	w := &Window{
		handle: cw,
		width: width, height: height,
		fbWidth: physW, fbHeight: physH,
		contentScaleX: scale, contentScaleY: scale,
		canvas: canvas,
	}
	// Wire up the NSTextInputClient IME handler with the NSView pointer.
	// The view's imeQueuePtr ivar is set by macosHandler.Init so the
	// text-input methods can push composition events into the queue.
	w.ime = ime.NewHandler()
	if w.ime != nil && cw.view != nil {
		w.ime.Init(uintptr(unsafe.Pointer(cw.view)))
	}
	return w, nil
}

// GPUSurface returns nil on the Cocoa raster backend.
func (w *Window) GPUSurface() *skia.Surface      { return nil }
func (w *Window) GPUContext() *skia.DirectContext { return nil }

// Canvas returns the CPU raster canvas backing this software-rendered window.
func (w *Window) Canvas() *graphics.Canvas { return w.canvas }

// Present reads the raster canvas and blits it to the NSView layer.
func (w *Window) Present() {
	if w.canvas == nil || w.handle == nil {
		return
	}
	pixels := w.canvas.Pixels()
	if len(pixels) < w.fbWidth*w.fbHeight*4 {
		return
	}
	C.cocoa_present(w.handle, unsafe.Pointer(&pixels[0]), C.int(w.fbWidth), C.int(w.fbHeight))
}

// Display draws a *skia.Image onto the NSWindow. Uses a temporary raster
// surface to read pixels, then blits to the view layer.
func (w *Window) Display(srcImg *skia.Image) {
	if srcImg == nil || w.handle == nil {
		return
	}
	imgW, imgH := srcImg.Width(), srcImg.Height()
	if imgW <= 0 || imgH <= 0 {
		return
	}
	tmpSurf, err := skia.NewRasterSurfaceN32Premul(imgW, imgH)
	if err != nil {
		return
	}
	defer tmpSurf.Release()
	tmpCvs := tmpSurf.Canvas()
	srcRect := skia.RectXYWH(0, 0, float32(imgW), float32(imgH))
	paint := skia.NewPaint()
	defer paint.Release()
	tmpCvs.DrawImageRect(srcImg, srcRect, srcRect, skia.SamplingLinear, paint)
	fromSurf := graphics.NewCanvasFromSurface(tmpSurf, imgW, imgH)
	if fromSurf == nil {
		return
	}
	defer fromSurf.Release()
	pixBuf := fromSurf.Pixels()
	if len(pixBuf) < imgW*imgH*4 {
		return
	}
	C.cocoa_present(w.handle, unsafe.Pointer(&pixBuf[0]), C.int(imgW), C.int(imgH))
}

// PollEvents processes Cocoa events and returns collected input events.
func (w *Window) PollEvents() []Event {
	if w.handle == nil {
		return nil
	}

	// Pump the Cocoa event loop
	var buf [64]C.NativeEvent
	count := C.cocoa_poll_events(w.handle, &buf[0], 64)

	w.eventsMu.Lock()
	defer w.eventsMu.Unlock()

	for i := 0; i < int(count); i++ {
		ne := buf[i]
		switch ne._type {
		case 0: // mouse button
			w.events = append(w.events, Event{
				Type: EventMouseButton,
				X: ne.x, Y: ne.y,
				Button: ne.button, Action: ne.action,
			})
		case 1: // cursor move / drag
			w.events = append(w.events, Event{
				Type: EventCursorMove,
				X: ne.x, Y: ne.y,
			})
		case 2: // key
			w.events = append(w.events, Event{
				Type: EventKey,
				Key: ne.key, Action: ne.action,
			})
		case 3: // resize
			newW, newH := int(ne.width), int(ne.height)
			if newW > 0 && newH > 0 {
				w.width = newW
				w.height = newH
				w.fbWidth = int(float64(newW) * w.contentScaleX)
				w.fbHeight = int(float64(newH) * w.contentScaleY)
				if w.canvas != nil {
					w.canvas.Release()
				}
				w.canvas = graphics.NewCanvas(w.fbWidth, w.fbHeight)
				w.events = append(w.events, Event{
					Type: EventResize, Width: newW, Height: newH,
				})
			}
		case 4: // scroll
			w.events = append(w.events, Event{
				Type: EventScroll,
				ScrollY: ne.scrollY,
			})
		case 5: // touch
			w.events = append(w.events, Event{
				Type:      EventTouch,
				X:         ne.x,
				Y:         ne.y,
				TouchID:   ne.key,
				TouchAction: ne.action,
			})
		case 6: // drop
			text := C.GoString(&ne.text[0])
			if text != "" {
				w.events = append(w.events, Event{
					Type:      EventDrop,
					DropFiles: []string{text},
				})
			}
		}
	}

	events := w.events
	w.events = nil
	return events
}

// ShouldClose reports whether the window has been asked to close.
func (w *Window) ShouldClose() bool {
	if w.handle == nil {
		return w.shouldClose
	}
	return w.handle.shouldClose != 0
}

// PostEvent appends an event to the internal queue, mirroring the GLFW
// backend's PostEvent. Used by tests and synthetic event injection.
func (w *Window) PostEvent(ev Event) {
	w.eventsMu.Lock()
	w.events = append(w.events, ev)
	w.eventsMu.Unlock()
}

// Focus brings the window to the front (Cocoa makeKeyAndOrderFront).
func (w *Window) Focus() {
	if w.handle == nil {
		return
	}
	C.cocoa_focus(w.handle)
}

// Width / Height return the window size in logical points (CSS pixels).
func (w *Window) Width() int                        { return w.width }
func (w *Window) Height() int                       { return w.height }
func (w *Window) FramebufferWidth() int             { return w.fbWidth }
func (w *Window) FramebufferHeight() int            { return w.fbHeight }
func (w *Window) ContentScale() (float64, float64)  { return w.contentScaleX, w.contentScaleY }
func (w *Window) SetCloseCallback(fn func())        { w.closeCallback = fn }
func (w *Window) SetDropCallback(fn func([]string)) { w.dropCallback = fn }
func (w *Window) IME() ime.Handler                  { return w.ime }
func (w *Window) PollIMEEvents() []ime.Event {
	if w.ime == nil {
		return nil
	}
	return w.ime.PopEvents()
}
func (w *Window) SetIMECompositionPos(x, y float64) {
	if w.ime == nil {
		return
	}
	scaleX, scaleY := 1.0, 1.0
	if w.width > 0 {
		scaleX = float64(w.fbWidth) / float64(w.width)
	}
	if w.height > 0 {
		scaleY = float64(w.fbHeight) / float64(w.height)
	}
	w.ime.SetCompositionPos(int32(x*scaleX), int32(y*scaleY))
}
func (w *Window) SetIMEEnabled(enabled bool) {
	if w.ime == nil {
		return
	}
	w.ime.SetEnabled(enabled)
}
func (w *Window) SetClipboardString(s string)       {}
func (w *Window) GetClipboardString() string        { return "" }

// Close destroys the window and releases resources.
func (w *Window) Close() {
	if w.canvas != nil {
		w.canvas.Release()
		w.canvas = nil
	}
	if w.handle != nil {
		C.cocoa_close(w.handle)
		w.handle = nil
	}
}
