package widget

import (
	"image/color"

	"golang.org/x/image/font"
)

// Theme holds the shared look. Widgets keep a pointer to it, so changing a
// field followed by a full invalidate restyles everything.
type Theme struct {
	Background color.Color // screen background
	Surface    color.Color // windows, panels
	Header     color.Color // header bars, dividers
	Accent     color.Color
	Text       color.Color
	TextDim    color.Color
	Button     color.Color
	ButtonOn   color.Color
	ButtonDown color.Color
	Backdrop   color.Color // drawn over everything below a dimming layer
	Touch      color.Color // debug touch marker
	Night      color.Color // night mode background
	NightText  color.Color // night mode clock

	Large, Medium, Small font.Face
	Radius               float64
}

func DefaultTheme(large, medium, small font.Face) *Theme {
	return &Theme{
		// Nord palette: https://www.nordtheme.com/docs/colors-and-palettes
		Background: color.RGBA{0x2e, 0x34, 0x40, 0xff},  // nord0
		Surface:    color.RGBA{0x3b, 0x42, 0x52, 0xff},  // nord1
		Header:     color.RGBA{0x43, 0x4c, 0x5e, 0xff},  // nord2
		Accent:     color.RGBA{0x88, 0xc0, 0xd0, 0xff},  // nord8
		Text:       color.RGBA{0xec, 0xef, 0xf4, 0xff},  // nord6
		TextDim:    color.RGBA{0x81, 0x8c, 0xa3, 0xff},  // between nord3 and nord4, readable on nord0/1
		Button:     color.RGBA{0x4c, 0x56, 0x6a, 0xff},  // nord3
		ButtonOn:   color.RGBA{0x5e, 0x81, 0xac, 0xff},  // nord10
		ButtonDown: color.RGBA{0x81, 0xa1, 0xc1, 0xff},  // nord9
		Backdrop:   color.NRGBA{0x2e, 0x34, 0x40, 0xb0}, // nord0, translucent
		Touch:      color.RGBA{0xbf, 0x61, 0x6a, 0xff},  // nord11
		// The panel's backlight can't be dimmed, so night mode is black.
		Night:     color.RGBA{0x00, 0x00, 0x00, 0xff},
		NightText: color.RGBA{0x4c, 0x56, 0x6a, 0xff}, // nord3
		Large:     large,
		Medium:    medium,
		Small:     small,
		Radius:    14,
	}
}
