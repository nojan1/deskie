package widget

// group is the child list + gesture capture shared by all containers.
// Children are drawn in order, so later children are on top and get first
// pick of Down events.
type group struct {
	children []Widget
	captured Widget
}

func (g *group) Children() []Widget { return g.children }

func (g *group) add(owner invalidator, w Widget) {
	attach(owner, w)
	g.children = append(g.children, w)
}

func (g *group) remove(w Widget) int {
	for i, ch := range g.children {
		if ch == w {
			g.release(w)
			detach(w)
			g.children = append(g.children[:i], g.children[i+1:]...)
			return i
		}
	}
	return -1
}

func (g *group) clear() {
	for _, ch := range g.children {
		g.release(ch)
		detach(ch)
	}
	g.children = nil
}

// release cancels an in-progress gesture on w before it leaves the tree.
func (g *group) release(w Widget) {
	if g.captured == w {
		g.captured = nil
		w.HandleEvent(Event{Kind: Cancel})
	}
}

func (g *group) drawChildren(c *Canvas) {
	for _, ch := range g.children {
		if ch.Visible() && c.Visible(ch.Bounds()) {
			ch.Draw(c)
		}
	}
}

// dispatch routes Down to the topmost child under the pointer and the rest
// of the gesture to whichever child consumed that Down.
func (g *group) dispatch(e Event) bool {
	if e.Kind == Down {
		g.captured = nil
		for i := len(g.children) - 1; i >= 0; i-- {
			ch := g.children[i]
			if ch.Visible() && e.Pos.In(ch.Bounds()) && ch.HandleEvent(e) {
				g.captured = ch
				return true
			}
		}
		return false
	}
	ch := g.captured
	if ch == nil {
		return false
	}
	if e.Kind == Up || e.Kind == Cancel {
		g.captured = nil
	}
	return ch.HandleEvent(e)
}
