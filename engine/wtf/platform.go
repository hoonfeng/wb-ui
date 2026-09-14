// Translation of: Source/WTF/wtf/Platform.h
//                  Source/WTF/wtf/PlatformOS.h
// Completeness: 75%
// Simplifications:
//   - WebKit's layered Platform{CPU,OS,Have,Use,Enable}.h collapse into Go runtime.GOOS/GOARCH
//   - HAVE()/USE()/ENABLE() feature macros are not modelled here (they belong to specific ports)

package wtf

import "runtime"

// Platform.h is the umbrella header that pulls in PlatformCPU.h, PlatformOS.h,
// PlatformHave.h, PlatformUse.h and PlatformEnable.h to describe the target
// environment (OS, CPU, available features, enabled features). In Go the equivalent
// information is exposed by the standard "runtime" package (GOOS / GOARCH), so this
// file reduces the OS/CPU detection to that single source of truth. The HAVE / USE /
// ENABLE policy macros are intentionally omitted; they will be added per-feature when
// downstream phases actually need them.

// OSKind enumerates the operating systems recognised by WebKit's PlatformOS.h.
type OSKind int

const (
	// OSOther covers any OS not explicitly listed below.
	OSOther OSKind = iota
	OSMacOS
	OSIOS
	OSLinux
	OSWindows
	OSBSD
	OSAndroid
	OSFuchsia
)

// ArchKind enumerates the CPU architectures recognised by WebKit's PlatformCPU.h.
type ArchKind int

const (
	// ArchOther covers any CPU not explicitly listed below.
	ArchOther ArchKind = iota
	ArchX86
	ArchX86_64
	ArchARM
	ArchARM64
	ArchWasm
)

// CurrentOS returns the OSKind derived from runtime.GOOS. It mirrors the role of the
// OS(...) macro family in PlatformOS.h.
func CurrentOS() OSKind {
	switch runtime.GOOS {
	case "darwin":
		// WebKit distinguishes macOS from iOS here; Go also reports "ios" on iOS.
		return OSMacOS
	case "ios":
		return OSIOS
	case "linux":
		return OSLinux
	case "windows":
		return OSWindows
	case "android":
		return OSAndroid
	case "freebsd", "netbsd", "openbsd", "dragonfly":
		return OSBSD
	case "fuchsia":
		return OSFuchsia
	default:
		return OSOther
	}
}

// CurrentArch returns the ArchKind derived from runtime.GOARCH. It mirrors the role
// of the CPU(...) macro family in PlatformCPU.h.
func CurrentArch() ArchKind {
	switch runtime.GOARCH {
	case "386":
		return ArchX86
	case "amd64":
		return ArchX86_64
	case "arm":
		return ArchARM
	case "arm64":
		return ArchARM64
	case "wasm", "wasm32", "wasm64":
		return ArchWasm
	default:
		return ArchOther
	}
}

// IsMacOS reports whether the build targets macOS (mirrors OS(MACOS)).
func IsMacOS() bool { return runtime.GOOS == "darwin" }

// IsIOS reports whether the build targets iOS (mirrors OS(IOS)).
func IsIOS() bool { return runtime.GOOS == "ios" }

// IsLinux reports whether the build targets Linux (mirrors OS(LINUX)).
func IsLinux() bool { return runtime.GOOS == "linux" }

// IsWindows reports whether the build targets Windows (mirrors OS(WINDOWS)).
func IsWindows() bool { return runtime.GOOS == "windows" }

// IsBSD reports whether the build targets a BSD-derived OS (mirrors OS(FREEBSD) etc.).
func IsBSD() bool {
	switch runtime.GOOS {
	case "freebsd", "netbsd", "openbsd", "dragonfly":
		return true
	default:
		return false
	}
}

// IsApple reports whether the build targets an Apple OS (macOS or iOS), mirroring
// WebKit's PLATFORM(COCOA) family at a coarse granularity.
func IsApple() bool { return IsMacOS() || IsIOS() }
