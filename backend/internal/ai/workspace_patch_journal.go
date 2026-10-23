package ai

import "webide-backend/internal/workspace"

func workspacePatchJournalEntries(files *workspace.Service, writes []plannedWorkspacePatchWrite, moves []plannedWorkspacePatchMove, deletes []plannedWorkspacePatchDelete) ([]workspace.PatchJournalEntry, error) {
	var entries []workspace.PatchJournalEntry
	for _, w := range writes {
		op := "update"
		if w.create {
			op = "create"
		}
		entries = append(entries, workspace.PatchJournalEntry{Operation: op, Path: w.path, Before: w.oldContent, BeforeHash: hashWorkspaceText(w.oldContent), AfterHash: hashWorkspaceText(w.content)})
	}
	for _, m := range moves {
		entries = append(entries, workspace.PatchJournalEntry{Operation: "move", Path: m.oldPath, NewPath: m.newPath, AfterHash: m.hash})
	}
	for _, d := range deletes {
		r, err := files.ReadFile(d.path)
		if err != nil {
			return nil, err
		}
		if r.Hash != d.hash {
			return nil, workspace.ErrFileConflict
		}
		entries = append(entries, workspace.PatchJournalEntry{Operation: "delete", Path: d.path, Before: r.Content, BeforeHash: d.hash})
	}
	return entries, nil
}
