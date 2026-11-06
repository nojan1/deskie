package main

import "image"

type EventKind int

const (
	TouchDown EventKind = iota
	TouchMove
	TouchUp
	Quit
)

// Event is a touch/mouse event in screen pixel coordinates.
type Event struct {
	Kind EventKind
	X, Y int
}

// Display is the only thing the UI knows about. The Pi uses the framebuffer
// backend, the dev machine uses the kitty graphics backend.
type Display interface {
	Size() (w, h int)
	// Flush pushes img to the screen. r is the changed region; a backend may
	// push more than r but must push at least r.
	Flush(img *image.RGBA, r image.Rectangle) error
	Events() <-chan Event
	Close() error
}
