// Command check-docs checks the repository's Markdown: required pages,
// one H1 per page, resolutions of relative links, the size of AGENTS.md, and
// that no document names the reference implementation.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	linkRE  = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	codeRE  = regexp.MustCompile("`[^`]*`")
	heading = regexp.MustCompile(`^# `)
	scheme  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	forbid  = regexp.MustCompile(`(?i)\b(herdr-app|gpui)\b`)
)

func main() {
	root := flag.String("root", "..", "repository root")
	flag.Parse()

	var problems []string
	required := []string{
		"README.md", "AGENTS.md", "DEVELOPMENT.md", "CONTRIBUTING.md",
		"REVIEW.md", "LESSONS.md", "docs/requirements.md",
		"docs/tech-design.md", "docs/status.md", "docs/roadmap.md",
		"docs/coding-standard.md", "docs/release.md",
	}
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(*root, name)); err != nil {
			problems = append(problems, "missing required page: "+name)
		}
	}

	err := filepath.WalkDir(*root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "dist", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		rel, _ := filepath.Rel(*root, path)

		if rel == "AGENTS.md" && strings.Count(text, "\n")+1 > 60 {
			problems = append(problems, fmt.Sprintf("%s: is longer than 60 lines", rel))
		}
		if strings.HasPrefix(rel, "docs/") && !heading.MatchString(text) {
			problems = append(problems, rel+": has no H1")
		}
		if forbid.MatchString(text) {
			problems = append(problems, rel+": names the reference implementation")
		}

		fences := fencedLines(text)
		for i, line := range strings.Split(text, "\n") {
			if fences[i] {
				continue
			}
			for _, target := range linkTargets(line) {
				if target == "" || strings.HasPrefix(target, "#") || scheme.MatchString(target) {
					continue
				}
				target = strings.SplitN(target, "#", 2)[0]
				if target == "" {
					continue
				}
				if strings.HasPrefix(target, "/") {
					continue
				}
				resolved := filepath.Join(filepath.Dir(path), filepath.FromSlash(target))
				if _, err := os.Stat(resolved); err != nil {
					problems = append(problems, fmt.Sprintf("%s:%d: broken link %q", rel, i+1, target))
				}
			}
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, p)
		}
		os.Exit(1)
	}
	fmt.Println("docs ok")
}

// fencedLines reports, per line of text, whether it starts inside a fenced
// code block. Documents are small, so one pass is enough.
func fencedLines(text string) []bool {
	lines := strings.Split(text, "\n")
	fences := make([]bool, len(lines))
	in := false
	for i, line := range lines {
		fences[i] = in
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			in = !in
		}
	}
	return fences
}

// linkTargets returns the targets of the Markdown links in line, leaving out
// links that are only code spans.
func linkTargets(line string) []string {
	line = codeRE.ReplaceAllString(line, "")
	var targets []string
	for _, m := range linkRE.FindAllStringSubmatch(line, -1) {
		targets = append(targets, strings.TrimSpace(m[1]))
	}
	return targets
}
