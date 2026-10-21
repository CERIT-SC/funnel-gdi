package widgets

import "testing"

func TestInput(t *testing.T) {
	i := NewInput()
	if i.Width != i.MaxLen+8 || i.Height != 3 {
		t.Errorf("size %dx%d", i.Width, i.Height)
	}
	i.Data = "abc"
	buf := i.Buffer()
	var got []rune
	for x := i.X + 4; x < i.X+7; x++ {
		for p, c := range buf.CellMap {
			if p.X == x && p.Y == i.Y+1 {
				got = append(got, c.Ch)
			}
		}
	}
	if string(got) != "abc" {
		t.Errorf("rendered %q", string(got))
	}
	if i.Stream() == nil {
		t.Error("Stream")
	}
}
