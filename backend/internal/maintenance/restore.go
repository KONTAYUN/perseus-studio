package maintenance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type DataDigest struct {
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

type UserDataManifest struct {
	Root   string `json:"root"`
	Files  int64  `json:"files"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type RestoreVerificationReport struct {
	BackupPath              string                `json:"backup_path"`
	BackupManifestPath      string                `json:"backup_manifest_path"`
	BackupSHA256            string                `json:"backup_sha256"`
	Integrity               string                `json:"integrity"`
	SchemaReadable          bool                  `json:"schema_readable"`
	ForeignKeysValid        bool                  `json:"foreign_keys_valid"`
	ProtectedTables         map[string]DataDigest `json:"protected_tables"`
	UserData                *UserDataManifest     `json:"user_data,omitempty"`
	RestoreVerified         bool                  `json:"restore_verified"`
	TemporaryRestoreRemoved bool                  `json:"temporary_restore_removed"`
	VerifiedAt              time.Time             `json:"verified_at"`
}

var protectedTables = []string{
	"users", "user_sessions", "conversations", "messages", "agent_turn_items",
	"tool_calls", "file_versions", "chat_attachments", "message_attachments", "trash_items",
}

func VerifyRestore(ctx context.Context, sourceDB *sql.DB, sourcePath, backupPath, userDataRoot string) (report RestoreVerificationReport, err error) {
	backupPath = filepath.Clean(backupPath)
	report.BackupPath = backupPath
	report.BackupManifestPath = backupPath + ".manifest.json"
	report.VerifiedAt = time.Now().UTC()

	manifestRaw, err := os.ReadFile(report.BackupManifestPath)
	if err != nil {
		return report, fmt.Errorf("read backup manifest: %w", err)
	}
	var manifest BackupReport
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return report, fmt.Errorf("decode backup manifest: %w", err)
	}
	actualSHA, err := fileSHA256(backupPath)
	if err != nil {
		return report, err
	}
	if manifest.SHA256 == "" || !strings.EqualFold(manifest.SHA256, actualSHA) {
		return report, fmt.Errorf("backup checksum does not match manifest")
	}
	report.BackupSHA256 = actualSHA

	workRoot := filepath.Join(filepath.Dir(filepath.Clean(sourcePath)), "backups")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		return report, err
	}
	tempDir, err := os.MkdirTemp(workRoot, ".restore-verify-")
	if err != nil {
		return report, err
	}
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr == nil {
			report.TemporaryRestoreRemoved = true
		} else if err == nil {
			err = fmt.Errorf("remove temporary restore: %w", removeErr)
		}
	}()
	restoredPath := filepath.Join(tempDir, "app.db")
	if err := copyFile(backupPath, restoredPath); err != nil {
		return report, fmt.Errorf("restore backup: %w", err)
	}
	restoredDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(restoredPath)+"?mode=ro")
	if err != nil {
		return report, err
	}
	defer restoredDB.Close()
	if err := restoredDB.PingContext(ctx); err != nil {
		return report, err
	}
	if err := restoredDB.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&report.Integrity); err != nil {
		return report, err
	}
	if report.Integrity != "ok" {
		return report, fmt.Errorf("restored database integrity check failed: %s", report.Integrity)
	}
	report.SchemaReadable = true
	for _, table := range requiredTables {
		exists, existsErr := tableExists(ctx, restoredDB, table)
		if existsErr != nil {
			return report, existsErr
		}
		if !exists {
			return report, fmt.Errorf("restored database is missing required table %s", table)
		}
	}
	var foreignKeyFailure int
	if err := restoredDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_foreign_key_check").Scan(&foreignKeyFailure); err != nil {
		return report, err
	}
	report.ForeignKeysValid = foreignKeyFailure == 0
	if !report.ForeignKeysValid {
		return report, fmt.Errorf("restored database has %d foreign key violation(s)", foreignKeyFailure)
	}

	report.ProtectedTables = map[string]DataDigest{}
	for _, table := range protectedTables {
		exists, existsErr := tableExists(ctx, sourceDB, table)
		if existsErr != nil {
			return report, existsErr
		}
		if !exists {
			continue
		}
		sourceDigest, digestErr := tableDigest(ctx, sourceDB, table)
		if digestErr != nil {
			return report, digestErr
		}
		restoredDigest, digestErr := tableDigest(ctx, restoredDB, table)
		if digestErr != nil {
			return report, digestErr
		}
		if sourceDigest != restoredDigest {
			return report, fmt.Errorf("restored table %s does not match source", table)
		}
		report.ProtectedTables[table] = restoredDigest
	}
	if strings.TrimSpace(userDataRoot) != "" {
		manifest, manifestErr := buildUserDataManifest(userDataRoot)
		if manifestErr != nil {
			return report, manifestErr
		}
		report.UserData = &manifest
	}
	report.RestoreVerified = true
	return report, nil
}

func tableDigest(ctx context.Context, db *sql.DB, table string) (DataDigest, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+quoteIdentifier(table)+")")
	if err != nil {
		return DataDigest{}, err
	}
	var columns []string
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return DataDigest{}, err
		}
		columns = append(columns, name)
	}
	if err := rows.Close(); err != nil {
		return DataDigest{}, err
	}
	if len(columns) == 0 {
		return DataDigest{}, fmt.Errorf("table %s has no readable columns", table)
	}
	quoted := make([]string, len(columns))
	for index, column := range columns {
		quoted[index] = quoteIdentifier(column)
	}
	query := "SELECT " + strings.Join(quoted, ",") + " FROM " + quoteIdentifier(table) + " ORDER BY " + strings.Join(quoted, ",")
	dataRows, err := db.QueryContext(ctx, query)
	if err != nil {
		return DataDigest{}, err
	}
	defer dataRows.Close()
	hash := sha256.New()
	var count int64
	for dataRows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := dataRows.Scan(destinations...); err != nil {
			return DataDigest{}, err
		}
		for _, value := range values {
			writeDigestValue(hash, value)
		}
		count++
	}
	if err := dataRows.Err(); err != nil {
		return DataDigest{}, err
	}
	return DataDigest{Rows: count, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func writeDigestValue(writer io.Writer, value any) {
	var raw []byte
	switch typed := value.(type) {
	case nil:
		raw = []byte("null")
	case []byte:
		raw = append([]byte("bytes:"), typed...)
	default:
		raw = []byte(fmt.Sprintf("%T:%v", value, typed))
	}
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(raw)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write(raw)
}

func buildUserDataManifest(root string) (UserDataManifest, error) {
	root = filepath.Clean(root)
	manifest := UserDataManifest{Root: root}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		manifest.SHA256 = hex.EncodeToString(sha256.New().Sum(nil))
		return manifest, nil
	} else if err != nil {
		return manifest, err
	}
	var files []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return manifest, err
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, path := range files {
		relative, _ := filepath.Rel(root, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return manifest, err
		}
		writeDigestValue(hash, filepath.ToSlash(relative))
		writeDigestValue(hash, content)
		manifest.Files++
		manifest.Bytes += int64(len(content))
	}
	manifest.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return manifest, nil
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}
