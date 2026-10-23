package workspace

import (
	"os"
	"webide-backend/internal/failure"
)

// Trusted-host execution is deliberately opt-in. It is NOT a sandbox: the
// subprocess has the service account's filesystem and network permissions.
func TrustedHostCommandsEnabled() bool {
	return os.Getenv("PERSEUS_COMMAND_EXECUTION") == "trusted_host"
}

func commandDisabledError() error {
	return failure.New("command_disabled", "命令执行未启用；仅可信管理员可显式启用 trusted_host 模式，此模式不提供操作系统隔离", nil)
}
