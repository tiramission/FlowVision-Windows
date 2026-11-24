package main

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSideBindingsDefaultForOldSettings(t *testing.T) {
	old := []byte(`{"theme":"dark","sidePrev":{"key":0,"mods":0},"sideNext":{"key":0,"mods":0}}`)
	s, ok := parseSettings(old)
	if !ok {
		t.Fatal("old settings did not parse")
	}
	if s.SidePrev != defaultSidePrev() || s.SideNext != defaultSideNext() {
		t.Fatalf("side bindings = %+v / %+v, want defaults", s.SidePrev, s.SideNext)
	}
}

func TestSideBindingsNeverShareOneKey(t *testing.T) {
	prev, next := mergeSideBindings(
		SideBinding{Key: ui.KeyBack},
		SideBinding{Key: ui.KeyBack},
		defaultSidePrev(),
		defaultSideNext(),
	)
	if prev == next {
		t.Fatalf("bindings still share %v", prev)
	}
	if prev != defaultSidePrev() || next != defaultSideNext() {
		t.Fatalf("duplicate bindings did not reset to defaults: %+v %+v", prev, next)
	}
}

func TestApplySideBindingAssignsAndMovesTheOtherDirection(t *testing.T) {
	prev, next := defaultSidePrev(), defaultSideNext()
	applySideBinding(&prev, &next, "prev", SideBinding{Key: ui.KeyForward})
	if prev.Key != ui.KeyForward || next.Key != ui.KeyBack {
		t.Fatalf("after assigning forward to previous: prev=%+v next=%+v", prev, next)
	}
	applySideBinding(&prev, &next, "next", SideBinding{Key: ui.KeyPageUp})
	if next.Key != ui.KeyPageUp || prev.Key != ui.KeyForward {
		t.Fatalf("after assigning PageUp to next: prev=%+v next=%+v", prev, next)
	}
}

func TestSideBindingLabels(t *testing.T) {
	cases := []struct {
		b    SideBinding
		want string
	}{
		{defaultSidePrev(), "后退键"},
		{defaultSideNext(), "前进键"},
		{SideBinding{Key: ui.KeyPageUp}, "Page Up"},
		{SideBinding{Key: ui.KeyJ, Mods: ui.Ctrl}, "Ctrl + J"},
		{SideBinding{}, "未设置"},
	}
	for _, tc := range cases {
		if got := sideBindingLabel(tc.b); got != tc.want {
			t.Errorf("sideBindingLabel(%+v) = %q, want %q", tc.b, got, tc.want)
		}
	}
}

func TestViewerReservedSideBindings(t *testing.T) {
	for _, key := range []ui.Key{ui.KeyLeft, ui.KeyRight, ui.KeySpace, ui.KeyF1, ui.KeyF11, ui.KeyI} {
		if viewerSideBindingAllowed(SideBinding{Key: key}) {
			t.Errorf("viewer shortcut key %d accepted as a side binding", key)
		}
	}
	for _, key := range []ui.Key{ui.KeyBack, ui.KeyForward, ui.KeyPageUp, ui.KeyPageDown} {
		if !viewerSideBindingAllowed(SideBinding{Key: key}) {
			t.Errorf("mouse side key %d rejected", key)
		}
	}
}

func TestSideCalibrationRecalibrateRestoresCaptureFocus(t *testing.T) {
	a := newViewTestApp()
	a.settings = defaultSettings()
	a.draft = a.settings
	a.showSettings = true
	a.beginSideCalibration("next")
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	tt.KeyDown(ui.KeyBack)
	tt.Frame()
	tt.KeyUp(ui.KeyBack)
	tt.Frame()
	if err := tt.Click("重新识别"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.KeyDown(ui.KeyPageDown)
	tt.Frame()
	if a.sideCaptureBinding.Key != ui.KeyPageDown {
		t.Fatalf("recalibration captured key = %d, want PageDown", a.sideCaptureBinding.Key)
	}
}

func TestSideCalibrationModalCapturesBackKey(t *testing.T) {
	a := newViewTestApp()
	a.settings = defaultSettings()
	a.draft = a.settings
	a.showSettings = true
	a.beginSideCalibration("prev")
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	tt.KeyDown(ui.KeyBack)
	tt.Frame()
	tt.KeyUp(ui.KeyBack)
	tt.Frame()
	if a.sideCaptureBinding.Key != ui.KeyBack {
		t.Fatalf("captured key = %d, want KeyBack", a.sideCaptureBinding.Key)
	}
	if err := tt.Click("应用侧键"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.sideCalibrationOpen {
		t.Fatal("calibration modal stayed open after applying")
	}
	if a.draft.SidePrev.Key != ui.KeyBack || a.draft.SideNext.Key != ui.KeyForward {
		t.Fatalf("draft bindings = %+v / %+v", a.draft.SidePrev, a.draft.SideNext)
	}
}

func TestSettingsJSONKeepsSideBindings(t *testing.T) {
	s := defaultSettings()
	s.SidePrev = SideBinding{Key: ui.KeyPageUp, Mods: ui.Ctrl}
	s.SideNext = SideBinding{Key: ui.KeyPageDown}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := parseSettings(data)
	if !ok {
		t.Fatal("serialized settings did not parse")
	}
	if got.SidePrev != s.SidePrev || got.SideNext != s.SideNext {
		t.Fatalf("round trip lost bindings: %+v / %+v", got.SidePrev, got.SideNext)
	}
}
