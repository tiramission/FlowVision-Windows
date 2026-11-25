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
	if s.SidePrevHold.Key != ui.KeyUnknown || s.SideNextHold.Key != ui.KeyUnknown {
		t.Fatalf("old settings invented long-press bindings: %+v / %+v", s.SidePrevHold, s.SideNextHold)
	}
	if s.ViewerPreload != 2 {
		t.Fatalf("old settings preload = %d, want default 2", s.ViewerPreload)
	}
}

func TestViewerPreloadAllowsExplicitOff(t *testing.T) {
	raw := []byte(`{"viewerPreload":0}`)
	s, ok := parseSettings(raw)
	if !ok {
		t.Fatal("settings with preload 0 did not parse")
	}
	if s.ViewerPreload != 0 {
		t.Fatalf("explicit preload 0 became %d", s.ViewerPreload)
	}
}

func TestViewerBindingsNeverShareOneClickOrHold(t *testing.T) {
	prev, next, prevHold, nextHold := mergeViewerBindings(
		SideBinding{Key: ui.KeyBack},
		SideBinding{Key: ui.KeyBack},
		SideBinding{Key: ui.KeyPageUp},
		SideBinding{Key: ui.KeyPageUp},
	)
	if prev == next {
		t.Fatalf("click bindings still share %v", prev)
	}
	if prev != defaultSidePrev() || next != defaultSideNext() {
		t.Fatalf("duplicate click bindings did not reset: %+v %+v", prev, next)
	}
	if prevHold.Key != ui.KeyPageUp || nextHold.Key != ui.KeyUnknown {
		t.Fatalf("duplicate hold bindings not cleared: %+v %+v", prevHold, nextHold)
	}
}

func TestViewerBindingsLetClickAndHoldShareSideKey(t *testing.T) {
	prev, next, prevHold, nextHold := mergeViewerBindings(
		SideBinding{Key: ui.KeyBack},
		SideBinding{Key: ui.KeyForward},
		SideBinding{Key: ui.KeyForward},
		SideBinding{Key: ui.KeyBack},
	)
	if prev != (SideBinding{Key: ui.KeyBack}) || next != (SideBinding{Key: ui.KeyForward}) {
		t.Fatalf("click bindings changed: %+v %+v", prev, next)
	}
	if prevHold != (SideBinding{Key: ui.KeyForward}) || nextHold != (SideBinding{Key: ui.KeyBack}) {
		t.Fatalf("click and hold sharing a side key was rejected: %+v %+v", prevHold, nextHold)
	}
}

func TestApplyViewerBindingMovesDuplicateClick(t *testing.T) {
	prev, next := defaultSidePrev(), defaultSideNext()
	prevHold, nextHold := SideBinding{}, SideBinding{}
	assignViewerBinding(&prev, &next, &prevHold, &nextHold, "prevClick", SideBinding{Key: ui.KeyForward})
	if prev.Key != ui.KeyForward || next.Key != ui.KeyBack {
		t.Fatalf("after assigning forward to previous: prev=%+v next=%+v", prev, next)
	}
	assignViewerBinding(&prev, &next, &prevHold, &nextHold, "nextClick", SideBinding{Key: ui.KeyPageUp})
	if next.Key != ui.KeyPageUp || prev.Key != ui.KeyForward {
		t.Fatalf("after assigning PageUp to next: prev=%+v next=%+v", prev, next)
	}
	assignViewerBinding(&prev, &next, &prevHold, &nextHold, "prevHold", SideBinding{Key: ui.KeyForward})
	if prevHold.Key != ui.KeyForward || nextHold.Key != ui.KeyUnknown {
		t.Fatalf("hold assignment = %+v %+v", prevHold, nextHold)
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

func TestSettingsInlineBindingCapture(t *testing.T) {
	a := newViewTestApp()
	a.settings = defaultSettings()
	a.draft = a.settings
	a.showSettings = true
	a.settingsUI.Category = "control"
	a.beginBindingCapture("prevClick")
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	tt.KeyDown(ui.KeyPageUp)
	tt.Frame()
	tt.KeyUp(ui.KeyPageUp)
	tt.Frame()
	if a.draft.SidePrev.Key != ui.KeyPageUp {
		t.Fatalf("captured key = %d, want PageUp", a.draft.SidePrev.Key)
	}
	if a.bindingCapture != "" {
		t.Fatalf("capture stayed on %q after applying", a.bindingCapture)
	}
}

func TestSettingsInlineBindingCaptureRejectsViewerKey(t *testing.T) {
	a := newViewTestApp()
	a.settings = defaultSettings()
	a.draft = a.settings
	a.showSettings = true
	a.settingsUI.Category = "control"
	a.applyBindingCapture("prevClick", SideBinding{Key: ui.KeyRight})
	if a.draft.SidePrev != defaultSidePrev() {
		t.Fatalf("reserved key changed binding to %+v", a.draft.SidePrev)
	}
	if a.bindingCaptureNote == "" {
		t.Fatal("reserved key produced no explanation")
	}
}

func TestSettingsJSONKeepsViewerBindings(t *testing.T) {
	s := defaultSettings()
	s.SidePrev = SideBinding{Key: ui.KeyPageUp, Mods: ui.Ctrl}
	s.SideNext = SideBinding{Key: ui.KeyPageDown}
	s.SidePrevHold = SideBinding{Key: ui.KeyForward}
	s.SideNextHold = SideBinding{Key: ui.KeyBack}
	s.ViewerPreload = 4
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := parseSettings(data)
	if !ok {
		t.Fatal("serialized settings did not parse")
	}
	if got.SidePrev != s.SidePrev || got.SideNext != s.SideNext ||
		got.SidePrevHold != s.SidePrevHold || got.SideNextHold != s.SideNextHold {
		t.Fatalf("round trip lost bindings: %+v %+v %+v %+v", got.SidePrev, got.SideNext, got.SidePrevHold, got.SideNextHold)
	}
	if got.ViewerPreload != 4 {
		t.Fatalf("round trip preload = %d, want 4", got.ViewerPreload)
	}
}

func TestViewerSideMouseClickAndHold(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.settings.SidePrev = SideBinding{Key: ui.KeyBack}
	a.settings.SideNext = SideBinding{Key: ui.KeyForward}
	a.settings.SidePrevHold = SideBinding{}
	a.settings.SideNextHold = SideBinding{Key: ui.KeyBack}
	a.viewer.Index = 0
	a.viewer.Item = a.tab().Items[0]

	// A short back-button press still moves to the previous item.
	a.viewer.Index = 1
	a.viewer.Item = a.tab().Items[1]
	a.viewerSideMouseEvent(1, true)
	a.viewerSideMouseEvent(1, false)
	if a.viewer.Index != 0 {
		t.Fatalf("short click went to index %d, want 0", a.viewer.Index)
	}

	// Holding the same back button runs its long-press action instead, which is
	// configured as next here. Release must not also run the short action.
	a.viewer.Index = 0
	a.viewer.Item = a.tab().Items[0]
	a.viewerSideMouseEvent(1, true)
	a.fireSideMouseHold(1)
	a.viewerSideMouseEvent(1, false)
	if a.viewer.Index != 1 {
		t.Fatalf("long press went to index %d, want 1", a.viewer.Index)
	}
}

func TestViewerSideMouseIgnoresRepeatedDown(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.settings.SideNextHold = SideBinding{Key: ui.KeyBack}
	a.viewer.Index = 0
	a.viewer.Item = a.tab().Items[0]
	a.viewerSideMouseEvent(1, true)
	a.viewerSideMouseEvent(1, true)
	a.fireSideMouseHold(1)
	a.viewerSideMouseEvent(1, false)
	if a.viewer.Index != 1 {
		t.Fatalf("repeated down moved to index %d, want one step", a.viewer.Index)
	}
}

func TestViewerSideMouseIgnoredOverSettings(t *testing.T) {
	a := newViewerGestureApp(t)
	defer a.closeViewer()
	a.settings.SidePrev = SideBinding{Key: ui.KeyBack}
	a.viewer.Index = 1
	a.viewer.Item = a.tab().Items[1]
	a.showSettings = true
	a.viewerSideMouseEvent(1, true)
	a.viewerSideMouseEvent(1, false)
	if a.viewer.Index != 1 {
		t.Fatalf("side button navigated behind settings to index %d", a.viewer.Index)
	}
	a.showSettings = false
	a.viewerSideMouseEvent(1, true)
	a.viewerSideMouseEvent(1, false)
	if a.viewer.Index != 0 {
		t.Fatalf("side button stopped working after settings closed: index %d", a.viewer.Index)
	}
}
