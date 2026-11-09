package api

import "testing"

func TestPanelsAreKeyedByPanelID(t *testing.T) {
	Register(Mod{
		ID:   "test-registry",
		Name: "Test registry",
		Panels: []Panel{{
			ID:     "test/panel",
			Title:  "Test",
			Render: func(*RenderContext) {},
		}},
	})
	if _, ok := Panels()["test/panel"]; !ok {
		t.Fatal("a panel is not reachable by its ID")
	}
}

func TestDuplicateModPanics(t *testing.T) {
	Register(Mod{ID: "test-duplicate"})
	defer func() {
		if recover() == nil {
			t.Fatal("a duplicate mod ID did not panic")
		}
	}()
	Register(Mod{ID: "test-duplicate"})
}
