package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestHelpFilterShowsOneActiveButton(t *testing.T) {
	a := newViewTestApp()
	a.openHelp()
	tt := ui.NewTester(a.view, 1320, 745)
	tt.Frame()
	all, ok := tt.Find("全部")
	if !ok {
		t.Fatal("全部 filter button not found")
	}
	browse, ok := tt.Find("浏览")
	if !ok {
		t.Fatal("浏览 filter button not found")
	}
	if gap := browse.X - (all.X + all.W); gap > 20 {
		t.Fatalf("gap between 全部 and 浏览 filters is %.1f DIP; an extra active button was built", gap)
	}
}

func TestOpenHelpResetsFilter(t *testing.T) {
	a := newViewTestApp()
	a.helpFilter = helpFilterVideo
	a.openHelp()
	if a.helpFilter != helpFilterAll {
		t.Fatalf("openHelp left filter at %d, want all", a.helpFilter)
	}
}
