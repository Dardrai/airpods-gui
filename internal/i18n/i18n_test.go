package i18n

import (
	"regexp"
	"slices"
	"testing"
)

func reset(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		SetLanguage(English)
		mu.Lock()
		listeners = nil
		mu.Unlock()
	})
}

// verbs are the formatting verbs of a text, sorted, with %% among them.
func verbs(text string) []string {
	found := verbPattern.FindAllString(text, -1)
	slices.Sort(found)
	return found
}

var verbPattern = regexp.MustCompile(`%(\[\d+\])?[a-z%]`)

func TestEveryLanguageHasEveryKeyAndTheSameVerbs(t *testing.T) {
	for code, table := range tables {
		for key, english := range tables[English] {
			text, ok := table[key]
			if !ok {
				t.Errorf("%s lacks %q", code, key)
				continue
			}
			if got, want := verbs(text), verbs(english); !slices.Equal(got, want) {
				t.Errorf("%s %q has verbs %v, English %v", code, key, got, want)
			}
		}
		for key := range table {
			if _, ok := tables[English][key]; !ok {
				t.Errorf("%s has an unknown key %q", code, key)
			}
		}
	}
}

func TestLanguagesChange(t *testing.T) {
	reset(t)
	if T("tray.quit") != "Quit" {
		t.Errorf("default %q", T("tray.quit"))
	}
	for code, want := range map[string]string{Portuguese: "Sair", French: "Quitter", Spanish: "Salir", German: "Beenden", English: "Quit"} {
		if got := SetLanguage(code); got != code || Language() != code || Active() != code {
			t.Errorf("%s: %q %q %q", code, got, Language(), Active())
		}
		if T("tray.quit") != want {
			t.Errorf("%s: %q", code, T("tray.quit"))
		}
	}
}

func TestMissingKeyFallsBack(t *testing.T) {
	reset(t)
	tables[English]["only.english"] = "English text"
	defer delete(tables[English], "only.english")
	SetLanguage(French)
	if got := T("only.english"); got != "English text" {
		t.Errorf("%q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("%q", got)
	}
}

func TestSystemLanguage(t *testing.T) {
	reset(t)
	old := systemLanguage
	defer func() { systemLanguage = old }()
	for locale, want := range map[string]string{"pt-BR": Portuguese, "fr_CA": French, "es": Spanish, "de": German, "de-DE": German, "de_DE.UTF-8": German, "de_CH": German, "de_AT": German, "it-IT": English, "": English, "system": English} {
		systemLanguage = func() string { return locale }
		if got := SetLanguage(System); got != System || Active() != want {
			t.Errorf("%q: %q active %q, want %q", locale, got, Active(), want)
		}
	}
	systemLanguage = func() string { return "es" }
	if got := SetLanguage("klingon"); got != System || Active() != Spanish {
		t.Errorf("unknown code: %q %q", got, Active())
	}
}

func TestListenersHearChanges(t *testing.T) {
	reset(t)
	var heard []string
	OnChange(func() { heard = append(heard, T("tray.quit")) })
	SetLanguage(French)
	SetLanguage(Spanish)
	if len(heard) != 2 || heard[0] != "Quitter" || heard[1] != "Salir" {
		t.Errorf("%q", heard)
	}
}

func TestUnsubscribeStopsAListener(t *testing.T) {
	reset(t)
	var first, second int
	unsubscribe := OnChange(func() { first++ })
	OnChange(func() { second++ })
	SetLanguage(French)
	unsubscribe()
	unsubscribe()
	SetLanguage(Spanish)
	if first != 1 || second != 2 {
		t.Errorf("first heard %d, second %d", first, second)
	}
}

func TestAListenerCanUnsubscribeItselfWhileCalled(t *testing.T) {
	reset(t)
	var unsubscribe func()
	calls := 0
	unsubscribe = OnChange(func() {
		calls++
		unsubscribe()
	})
	SetLanguage(French)
	SetLanguage(Spanish)
	if calls != 1 {
		t.Errorf("called %d times", calls)
	}
}

func TestChoices(t *testing.T) {
	reset(t)
	choices := Choices()
	if len(choices) != 6 || choices[0].Code != System || choices[0].Name != "System" || choices[2].Name != "Português" || choices[5].Code != German || choices[5].Name != "Deutsch" {
		t.Errorf("%+v", choices)
	}
}
