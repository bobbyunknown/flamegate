package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

const sqliteBackupMaxBytes = 512 << 20 // 512 MiB

type sqliteStatus struct {
	Available bool   `json:"available"`
	Dialect   string `json:"dialect"`
	Path      string `json:"path,omitempty"`
}

func (s *Handler) adminSQLiteStatus(w http.ResponseWriter, r *http.Request) {
	path, ok := s.sqliteDBPath()
	status := sqliteStatus{
		Available: ok,
		Dialect:   string(s.dbDialect()),
	}
	if ok {
		status.Path = path
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Handler) adminSQLiteBackup(w http.ResponseWriter, r *http.Request) {
	path, ok := s.sqliteDBPath()
	if !ok {
		WriteError(w, http.StatusBadRequest, "SQLite backup is only available when database.driver=sqlite")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	if err := s.checkpointSQLite(ctx); err != nil {
		WriteError(w, http.StatusInternalServerError, "sqlite checkpoint failed: "+err.Error())
		return
	}

	tmp, err := os.CreateTemp(s.dataDirOrTemp(), "flamegate-sqlite-backup-*.db")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "create backup file failed: "+err.Error())
		return
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := s.db.SQL().ExecContext(ctx, "VACUUM INTO ?", tmpPath); err != nil {
		WriteError(w, http.StatusInternalServerError, "sqlite backup failed: "+err.Error())
		return
	}

	filename := "flamegate-sqlite-" + time.Now().UTC().Format("20060102-150405") + ".db"
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, tmpPath)

	_ = path // documents current DB path is intentionally resolved before backup.
}

func (s *Handler) adminSQLiteRestore(w http.ResponseWriter, r *http.Request) {
	path, ok := s.sqliteDBPath()
	if !ok {
		WriteError(w, http.StatusBadRequest, "SQLite restore is only available when database.driver=sqlite")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, sqliteBackupMaxBytes)
	if err := r.ParseMultipartForm(sqliteBackupMaxBytes); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid upload: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	if header.Size <= 0 {
		WriteError(w, http.StatusBadRequest, "uploaded file is empty")
		return
	}

	tmp, err := os.CreateTemp(s.dataDirOrTemp(), "flamegate-sqlite-restore-*.db")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "create restore file failed: "+err.Error())
		return
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := io.Copy(tmp, file); err != nil {
		_ = tmp.Close()
		WriteError(w, http.StatusInternalServerError, "save restore file failed: "+err.Error())
		return
	}
	if err := tmp.Close(); err != nil {
		WriteError(w, http.StatusInternalServerError, "close restore file failed: "+err.Error())
		return
	}

	if err := validateSQLiteFile(r.Context(), tmpPath); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid SQLite backup: "+err.Error())
		return
	}

	if err := s.checkpointSQLite(r.Context()); err != nil {
		WriteError(w, http.StatusInternalServerError, "sqlite checkpoint failed: "+err.Error())
		return
	}

	safetyPath := path + ".before-restore-" + time.Now().UTC().Format("20060102-150405")
	if err := copyFile(path, safetyPath); err != nil {
		WriteError(w, http.StatusInternalServerError, "safety backup failed: "+err.Error())
		return
	}

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		WriteError(w, http.StatusInternalServerError, "chmod restore file failed: "+err.Error())
		return
	}

	if err := os.Rename(tmpPath, path); err != nil {
		WriteError(w, http.StatusInternalServerError, "replace database failed: "+err.Error())
		return
	}

	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"restart_required": true,
		"safety_backup":    safetyPath,
	})
}

func (s *Handler) sqliteDBPath() (string, bool) {
	if s == nil || s.db == nil {
		return "", false
	}
	if s.db.Dialect() != schema.DialectSQLite {
		return "", false
	}
	path := strings.TrimSpace(s.db.SQLitePath())
	if path == "" || path == ":memory:" {
		return "", false
	}
	return path, true
}

func (s *Handler) dbDialect() schema.Dialect {
	if s == nil || s.db == nil {
		return ""
	}
	return s.db.Dialect()
}

func (s *Handler) dataDirOrTemp() string {
	if strings.TrimSpace(s.dataDir) != "" {
		return s.dataDir
	}
	return os.TempDir()
}

func (s *Handler) checkpointSQLite(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("database is not initialized")
	}
	_, err := s.db.SQL().ExecContext(ctx, "PRAGMA wal_checkpoint(FULL);")
	return err
}

func validateSQLiteFile(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=foreign_keys(ON)")
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master").Scan(&count); err != nil {
		return err
	}

	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(result)) != "ok" {
		return errors.New(result)
	}
	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return dstFile.Sync()
}
