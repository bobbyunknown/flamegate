package extstore

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// UnpackArchive extracts zipPath into dest, refusing symlinks and any path that
// escapes dest (Zip-Slip / arbitrary file write). dest must already exist.
func UnpackArchive(ctx context.Context, zipPath, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("unpack: open zip: %w", err)
	}
	defer zr.Close()

	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("unpack: abs dest: %w", err)
	}

	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Symlinks are always refused: they could point outside dest.
		if f.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink %q", ErrZipTraversal, f.Name)
		}
		// Reject any ".." / "." path component and absolute paths in the raw
		// name BEFORE normalization, since Clean("/"+name) would silently drop
		// leading ".." and turn an escape into a benign-looking path.
		for _, comp := range strings.Split(f.Name, "/") {
			if comp == ".." || comp == "." || filepath.IsAbs(comp) {
				return fmt.Errorf("%w: unsafe path %q", ErrZipTraversal, f.Name)
			}
		}
		clean := filepath.Clean("/" + f.Name)
		rel := strings.TrimPrefix(clean, "/")
		if rel == "." || strings.HasPrefix(rel, "../") {
			return fmt.Errorf("%w: unsafe path %q", ErrZipTraversal, f.Name)
		}
		outPath := filepath.Join(destAbs, filepath.FromSlash(rel))
		if !strings.HasPrefix(outPath, destAbs+string(os.PathSeparator)) {
			return fmt.Errorf("%w: escape %q", ErrZipTraversal, f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(outPath, 0o755); err != nil {
				return fmt.Errorf("unpack: mkdir %s: %w", outPath, err)
			}
			continue
		}

		if err := extractFile(f, outPath); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(f *zip.File, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("unpack: mkdir %s: %w", outPath, err)
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("unpack: open %s: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode().Perm())
	if err != nil {
		return fmt.Errorf("unpack: create %s: %w", outPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		os.Remove(outPath)
		return fmt.Errorf("unpack: copy %s: %w", outPath, err)
	}
	return nil
}