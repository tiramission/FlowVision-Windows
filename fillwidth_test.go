package main

import (
	"fmt"
	"testing"
	"time"
)

// TestFillWidthFollowsScrolling covers the policy: fills run four wide when the
// app is at rest and narrow to two while the grid is moving, so the cards under
// the pointer keep most of the workers.
func TestFillWidthFollowsScrolling(t *testing.T) {
	a := newViewTestApp()
	now := time.Now()

	if got := a.fillWidth(now); got != thumbFillIdle {
		t.Fatalf("at rest the fill runs %d wide, want %d", got, thumbFillIdle)
	}

	tab := a.tab()
	tab.Scroll.Y = 0
	a.noteGridActivity(tab)
	if got := a.fillWidth(now); got != thumbFillIdle {
		t.Fatalf("an unchanged offset narrowed the fill to %d", got)
	}

	tab.Scroll.Y = 480
	a.noteGridActivity(tab)
	if got := a.fillWidth(now); got != thumbFillBusy {
		t.Fatalf("scrolling left the fill at %d wide, want %d", got, thumbFillBusy)
	}

	// It widens again once the scroll has settled.
	later := now.Add(thumbFillBusyWindow + 10*time.Millisecond)
	if got := a.fillWidth(later); got != thumbFillIdle {
		t.Fatalf("after settling the fill stayed at %d wide", got)
	}
}

// TestFillWidthIsTheQueueDepth covers the other half: the width is enforced by
// how many tasks are queued, so no worker ever parks waiting for a fill slot
// while a visible card is waiting behind it.
func TestFillWidthIsTheQueueDepth(t *testing.T) {
	thumbDir = t.TempDir()
	thumbSlots = make(chan struct{}, 8)
	tab := &Tab{ID: 1, Loaded: true, Gen: 1, Selected: map[string]bool{}}
	for i := 0; i < 40; i++ {
		it := &Item{ID: fmt.Sprintf("width-%d", i), Name: "p.png", Kind: "image", Size: 1, W: 10, H: 10}
		tab.All = append(tab.All, it)
		tab.Items = append(tab.Items, it)
	}
	a := newPrefetchTestApp(t, tab)

	// At rest: four queued, which is exactly the idle width.
	for i := 0; i < 10; i++ {
		a.pumpThumbPrefetch(tab)
	}
	if a.prefetchActive != thumbFillIdle {
		t.Fatalf("queued %d fills at rest, want %d", a.prefetchActive, thumbFillIdle)
	}

	// One finishes: the pump refills only up to the width again.
	task := a.thumbQueue.popNow()
	if task == nil {
		t.Fatal("nothing was queued")
	}
	a.runThumbTask(task)
	a.pumpThumbPrefetch(tab)
	if a.prefetchActive > thumbFillIdle {
		t.Fatalf("after a completion the fill runs %d wide, want <= %d", a.prefetchActive, thumbFillIdle)
	}
	a.cancelThumbPrefetch()
}

// TestThumbWorkersStaysWithinBounds pins the pool: ten at most, two at least,
// and no more than six on a machine with little memory, where whole-frame
// decodes of PNG and unknown formats would not fit.
func TestThumbWorkersStaysWithinBounds(t *testing.T) {
	n := thumbWorkers()
	if n < 2 || n > 10 {
		t.Fatalf("worker count %d is outside 2..10", n)
	}
	if totalMemoryBytes() < 8<<30 && n > 6 {
		t.Fatalf("%d workers on a machine with %d bytes of memory", n, totalMemoryBytes())
	}
	if totalMemoryBytes() == 0 {
		t.Log("memory query unavailable; the conservative cap was used")
	}
}
