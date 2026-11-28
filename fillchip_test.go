package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// chipApp builds a browser view with a loaded tab holding n pictures and the
// fill marked stopped, so the grid frame does not queue real work or drop the
// task a test is inspecting.
func chipApp(t *testing.T, n int) *App {
	t.Helper()
	a := newViewTestApp()
	thumbDir = t.TempDir()
	thumbSlots = make(chan struct{}, 4)
	a.thumbQueue = newThumbQueue()
	a.thumbOnce.Do(func() {})

	tab := a.tab()
	tab.All, tab.Items = nil, nil
	tab.Loaded = true
	for i := 0; i < n; i++ {
		it := &Item{ID: "chip-" + string(rune('a'+i%26)), Name: "p.png", Kind: "image", Size: 1, W: 10, H: 10}
		tab.All = append(tab.All, it)
		tab.Items = append(tab.Items, it)
	}
	a.viewer = Viewer{}
	a.fillStoppedTab, a.fillStoppedGen = tab, tab.Gen
	return a
}

// TestFillChipIsResident covers the point of the change: the chip is a readout
// that is always in the bar, not something that only appears while work runs.
func TestFillChipIsResident(t *testing.T) {
	a := chipApp(t, 3)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("补载 已停止") {
		t.Fatalf("the idle chip is missing: %q", tt.Texts())
	}

	// An empty folder still has a chip, with a dash rather than a stale claim.
	b := chipApp(t, 0)
	tt = ui.NewTester(b.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("补载 —") {
		t.Fatalf("an empty folder has no chip: %q", tt.Texts())
	}
}

// TestFillChipNamesTheReasonItIsQuiet covers the different reasons nothing is
// being filled: each is a different answer, so the chip names which one.
func TestFillChipNamesTheReasonItIsQuiet(t *testing.T) {
	a := chipApp(t, 3)
	a.settings.ThumbPrefetch = false
	if got := a.fillChip().Label; got != "补载 关闭" {
		t.Fatalf("a closed fill reads %q", got)
	}

	a.settings.ThumbPrefetch = true
	a.settings.ThumbPrefetchMax = 2
	if got := a.fillChip().Label; got != "补载 按需" {
		t.Fatalf("a folder past the limit reads %q", got)
	}

	a.settings.ThumbPrefetchMax = 200
	if got := a.fillChip().Label; got != "补载 已停止" {
		t.Fatalf("a stopped fill reads %q", got)
	}

	// A finished fill says so rather than falling back to a generic idle word.
	a.fillStoppedTab, a.fillStoppedGen = nil, 0
	a.prefetchNext, a.prefetchActive = len(a.tab().All), 0
	if got := a.fillChip().Label; got != "补载 就绪" {
		t.Fatalf("a finished fill reads %q", got)
	}

	// Before the cursor has moved the fill has not started yet.
	a.prefetchNext = 0
	if got := a.fillChip().Label; got != "补载 等待" {
		t.Fatalf("a pending fill reads %q", got)
	}
}

// TestFillChipReportsProgress covers the running state: the chip names the work
// and how far along it is, and opens the panel where it can be cancelled.
func TestFillChipReportsProgress(t *testing.T) {
	a := chipApp(t, 100)
	task := a.startTask(taskKindFill, "补载缩略图", "照片", 100, func() {})
	task.Started = task.Started.Add(-2 * time.Second)
	a.setTaskProgress(task.ID, 40, 100)

	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("补载 40%") {
		t.Fatalf("the running chip is missing: %q", tt.Texts())
	}

	a.tasksOpen = true
	tt.Frame()
	if !tt.HasText("后台任务") || !tt.HasText("取消") {
		t.Fatalf("the running chip did not open the panel: %q", tt.Texts())
	}
}

// TestFillChipReportsFailure covers the state that stays until it is read.
func TestFillChipReportsFailure(t *testing.T) {
	a := chipApp(t, 10)
	task := a.startTask(taskKindFill, "补载缩略图", "照片", 10, func() {})
	task.Started = task.Started.Add(-2 * time.Second)
	a.failTask(task.ID, "磁盘错误")

	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("补载 失败") {
		t.Fatalf("a failed fill is not in the bar: %q", tt.Texts())
	}
	if !a.fillChip().Actionable {
		t.Fatal("a failure is not clickable")
	}
	// A quiet state is not a button: there is nothing to cancel or read.
	quiet := chipApp(t, 3)
	if quiet.fillChip().Actionable {
		t.Fatal("a quiet chip is clickable")
	}
}

// TestFillProgressCountsFinishedWork covers the arithmetic: progress counts
// pictures that came back, not pictures handed to the workers. Subtracting the
// in-flight count from the total reported almost 100% from the first frame, so
// the chip could never be used to judge how much was left.
func TestFillProgressCountsFinishedWork(t *testing.T) {
	thumbDir = t.TempDir()
	thumbSlots = make(chan struct{}, 4)

	items := make([]*Item, 0, 60)
	for i := 0; i < 60; i++ {
		items = append(items, &Item{ID: "fillp-" + string(rune('a'+i%26)) + string(rune('0'+i/26)), Name: "p.png", Kind: "image", Size: 1, W: 10, H: 10})
	}
	tab := &Tab{ID: 1, Loaded: true, Gen: 1, All: items, Items: items, Selected: map[string]bool{}}
	a := newPrefetchTestApp(t, tab)

	a.pumpThumbPrefetch(tab)
	task := a.taskByKind(taskKindFill)
	if task == nil {
		t.Fatal("the fill did not start a task")
	}
	// Only the few pictures in flight are outstanding; everything else is still
	// ahead of the cursor, so progress must be near zero, not near done.
	if task.Done > len(items)/2 {
		t.Fatalf("progress starts at %d/%d, want well under half", task.Done, task.Total)
	}
	a.cancelThumbPrefetch()
}
