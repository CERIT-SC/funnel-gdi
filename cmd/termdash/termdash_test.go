package termdash

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/ohsu-comp-bio/funnel/cmd/termdash/config"
	"github.com/ohsu-comp-bio/funnel/tes"
)

// Parts of the dashboard that need the terminal size (the header, the
// expanded view, RefreshTaskList) cannot run without a TTY: termui panics.
func init() {
	config.Init()
}

func widgetsFor(tasks ...*tes.Task) TaskWidgets {
	var out TaskWidgets
	for _, t := range tasks {
		out = append(out, NewTaskWidget(t))
	}
	return out
}

func ids(ws TaskWidgets) string {
	var out []string
	for _, w := range ws {
		out = append(out, w.Task.Id)
	}
	return strings.Join(out, ",")
}

func TestSortAndFilter(t *testing.T) {
	tasks := func() TaskWidgets {
		return widgetsFor(
			&tes.Task{Id: "b", Name: "zeta", State: tes.Running, Description: "2"},
			&tes.Task{Id: "a", Name: "alpha", State: tes.Complete, Description: "3"},
			&tes.Task{Id: "c", Name: "mid", State: tes.Running, Description: "1"},
		)
	}
	defer config.Update("sortField", "id")
	reversed := config.GetSwitchVal("sortReversed")
	defer func() {
		if config.GetSwitchVal("sortReversed") != reversed {
			config.Toggle("sortReversed")
		}
	}()
	if reversed {
		config.Toggle("sortReversed")
	}

	for field, want := range map[string]string{"id": "a,b,c", "name": "a,c,b", "description": "c,b,a", "state": "a,c,b"} {
		config.Update("sortField", field)
		ws := tasks()
		sort.Sort(ws)
		if got := ids(ws); got != want {
			t.Errorf("sort by %s: %s, want %s", field, got, want)
		}
	}
	config.Toggle("sortReversed")
	config.Update("sortField", "id")
	ws := tasks()
	sort.Sort(ws)
	if got := ids(ws); got != "c,b,a" {
		t.Errorf("reversed: %s", got)
	}
	if fields := SortFields(); len(fields) != 4 {
		t.Errorf("sort fields %v", fields)
	}

	defer config.Update("filterStr", "")
	config.Update("filterStr", "RUNNING")
	ws = tasks()
	ws.Filter()
	var shown []string
	for _, w := range ws {
		if w.display {
			shown = append(shown, w.Task.Id)
		}
	}
	sort.Strings(shown)
	if strings.Join(shown, ",") != "b,c" {
		t.Errorf("filter RUNNING shows %v", shown)
	}
}

// pagedTES is a TES server with three tasks, served two per page.
func pagedTES(t *testing.T) *httptest.Server {
	t.Helper()
	all := []*tes.Task{{Id: "t1", Name: "one"}, {Id: "t2", Name: "two"}, {Id: "t3", Name: "three"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/v1/tasks/") {
			id := strings.TrimPrefix(r.URL.Path, "/v1/tasks/")
			for _, task := range all {
				if task.Id == id {
					json.NewEncoder(w).Encode(map[string]string{"id": task.Id, "name": task.Name, "state": "COMPLETE"})
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "not found"}`))
			return
		}
		page := all[:2]
		next := "page2"
		if r.URL.Query().Get("page_token") == "page2" {
			page, next = all[2:], ""
		}
		var list []map[string]string
		for _, task := range page {
			list = append(list, map[string]string{"id": task.Id, "name": task.Name})
		}
		json.NewEncoder(w).Encode(map[string]any{"tasks": list, "next_page_token": next})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTaskSourcePaging(t *testing.T) {
	ts, err := NewTaskSource(pagedTES(t).URL, 2)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ts.List(false, false)
	if err != nil || len(first) != 2 || ts.GetNextPage() != "page2" || ts.GetPreviousPage() != "" {
		t.Fatalf("first page: %s next %q prev %q %v", ids(first), ts.GetNextPage(), ts.GetPreviousPage(), err)
	}
	second, err := ts.List(false, true)
	if err != nil || ids(second) != "t3" || ts.GetNextPage() != "" || ts.GetPreviousPage() != "page2" {
		t.Fatalf("second page: %s next %q prev %q %v", ids(second), ts.GetNextPage(), ts.GetPreviousPage(), err)
	}
	back, err := ts.List(true, false)
	if err != nil || len(back) != 2 {
		t.Fatalf("back: %s %v", ids(back), err)
	}

	w, err := ts.Get("t2")
	if err != nil || w.Task.Name != "two" || w.Task.State != tes.Complete {
		t.Errorf("get: %+v %v", w.Task, err)
	}
	if _, err := ts.Get("missing"); err == nil {
		t.Error("expected an error for an unknown task")
	}
}

// fakeSource is a TesTaskSource with fixed tasks.
type fakeSource struct {
	tasks TaskWidgets
	err   error
	next  string
}

func (f *fakeSource) List(bool, bool) (TaskWidgets, error) { return f.tasks, f.err }
func (f *fakeSource) Get(id string) (*TaskWidget, error) {
	return NewTaskWidget(&tes.Task{Id: id}), nil
}
func (f *fakeSource) GetNextPage() string     { return f.next }
func (f *fakeSource) GetPreviousPage() string { return "" }

func TestGridCursor(t *testing.T) {
	src := &fakeSource{tasks: widgetsFor(&tes.Task{Id: "a"}, &tes.Task{Id: "b"}), next: "p2"}
	gc := &GridCursor{tSource: src, filtered: src.tasks}

	gc.Reset()
	if gc.Len() != 2 || gc.Selected().Task.Id != "a" || gc.Idx() != 0 {
		t.Fatalf("len %d selected %v", gc.Len(), gc.Selected())
	}
	if !gc.NextPageExists() || gc.PreviousPageExists() {
		t.Error("page flags")
	}
	gc.selectedID = "b"
	if gc.Idx() != 1 || gc.Selected().Task.Id != "b" {
		t.Errorf("selected %s", gc.Selected().Task.Id)
	}
	// A selected task that is gone resets the cursor to the first task.
	gc.selectedID = "gone"
	if gc.Idx() != 0 || gc.selectedID != "a" {
		t.Errorf("after reset: %s", gc.selectedID)
	}
	w, err := gc.RefreshTask("x")
	if err != nil || w.Task.Id != "x" {
		t.Errorf("RefreshTask: %v %v", w, err)
	}
	if (&GridCursor{tSource: src}).Selected() != nil {
		t.Error("an empty cursor has no selection")
	}
}
