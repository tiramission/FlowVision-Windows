package main

import (
	"fmt"
	"time"
)

// Background tasks are the slow work the app does behind the grid: filling a
// folder's thumbnail cache and decoding the viewer's original. Listing them in
// one place lets the user see the work and stop it instead of inferring it from
// a suddenly faster scroll.
//
// Only the main thread mutates this list; workers report through a.update.
// Nothing is persisted: a task that outlives the session is not a task.

const (
	// taskVisibleDelay keeps short work from flashing a chip in the status bar.
	taskVisibleDelay = time.Second
	// taskDoneLinger is how long a finished row stays readable.
	taskDoneLinger = 4 * time.Second
)

type taskState int

const (
	taskRunning taskState = iota
	taskDone
	taskStopped
	taskFailed
)

type backgroundTask struct {
	ID        string
	Kind      string // fill | warmup
	Title     string
	Detail    string
	Done      int
	Total     int
	Err       string
	State     taskState
	Started   time.Time
	DismissAt time.Time
	cancel    func()
}

// startTask adds a running task. total <= 0 means "no measurable progress".
func (a *App) startTask(kind, title, detail string, total int, cancel func()) *backgroundTask {
	a.taskSeq++
	t := &backgroundTask{
		ID:      fmt.Sprintf("task-%d", a.taskSeq),
		Kind:    kind,
		Title:   title,
		Detail:  detail,
		Total:   total,
		State:   taskRunning,
		Started: time.Now(),
		cancel:  cancel,
	}
	a.tasks = append(a.tasks, t)
	a.scheduleTaskWake()
	return t
}

func (a *App) taskByID(id string) *backgroundTask {
	for _, t := range a.tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (a *App) taskByKind(kind string) *backgroundTask {
	for _, t := range a.tasks {
		if t.Kind == kind {
			return t
		}
	}
	return nil
}

func (a *App) setTaskProgress(id string, done, total int) {
	t := a.taskByID(id)
	if t == nil || t.State != taskRunning {
		return
	}
	if done < 0 {
		done = 0
	}
	if total > 0 && done > total {
		done = total
	}
	if t.Done == done && t.Total == total {
		return
	}
	t.Done, t.Total = done, total
	a.refreshTasks()
}

func (a *App) finishTask(id string) {
	t := a.taskByID(id)
	if t == nil || t.State != taskRunning {
		return
	}
	t.State = taskDone
	t.Done = t.Total
	t.DismissAt = time.Now().Add(taskDoneLinger)
	a.scheduleTaskWake()
	a.refreshTasks()
}

func (a *App) failTask(id, err string) {
	t := a.taskByID(id)
	if t == nil || t.State != taskRunning {
		return
	}
	t.State = taskFailed
	t.Err = err
	// A failure stays until the user closes it; it is the only state worth
	// reading twice.
	t.DismissAt = time.Time{}
	a.refreshTasks()
}

// stopTask is the user pressing cancel. The row stays visible as "stopped" so
// the action has a receipt, then fades out on its own.
func (a *App) stopTask(id string) {
	t := a.taskByID(id)
	if t == nil || t.State != taskRunning {
		return
	}
	a.stopTaskNow(t)
	a.refreshTasks()
}

func (a *App) stopTaskNow(t *backgroundTask) {
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.State = taskStopped
	t.DismissAt = time.Now().Add(taskDoneLinger)
	a.scheduleTaskWake()
}

// dropTask removes a task without a trace. It is for work that ends because
// its subject went away (another folder, another image), which is not
// something the user asked to stop.
func (a *App) dropTask(id string) {
	for i, t := range a.tasks {
		if t.ID == id {
			a.tasks = append(a.tasks[:i], a.tasks[i+1:]...)
			a.refreshTasks()
			return
		}
	}
}

func (a *App) dropTasksOfKind(kind string) {
	kept := a.tasks[:0]
	for _, t := range a.tasks {
		if t.Kind != kind {
			kept = append(kept, t)
		}
	}
	if len(kept) != len(a.tasks) {
		a.tasks = kept
		a.refreshTasks()
	}
}

// visibleTasks returns the rows the status bar and panel should show now. A
// running task is hidden for its first second so nothing flashes for work that
// already finished.
func (a *App) visibleTasks(now time.Time) []*backgroundTask {
	out := make([]*backgroundTask, 0, len(a.tasks))
	for _, t := range a.tasks {
		switch t.State {
		case taskRunning:
			if now.Sub(t.Started) >= taskVisibleDelay {
				out = append(out, t)
			}
		default:
			if t.DismissAt.IsZero() || now.Before(t.DismissAt) {
				out = append(out, t)
			}
		}
	}
	return out
}

func (a *App) runningTaskCount() int {
	n := 0
	for _, t := range a.tasks {
		if t.State == taskRunning {
			n++
		}
	}
	return n
}

// pruneTasks drops faded rows. It runs on the wake timer, which is also what
// makes a task appear once its one-second delay has passed.
func (a *App) pruneTasks() bool {
	now := time.Now()
	kept := a.tasks[:0]
	changed := false
	for _, t := range a.tasks {
		if t.State != taskRunning && !t.DismissAt.IsZero() && !now.Before(t.DismissAt) {
			changed = true
			continue
		}
		kept = append(kept, t)
	}
	if changed {
		a.tasks = kept
	}
	return changed
}

// scheduleTaskWake arms one timer for the next boundary that changes what is
// visible: a task reaching its one-second delay, or a finished row expiring.
func (a *App) scheduleTaskWake() {
	if a.win == nil {
		return
	}
	if a.taskTimer != nil {
		a.taskTimer.Stop()
		a.taskTimer = nil
	}
	now := time.Now()
	var next time.Time
	for _, t := range a.tasks {
		var at time.Time
		if t.State == taskRunning {
			at = t.Started.Add(taskVisibleDelay)
		} else if !t.DismissAt.IsZero() {
			at = t.DismissAt
		}
		if at.IsZero() || !at.After(now) {
			continue
		}
		if next.IsZero() || at.Before(next) {
			next = at
		}
	}
	if next.IsZero() {
		return
	}
	delay := next.Sub(now)
	a.taskTimer = time.AfterFunc(delay, func() {
		a.update(func() {
			a.taskTimer = nil
			if a.pruneTasks() {
				a.refresh()
			}
			a.scheduleTaskWake()
		})
	})
}

// refreshTasks coalesces progress into a few repaints per second. A folder
// fill reports one finished thumbnail at a time; rebuilding the window for
// every one of them would cost more than the decode.
func (a *App) refreshTasks() {
	if a.win == nil {
		return
	}
	if a.taskTimerPainted.Add(250 * time.Millisecond).After(time.Now()) {
		if a.taskRefreshTimer == nil {
			a.taskRefreshTimer = time.AfterFunc(250*time.Millisecond, func() {
				a.update(func() {
					a.taskRefreshTimer = nil
					a.taskTimerPainted = time.Now()
					a.refresh()
				})
			})
		}
		return
	}
	a.taskTimerPainted = time.Now()
	a.refresh()
}

// taskProgressText is the one-line summary the debug state and the panel use.
func taskProgressText(t *backgroundTask) string {
	if t == nil {
		return ""
	}
	switch t.State {
	case taskDone:
		return t.Title + " · 已完成"
	case taskStopped:
		return t.Title + " · 已停止"
	case taskFailed:
		if t.Detail != "" {
			return t.Title + " · " + t.Detail + " 失败"
		}
		return t.Title + " · 失败"
	}
	if t.Total > 0 {
		pct := t.Done * 100 / t.Total
		return fmt.Sprintf("%s %d%%（%s / %s）", t.Title, pct, formatCount(t.Done), formatCount(t.Total))
	}
	if t.Detail != "" {
		return t.Title + " · " + t.Detail
	}
	return t.Title
}

func formatCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}
