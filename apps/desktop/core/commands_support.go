package core

import (
	"context"
	"fmt"
	"time"
)

func init() {
	registerCommand(&cmd{id: "app.check_updates", title: "Check for updates", run: runCheckUpdates})
	registerCommand(&cmd{id: "app.download_update", title: "Download update", run: runDownloadUpdate})
	registerCommand(&cmd{id: "app.install_update", title: "Install update", run: runInstallUpdate})
	registerCommand(&cmd{id: "app.update_core", title: "Update core", args: argsSpec{
		"confirm": kindBool,
	}, run: runUpdateCore})
	registerCommand(&cmd{id: "app.new_window", title: "New window", run: runNewWindow})
	registerCommand(&cmd{id: "app.whats_new", title: "What's new", run: runWhatsNew})
	registerCommand(&cmd{id: "app.feedback", title: "Save feedback", args: argsSpec{
		"text": kindString,
	}, run: runFeedback})
}

func runCheckUpdates(a *App, _ map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	st, err := a.CheckUpdates(ctx)
	a.SetOverlay("about")
	if err != nil {
		return err
	}
	if st.Latest == "" {
		return fmt.Errorf("no release found")
	}
	if Newer(st.Current, st.Latest) {
		return fmt.Errorf("hiik %s is available: %s", st.Latest, st.URL)
	}
	return nil
}

func runWhatsNew(a *App, _ map[string]any) error {
	a.SetOverlay("whats_new")
	return nil
}

// runDownloadUpdate fetches the release asset for this machine to
// ~/Downloads and opens it. The running app is never replaced.
func runDownloadUpdate(a *App, _ map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	path, err := a.DownloadUpdate(ctx)
	if err != nil {
		return err
	}
	return fmt.Errorf("update downloaded to %s; open it to install", path)
}

// runInstallUpdate downloads the release package and asks the shell to take
// over: the install overlay opens the package and quits hiik, so the package
// can replace the installed app. hiik never swaps its own binary, and the
// running core and its terminals are left alone until the app quits.
func runInstallUpdate(a *App, _ map[string]any) error {
	plan, ok := a.PendingInstall()
	if !ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		var err error
		plan, err = a.InstallUpdate(ctx)
		if err != nil {
			return err
		}
	}
	a.SetOverlay("install_update")
	return fmt.Errorf(
		"hiik %s is ready at %s; open the package, then quit hiik to finish",
		plan.Version, plan.Path)
}

// runUpdateCore asks the shell for confirmation first, then runs
// `hc update`. Terminals are left running.
func runUpdateCore(a *App, args map[string]any) error {
	confirm, _ := argBool(args, "confirm")
	if !confirm {
		a.SetOverlay("update_core")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	msg, err := a.UpdateCore(ctx)
	if err != nil {
		if msg != "" {
			return fmt.Errorf("%v\n%s", err, msg)
		}
		return err
	}
	if msg != "" {
		return fmt.Errorf("core updated: %s", firstLine(msg))
	}
	return nil
}

// runNewWindow asks the open window to make another one. The shell owns
// windows, so the command records the request and consumes it next frame.
func runNewWindow(a *App, _ map[string]any) error {
	a.SetOverlay("new_window")
	return nil
}

func runFeedback(a *App, args map[string]any) error {
	path, err := a.SaveFeedback(argString(args, "text"))
	if err != nil {
		return err
	}
	a.SetOverlay("")
	return fmt.Errorf("feedback saved to %s", path)
}
