package workspace

import (
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCommandEnvironmentOnlyInheritsRuntimeVariables(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
	inherited := []string{"Path=bin", "SystemRoot=system", "LANG=C.UTF-8", "OPENAI_API_KEY=secret", "custom_provider=secret", "PERSEUS_ADMIN_PASSWORD=secret", "HTTP_PROXY=credential", "NODE_OPTIONS=--require=evil", "LD_PRELOAD=evil", "GIT_CONFIG_COUNT=1", "HOME=host"}
	if got, want := filterCommandEnvironment(inherited), inherited[:3]; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected inherited environment: %v", got)
	}
}

func TestRunCommandDoesNotInheritServiceSecretOrHostHome(t *testing.T) {
	t.Setenv("PERSEUS_COMMAND_EXECUTION", "trusted_host")
	t.Setenv("PERSEUS_TEST_SERVICE_SECRET", "must-not-reach-child")
	s := newTestService(t)
	command := `printf '%s|%s' "$PERSEUS_TEST_SERVICE_SECRET" "$HOME"`
	if runtime.GOOS == "windows" {
		command = `[Console]::Write("$env:PERSEUS_TEST_SERVICE_SECRET|$env:USERPROFILE")`
	}
	result, err := s.RunCommand(t.Context(), command, "", 10)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("command failed: %+v %v", result, err)
	}
	wantHome := filepath.Join(filepath.Dir(s.rootAbs), ".command-home")
	if got := strings.TrimSpace(result.Stdout); got != "|"+wantHome {
		t.Fatalf("unexpected command environment: %q", got)
	}
}
