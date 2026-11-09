// Command desktop is hiik's desktop app: a native client of herdr-core.
package main

import (
	"context"
	"log"

	"github.com/egoist/mygo"

	"hiik/core"
	_ "hiik/mods/builtin"
	"hiik/ui"
)

func main() {
	app := core.New()
	mygo.App.WhenReady(func() {
		ui.New(app).Open()
		go func() {
			if err := app.Connect(context.Background()); err != nil {
				log.Printf("herdr-core: %v", err)
			}
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	app.Close()
}
