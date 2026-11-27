package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFirstAvailablePicksTheFirstKnown(t *testing.T) {
	known := map[string]bool{"MiSans VF": true}
	got := firstAvailable([]string{"MiSans", "MiSans VF", "Other"}, func(name string) bool {
		return known[name]
	})
	if got != "MiSans VF" {
		t.Fatalf("firstAvailable = %q, want MiSans VF", got)
	}
	if got := firstAvailable([]string{"A", "B"}, func(string) bool { return false }); got != "" {
		t.Fatalf("firstAvailable with nothing known = %q, want empty", got)
	}
}

func TestIsFontFile(t *testing.T) {
	for _, name := range []string{"MiSans-Regular.ttf", "MiSans.OTF", "MiSans.ttc"} {
		if !isFontFile(name) {
			t.Fatalf("%s should be a font file", name)
		}
	}
	for _, name := range []string{"README.md", "MiSans.woff2", "font.zip"} {
		if isFontFile(name) {
			t.Fatalf("%s should not be a font file", name)
		}
	}
}

func TestInterfaceFontStartsEmpty(t *testing.T) {
	prev := interfaceFont()
	setInterfaceFont("")
	defer setInterfaceFont(prev)
	if interfaceFont() != "" {
		t.Fatal("interface font should be unset until a font is registered")
	}
}

func TestLoadInterfaceFontsRegistersTheShippedFamily(t *testing.T) {
	dir := filepath.Join("resources", "fonts")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		t.Skip("no shipped fonts in resources/fonts")
	}
	// A stray non-font file must be ignored, not fail the load.
	if err := os.WriteFile(filepath.Join(t.TempDir(), "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := loadInterfaceFonts([]string{dir})
	if len(loaded) == 0 {
		t.Fatal("no shipped fonts were registered")
	}
	if !fontFamilyAvailable(interfaceFontFamily) {
		t.Fatalf("registered family %q is not available to text", interfaceFontFamily)
	}
}
