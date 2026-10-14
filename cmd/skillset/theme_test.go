package main

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestEveryThemeFillsEveryColourField walks the struct rather than a list of
// names, so a role added later can't be left empty in a theme and render black
// on black until someone runs that theme.
func TestEveryThemeFillsEveryColourField(t *testing.T) {
	defer useTheme(defaultTheme)
	colour := reflect.TypeOf(lipgloss.Color(""))
	for name, th := range themes {
		v := reflect.ValueOf(th)
		for i := range v.NumField() {
			if v.Type().Field(i).Type != colour {
				continue
			}
			if c := v.Field(i).String(); len(c) != 7 || c[0] != '#' {
				t.Errorf("theme %q has %s = %q, want a hex colour", name, v.Type().Field(i).Name, c)
			}
		}
	}
}

// TestThemeFallsBackToTheDefault: a name nobody ever added must still give a
// usable screen rather than a blank one.
func TestThemeFallsBackToTheDefault(t *testing.T) {
	defer useTheme(defaultTheme)
	for _, name := range []string{"", "nineties", defaultTheme} {
		useTheme(name)
		if ui.theme.Accent == "" || len(ui.rules) == 0 {
			t.Fatalf("theme %q left the screen without colours: %+v", name, ui.theme)
		}
	}
}

// TestThemeSurvivesASave: the field is this machine's, and an apply must not eat it.
func TestThemeSurvivesASave(t *testing.T) {
	_, repo := setup(t)
	cfg := &Config{Repo: repo, Theme: "atlas"}
	if err := cfg.save(); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig()
	if err != nil || got.Theme != "atlas" {
		t.Fatalf("theme = %q (%v), want it to round-trip", got.Theme, err)
	}
}

// TestSourceRulesCycle: more sources than colours must cycle, and a theme that
// sets no rules at all must still draw a row rather than panic.
func TestSourceRulesCycle(t *testing.T) {
	useTheme(defaultTheme)
	if n := len(ui.rules); n < 2 || ui.rule(0) != ui.rule(n) || ui.rule(0) == "" {
		t.Fatalf("%d rules, and rule 0 and rule %d should be the same one", n, n)
	}
	if bare := (styles{}).rule(0); bare == "" {
		t.Fatal("a theme without rules should still give the row something to start with")
	}
}
