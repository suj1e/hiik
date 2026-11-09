package builtin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func init() {
	api.Register(api.Mod{
		ID:   "machines",
		Name: "Machines",
		Panels: []api.Panel{{
			ID:     api.PanelMachines,
			Title:  "Machines",
			Render: renderMachines,
		}},
	})
}

// The panel is a single instance, so its unfinished add form and rename
// field live at package scope instead of on the app. Nothing here starts a
// goroutine or timer; a frame only reads and writes these strings.
var (
	machineFormName    string
	machineFormTarget  string
	machineFormMessage string
	machineFormErr     bool

	machineRenameName  string
	machineRenameValue string

	// When the core says a person must answer something first, this is the
	// command to run in a terminal and the machine to retry, if known.
	machineHelpCommand string
	machineHelpMachine string
)

// sshConfigHost is one named host from ~/.ssh/config: the alias a person
// types, and the HostName/User that ssh resolves it to when both are set.
type sshConfigHost struct {
	Alias    string
	Aliases  []string
	HostName string
	User     string
}

// Target is the address shown beside an alias. The alias is what ssh is
// given; this only helps a person recognise the host.
func (h sshConfigHost) Target() string {
	host := strings.TrimSpace(h.HostName)
	if host == "" {
		host = h.Alias
	}
	user := strings.TrimSpace(h.User)
	if user == "" {
		return host
	}
	return user + "@" + host
}

// Detail lists the resolved address and any other names the block answers
// to, or the alias when there is nothing more to say.
func (h sshConfigHost) Detail() string {
	parts := []string{}
	if target := h.Target(); target != "" && target != h.Alias {
		parts = append(parts, target)
	}
	if len(h.Aliases) > 0 {
		parts = append(parts, "also "+strings.Join(h.Aliases, ", "))
	}
	if len(parts) == 0 {
		return "ssh config host"
	}
	return strings.Join(parts, " · ")
}

// machineSSHConfigPath is the file the Add flow reads. Tests replace it.
var machineSSHConfigPath = func() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".ssh", "config")
}

// machineSSHState caches the parsed hosts while the file is unchanged, so a
// frame never reads the file it just read.
var machineSSHState struct {
	sync.Mutex
	path    string
	modTime int64
	size    int64
	loaded  bool
	hosts   []sshConfigHost
}

// machineSSHHosts returns the named hosts from ~/.ssh/config, re-reading it
// when it changed. A missing or unreadable file is simply no hosts.
func machineSSHHosts() []sshConfigHost {
	path := machineSSHConfigPath()
	var modTime, size int64
	if info, err := os.Stat(path); err == nil {
		modTime = info.ModTime().UnixNano()
		size = info.Size()
	}

	machineSSHState.Lock()
	defer machineSSHState.Unlock()
	if machineSSHState.loaded && machineSSHState.path == path &&
		machineSSHState.modTime == modTime && machineSSHState.size == size {
		return cloneSSHHosts(machineSSHState.hosts)
	}
	var hosts []sshConfigHost
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			hosts = parseSSHConfigHosts(string(data))
		}
	}
	machineSSHState.path = path
	machineSSHState.modTime = modTime
	machineSSHState.size = size
	machineSSHState.hosts = hosts
	machineSSHState.loaded = true
	return cloneSSHHosts(hosts)
}

func cloneSSHHosts(list []sshConfigHost) []sshConfigHost {
	if len(list) == 0 {
		return nil
	}
	out := make([]sshConfigHost, len(list))
	for i, h := range list {
		h.Aliases = append([]string(nil), h.Aliases...)
		out[i] = h
	}
	return out
}

// parseSSHConfigHosts reads the named Host blocks of an ssh config. Patterns
// (names with * ? ! or [) are not pickable hosts, a Match block ends the
// current host, and later HostName/User lines apply to it. Aliases are kept
// only once, in the order the file names them.
func parseSSHConfigHosts(text string) []sshConfigHost {
	var out []sshConfigHost
	seen := map[string]bool{}
	cur := -1
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value := line, ""
		if i := strings.IndexAny(line, " \t="); i >= 0 {
			key = line[:i]
			value = strings.TrimSpace(strings.TrimLeft(line[i:], " \t="))
		}
		if value != "" {
			if i := strings.Index(value, " #"); i >= 0 {
				value = strings.TrimSpace(value[:i])
			}
		}
		switch strings.ToLower(key) {
		case "host":
			cur = -1
			var names []string
			for _, name := range strings.Fields(value) {
				if strings.HasPrefix(name, "#") {
					break
				}
				if strings.ContainsAny(name, "*?![]") || seen[name] {
					continue
				}
				seen[name] = true
				names = append(names, name)
			}
			if len(names) == 0 {
				continue
			}
			out = append(out, sshConfigHost{
				Alias:   names[0],
				Aliases: append([]string(nil), names[1:]...),
			})
			cur = len(out) - 1
		case "match":
			cur = -1
		case "hostname":
			if cur >= 0 && value != "" {
				out[cur].HostName = value
			}
		case "user":
			if cur >= 0 && value != "" {
				out[cur].User = value
			}
		}
	}
	return out
}

// machineVisibleMachines leaves out temporary machines: they are agents'
// short-lived sandboxes and are not something a person manages here.
func machineVisibleMachines(list []api.Machine) []api.Machine {
	out := make([]api.Machine, 0, len(list))
	for _, m := range list {
		if m.Temporary {
			continue
		}
		out = append(out, m)
	}
	return out
}

// machinePickerHosts leaves out hosts the core already knows, whether an
// ordinary or a temporary machine. An ssh alias is offered once.
func machinePickerHosts(hosts []sshConfigHost, machines []api.Machine) []sshConfigHost {
	added := func(h sshConfigHost) bool {
		names := []string{h.Alias, h.HostName, h.Target()}
		names = append(names, h.Aliases...)
		for _, m := range machines {
			if m.Name != "" && strings.EqualFold(m.Name, h.Alias) {
				return true
			}
			for _, name := range names {
				if name == "" {
					continue
				}
				if strings.EqualFold(m.Host, name) || strings.EqualFold(m.Name, name) {
					return true
				}
				if at := strings.LastIndex(m.Host, "@"); at >= 0 &&
					strings.EqualFold(m.Host[at+1:], name) {
					return true
				}
			}
		}
		return false
	}
	out := make([]sshConfigHost, 0, len(hosts))
	for _, h := range hosts {
		if !added(h) {
			out = append(out, h)
		}
	}
	return out
}

func renderMachines(rc *api.RenderContext) {
	if rc == nil || rc.C == nil || rc.App == nil {
		return
	}
	c := rc.C
	t := c.Theme()
	allMachines := rc.App.Machines()
	machines := machineVisibleMachines(allMachines)

	ui.Column(c).Fill().Gap(6).Padding(8).Children(func() {
		ui.Row(c).Height(26).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "Machines").FontSize(t.FontSize - 2).Bold().TextColor(t.TextMuted)
			ui.Box(c).Grow(1)
			if n := len(machines); n > 0 {
				ui.Text(c, machineCount(n)).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			}
			if ui.Button(c.Key("machines/refresh"), "Refresh").Clicked() {
				machineRun(rc, api.CmdMachineList, nil, "")
			}
		})

		if len(machines) == 0 {
			ui.Text(c, "No machines yet").FontSize(t.FontSize - 1).TextColor(t.TextMuted)
			ui.Text(c, "Add one below to run spaces and terminals there.").
				FontSize(t.FontSize - 2).
				TextColor(t.TextMuted)
		} else {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					for _, m := range machines {
						machineRow(rc, m, t)
					}
				})
			})
		}

		machineAddForm(rc, t)
		machineSSHHostList(rc, t, allMachines)
		machineHelpBlock(rc, t)
	})
}

// machineSSHHostList offers the named hosts in ~/.ssh/config as one-click
// adds. A host is what ssh is given, so clicking Add passes the alias.
func machineSSHHostList(rc *api.RenderContext, t *ui.Theme, machines []api.Machine) {
	c := rc.C
	hosts := machinePickerHosts(machineSSHHosts(), machines)
	ui.Column(c).
		Gap(4).
		Padding(8, 0, 0, 0).
		BorderWidth(1, 0, 0, 0).
		BorderColor(t.Border).
		Children(func() {
			ui.Row(c).Height(20).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, "From ~/.ssh/config").FontSize(t.FontSize - 2).Bold().TextColor(t.TextMuted)
				ui.Box(c).Grow(1)
				ui.Text(c, fmt.Sprintf("%d", len(hosts))).
					FontSize(t.FontSize - 2).
					TextColor(t.TextMuted)
			})
			if len(hosts) == 0 {
				ui.Text(c, "No new hosts in ~/.ssh/config").
					FontSize(t.FontSize - 2).
					TextColor(t.TextMuted).
					SingleLine()
				return
			}
			ui.Scroll(c).MaxHeight(150).Children(func() {
				ui.Column(c).Gap(2).Children(func() {
					for _, h := range hosts {
						machineSSHHostRow(rc, h, t)
					}
				})
			})
		})
}

func machineSSHHostRow(rc *api.RenderContext, h sshConfigHost, t *ui.Theme) {
	c := rc.C
	key := h.Alias
	if key == "" {
		return
	}
	ui.Row(c).Key("machines/add/host/" + key).Height(24).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, h.Alias).FontSize(t.FontSize - 1).Bold().SingleLine()
		ui.Text(c, h.Detail()).
			FontSize(t.FontSize - 2).
			TextColor(t.TextMuted).
			SingleLine().
			Grow(1).
			Tooltip(h.Detail())
		if ui.Button(c.Key("machines/add/host/add/"+key), "Add").Clicked() {
			machineAddHost(rc, h)
		}
	})
}

// machineAddHost adds one ssh config alias. A failure keeps the core's
// words and, when someone must answer a password or a host key, the exact
// command to run.
func machineAddHost(rc *api.RenderContext, h sshConfigHost) {
	target := strings.TrimSpace(h.Alias)
	if target == "" {
		machineNotice("An ssh host is required", true)
		return
	}
	if err := rc.App.Run(api.CmdMachineAdd, map[string]any{"target": target}); err != nil {
		machineNotice(err.Error(), true)
		if machineNeedsHelp(err.Error()) {
			machineHelpCommand = "hc machine add " + machineQuote(target)
			machineHelpMachine = h.Alias
		} else {
			machineHelpCommand, machineHelpMachine = "", ""
		}
		return
	}
	machineHelpCommand, machineHelpMachine = "", ""
	machineNotice("Added "+target, false)
}

func machineCount(n int) string {
	if n == 1 {
		return "1 machine"
	}
	return fmt.Sprintf("%d machines", n)
}

func machineRow(rc *api.RenderContext, m api.Machine, t *ui.Theme) {
	c := rc.C
	label := m.Name
	if label == "" {
		label = m.Host
	}
	if label == "" {
		label = "unnamed"
	}

	ui.Column(c).Key("machines/"+m.Name).Gap(2).Padding(4, 6).Children(func() {
		if machineRenameName == m.Name {
			machineRenameRow(rc, m, label, t)
			return
		}
		ui.Row(c).Height(24).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Box(c).
				Size(7, 7).
				Radius(4).
				Background(machineStateColor(t, m.State)).
				Tooltip(machineStateLabel(m.State))
			ui.Text(c, label).FontSize(t.FontSize).Bold().SingleLine()
			host := m.Host
			if host == "" {
				host = m.Name
			}
			ui.Text(c, host).
				FontSize(t.FontSize - 1).
				TextColor(t.TextMuted).
				SingleLine().
				Grow(1).
				Tooltip(host)
			if ui.Button(c.Key("machines/space/"+m.Name), "New space here").Clicked() {
				machineNewSpace(rc, m.Name, label)
			}
			if ui.Button(c.Key("machines/connect/"+m.Name), "Connect").Clicked() {
				machineRun(rc, api.CmdMachineConnect, map[string]any{"machine": m.Name}, "")
			}
			if ui.Button(c.Key("machines/rename/"+m.Name), "Rename").Clicked() {
				machineRenameName = m.Name
				machineRenameValue = m.Name
			}
			if ui.Button(c.Key("machines/remove/"+m.Name), "Remove").Clicked() {
				if err := rc.App.Run(api.CmdMachineRm, map[string]any{"machine": m.Name}); err != nil {
					machineNotice(err.Error(), true)
				} else {
					if machineRenameName == m.Name {
						machineRenameName = ""
					}
					machineNotice("Removed "+label, false)
				}
			}
		})
		if m.Error != "" {
			ui.Text(c, m.Error).
				FontSize(t.FontSize - 2).
				TextColor(t.Danger).
				SingleLine().
				Tooltip(m.Error)
		}
	})
}

// machineNewSpace makes a space whose terminals start on the machine. The
// core runs them with `on`; a failure keeps the core's own message.
func machineNewSpace(rc *api.RenderContext, machine, label string) {
	if err := rc.App.Run(api.CmdSpaceNew, map[string]any{"machine": machine}); err != nil {
		machineNotice(err.Error(), true)
		return
	}
	machineNotice("New space on "+label, false)
}

func machineRenameRow(rc *api.RenderContext, m api.Machine, label string, t *ui.Theme) {
	c := rc.C
	ui.Row(c).Height(24).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(t.FontSize - 1).Bold().Width(80).SingleLine()
		ui.TextInput(c.Key("machines/rename/input"), &machineRenameValue).
			Label("New machine name").
			Placeholder("new name").
			Grow(1)
		if ui.Button(c.Key("machines/rename/save"), "Save").Clicked() {
			name := strings.TrimSpace(machineRenameValue)
			if name == "" {
				machineNotice("A machine name is required", true)
				return
			}
			if err := rc.App.Run(api.CmdMachineRename, map[string]any{
				"machine":  m.Name,
				"new_name": name,
			}); err != nil {
				machineNotice(err.Error(), true)
				return
			}
			machineRenameName = ""
			machineNotice("Renamed "+label+" to "+name, false)
		}
		if ui.Button(c.Key("machines/rename/cancel"), "Cancel").Clicked() {
			machineRenameName = ""
		}
	})
}

func machineAddForm(rc *api.RenderContext, t *ui.Theme) {
	c := rc.C
	ui.Column(c).
		Gap(4).
		Padding(8, 0, 0, 0).
		BorderWidth(1, 0, 0, 0).
		BorderColor(t.Border).
		Children(func() {
			ui.Text(c, "Add machine").FontSize(t.FontSize - 1).Bold()
			ui.Row(c).Gap(6).Children(func() {
				ui.TextInput(c.Key("machines/add/name"), &machineFormName).
					Label("Machine name").
					Placeholder("name").
					Width(140).
					Shrink(0)
				ui.TextInput(c.Key("machines/add/target"), &machineFormTarget).
					Label("Machine target").
					Placeholder("user@host or an ssh Host").
					Grow(1)
				if ui.Button(c.Key("machines/add"), "Add").Clicked() {
					machineAdd(rc)
				}
			})
			ui.Text(c, "The core runs `hc` there over ssh; it may ask for a password or host key.").
				FontSize(t.FontSize - 2).
				TextColor(t.TextMuted).
				SingleLine()
			if machineFormMessage != "" {
				color := t.TextMuted
				if machineFormErr {
					color = t.Danger
				}
				ui.Text(c, machineFormMessage).
					FontSize(t.FontSize - 2).
					TextColor(color).
					SingleLine().
					Tooltip(machineFormMessage)
			}
		})
}

func machineAdd(rc *api.RenderContext) {
	name := strings.TrimSpace(machineFormName)
	target := strings.TrimSpace(machineFormTarget)
	if target == "" {
		machineNotice("A target is required, such as user@host or an ssh Host", true)
		return
	}
	args := map[string]any{"target": target}
	if name != "" {
		args["name"] = name
	}
	if err := rc.App.Run(api.CmdMachineAdd, args); err != nil {
		// Keep the core's own words; add the command that lets a person
		// answer the password, host-key or missing-core question.
		machineNotice(err.Error(), true)
		if machineNeedsHelp(err.Error()) {
			machineHelpCommand = "hc machine add " + machineQuote(target)
			machineHelpMachine = name
		} else {
			machineHelpCommand, machineHelpMachine = "", ""
		}
		return
	}
	machineFormName, machineFormTarget = "", ""
	machineHelpCommand, machineHelpMachine = "", ""
	added := name
	if added == "" {
		added = target
	}
	machineNotice("Added "+added, false)
}

// machineHelpBlock shows the command a person runs in a terminal when the
// core needs a password, a host key or a core installed on the far side.
func machineHelpBlock(rc *api.RenderContext, t *ui.Theme) {
	if machineHelpCommand == "" {
		return
	}
	c := rc.C
	ui.Column(c).
		Gap(4).
		Padding(8, 0, 0, 0).
		BorderWidth(1, 0, 0, 0).
		BorderColor(t.Border).
		Children(func() {
			ui.Text(c, "Needs you at a terminal").FontSize(t.FontSize - 1).Bold().TextColor(t.Warning)
			ui.TextInput(c.Key("machines/help/command"), &machineHelpCommand).
				Label("Run this command").
				ReadOnly(true).
				Grow(1)
			ui.Row(c).Gap(6).Children(func() {
				if ui.Button(c.Key("machines/help/refresh"), "Refresh").Clicked() {
					machineRun(rc, api.CmdMachineList, nil, "")
				}
				if machineHelpMachine != "" {
					if ui.Button(c.Key("machines/help/connect"), "Connect again").Clicked() {
						machineRun(rc, api.CmdMachineConnect,
							map[string]any{"machine": machineHelpMachine},
							"Connected "+machineHelpMachine)
					}
				}
			})
			ui.Text(c, "After it answers, refresh to see the machine here.").
				FontSize(t.FontSize - 2).
				TextColor(t.TextMuted).
				SingleLine()
		})
}

// machineNeedsHelp reports messages that a person must resolve in a
// terminal: a password, a host key, a login or a missing core.
func machineNeedsHelp(text string) bool {
	text = strings.ToLower(text)
	for _, key := range []string{
		"password", "passphrase", "host key", "host_key", "permission denied",
		"login_needed", "needs login", "needs_login", "auth_failed",
		"hc_missing", "incompatible", "missing core", "core not found",
		"not installed", "no such file",
	} {
		if strings.Contains(text, key) {
			return true
		}
	}
	return false
}

// machineQuote quotes a target as one shell word, so a copied command runs
// unchanged even for an ssh target with spaces or quotes.
func machineQuote(s string) string {
	safe := s != ""
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_@%+=:,./-", r):
		default:
			safe = false
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// machineRun forwards one action to the core and shows its message on
// failure. The core's own error text is kept so a login or host-key problem
// reaches the user unchanged.
func machineRun(rc *api.RenderContext, command string, args map[string]any, success string) {
	if err := rc.App.Run(command, args); err != nil {
		machineNotice(err.Error(), true)
		return
	}
	if machineRenameName != "" {
		machineRenameName = ""
	}
	machineHelpCommand, machineHelpMachine = "", ""
	machineNotice(success, false)
}

func machineNotice(message string, isErr bool) {
	machineFormMessage = message
	machineFormErr = isErr
}

func machineStateColor(t *ui.Theme, state string) ui.Color {
	switch state {
	case "ready":
		return t.Success
	case "loading":
		return t.Accent
	case "unavailable":
		return t.Danger
	default:
		return t.TextMuted
	}
}

func machineStateLabel(state string) string {
	switch state {
	case "ready":
		return "Ready"
	case "loading":
		return "Connecting"
	case "unavailable":
		return "Unavailable"
	default:
		return state
	}
}
