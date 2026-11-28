//go:build !windows

package main

import "fmt"

// shellThumb is a stub on non-Windows hosts.
func shellThumb(it *Item, size int) (thumbRender, error) {
	return thumbRender{}, fmt.Errorf("Shell thumbnails are only available on Windows")
}
