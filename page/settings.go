// Translation of: Source/WebCore/page/SettingsBase.h
//                  Source/WebCore/page/Settings.yaml
//                  Source/WebCore/page/SettingsBase.cpp
// Completeness: 40%
// Simplifications:
//   - only a representative subset of WebKit's settings is modeled as typed fields;
//     WebKit generates hundreds of getters/setters from Settings.yaml
//   - fields are exported for direct mutation rather than generated set/get pairs
//   - no per-page invalidation / recalc-style scheduling when a setting changes
//   - no font-generic-families map, no media-content-type filtering

package page

// Settings is the Go translation of WebCore::Settings (which subclasses
// SettingsBase). It holds the browser-wide configuration that influences how a
// Page renders and behaves: font sizes, feature toggles for scripting, images,
// plug-ins, animations, viewport handling and so on. In WebKit the full set is
// generated from Settings.yaml and each setter notifies the owning Page so it can
// invalidate style; this port models the commonly used flags directly and leaves
// invalidation to the caller (re-run layout / rebuild the render tree as needed).
type Settings struct {
	// Font sizes, mirroring Settings::minimumFontSize() / defaultFontSize() /
	// defaultFixedFontSize(). Values are in CSS pixels.
	MinimumFontSize       float64
	DefaultFontSize       float64
	DefaultFixedFontSize  float64
	MinimumLogicalFontSize float64

	// MaximumRenderTreeDepth mirrors SettingsBase::defaultMaximumRenderTreeDepth.
	MaximumRenderTreeDepth int

	// Feature toggles. These map 1:1 to the similarly named WebKit booleans.
	JavaScriptEnabled             bool
	LoadsImagesAutomatically      bool
	PluginsEnabled                bool
	CSSAnimationEnabled           bool
	AllowsAirPlayForMediaPlayback bool
	TextAutosizingEnabled         bool
	ViewportEnabled               bool

	// JavaScriptCanOpenWindowsAutomatically mirrors
	// Settings::javaScriptCanOpenWindowsAutomatically().
	JavaScriptCanOpenWindowsAutomatically bool

	// AllowFileAccessFromFileURLs / AllowUniversalAccessFromFileURLs mirror the
	// WebKit security settings governing local-file URL access.
	AllowFileAccessFromFileURLs       bool
	AllowUniversalAccessFromFileURLs   bool

	// LoadsSiteIconsSeparately mirrors Settings::loadsSiteIconsSeparately().
	LoadsSiteIconsSeparately bool

	// ShouldPrintBackgrounds mirrors Settings::shouldPrintBackgrounds().
	ShouldPrintBackgrounds bool

	// UseSystemAppearance mirrors Settings::useSystemAppearance().
	UseSystemAppearance bool

	// MediaEnabled mirrors Settings::mediaEnabled().
	MediaEnabled bool
}

// NewSettings constructs a Settings value with the WebKit defaults applied,
// mirroring SettingsBase::SettingsBase() plus the per-setting default initializers
// declared in Settings.yaml. The returned pointer is ready to use as-is.
func NewSettings() *Settings {
	return &Settings{
		MinimumFontSize:                        0,
		DefaultFontSize:                        16,
		DefaultFixedFontSize:                   13,
		MinimumLogicalFontSize:                 9,
		MaximumRenderTreeDepth:                 512,
		JavaScriptEnabled:                      true,
		LoadsImagesAutomatically:               true,
		PluginsEnabled:                         false,
		CSSAnimationEnabled:                    true,
		AllowsAirPlayForMediaPlayback:          false,
		TextAutosizingEnabled:                  false,
		ViewportEnabled:                        true,
		JavaScriptCanOpenWindowsAutomatically:  false,
		AllowFileAccessFromFileURLs:            false,
		AllowUniversalAccessFromFileURLs:       false,
		LoadsSiteIconsSeparately:               true,
		ShouldPrintBackgrounds:                 false,
		UseSystemAppearance:                    false,
		MediaEnabled:                           true,
	}
}

// ResetToConsistentState restores the WebKit defaults, mirroring
// SettingsBase::resetToConsistentState(). It is a no-op on the page field (which
// is owned by the Page) and only resets the value fields.
func (s *Settings) ResetToConsistentState() {
	*s = *NewSettings()
}
