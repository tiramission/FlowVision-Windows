//go:build !windows

package main

import "fmt"

// wicThumb is a stub on non-Windows hosts. FlowVision ships for Windows, but
// keeping the symbol available lets the package still compile and test on
// other operating systems.
func wicThumb(it *Item, size int) (thumbRender, error) {
	return thumbRender{}, fmt.Errorf("WIC is only available on Windows")
}
