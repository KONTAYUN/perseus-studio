package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Build an explicit subprocess environment. Removing a few known secret names
// is insufficient: provider/plugin credentials can use arbitrary variable names.
func (s *Service) commandEnvironment() ([]string, error) {
	home := filepath.Join(filepath.Dir(s.rootAbs), ".command-home")
	temp := filepath.Join(home, "tmp")
	for _, dir := range []string{home, temp, filepath.Join(home, "AppData", "Roaming"), filepath.Join(home, "AppData", "Local")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("prepare command home: %w", err)
		}
	}
	env := filterCommandEnvironment(os.Environ())
	env = append(env,
		"HOME="+home, "USERPROFILE="+home,
		"APPDATA="+filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
		"XDG_DATA_HOME="+filepath.Join(home, ".local", "share"),
		"TEMP="+temp, "TMP="+temp, "TMPDIR="+temp,
		"GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1",
	)
	return env, nil
}

func filterCommandEnvironment(inherited []string) []string {
	allowed := map[string]bool{
		"PATH": true, "PATHEXT": true, "SYSTEMROOT": true, "WINDIR": true,
		"SYSTEMDRIVE": true, "COMSPEC": true, "OS": true,
		"NUMBER_OF_PROCESSORS": true, "PROCESSOR_ARCHITECTURE": true,
		"LANG": true, "LC_ALL": true, "LC_CTYPE": true, "LC_MESSAGES": true,
		"TERM": true, "COLORTERM": true, "TZ": true,
	}
	env := make([]string, 0, len(allowed))
	for _, entry := range inherited {
		name, _, ok := strings.Cut(entry, "=")
		if ok && allowed[strings.ToUpper(name)] {
			env = append(env, entry)
		}
	}
	return env
}
