package menu

import (
	"image"
	"sort"
	"strings"
	"testing"
)

func TestMenu(t *testing.T) {
	m := NewMenu()
	m.Selectable = true
	// AddItems and DelItem redraw the screen; set the items directly.
	m.items = NewItems(Item{Val: "id"}, Item{Val: "name", Label: "Task Name"}, Item{Val: "state"})
	m.calcSize()
	if m.Width != len("Task Name")+8 || m.Height != 3+4 {
		t.Errorf("size %dx%d", m.Width, m.Height)
	}
	if !m.SetCursor("Task Name") || m.SelectedItem().Val != "name" {
		t.Errorf("cursor on %v", m.SelectedItem())
	}
	if m.SetCursor("missing") {
		t.Error("SetCursor on a missing item")
	}

	var rows []string
	buf := m.Buffer()
	for y := m.Y + 2; y < m.Y+5; y++ {
		var sb strings.Builder
		for x := m.X + 4; x < m.X+4+len("Task Name"); x++ {
			if c, ok := buf.CellMap[pt(x, y)]; ok {
				sb.WriteRune(c.Ch)
			}
		}
		rows = append(rows, strings.TrimSpace(sb.String()))
	}
	if strings.Join(rows, ",") != "id,Task Name,state" {
		t.Errorf("rendered %v", rows)
	}

	items := NewItems(Item{Val: "b"}, Item{Val: "c", Label: "a"})
	sort.Sort(items)
	if items[0].Text() != "a" {
		t.Errorf("sorted by text: %v", items)
	}
}

func pt(x, y int) image.Point { return image.Point{X: x, Y: y} }
