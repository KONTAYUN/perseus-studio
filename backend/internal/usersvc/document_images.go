package usersvc

import (
	"context"
	"log"
	"os"
	"time"
)

// The server lifetime owns this worker. No per-request timers or goroutines.
func (f *Factory) RunDocumentImageCollection(ctx context.Context) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			entries, err := os.ReadDir(f.userDataRoot)
			if err != nil {
				log.Printf("[images] list user directories: %v", err)
			}
			for _, entry := range entries {
				if ctx.Err() != nil {
					return
				}
				if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
					continue
				}
				files, err := f.Workspace(entry.Name())
				if err != nil {
					log.Printf("[images] open workspace: %v", err)
					continue
				}
				scan, cancel := context.WithTimeout(ctx, 30*time.Second)
				count, err := files.CollectDocumentImages(scan, time.Now())
				cancel()
				if err != nil {
					log.Printf("[images] collection skipped user=%s: %v", entry.Name(), err)
				}
				if count > 0 {
					log.Printf("[images] collected %d unused images user=%s", count, entry.Name())
				}
			}
			timer.Reset(time.Hour)
		}
	}
}
