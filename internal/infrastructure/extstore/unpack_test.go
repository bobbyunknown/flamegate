package extstore

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnpackValid(t *testing.T) {
	zipPath := buildTestZip(t, map[string]string{
		"schema.json": "{}",
		"sub/codex.wasm": "wasm",
	})
	dest := t.TempDir()
	if err := UnpackArchive(context.Background(), zipPath, dest); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"schema.json", filepath.Join("sub", "codex.wasm")} {
		if _, err := os.Stat(filepath.Join(dest, p)); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
}

func TestUnpackRejectsTraversal(t *testing.T) {
	zipPath := buildTestZip(t, map[string]string{
		"../evil":      "x",
		"a/../../evil2": "x",
	})
	dest := t.TempDir()
	err := UnpackArchive(context.Background(), zipPath, dest)
	if err == nil {
		t.Fatal("expected zip-slip rejection")
	}
	if !errors.Is(err, ErrZipTraversal) {
		t.Fatalf("err = %v, want ErrZipTraversal", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "evil")); statErr == nil {
		t.Fatal("evil file escaped dest")
	}
}

func TestUnpackRejectsSymlink(t *testing.T) {
	zipPath := t.TempDir() + "/sym.zip"
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	// mark as symlink via mode bits before creating the entry
	hdr := &zip.FileHeader{Name: "link", Method: zip.Store}
	hdr.SetMode(os.ModeSymlink)
	hw, err := w.CreateHeader(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hw.Write([]byte("/etc/passwd")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	err = UnpackArchive(context.Background(), zipPath, dest)
	if err == nil {
		t.Fatal("expected symlink rejection")
	}
	if !errors.Is(err, ErrZipTraversal) {
		t.Fatalf("err = %v, want ErrZipTraversal", err)
	}
}

func TestUnpackWithoutDirContext(t *testing.T) {
	zipPath := buildTestZip(t, map[string]string{"a.txt": "x"})
	dest := t.TempDir()
	if err := UnpackArchive(context.Background(), zipPath, dest); err != nil {
		t.Fatal(err)
	}
}