package compact

import (
	"image"
	"sort"
	"strings"
	"testing"

	ui "github.com/gizak/termui"
	"github.com/ohsu-comp-bio/funnel/tes"
)

// text returns the text of a rendered buffer, line by line.
func text(buf ui.Buffer) string {
	var pts []image.Point
	for p := range buf.CellMap {
		pts = append(pts, p)
	}
	sort.Slice(pts, func(i, j int) bool {
		if pts[i].Y != pts[j].Y {
			return pts[i].Y < pts[j].Y
		}
		return pts[i].X < pts[j].X
	})
	var sb strings.Builder
	y := -1
	for _, p := range pts {
		if p.Y != y && y != -1 {
			sb.WriteByte('\n')
		}
		y = p.Y
		sb.WriteRune(buf.CellMap[p].Ch)
	}
	return sb.String()
}

func TestGridRendersRows(t *testing.T) {
	g := NewGrid()
	g.AddRows(
		NewCompact(&tes.Task{Id: "task-1", Name: "first", State: tes.Running, Description: "desc one"}),
		NewCompact(&tes.Task{Id: "task-2", Name: "second", State: tes.Queued}),
	)
	g.SetY(1)
	g.SetWidth(120)
	g.Offset = 99 // out of range: reset by Align
	g.Align()
	if g.Offset != 0 {
		t.Errorf("offset %d", g.Offset)
	}
	out := text(g.Buffer())
	for _, want := range []string{"ID", "STATE", "NAME", "DESCRIPTION", "task-1", "RUNNING", "first", "desc one", "task-2", "QUEUED"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from:\n%s", want, out)
		}
	}
	if g.GetHeight() != 2+header.Height {
		t.Errorf("height %d", g.GetHeight())
	}
	// Rows are laid out one below the other, under the column header.
	r1, r2 := g.Rows[0].(*Compact), g.Rows[1].(*Compact)
	if r1.Y != 1+header.Height || r2.Y != r1.Y+1 {
		t.Errorf("rows at y=%d and y=%d", r1.Y, r2.Y)
	}

	g.Offset = 1
	g.Align()
	if out := text(g.Buffer()); strings.Contains(out, "task-1") || !strings.Contains(out, "task-2") {
		t.Errorf("with offset 1 only the second row is shown:\n%s", out)
	}
	g.Clear()
	if len(g.Rows) != 0 {
		t.Error("Clear")
	}
	g.SetX(3)
	if g.X != 3 {
		t.Error("SetX")
	}
}

func TestRowColumns(t *testing.T) {
	row := NewCompact(&tes.Task{Id: "id", Name: "name"})
	row.SetX(2)
	row.SetY(5)
	row.SetWidth(80)
	if row.ID.X != 2+3 || row.Status.X != 2 || row.ID.Y != 5 || row.GetHeight() != 1 {
		t.Errorf("columns at x=%d,%d y=%d", row.Status.X, row.ID.X, row.ID.Y)
	}
	// Auto-width columns share the width left after the status column.
	if w := calcWidth(80); w != (80-3-5)/4 {
		t.Errorf("calcWidth %d", w)
	}

	row.ID.Highlight()
	if row.ID.TextFgColor == ui.ColorWhite && row.ID.Bg == ui.ColorDefault {
		t.Error("Highlight did not change the colors")
	}
	row.ID.UnHighlight()
	row.ID.Set("other")
	if row.ID.Text != "other" {
		t.Errorf("Set %q", row.ID.Text)
	}
	row.ID.Reset()
}

func TestStatus(t *testing.T) {
	s := NewStatus()
	for state, color := range map[string]ui.Attribute{
		"QUEUED":         ui.ColorWhite,
		"RUNNING":        ui.ColorGreen,
		"INITIALIZING":   ui.ColorGreen,
		"COMPLETE":       ui.ColorRed,
		"EXECUTOR_ERROR": ui.ColorRed,
		"CANCELED":       ui.ColorRed,
	} {
		s.Set(state)
		if s.TextFgColor != color || s.Text != mark {
			t.Errorf("%s: color %v text %q", state, s.TextFgColor, s.Text)
		}
	}
	s.Set("PAUSED")
	if s.Text != vBar+vBar {
		t.Errorf("PAUSED: %q", s.Text)
	}
}
