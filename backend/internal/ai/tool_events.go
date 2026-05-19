package ai

func emitToolLifecycleEvent(opts RunOptions, eventType, level string, payload map[string]any) {
	if opts.Event != nil {
		opts.Event(eventType, level, payload)
	}
}

func logTask(opts RunOptions, level, message string) {
	if opts.Log != nil {
		opts.Log(level, message)
	}
}

func eventLevel(status string) string {
	if status == "success" {
		return "info"
	}
	return "error"
}
