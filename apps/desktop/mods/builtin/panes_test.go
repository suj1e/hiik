package builtin

import (
	"testing"

	"hiik/mods/api"
)

// paneProgramApp is a panel app that also carries a program table.
type paneProgramApp struct {
	btPanelApp
	programs []api.Program
}

func (a *paneProgramApp) Programs() []api.Program { return a.programs }

func TestPaneHeaderShowsKnownAgentProgram(t *testing.T) {
	t.Setenv("HIIK_HOME", t.TempDir())

	app := &paneProgramApp{
		btPanelApp: btPanelApp{agents: []api.Agent{{
			Address: "t1", App: "claude", Title: "Claude", Status: "working", Space: "s1",
		}}},
		programs: []api.Program{{
			Key: "claude", Name: "Claude Code", Run: []string{"claude"},
			Icon: "icons/claude.svg", Tint: true, Tile: "#1c1d21", Color: "#ffffff",
			Match: []string{"claude-code"},
		}},
	}

	agent, known := sideFindAgent(app, "t1")
	program, ok := sideProgramFor(app, agent, known)
	if !ok || program.Key != "claude" {
		t.Fatalf("program matching = %#v, %v", program, ok)
	}
	if svg := sideIconSVG(program.Icon); svg == nil {
		t.Fatal("the agent's icon did not load")
	}

	btRenderPanel(t, api.Panels()[api.PanelPaneHead], app,
		map[string]any{"terminal": "t1"}, "Claude")
	btRenderPanel(t, api.Panels()[api.PanelPaneHead], app,
		map[string]any{"terminal": "t1"}, "Claude Code")
}

func TestPaneHeaderFallsBackWithoutProgram(t *testing.T) {
	app := &btPanelApp{agents: []api.Agent{{
		Address: "t2", App: "claude", Title: "No table", Status: "idle",
	}}}
	agent, known := sideFindAgent(app, "t2")
	if _, ok := sideProgramFor(app, agent, known); ok {
		t.Fatal("an app without Programs matched a program")
	}
	btRenderPanel(t, api.Panels()[api.PanelPaneHead], app,
		map[string]any{"terminal": "t2"}, "No table")
}
