package widget

import "image"

const windowTitleH = 44

// Window is an opaque, titled container with a single content widget
// (usually a Stack). It is the natural root for a modal layer, see
// Screen.PushModal.
type Window struct {
	Base
	group
	th      *Theme
	title   string
	Padding Insets
	content Widget
}

func NewWindow(th *Theme, title string, content Widget) *Window {
	w := &Window{th: th, title: title, Padding: Pad(10)}
	w.content = content
	w.add(w, content)
	return w
}

func (w *Window) Content() Widget { return w.content }

func (w *Window) SetTitle(s string) {
	if s != w.title {
		w.title = s
		w.InvalidateRect(w.titleRect())
	}
}

func (w *Window) titleRect() image.Rectangle {
	r := w.bounds
	r.Max.Y = min(r.Min.Y+windowTitleH, r.Max.Y)
	return r
}

func (w *Window) contentRect() image.Rectangle {
	r := w.bounds
	if w.title != "" {
		r.Min.Y = min(r.Min.Y+windowTitleH, r.Max.Y)
	}
	return w.Padding.Shrink(r)
}

func (w *Window) SetBounds(r image.Rectangle) {
	w.Base.SetBounds(r)
	w.content.SetBounds(w.contentRect())
}

func (w *Window) PreferredSize() image.Point {
	p := w.content.PreferredSize().Add(w.Padding.Size())
	if w.title != "" {
		p.Y += windowTitleH
	}
	return p
}

func (w *Window) Draw(c *Canvas) {
	c.FillRoundRect(w.bounds, w.th.Radius, w.th.Surface)
	if w.title != "" {
		tr := w.titleRect()
		c.Text(w.title, w.th.Medium, w.th.Text, PadXY(16, 0).Shrink(tr), 0, 0.5)
		c.FillRect(image.Rect(tr.Min.X, tr.Max.Y-2, tr.Max.X, tr.Max.Y), w.th.Accent)
	}
	w.drawChildren(c)
}

// HandleEvent: a window is opaque, so it swallows taps that miss its content.
func (w *Window) HandleEvent(e Event) bool {
	return w.dispatch(e) || e.Kind == Down
}

func (w *Window) OpaqueRect() image.Rectangle {
	return opaqueRect(w.bounds, w.th.Radius, w.th.Surface)
}
