package builtin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func TestMachinesPanel(t *testing.T) {
	// A previous test or frame may have left form text behind.
	machineFormName, machineFormTarget = "", ""
	machineFormMessage, machineFormErr = "", false
	machineRenameName, machineRenameValue = "", ""
	machineHelpCommand, machineHelpMachine = "", ""

	// Read a throwaway ssh config, never the real ~/.ssh/config.
	cfg := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(cfg, []byte("Host devbox\n  HostName devbox.example\n  User me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldPath := machineSSHConfigPath
	machineSSHConfigPath = func() string { return cfg }
	defer func() { machineSSHConfigPath = oldPath }()
	machineSSHState.Lock()
	machineSSHState.loaded = false
	machineSSHState.Unlock()

	app := &btPanelApp{}
	p, ok := api.Panels()[api.PanelMachines]
	if !ok || p.Render == nil {
		t.Fatalf("panel %s is not registered", api.PanelMachines)
	}

	btRenderPanel(t, p, app, nil, "No machines yet")
	btRenderPanel(t, p, app, nil, "Add machine")
	btRenderPanel(t, p, app, nil, "From ~/.ssh/config")
	btRenderPanel(t, p, app, nil, "devbox")

	app.machines = []api.Machine{
		{Name: "builder", Host: "builder.local", State: "ready", OS: "Linux", Arch: "arm64"},
		{
			Name:      "sandbox",
			Host:      "10.0.0.7",
			State:     "unavailable",
			Error:     "connection refused",
			Temporary: true,
		},
	}
	btRenderPanel(t, p, app, nil, "builder")
	btRenderPanel(t, p, app, nil, "builder.local")
	btRenderPanel(t, p, app, nil, "Linux")

	// A temporary machine is an agent's sandbox, not a row a person manages.
	tt := ui.NewTester(func(c *ui.Context) {
		p.Render(&api.RenderContext{C: c, App: app})
	}, 900, 600)
	tt.Frame()
	for _, hidden := range []string{"sandbox", "connection refused", "temporary"} {
		if tt.HasText(hidden) {
			t.Errorf("machines panel showed temporary machine text %q", hidden)
		}
	}

	machineFormName, machineFormTarget = "box", "me@box"
	machineAdd(&api.RenderContext{App: app})
	if len(app.ran) != 1 || app.ran[0] != api.CmdMachineAdd {
		t.Fatalf("ran = %v, want %s", app.ran, api.CmdMachineAdd)
	}
	if got := app.args[0]["name"]; got != "box" {
		t.Errorf("name = %v, want box", got)
	}
	if got := app.args[0]["target"]; got != "me@box" {
		t.Errorf("target = %v, want me@box", got)
	}
	if machineFormName != "" || machineFormTarget != "" {
		t.Errorf("form was not cleared: %q %q", machineFormName, machineFormTarget)
	}
	machineFormMessage, machineFormErr = "", false
	machineHelpCommand, machineHelpMachine = "", ""
}

func TestMachinesPanelNewSpaceHere(t *testing.T) {
	machineHelpCommand, machineHelpMachine = "", ""
	app := &btPanelApp{}
	machineNewSpace(&api.RenderContext{App: app}, "builder", "builder")
	if len(app.ran) != 1 || app.ran[0] != api.CmdSpaceNew {
		t.Fatalf("ran = %v, want %s", app.ran, api.CmdSpaceNew)
	}
	if got := app.args[0]["machine"]; got != "builder" {
		t.Fatalf("machine = %v", got)
	}

	app = &btPanelApp{runErr: errors.New("start terminal: hc: terminal.run: ssh: password required (auth_failed)")}
	machineNewSpace(&api.RenderContext{App: app}, "builder", "builder")
	if !machineFormErr || !strings.Contains(machineFormMessage, "password required") {
		t.Fatalf("message = %q err = %v", machineFormMessage, machineFormErr)
	}
}

func TestMachineAddKeepsCoreMessageAndCommand(t *testing.T) {
	machineFormName, machineFormTarget = "", "me@box"
	machineFormMessage, machineFormErr = "", false
	machineHelpCommand, machineHelpMachine = "", ""

	coreErr := "machine.add: hc: machine.add: Enter password (login_needed)"
	machineAdd(&api.RenderContext{App: &btPanelApp{runErr: errors.New(coreErr)}})
	if machineFormMessage != coreErr {
		t.Fatalf("message = %q, want the core's exact text", machineFormMessage)
	}
	if got := machineHelpCommand; got != "hc machine add me@box" {
		t.Fatalf("command = %q", got)
	}
	if machineHelpMachine != "" {
		t.Fatalf("unnamed add should not offer a retry machine: %q", machineHelpMachine)
	}

	// A target with shell metacharacters is one quoted argument.
	machineFormTarget = "host';echo $HOME"
	machineAdd(&api.RenderContext{App: &btPanelApp{runErr: errors.New("new host key")}})
	if got := machineHelpCommand; got != `hc machine add 'host'\'';echo $HOME'` {
		t.Fatalf("quoted command = %q", got)
	}

	// Other failures keep the message but promise no command.
	machineFormTarget = "me@box"
	machineAdd(&api.RenderContext{App: &btPanelApp{runErr: errors.New("connection refused")}})
	if machineHelpCommand != "" {
		t.Fatalf("command = %q, want none", machineHelpCommand)
	}
	machineFormName, machineFormTarget = "", ""
	machineFormMessage, machineFormErr = "", false
	machineHelpCommand, machineHelpMachine = "", ""
}

func TestParseSSHConfigHosts(t *testing.T) {
	text := `
# a comment
Host builder builder.lan
  HostName builder.example.com
  User me

host = web
  hostname 10.0.0.5

Host *.internal ?single !negated
  HostName ignored

Match host bastion
  User jump

Host builder
  HostName duplicate.example
Host db
  HostName db.example
  User admin # trailing comment
`
	hosts := parseSSHConfigHosts(text)
	if len(hosts) != 3 {
		t.Fatalf("hosts = %d, want 3: %+v", len(hosts), hosts)
	}
	if hosts[0].Alias != "builder" || hosts[0].HostName != "builder.example.com" || hosts[0].User != "me" {
		t.Errorf("builder = %+v", hosts[0])
	}
	if len(hosts[0].Aliases) != 1 || hosts[0].Aliases[0] != "builder.lan" {
		t.Errorf("builder aliases = %v", hosts[0].Aliases)
	}
	if hosts[0].Target() != "me@builder.example.com" {
		t.Errorf("builder target = %q", hosts[0].Target())
	}
	if hosts[1].Alias != "web" || hosts[1].HostName != "10.0.0.5" || hosts[1].User != "" {
		t.Errorf("web = %+v", hosts[1])
	}
	if hosts[2].Alias != "db" || hosts[2].User != "admin" {
		t.Errorf("db = %+v", hosts[2])
	}
}

func TestMachineVisibleMachinesHidesTemporary(t *testing.T) {
	got := machineVisibleMachines([]api.Machine{
		{Name: "builder"},
		{Name: "sandbox", Temporary: true},
		{Name: "box"},
	})
	if len(got) != 2 || got[0].Name != "builder" || got[1].Name != "box" {
		t.Fatalf("visible = %+v", got)
	}
}

func TestMachinePickerHostsSkipsKnownAndTemporary(t *testing.T) {
	hosts := []sshConfigHost{
		{Alias: "builder"},
		{Alias: "sandbox"},
		{Alias: "db", HostName: "db.example.com", User: "admin"},
		{Alias: "fresh", HostName: "fresh.example.com"},
	}
	got := machinePickerHosts(hosts, []api.Machine{
		{Name: "builder", Host: "builder.local"},
		{Name: "sandbox", Host: "sandbox", Temporary: true},
		{Name: "database", Host: "admin@db.example.com"},
	})
	if len(got) != 1 || got[0].Alias != "fresh" {
		t.Fatalf("hosts = %+v, want only fresh", got)
	}
}

func TestMachineAddHostArgs(t *testing.T) {
	machineFormMessage, machineFormErr = "", false
	machineHelpCommand, machineHelpMachine = "", ""

	app := &btPanelApp{}
	machineAddHost(&api.RenderContext{App: app}, sshConfigHost{
		Alias:    "builder",
		HostName: "builder.example.com",
		User:     "me",
	})
	if len(app.ran) != 1 || app.ran[0] != api.CmdMachineAdd {
		t.Fatalf("ran = %v", app.ran)
	}
	if got := app.args[0]["target"]; got != "builder" {
		t.Fatalf("target = %v, want the ssh alias", got)
	}
	if _, ok := app.args[0]["name"]; ok {
		t.Fatalf("an ssh host add should let the core name it: %v", app.args[0])
	}
	if !strings.Contains(machineFormMessage, "Added builder") || machineFormErr {
		t.Fatalf("message = %q err = %v", machineFormMessage, machineFormErr)
	}

	app = &btPanelApp{runErr: errors.New("machine.add: host key verification failed")}
	machineAddHost(&api.RenderContext{App: app}, sshConfigHost{Alias: "box"})
	if machineHelpCommand != "hc machine add box" || machineHelpMachine != "box" {
		t.Fatalf("help = %q machine = %q", machineHelpCommand, machineHelpMachine)
	}
	machineFormMessage, machineFormErr = "", false
	machineHelpCommand, machineHelpMachine = "", ""
}
