package ai

import (
	"fmt"
	"strings"
)

func parseWorkspacePatch(input string) (workspacePatch, error) {
	text, err := normalizeWorkspacePatchEnvelope(input)
	if err != nil {
		return workspacePatch{}, err
	}
	lines := splitPatchLines(text)
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		return workspacePatch{}, parsePatchApplyError("patch apply failed: patch must start with *** Begin Patch", "patch must start with *** Begin Patch", 1, patchDiagnosticLineWindow(lines, 0), workspacePatchCandidatePathsLoose(text))
	}
	var patch workspacePatch
	var current *workspacePatchOperation
	var currentHunk *workspacePatchHunk
	finishHunk := func() {
		if current != nil && currentHunk != nil {
			current.Hunks = append(current.Hunks, *currentHunk)
			currentHunk = nil
		}
	}
	finishOp := func() {
		finishHunk()
		if current != nil {
			patch.Operations = append(patch.Operations, *current)
			current = nil
		}
	}
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		header := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		switch {
		case line == "\n" && isWorkspacePatchSeparator(lines, i):
			continue
		case header == "*** End Patch":
			finishOp()
			if i != len(lines)-1 {
				for _, rest := range lines[i+1:] {
					if strings.TrimSpace(rest) != "" {
						return workspacePatch{}, parsePatchApplyError("patch apply failed: content after *** End Patch", "content after *** End Patch", i+2, patchDiagnosticLineWindow(lines, i+1), workspacePatchCandidatePathsLoose(text))
					}
				}
			}
			return patch, nil
		case strings.HasPrefix(header, "*** Add File: "):
			finishOp()
			current = &workspacePatchOperation{Kind: workspacePatchAdd, Path: strings.TrimSpace(strings.TrimPrefix(header, "*** Add File: "))}
		case strings.HasPrefix(header, "*** Replace File: "):
			finishOp()
			current = &workspacePatchOperation{Kind: workspacePatchReplace, Path: strings.TrimSpace(strings.TrimPrefix(header, "*** Replace File: "))}
		case strings.HasPrefix(header, "*** Update File: "):
			finishOp()
			current = &workspacePatchOperation{Kind: workspacePatchUpdate, Path: strings.TrimSpace(strings.TrimPrefix(header, "*** Update File: "))}
		case strings.HasPrefix(header, "*** Move to: "):
			if current == nil || current.Kind != workspacePatchUpdate {
				return workspacePatch{}, parsePatchApplyError("patch apply failed: move directive outside update operation", "move directive outside update operation", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			if current.MovePath != "" {
				return workspacePatch{}, parsePatchApplyError(fmt.Sprintf("patch apply failed: duplicate move directive for %q", current.Path), "duplicate move directive", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			current.MovePath = strings.TrimSpace(strings.TrimPrefix(header, "*** Move to: "))
		case strings.HasPrefix(header, "*** Delete File: "):
			finishOp()
			current = &workspacePatchOperation{Kind: workspacePatchDelete, Path: strings.TrimSpace(strings.TrimPrefix(header, "*** Delete File: "))}
		case header == "*** End of File":
			if current == nil || current.Kind != workspacePatchUpdate || currentHunk == nil {
				return workspacePatch{}, parsePatchApplyError("patch apply failed: end-of-file marker outside update hunk", "end-of-file marker outside update hunk", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			currentHunk.EndOfFile = true
		case strings.HasPrefix(header, "*** "):
			return workspacePatch{}, parsePatchApplyError(fmt.Sprintf("patch apply failed: unsupported patch directive %q", header), "unsupported patch directive", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
		case strings.HasPrefix(header, "@@"):
			if current == nil || current.Kind != workspacePatchUpdate {
				return workspacePatch{}, parsePatchApplyError("patch apply failed: hunk marker outside update operation", "hunk marker outside update operation", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			context := strings.TrimSpace(strings.TrimPrefix(header, "@@"))
			if currentHunk == nil {
				currentHunk = &workspacePatchHunk{}
			} else if len(currentHunk.OldLines) > 0 || len(currentHunk.NewLines) > 0 {
				finishHunk()
				currentHunk = &workspacePatchHunk{}
			}
			if context != "" {
				currentHunk.Contexts = append(currentHunk.Contexts, context)
			}
		default:
			if current == nil {
				if strings.TrimSpace(line) == "" {
					continue
				}
				return workspacePatch{}, parsePatchApplyError("patch apply failed: content outside operation", "content outside operation", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			if current.Kind == workspacePatchDelete {
				if strings.TrimSpace(line) == "" {
					continue
				}
				return workspacePatch{}, parsePatchApplyError("patch apply failed: delete operation must not contain content", "delete operation must not contain content", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
			}
			if line == "" {
				continue
			}
			prefix := line[0]
			content := line[1:]
			switch current.Kind {
			case workspacePatchAdd, workspacePatchReplace:
				if prefix != '+' {
					return workspacePatch{}, parsePatchApplyError(fmt.Sprintf("patch apply failed: %s file lines must start with +", current.Kind), fmt.Sprintf("%s file lines must start with +", current.Kind), i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
				}
				current.Lines = append(current.Lines, content)
			case workspacePatchUpdate:
				if currentHunk == nil {
					currentHunk = &workspacePatchHunk{}
				}
				switch prefix {
				case ' ':
					currentHunk.OldLines = append(currentHunk.OldLines, content)
					currentHunk.NewLines = append(currentHunk.NewLines, content)
				case '-':
					currentHunk.OldLines = append(currentHunk.OldLines, content)
				case '+':
					currentHunk.NewLines = append(currentHunk.NewLines, content)
				default:
					return workspacePatch{}, parsePatchApplyError("patch apply failed: update hunk lines must start with space, -, or +", "update hunk lines must start with space, -, or +", i+1, patchDiagnosticLineWindow(lines, i), workspacePatchCandidatePathsLoose(text))
				}
			}
		}
	}
	return workspacePatch{}, parsePatchApplyError("patch apply failed: missing *** End Patch", "missing *** End Patch", len(lines), patchDiagnosticLineWindow(lines, len(lines)-1), workspacePatchCandidatePathsLoose(text))
}

// Blank lines between operations are formatting, not an unmarked hunk line.
// A blank line in file content must still use its explicit space/+/− prefix.
func isWorkspacePatchSeparator(lines []string, index int) bool {
	for _, line := range lines[index+1:] {
		if line == "\n" {
			continue
		}
		header := strings.TrimSpace(line)
		return header == "*** End Patch" || strings.HasPrefix(header, "*** Add File: ") || strings.HasPrefix(header, "*** Replace File: ") ||
			strings.HasPrefix(header, "*** Update File: ") || strings.HasPrefix(header, "*** Delete File: ") || strings.HasPrefix(header, "@@")
	}
	return false
}
