package widget

import "image"

// press tracks the pressed state of a tappable widget: pressed while the
// finger is down and inside, tapped if released inside.
type press struct{ down bool }

func (p *press) set(b *Base, v bool) {
	if p.down != v {
		p.down = v
		b.Invalidate()
	}
}

func (p *press) handle(b *Base, e Event) (handled, tapped bool) {
	switch e.Kind {
	case Down:
		p.set(b, true)
	case Move:
		p.set(b, e.Pos.In(b.bounds))
	case Up:
		tapped = p.down
		p.set(b, false)
	case Cancel:
		p.set(b, false)
	default:
		return false, false
	}
	return true, tapped
}

// Button is a rounded, labelled button. With Toggle set, each tap flips On.
type Button struct {
	Base
	press
	th      *Theme
	label   string
	Toggle  bool
	on      bool
	OnClick func(b *Button)
}

func NewButton(th *Theme, label string, onClick func(*Button)) *Button {
	return &Button{th: th, label: label, OnClick: onClick}
}

func NewToggle(th *Theme, label string, onChange func(*Button)) *Button {
	return &Button{th: th, label: label, Toggle: true, OnClick: onChange}
}

func (b *Button) On() bool { return b.on }

func (b *Button) SetOn(v bool) {
	if v != b.on {
		b.on = v
		b.Invalidate()
	}
}

func (b *Button) SetLabel(s string) {
	if s != b.label {
		b.label = s
		b.Invalidate()
	}
}

func (b *Button) PreferredSize() image.Point {
	return textSize(b.th.Medium, b.label).Add(image.Pt(48, 32))
}

func (b *Button) HandleEvent(e Event) bool {
	handled, tapped := b.press.handle(&b.Base, e)
	if tapped {
		if b.Toggle {
			b.SetOn(!b.on)
		}
		if b.OnClick != nil {
			b.OnClick(b)
		}
	}
	return handled
}

func (b *Button) Draw(c *Canvas) {
	col := b.th.Button
	switch {
	case b.down:
		col = b.th.ButtonDown
	case b.on:
		col = b.th.ButtonOn
	}
	c.FillRoundRect(b.bounds, b.th.Radius, col)
	c.Text(b.label, b.th.Medium, b.th.Text, b.bounds, 0.5, 0.5)
}

// ListItem is a full-width row for Lists: text, press highlight, divider.
type ListItem struct {
	Base
	press
	th    *Theme
	text  string
	Data  any // free for the caller, e.g. the model value the row shows
	OnTap func(it *ListItem)
}

func NewListItem(th *Theme, text string, onTap func(*ListItem)) *ListItem {
	return &ListItem{th: th, text: text, OnTap: onTap}
}

func (it *ListItem) Text() string { return it.text }

func (it *ListItem) SetText(s string) {
	if s != it.text {
		it.text = s
		it.Invalidate()
	}
}

func (it *ListItem) PreferredSize() image.Point {
	return textSize(it.th.Small, it.text).Add(image.Pt(32, 28))
}

func (it *ListItem) HandleEvent(e Event) bool {
	handled, tapped := it.press.handle(&it.Base, e)
	if tapped && it.OnTap != nil {
		it.OnTap(it)
	}
	return handled
}

func (it *ListItem) Draw(c *Canvas) {
	r := it.bounds
	if it.down {
		c.FillRect(r, it.th.ButtonDown)
	}
	c.Text(it.text, it.th.Small, it.th.Text, PadXY(16, 0).Shrink(r), 0, 0.5)
	c.FillRect(image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), it.th.Header)
}
