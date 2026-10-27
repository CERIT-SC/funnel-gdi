package datastore

import (
	"strings"

	"cloud.google.com/go/datastore"
	"github.com/ohsu-comp-bio/funnel/server"
	"github.com/ohsu-comp-bio/funnel/tes"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
)

// GetTask implements the TES GetTask interface.
func (d *Datastore) GetTask(ctx context.Context, req *tes.GetTaskRequest) (*tes.Task, error) {
	view, err := tes.ParseView(req.View)
	if err != nil {
		return nil, err
	}

	key := taskKey(req.Id)
	entity := &task{}

	err = d.client.Get(ctx, key, entity)
	if err == datastore.ErrNoSuchEntity {
		return nil, tes.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !server.GetUser(ctx).IsAccessible(entity.Owner) {
		return nil, tes.ErrNotPermitted
	}

	task := entity.unmarshal()

	switch view {
	case tes.View_MINIMAL:
		task = task.GetMinimalView()
	case tes.View_FULL:
		// Determine the keys needed to load the various parts of the full view.
		parts := viewPartKeys(task)
		err := d.getFullView(ctx, parts, map[string]*tes.Task{
			task.Id: task,
		})
		if err != nil {
			return nil, err
		}
	}

	return task, nil
}

// getFullView retrieve the various parts of the full view from the database
// and unmarshals those into their respective tasks. This handles unmarshaling
// multiple tasks in one call, in order to support ListTasks. The "tasks" arg
// is a map of task ID -> task.
func (d *Datastore) getFullView(ctx context.Context, keys []*datastore.Key, tasks map[string]*tes.Task) error {
	proplists := make([]datastore.PropertyList, len(keys))
	err := d.client.GetMulti(ctx, keys, proplists)
	merr, isMerr := err.(datastore.MultiError)
	if err != nil && !isMerr {
		return err
	}

	for i, props := range proplists {
		if merr != nil && merr[i] != nil {
			// The task doesn't have this part, e.g. it doesn't have stdout for an executor.
			// That's ok, skip this part.
			continue
		}
		id := keys[i].Parent.Name
		task := tasks[id]
		if err := unmarshalPart(task, props); err != nil {
			return err
		}
	}
	return nil
}

// ListTasks implements the TES ListTasks interface.
func (d *Datastore) ListTasks(ctx context.Context, req *tes.ListTasksRequest) (*tes.ListTasksResponse, error) {
	view, err := tes.ParseView(req.View)
	if err != nil {
		return nil, err
	}

	page := req.PageToken
	size := tes.GetPageSize(req.GetPageSize())
	q := datastore.NewQuery("Task").Limit(size).Order("-CreationTime")

	if page != "" {
		c, err := datastore.DecodeCursor(page)
		if err != nil {
			return nil, err
		}
		q = q.Start(c)
	}

	if userInfo := server.GetUser(ctx); !userInfo.CanSeeAllTasks() {
		q = q.FilterField("Owner", "=", userInfo.Username)
	}

	if req.State != tes.Unknown {
		q = q.FilterField("State", "=", int32(req.State))
	}

	// A tag key without a value matches any value of the tag; TagStrings
	// only has key:value pairs, so such tags are checked below.
	var tagKeys []string
	for k, v := range req.GetTags() {
		if v == "" {
			tagKeys = append(tagKeys, k)
			continue
		}
		q = q.FilterField("TagStrings", "=", encodeKV(k, v))
	}

	var tasks []*tes.Task
	var parts []*datastore.Key
	byID := map[string]*tes.Task{}

	// Name is not indexed, so the name prefix is filtered here; a page can
	// have fewer tasks than its size.
	read := 0
	it := d.client.Run(ctx, q)
	for {
		t := &task{}
		_, err := it.Next(t)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		read++
		if !strings.HasPrefix(t.Name, req.NamePrefix) {
			continue
		}
		task := t.unmarshal()
		if !hasTagKeys(task, tagKeys) {
			continue
		}

		switch view {
		case tes.View_MINIMAL:
			task = task.GetMinimalView()
		case tes.View_FULL:
			// Determine the keys needed to load the various parts of the full view.
			parts = append(parts, viewPartKeys(task)...)
			byID[task.Id] = task
		}
		tasks = append(tasks, task)
	}

	// Load the full view parts
	if view == tes.View_FULL {
		err := d.getFullView(ctx, parts, byID)
		if err != nil {
			return nil, err
		}
	}

	resp := &tes.ListTasksResponse{Tasks: tasks}

	if read == size {
		c, err := it.Cursor()
		if err != nil {
			return nil, err
		}
		resp.NextPageToken = c.String()
	}

	return resp, nil
}

func hasTagKeys(task *tes.Task, keys []string) bool {
	for _, k := range keys {
		if _, ok := task.Tags[k]; !ok {
			return false
		}
	}
	return true
}
