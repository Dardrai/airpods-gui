package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Theme preference values.
const (
	ThemeDark   = "dark"
	ThemeLight  = "light"
	ThemeSystem = "system"
)

const themePreference = "theme"

// defaultTheme is what the app uses until a theme is chosen, and what the picker then shows.
const defaultTheme = ThemeDark

func themeFor(name string) fyne.Theme {
	switch name {
	case ThemeLight:
		return pageTheme{Theme: theme.DefaultTheme(), forced: true, variant: theme.VariantLight}
	case ThemeSystem:
		return pageTheme{Theme: theme.DefaultTheme()}
	}
	return pageTheme{Theme: theme.DefaultTheme(), forced: true, variant: theme.VariantDark}
}

// ApplyTheme activates the stored theme.
func ApplyTheme(app fyne.App) {
	app.Settings().SetTheme(themeFor(app.Preferences().StringWithFallback(themePreference, defaultTheme)))
}
