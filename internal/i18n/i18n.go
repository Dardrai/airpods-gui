// Package i18n holds the app's texts and the chosen language. Texts are looked up by key;
// a key a language lacks falls back to English, and a key nobody has shows as itself.
package i18n

import (
	"slices"
	"strings"
	"sync"

	"fyne.io/fyne/v2/lang"
)

// Language codes. System follows the desktop's language.
const (
	System     = "system"
	English    = "en"
	Portuguese = "pt"
	French     = "fr"
	Spanish    = "es"
	German     = "de"
)

// Preference is the name of the persisted language choice.
const Preference = "language"

// Choice is one entry of a language picker. Languages are named in their own language;
// System's name is a key, as it reads in the current one.
type Choice struct {
	Code string
	Name string
}

// Choices lists the languages in picker order.
func Choices() []Choice {
	return []Choice{
		{System, T("language.system")},
		{English, "English"},
		{Portuguese, "Português"},
		{French, "Français"},
		{Spanish, "Español"},
		{German, "Deutsch"},
	}
}

// tables holds the texts of each language, one file each; English is the fallback.
var tables = map[string]map[string]string{
	English:    english,
	Portuguese: portuguese,
	French:     french,
	Spanish:    spanish,
	German:     german,
}

var (
	mu        sync.RWMutex
	chosen    = English
	active    = English
	listeners []*listener
	// systemLanguage asks the desktop; tests replace it.
	systemLanguage = func() string { return lang.SystemLocale().String() }
)

// Base reduces a locale such as pt-BR or fr_CA to a supported language, English if it
// is none of them.
func Base(locale string) string {
	code := strings.ToLower(locale)
	if i := strings.IndexAny(code, "-_."); i >= 0 {
		code = code[:i]
	}
	if _, ok := tables[code]; ok {
		return code
	}
	return English
}

// SetLanguage selects a language by code; anything unknown means System. It returns the
// code in effect as a choice and tells the listeners afterwards.
func SetLanguage(code string) string {
	effective := code
	if _, ok := tables[code]; !ok {
		code, effective = System, Base(systemLanguage())
	}
	mu.Lock()
	chosen, active = code, effective
	notified := append([]*listener{}, listeners...)
	mu.Unlock()
	for _, entry := range notified {
		entry.call()
	}
	return code
}

// Language returns the choice, which can be System.
func Language() string {
	mu.RLock()
	defer mu.RUnlock()
	return chosen
}

// Active returns the language texts are coming from.
func Active() string {
	mu.RLock()
	defer mu.RUnlock()
	return active
}

// listener is one OnChange registration; its address tells it from the others.
type listener struct{ call func() }

// OnChange adds a function called after each SetLanguage, on the caller's goroutine. What it
// returns removes the function again, and is safe to call more than once.
func OnChange(call func()) (unsubscribe func()) {
	entry := &listener{call: call}
	mu.Lock()
	defer mu.Unlock()
	listeners = append(listeners, entry)
	return func() {
		mu.Lock()
		defer mu.Unlock()
		listeners = slices.DeleteFunc(listeners, func(other *listener) bool { return other == entry })
	}
}

// Has reports whether a key has a text in English, which every key must.
func Has(key string) bool {
	_, ok := tables[English][key]
	return ok
}

// T returns the text for a key.
func T(key string) string {
	mu.RLock()
	table := tables[active]
	mu.RUnlock()
	if text, ok := table[key]; ok {
		return text
	}
	if text, ok := tables[English][key]; ok {
		return text
	}
	return key
}
