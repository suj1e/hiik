// Command hiik-probe exercises the herdr-core client without the UI: it
// attaches to a terminal and forwards lines from stdin. It is a development
// tool for the M0 protocol spike.
//
//	HC_SESSION=hiik-test go run ./cmd/hiik-probe -run /bin/sh
//
// Use HC_SESSION=hiik-test so it never touches your everyday core.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"hiik/core/hc"
)

func main() {
	var (
		session = flag.String("session", "", "core session (default: HC_SESSION)")
		attach  = flag.String("attach", "", "terminal address to attach; empty: first live or a new shell")
		run     = flag.String("run", "/bin/sh", "program to run when starting a terminal")
		cols    = flag.Int("cols", 100, "columns")
		rows    = flag.Int("rows", 30, "rows")
		history = flag.Int("history", 200, "history lines")
		resize  = flag.String("resize", "", "resize after attach, as COLSxROWS")
		events  = flag.Bool("events", false, "print events for two seconds after attach")
		list    = flag.Bool("list", false, "list live terminals and exit")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := hc.Dial(ctx, hc.Options{Session: *session})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	hello := client.Hello()
	fmt.Printf("hello: generation=%d version=%s methods=%d\n", hello.Generation, hello.Version, len(hello.Methods))

	terms, err := client.List(ctx, false)
	if err != nil {
		log.Fatal(err)
	}
	if *list {
		for _, t := range terms {
			fmt.Printf("%s\t%s\t%s\t%s\n", t.Address, t.State, t.Title, t.Cwd)
		}
		return
	}

	address := *attach
	if address == "" {
		if len(terms) > 0 {
			address = terms[0].Address
			fmt.Printf("using live terminal %s\n", address)
		} else {
			argv := strings.Fields(*run)
			if len(argv) == 0 {
				argv = []string{"/bin/sh"}
			}
			term, err := client.Run(ctx, hc.RunRequest{
				Argv:   argv,
				Labels: map[string]string{"hiik.probe": "1"},
				Cols:   *cols,
				Rows:   *rows,
			})
			if err != nil {
				log.Fatal(err)
			}
			address = term.Address
			fmt.Printf("started %s\n", address)
			defer func() { _ = client.Kill(context.Background(), address) }()
		}
	}

	at, err := client.Attach(ctx, hc.AttachRequest{
		Terminal: address,
		Cols:     *cols,
		Rows:     *rows,
		History:  *history,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer at.Close()

	if *resize != "" {
		c, r, ok := strings.Cut(*resize, "x")
		if !ok {
			log.Fatal("-resize must be COLSxROWS")
		}
		nc, err1 := strconv.Atoi(c)
		nr, err2 := strconv.Atoi(r)
		if err1 != nil || err2 != nil {
			log.Fatal("-resize must be COLSxROWS")
		}
		if resizer, ok := at.Conn.(interface{ Resize(int, int) error }); ok {
			if err := resizer.Resize(nc, nr); err != nil {
				log.Fatal(err)
			}
			fmt.Printf("resized to %dx%d\n", nc, nr)
		} else {
			log.Print("the attachment does not resize")
		}
	}

	if *events {
		sub, err := client.Subscribe(ctx)
		if err != nil {
			log.Fatal(err)
		}
		defer sub.Close()
		fmt.Printf("snapshot: seq=%d terminals=%d\n", sub.Snapshot.Seq, len(sub.Snapshot.Terminals))
		deadline := time.After(2 * time.Second)
		for {
			select {
			case ev, ok := <-sub.Events:
				if !ok {
					return
				}
				fmt.Printf("event seq=%d type=%s terminal=%s data=%s\n", ev.Seq, ev.Type, ev.Terminal, ev.Data)
			case <-deadline:
				goto REPL
			}
		}
	}

REPL:
	fmt.Fprintf(os.Stderr, "attached %s; type lines, ctrl-d to quit\n", address)
	go func() { _, _ = io.Copy(os.Stdout, at.Conn) }()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if _, err := fmt.Fprintf(at.Conn, "%s\n", scanner.Text()); err != nil {
			log.Fatal(err)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	// Let the shell answer and the reader goroutine print before detaching.
	time.Sleep(300 * time.Millisecond)
}
