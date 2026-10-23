package workspace

import (
	"bytes"
	"strings"
	"testing"
)

func TestCommandOutputIsBoundedDuringExecution(t *testing.T) {
	var output boundedCommandOutput
	chunk := bytes.Repeat([]byte("a"), 1024*1024)
	for i := 0; i < 20; i++ {
		n, err := output.Write(chunk)
		if err != nil || n != len(chunk) {
			t.Fatalf("output writer stopped draining the command: %d %v", n, err)
		}
	}
	if output.buffer.Len() > 80000 || !strings.HasSuffix(output.String(), "[output truncated]") {
		t.Fatalf("unbounded or silently truncated output: %d", output.buffer.Len())
	}
}
