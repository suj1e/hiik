package builtin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinFileTreeLoadsAndPreviews(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "inner.go"), []byte("package sub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("be\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "image.png"), []byte("not really a png"), 0o644); err != nil {
		t.Fatal(err)
	}

	resetFilesState()
	filesMu.Lock()
	st := filesTreeLocked(dir)
	filesMu.Unlock()
	if st.err != "" {
		t.Fatalf("filesTreeLocked: %s", st.err)
	}
	if st.root == nil || len(st.root.children) != 4 {
		t.Fatalf("root children = %d, want 4", len(st.root.children))
	}
	if !st.root.children[0].dir || st.root.children[0].name != "sub" {
		t.Errorf("first child = %+v, want the folder first", st.root.children[0])
	}
	if got := []string{st.root.children[1].name, st.root.children[2].name, st.root.children[3].name}; got[0] != "a.txt" || got[1] != "b.txt" || got[2] != "image.png" {
		t.Errorf("sorted children = %v", got)
	}

	folder := st.root.children[0]
	filesMu.Lock()
	st.ensureLoadedLocked(folder)
	filesMu.Unlock()
	if folder.err != "" {
		t.Fatalf("load sub: %s", folder.err)
	}
	if len(folder.children) != 1 || folder.children[0].name != "inner.go" {
		t.Fatalf("sub children = %+v", folder.children)
	}

	filesMu.Lock()
	st.selectLocked(filepath.Join(dir, "a.txt"))
	filesMu.Unlock()
	if st.previewErr != "" {
		t.Fatalf("preview: %s", st.previewErr)
	}
	if st.preview == nil || st.preview.Text != "hello\n" || st.preview.Image {
		t.Errorf("preview = %+v, want hello text", st.preview)
	}

	filesMu.Lock()
	st.selectLocked(filepath.Join(dir, "image.png"))
	filesMu.Unlock()
	if st.preview == nil || !st.preview.Image {
		t.Errorf("image preview = %+v, want an image placeholder", st.preview)
	}
}

func TestBuiltinFileTreeMissingPathIsAnError(t *testing.T) {
	resetFilesState()
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	filesMu.Lock()
	st := filesTreeLocked(missing)
	filesMu.Unlock()
	if st.err == "" {
		t.Fatal("missing folder has no error")
	}
	if st.root == nil || len(st.root.children) != 0 {
		t.Fatalf("missing folder children = %+v", st.root)
	}
}

func TestBuiltinReadFilePreviewBinaryAndTruncated(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin.dat")
	if err := os.WriteFile(bin, []byte("ab\x00cd"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := readFilePreview(bin)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Binary || p.Text != "" {
		t.Errorf("binary preview = %+v", p)
	}

	big := filepath.Join(dir, "big.txt")
	buf := make([]byte, filesMaxPreviewBytes+16)
	for i := range buf {
		buf[i] = 'a'
	}
	if err := os.WriteFile(big, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err = readFilePreview(big)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Truncated || len(p.Text) != filesMaxPreviewBytes {
		t.Errorf("truncated = %v, text = %d, want %d", p.Truncated, len(p.Text), filesMaxPreviewBytes)
	}
}
