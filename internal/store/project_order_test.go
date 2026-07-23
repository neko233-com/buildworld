package store

import (
	"reflect"
	"testing"
)

func TestReorderProjectsPersistsExactGlobalOrder(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()

	first, err := data.CreateProject("first", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := data.CreateProject("second", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	third, err := data.CreateProject("third", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	want := []int64{second.ID, first.ID, third.ID}
	if err := data.ReorderProjects(want); err != nil {
		t.Fatal(err)
	}
	projects, err := data.ListProjectSummaries()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]int64, len(projects))
	for index, project := range projects {
		got[index] = project.ID
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("project order = %v, want %v", got, want)
	}
}

func TestReorderProjectsRejectsIncompleteAndDuplicateOrders(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()

	first, err := data.CreateProject("first", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := data.CreateProject("second", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.ReorderProjects([]int64{first.ID}); err == nil {
		t.Fatal("incomplete order succeeded")
	}
	if err := data.ReorderProjects([]int64{first.ID, first.ID}); err == nil {
		t.Fatal("duplicate order succeeded")
	}
	if err := data.ReorderProjects([]int64{first.ID, second.ID}); err != nil {
		t.Fatal(err)
	}
}
