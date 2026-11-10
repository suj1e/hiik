package api

// Machine is another core this app's core reaches over ssh. Machines are
// local to a core: each core keeps its own names and targets.
type Machine struct {
	// Name is what the core calls it, as in "builder/t3".
	Name string `json:"name"`
	// Host is the machine's own hostname, or its target when the core has
	// not learned the hostname yet.
	Host string `json:"host,omitempty"`
	// State is "ready", "loading" or "unavailable".
	State string `json:"state"`
	// Error says why it is not ready, or "".
	Error string `json:"error,omitempty"`
	// Temporary marks a short-lived machine, such as a task's sandbox.
	Temporary bool `json:"temporary,omitempty"`
	// OS is the machine's operating system, when the app could detect it:
	// "macOS", "Linux", "Windows" or a kernel name.
	OS string `json:"os,omitempty"`
	// Arch is the machine's CPU architecture, when detected: "arm64",
	// "amd64" or a kernel name.
	Arch string `json:"arch,omitempty"`
}

// Panel and command IDs for machines.
const (
	PanelMachines = "machines/panel"

	CmdMachineList    = "machine.list"
	CmdMachineAdd     = "machine.add"
	CmdMachineConnect = "machine.connect"
	CmdMachineRename  = "machine.rename"
	CmdMachineRm      = "machine.rm"
)
