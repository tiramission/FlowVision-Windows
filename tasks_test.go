package main

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestTaskVisibilityWindow(t *testing.T) {
	a := newViewTestApp()
	now := time.Now()

	fresh := a.startTask(taskKindFill, "补载缩略图", "照片", 100, nil)
	if got := a.visibleTasks(now); len(got) != 0 {
		t.Fatalf("a task younger than the delay is already visible: %d", len(got))
	}

	// One second later the chip may show it.
	fresh.Started = now.Add(-taskVisibleDelay - time.Second)
	got := a.visibleTasks(now)
	if len(got) != 1 || got[0].ID != fresh.ID {
		t.Fatalf("running task not visible after the delay: %+v", got)
	}
	if a.runningTaskCount() != 1 {
		t.Fatalf("running count = %d, want 1", a.runningTaskCount())
	}

	a.finishTask(fresh.ID)
	if fresh.State != taskDone {
		t.Fatalf("state = %v after finish, want done", fresh.State)
	}
	if len(a.visibleTasks(now)) != 1 {
		t.Fatal("a finished task disappeared before its linger window")
	}
	fresh.DismissAt = now.Add(-time.Second)
	if !a.pruneTasks() {
		t.Fatal("pruneTasks reported no change after the linger window")
	}
	if len(a.tasks) != 0 {
		t.Fatalf("finished task was not pruned: %d left", len(a.tasks))
	}

	// A failure stays until it is read.
	failed := a.startTask(taskKindWarmup, "载入原图", "a.jpg", 0, nil)
	failed.Started = now.Add(-time.Minute)
	a.failTask(failed.ID, "磁盘错误")
	if failed.DismissAt != (time.Time{}) {
		t.Fatal("a failed task scheduled its own dismissal")
	}
	if len(a.visibleTasks(time.Now().Add(time.Hour))) != 1 {
		t.Fatal("a failed task faded out without being read")
	}
	a.dropTask(failed.ID)
	if len(a.tasks) != 0 {
		t.Fatalf("%d tasks left after dismiss", len(a.tasks))
	}
}

func TestStopTaskCallsCancelOnce(t *testing.T) {
	a := newViewTestApp()
	calls := 0
	task := a.startTask(taskKindFill, "补载缩略图", "照片", 10, func() { calls++ })

	a.setTaskProgress(task.ID, 4, 10)
	if task.Done != 4 || task.Total != 10 {
		t.Fatalf("progress = %d/%d, want 4/10", task.Done, task.Total)
	}

	a.stopTask(task.ID)
	a.stopTask(task.ID) // pressing twice must not call the cancel again
	if calls != 1 {
		t.Fatalf("cancel called %d times, want 1", calls)
	}
	if task.State != taskStopped {
		t.Fatalf("state = %v, want stopped", task.State)
	}
	if task.DismissAt.IsZero() {
		t.Fatal("a stopped task never fades out")
	}
}

func TestCancelFillStopsForTheListing(t *testing.T) {
	thumbDir = t.TempDir()
	thumbSlots = make(chan struct{}, 4)
	items := make([]*Item, 0, 40)
	for i := 0; i < 40; i++ {
		items = append(items, &Item{ID: "fill-task-" + string(rune('a'+i%26)) + string(rune('0'+i/26)), Name: "p.png", Kind: "image", Size: 1, W: 10, H: 10})
	}
	tab := &Tab{ID: 1, Loaded: true, Gen: 1, All: items, Items: items, Selected: map[string]bool{}}
	a := newPrefetchTestApp(t, tab)

	a.pumpThumbPrefetch(tab)
	task := a.taskByKind(taskKindFill)
	if task == nil {
		t.Fatal("the folder fill did not appear as a task")
	}
	if a.prefetchActive == 0 {
		t.Fatal("no fills were queued")
	}

	a.stopTask(task.ID)
	if a.prefetchActive != 0 {
		t.Fatalf("%d fills are still queued after cancel", a.prefetchActive)
	}

	// The next grid frame must not quietly restart the same work.
	a.pumpThumbPrefetch(tab)
	if a.prefetchActive != 0 {
		t.Fatalf("cancel was undone by the next frame: %d fills", a.prefetchActive)
	}
	if again := a.taskByKind(taskKindFill); again == nil || again.State != taskStopped {
		t.Fatalf("the stopped row did not survive the next frame: %+v", again)
	}

	// A new listing is a new fill.
	tab.Gen++
	a.pumpThumbPrefetch(tab)
	if a.prefetchActive == 0 {
		t.Fatal("a reloaded folder did not restart the fill")
	}
}

func TestWarmupTaskFinishesWithTheOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.png")
	writePNG(t, path, 2700, 1400)
	it := &Item{ID: "warm-task", Name: "big.png", Kind: "image", Path: path, Size: 1, W: 2700, H: 1400}
	a := autoFullResApp(t, it, nil)

	if !a.maybeAutoViewerFullRes() {
		t.Fatal("the original did not start")
	}
	task := a.taskByKind(taskKindWarmup)
	if task == nil || task.State != taskRunning {
		t.Fatalf("warm-up task = %+v, want running", task)
	}
	waitFullResReady(t, a, 10*time.Second)
	if task.State != taskDone {
		t.Fatalf("task state = %v after the original arrived, want done", task.State)
	}
	if task.DismissAt.IsZero() {
		t.Fatal("finished warm-up never fades out")
	}

	// Switching images clears the row instead of leaving a stale "done".
	a.dropTasksOfKind(taskKindWarmup)
	if len(a.tasks) != 0 {
		t.Fatalf("%d tasks left after leaving the image", len(a.tasks))
	}
}

func TestCancelWarmupKeepsThePreview(t *testing.T) {
	a := newViewTestApp()
	preview := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 32, 32)))
	it := &Item{ID: "warm-cancel", Name: "big.png", Kind: "image", W: 4000, H: 3000}
	tab := a.tab()
	tab.All, tab.Items = []*Item{it}, []*Item{it}
	a.viewer = Viewer{Open: true, Item: it, Index: 0, Bitmap: preview, Zoom: 1}

	key := viewerFullResKey(it)
	a.viewer.FullResLoading = true
	viewerFullResWanted.Store(key)

	task := a.startTask(taskKindWarmup, "载入原图", it.Name, 0, func() { a.cancelViewerWarmup(key) })
	a.stopTask(task.ID)

	if viewerFullResIsWanted(key) {
		t.Fatal("cancelling left the decode wanted")
	}
	if a.viewer.FullResLoading {
		t.Fatal("viewer still reports a pending original")
	}
	if a.viewer.Bitmap != preview {
		t.Fatal("cancelling the warm-up replaced the visible preview")
	}
}

func TestTaskChipAndPanel(t *testing.T) {
	a := newViewTestApp()
	tab := a.tab()
	tab.All, tab.Items = nil, nil
	a.viewer = Viewer{}

	// No tasks: no chip.
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if tt.HasText("后台") {
		t.Fatalf("status bar shows a task chip without tasks: %q", tt.Texts())
	}

	task := a.startTask(taskKindFill, "补载缩略图", "照片", 100, func() {})
	task.Started = task.Started.Add(-2 * time.Second)
	a.setTaskProgress(task.ID, 40, 100)

	tt = ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("后台 1") {
		t.Fatalf("task chip missing: %q", tt.Texts())
	}

	a.tasksOpen = true
	tt.Frame()
	if !tt.HasText("后台任务") || !tt.HasText("取消") {
		t.Fatalf("task panel did not open: %q", tt.Texts())
	}
	if !tt.HasText("40 / 100") {
		t.Fatalf("task panel progress missing: %q", tt.Texts())
	}

	// Escape closes the panel.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.tasksOpen {
		t.Fatal("escape did not close the task panel")
	}
}
