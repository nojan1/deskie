package main

import (
	"image"

	"deskui/widget"
)

func newTheme() *widget.Theme {
	return widget.DefaultTheme(
		mustFace(firaBold, 64),
		mustFace(firaBold, 24),
		mustFace(firaRegular, 18),
	)
}

// buildUI assembles the home screen as the base layer of s.
func buildUI(s *widget.Screen) {
	th := s.Theme

	title := widget.NewLabel("deskpi", th.Small, th.Text)
	title.AlignX = 0
	date := widget.NewClock("Mon 02 Jan", th.Small, th.Text)
	date.AlignX = 1

	headerRow := widget.NewHStack()
	headerRow.Padding = widget.PadXY(16, 0)
	headerRow.
		Add(title, widget.Flex(1)).
		Add(date, widget.Auto)

	header := widget.NewVStack()
	header.Background = th.Header
	header.
		Add(headerRow, widget.Flex(1)).
		Add(widget.NewBox(th.Accent), widget.Fixed(2))

	buttons := widget.NewHStack()
	buttons.Spacing = 20
	buttons.Padding = widget.Insets{Top: 10, Left: 20, Right: 20, Bottom: 20}
	buttons.
		Add(widget.NewToggle(th, "Lamp", nil), widget.Flex(1)).
		Add(widget.NewToggle(th, "Fan", nil), widget.Flex(1)).
		Add(widget.NewButton(th, "Scene", func(*widget.Button) {
			openScenePicker(s, func(name string) { title.SetText("deskpi · " + name) })
		}), widget.Flex(1))

	root := widget.NewVStack()
	root.
		Add(header, widget.Fixed(40)).
		Add(widget.NewClock("15:04:05", th.Large, th.Text), widget.Fixed(140)).
		Add(buttons, widget.Flex(1))

	s.SetRoot(root)
}

// openScenePicker shows a modal window with a scrollable list.
func openScenePicker(s *widget.Screen, picked func(name string)) {
	th := s.Theme
	var layer *widget.Layer

	list := widget.NewList(th)
	for _, name := range []string{
		"Focus", "Meeting", "Evening", "Reading", "Movie",
		"Night light", "Cleaning", "Away", "Party", "Off",
	} {
		list.Add(widget.NewListItem(th, name, func(it *widget.ListItem) {
			picked(it.Text())
			layer.Close()
		}))
	}

	body := widget.NewVStack()
	body.Spacing = 8
	body.
		Add(list, widget.Flex(1)).
		Add(widget.NewButton(th, "Close", func(*widget.Button) { layer.Close() }), widget.Fixed(48))

	layer = s.PushModal(widget.NewWindow(th, "Scenes", body), s.Bounds().Size().Sub(image.Pt(60, 30)))
}
