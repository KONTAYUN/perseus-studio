package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"webide-backend/internal/idgen"

	_ "modernc.org/sqlite"
)

type DBRuntimeInfo struct {
	Path          string
	JournalMode   string
	Synchronous   string
	MountPoint    string
	MountType     string
	MountSource   string
	RiskyWALMount bool
}

var (
	lastDBRuntimeInfoMu sync.RWMutex
	lastDBRuntimeInfo   DBRuntimeInfo
)

func nextStoreID(prefix string, now time.Time) string {
	return idgen.NewAt(prefix, now)
}

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	requestedJournalMode, err := sqliteJournalModeFromEnv()
	if err != nil {
		db.Close()
		return nil, err
	}
	requestedSynchronous, err := sqliteSynchronousFromEnv()
	if err != nil {
		db.Close()
		return nil, err
	}

	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		db.Close()
		return nil, err
	}
	actualJournalMode, err := setSQLiteJournalMode(db, requestedJournalMode)
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA synchronous=" + requestedSynchronous + ";"); err != nil {
		db.Close()
		return nil, err
	}
	actualSynchronous := querySQLitePragmaString(db, "synchronous")
	if actualSynchronous == "" {
		actualSynchronous = requestedSynchronous
	}

	info := DBRuntimeInfo{
		Path:        path,
		JournalMode: strings.ToUpper(actualJournalMode),
		Synchronous: strings.ToUpper(actualSynchronous),
	}
	if mount := detectDBMount(path); mount.MountPoint != "" {
		info.MountPoint = mount.MountPoint
		info.MountType = mount.MountType
		info.MountSource = mount.MountSource
	}
	info.RiskyWALMount = strings.EqualFold(info.JournalMode, "WAL") && isRiskySQLiteWALMount(info.MountType)
	setLastDBRuntimeInfo(info)
	log.Printf("sqlite opened path=%s journal_mode=%s synchronous=%s mount_type=%s mount_point=%s", info.Path, info.JournalMode, info.Synchronous, info.MountType, info.MountPoint)
	if info.RiskyWALMount {
		log.Printf("sqlite warning: WAL is enabled on mount_type=%s mount_point=%s; SQLite WAL is not reliable on network-style filesystems", info.MountType, info.MountPoint)
	}
	return db, nil
}

func LastDBRuntimeInfo() DBRuntimeInfo {
	lastDBRuntimeInfoMu.RLock()
	defer lastDBRuntimeInfoMu.RUnlock()
	return lastDBRuntimeInfo
}

func setLastDBRuntimeInfo(info DBRuntimeInfo) {
	lastDBRuntimeInfoMu.Lock()
	defer lastDBRuntimeInfoMu.Unlock()
	lastDBRuntimeInfo = info
}

func sqliteJournalModeFromEnv() (string, error) {
	mode := strings.TrimSpace(os.Getenv("SQLITE_JOURNAL_MODE"))
	if mode == "" {
		mode = "DELETE"
	}
	mode = strings.ToUpper(mode)
	switch mode {
	case "DELETE", "TRUNCATE", "PERSIST", "MEMORY", "WAL", "OFF":
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported SQLITE_JOURNAL_MODE %q", mode)
	}
}

func sqliteSynchronousFromEnv() (string, error) {
	mode := strings.TrimSpace(os.Getenv("SQLITE_SYNCHRONOUS"))
	if mode == "" {
		mode = "FULL"
	}
	mode = strings.ToUpper(mode)
	switch mode {
	case "OFF", "NORMAL", "FULL", "EXTRA":
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported SQLITE_SYNCHRONOUS %q", mode)
	}
}

func setSQLiteJournalMode(db *sql.DB, mode string) (string, error) {
	rows, err := db.Query("PRAGMA journal_mode=" + mode + ";")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if rows.Next() {
		var actual string
		if err := rows.Scan(&actual); err != nil {
			return "", err
		}
		return actual, rows.Err()
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return mode, nil
}

func querySQLitePragmaString(db *sql.DB, name string) string {
	row := db.QueryRow("PRAGMA " + name + ";")
	var value string
	if err := row.Scan(&value); err == nil {
		if name == "synchronous" {
			switch strings.TrimSpace(value) {
			case "0":
				return "OFF"
			case "1":
				return "NORMAL"
			case "2":
				return "FULL"
			case "3":
				return "EXTRA"
			}
		}
		return value
	}
	var intValue int
	if err := db.QueryRow("PRAGMA " + name + ";").Scan(&intValue); err == nil {
		switch name {
		case "synchronous":
			switch intValue {
			case 0:
				return "OFF"
			case 1:
				return "NORMAL"
			case 2:
				return "FULL"
			case 3:
				return "EXTRA"
			}
		}
		return fmt.Sprint(intValue)
	}
	return ""
}

func detectDBMount(path string) DBRuntimeInfo {
	if runtime.GOOS == "windows" {
		return DBRuntimeInfo{}
	}
	raw, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return DBRuntimeInfo{}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)
	best := DBRuntimeInfo{}
	bestLen := -1
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		sep := -1
		for index, field := range fields {
			if field == "-" {
				sep = index
				break
			}
		}
		if sep < 5 || sep+2 >= len(fields) {
			continue
		}
		mountPoint := unescapeMountInfoField(fields[4])
		if !pathWithinMount(abs, mountPoint) || len(mountPoint) <= bestLen {
			continue
		}
		best = DBRuntimeInfo{
			MountPoint:  mountPoint,
			MountType:   strings.ToLower(fields[sep+1]),
			MountSource: unescapeMountInfoField(fields[sep+2]),
		}
		bestLen = len(mountPoint)
	}
	return best
}

func unescapeMountInfoField(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}

func pathWithinMount(path, mountPoint string) bool {
	mountPoint = filepath.Clean(mountPoint)
	if path == mountPoint {
		return true
	}
	if mountPoint == string(filepath.Separator) {
		return strings.HasPrefix(path, mountPoint)
	}
	return strings.HasPrefix(path, mountPoint+string(filepath.Separator))
}

func isRiskySQLiteWALMount(mountType string) bool {
	mountType = strings.ToLower(strings.TrimSpace(mountType))
	switch mountType {
	case "9p", "drvfs", "nfs", "nfs4", "cifs", "smb", "smbfs", "fuse.sshfs":
		return true
	default:
		return strings.Contains(mountType, "fuse") || strings.Contains(mountType, "network")
	}
}

func AddColumnIfNotExists(ctx context.Context, db *sql.DB, table, column, colDef string) error {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colDef))
	return err
}
