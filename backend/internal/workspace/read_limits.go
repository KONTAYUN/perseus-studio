package workspace

import (
	"fmt"
	"io"
	"os"
	"time"
)

const maxFileReadBytes int64 = 64 << 20
const maxTextReadBytes int64 = 8 << 20

func readFileBounded(path string, limit int64) ([]byte, error) {
	// Windows can briefly deny opens during an atomic replacement or a scanner
	// lock. Retry only that OS error, with a bounded 150 ms total backoff.
	var raw []byte
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		raw, err = readFileBoundedOnce(path, limit)
		if !transientFileSharingError(err) || attempt == 5 {
			return raw, err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return raw, err
}

func readFileBoundedOnce(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file")
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("文件超过读取上限 %d MiB，请使用命令分段读取", limit>>20)
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("文件超过读取上限 %d MiB", limit>>20)
	}
	return raw, err
}
