package tui

import (
	"strings"
	"testing"
)

// { and } move the border by 2 columns, the width stays inside the limits, and the share
// is saved after the pause.
func TestMoveBorder(t *testing.T) {
	var saved float64
	m := openWork(t)
	m.saveShare = func(s float64) error { saved = s; return nil }
	w0 := m.layout().mid.w
	m = send(t, m, key("}"), key("}"))
	if w := m.layout().mid.w; w != w0+4 || m.layout().detail.w != m.width-m.navWidth()-w {
		t.Fatalf("after } }: list = %d, want %d", w, w0+4)
	}
	m = send(t, m, key("{"))
	if w := m.layout().mid.w; w != w0+2 {
		t.Errorf("after {: list = %d, want %d", w, w0+2)
	}
	nm, cmd := m.Update(shareSaveMsg{gen: m.shareGen - 1}) // an old pause: no save
	if cmd != nil {
		t.Error("an old pause saved")
	}
	m = nm.(Model)
	nm, cmd = m.Update(shareSaveMsg{gen: m.shareGen})
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("the last pause did not save")
	}
	cmd()
	if saved != m.listShare || saved <= 0.5 {
		t.Errorf("saved = %v share = %v", saved, m.listShare)
	}
	for range 100 {
		m = send(t, m, key("{"))
	}
	if m.layout().mid.w != minPaneW {
		t.Errorf("list width = %d, want the limit %d", m.layout().mid.w, minPaneW)
	}
}

// A drag on the border moves it with the mouse.
func TestDragBorder(t *testing.T) {
	m := openWork(t)
	l := m.layout()
	x := l.detail.x
	m = send(t, m, click(x, 5), motion(x+6, 5), release(x+6, 5))
	if w := m.layout().mid.w; w != l.mid.w+6 {
		t.Errorf("list width = %d, want %d (the mouse moved 6 columns)", w, l.mid.w+6)
	}
	if m.paneDrag {
		t.Error("the drag did not end")
	}
}

// On a narrow terminal, { explains that there is no details pane next to the list.
func TestMoveBorderNarrow(t *testing.T) {
	m := openWork(t)
	m.width = 90
	m = send(t, m, key("{"))
	if !strings.Contains(m.status, "wide terminal") {
		t.Errorf("status = %q", m.status)
	}
}
