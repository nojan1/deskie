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

	Large, Medium, Small font.Face
	Radius               float64
}

func DefaultTheme(large, medium, small font.Face) *Theme {
	return &Theme{
		Background: color.RGBA{0x10, 0x12, 0x1a, 0xff},
		Surface:    color.RGBA{0x18, 0x1c, 0x2b, 0xff},
		Header:     color.RGBA{0x1f, 0x25, 0x3a, 0xff},
		Accent:     color.RGBA{0x4f, 0xd1, 0xc5, 0xff},
		Text:       color.RGBA{0xe6, 0xe9, 0xf2, 0xff},
		TextDim:    color.RGBA{0x8a, 0x93, 0xad, 0xff},
		Button:     color.RGBA{0x2a, 0x31, 0x4d, 0xff},
		ButtonOn:   color.RGBA{0x3b, 0x82, 0xf6, 0xff},
		ButtonDown: color.RGBA{0x4a, 0x55, 0x80, 0xff},
		Backdrop:   color.NRGBA{0x00, 0x00, 0x00, 0xa0},
		Touch:      color.RGBA{0xff, 0x5d, 0x73, 0xff},
		Large:      large,
		Medium:     medium,
		Small:      small,
		Radius:     14,
	}
}
