package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"webide-backend/internal/ai"
)

func (s *Server) parseConversationMessageRequest(r *http.Request) (string, string, string, []byte, []ai.AttachmentPayload, []string, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return s.parseMultipartConversationMessage(r)
	}

	var req createConversationMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return "", "", "", nil, nil, nil, fmt.Errorf("invalid json body")
	}
	attachmentsJSON := []byte("[]")
	if len(req.AttachmentsJSON) > 0 {
		attachmentsJSON = req.AttachmentsJSON
	}
	attachmentIDs := uniqueAttachmentIDs(req.AttachmentIDs)
	payloads, imageMetadata, err := s.loadStagedChatAttachmentInputs(r, attachmentIDs)
	if err != nil {
		return "", "", "", nil, nil, nil, err
	}
	if len(imageMetadata) > 0 {
		var metadata []chatAttachmentMetadata
		_ = json.Unmarshal(attachmentsJSON, &metadata)
		metadata = append(metadata, imageMetadata...)
		attachmentsJSON, _ = json.Marshal(metadata)
	}
	return req.Content, req.ModelProviderID, req.ThinkingMode, attachmentsJSON, payloads, attachmentIDs, nil
}

func (s *Server) parseMultipartConversationMessage(r *http.Request) (string, string, string, []byte, []ai.AttachmentPayload, []string, error) {
	if err := r.ParseMultipartForm(80 << 20); err != nil {
		return "", "", "", nil, nil, nil, fmt.Errorf("invalid multipart body")
	}

	var metadata []chatAttachmentMetadata
	var payloads []ai.AttachmentPayload
	totalTextChars := 0
	persistentIDs := uniqueAttachmentIDs(append(append([]string{}, r.MultipartForm.Value["attachment_ids[]"]...), r.MultipartForm.Value["attachment_ids"]...))
	storedPayloads, storedMetadata, err := s.loadStagedChatAttachmentInputs(r, persistentIDs)
	if err != nil {
		return "", "", "", nil, nil, nil, err
	}
	payloads = append(payloads, storedPayloads...)
	metadata = append(metadata, storedMetadata...)
	user := MustUserFromContext(r.Context())

	for _, fieldName := range []string{"files[]", "files"} {
		for _, header := range r.MultipartForm.File[fieldName] {
			file, err := header.Open()
			if err != nil {
				return "", "", "", nil, nil, nil, fmt.Errorf("failed to open attachment")
			}
			raw, readErr := io.ReadAll(io.LimitReader(file, 64<<20))
			closeErr := file.Close()
			if readErr != nil {
				return "", "", "", nil, nil, nil, fmt.Errorf("failed to read attachment")
			}
			if closeErr != nil {
				return "", "", "", nil, nil, nil, fmt.Errorf("failed to close attachment")
			}

			mimeType := detectAttachmentMIME(header.Filename, raw)
			if isSupportedImageMIME(mimeType) {
				stored, persistErr := s.persistStagedChatImageBytes(r.Context(), user.UserID, header.Filename, raw)
				if persistErr != nil {
					return "", "", "", nil, nil, nil, persistErr
				}
				storedPayload, payloadErr := s.chatAttachmentPayload(stored, "")
				if payloadErr != nil {
					return "", "", "", nil, nil, nil, payloadErr
				}
				persistentIDs = append(persistentIDs, stored.ID)
				payloads = append(payloads, storedPayload)
				projected := chatAttachmentMetadataFromRecord(stored)
				projected.Status = "attached"
				metadata = append(metadata, projected)
				continue
			}
			payload, item := buildChatAttachment("", header.Filename, header.Size, raw, &totalTextChars)
			payloads = append(payloads, payload)
			metadata = append(metadata, item)
		}
	}

	referencePayloads, referenceMetadata, err := s.parseChatReferences(r)
	if err != nil {
		return "", "", "", nil, nil, nil, err
	}
	payloads = append(payloads, referencePayloads...)
	metadata = append(metadata, referenceMetadata...)

	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		return "", "", "", nil, nil, nil, fmt.Errorf("failed to encode attachment metadata")
	}
	return r.FormValue("content"), r.FormValue("model_provider_id"), r.FormValue("thinking_mode"), rawMetadata, payloads, uniqueAttachmentIDs(persistentIDs), nil
}

func (s *Server) loadStagedChatAttachmentInputs(r *http.Request, attachmentIDs []string) ([]ai.AttachmentPayload, []chatAttachmentMetadata, error) {
	user := MustUserFromContext(r.Context())
	var payloads []ai.AttachmentPayload
	var metadata []chatAttachmentMetadata
	for _, attachmentID := range attachmentIDs {
		item, err := s.conversations.GetChatAttachment(r.Context(), attachmentID)
		if err != nil || item.UserID != user.UserID || item.Status != "staged" {
			return nil, nil, fmt.Errorf("attachment is unavailable")
		}
		payload, err := s.chatAttachmentPayload(item, "")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load image attachment")
		}
		payloads = append(payloads, payload)
		projected := chatAttachmentMetadataFromRecord(item)
		projected.Status = "attached"
		metadata = append(metadata, projected)
	}
	return payloads, metadata, nil
}

func uniqueAttachmentIDs(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func (s *Server) parseChatReferences(r *http.Request) ([]ai.AttachmentPayload, []chatAttachmentMetadata, error) {
	var payloads []ai.AttachmentPayload
	var metadata []chatAttachmentMetadata

	for _, ref := range parseWorkspaceFileRefs(r.FormValue("workspace_refs_json")) {
		cleanPath, err := cleanWorkspaceReferencePath(ref.Path)
		if err != nil {
			return nil, nil, err
		}
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			name = filepath.Base(cleanPath)
		}
		fileType := strings.TrimSpace(ref.FileType)
		if fileType == "" {
			fileType = "file"
		}
		payload := ai.AttachmentPayload{
			ID:       ref.ID,
			Name:     name,
			Kind:     "workspace_file",
			Path:     cleanPath,
			FileType: fileType,
			Note:     fmt.Sprintf("workspace_file metadata only path=%s; content must be read with workspace tools", cleanPath),
		}
		item := chatAttachmentMetadata{
			ID:                ref.ID,
			Name:              name,
			Kind:              "workspace_file",
			Type:              "workspace_file",
			Path:              cleanPath,
			FileType:          fileType,
			IncludedInContext: payload.IncludedInContext,
			Truncated:         payload.Truncated,
		}
		payloads = append(payloads, payload)
		metadata = append(metadata, item)
	}

	editorContextJSON := strings.TrimSpace(r.FormValue("editor_context_json"))
	if editorContextJSON == "" {
		editorContextJSON = r.FormValue("current_file_context_json")
	}
	if panelsContext := parseEditorPanelsContext(editorContextJSON); panelsContext.Type == "editor_panels_context" && len(panelsContext.Panels) > 0 {
		cleanPanels := make([]ai.EditorPanelContext, 0, len(panelsContext.Panels))
		for _, panel := range panelsContext.Panels {
			if panel.ActiveFilePath != "" {
				cleanPath, err := cleanWorkspaceReferencePath(panel.ActiveFilePath)
				if err != nil {
					return nil, nil, err
				}
				panel.ActiveFilePath = cleanPath
				if panel.ActiveFileName == "" {
					panel.ActiveFileName = filepath.Base(cleanPath)
				}
			}
			var cleanTabs []string
			for _, tab := range panel.OpenTabs {
				cleanPath, err := cleanWorkspaceReferencePath(tab)
				if err != nil {
					return nil, nil, err
				}
				cleanTabs = append(cleanTabs, cleanPath)
			}
			panel.OpenTabs = cleanTabs
			cleanPanels = append(cleanPanels, panel)
		}
		payloads = append(payloads, ai.AttachmentPayload{
			ID:           "editor_panels_context",
			Name:         "editor_panels_context",
			Kind:         "editor_panels_context",
			EditorPanels: cleanPanels,
		})
		metadata = append(metadata, chatAttachmentMetadata{
			ID:   "editor_panels_context",
			Name: "editor_panels_context",
			Kind: "editor_panels_context",
			Type: "editor_panels_context",
		})
	} else if current := parseCurrentFileContext(editorContextJSON); current.Path != "" {
		cleanPath, err := cleanWorkspaceReferencePath(current.Path)
		if err != nil {
			return nil, nil, err
		}
		name := strings.TrimSpace(current.Name)
		if name == "" {
			name = filepath.Base(cleanPath)
		}
		payloads = append(payloads, ai.AttachmentPayload{
			ID:       "current_file",
			Name:     name,
			Kind:     "current_file",
			Path:     cleanPath,
			FileType: current.FileType,
			Note:     fmt.Sprintf("current_file metadata only path=%s; content must be read with workspace tools", cleanPath),
		})
		metadata = append(metadata, chatAttachmentMetadata{
			ID:                "current_file",
			Name:              name,
			Kind:              "current_file",
			Type:              "current_file",
			Path:              cleanPath,
			FileType:          current.FileType,
			IncludedInContext: payloads[len(payloads)-1].IncludedInContext,
			Truncated:         payloads[len(payloads)-1].Truncated,
		})
	}

	for _, skill := range parseSelectedSkillRefs(r.FormValue("selected_skills_json")) {
		skillID := skill.SkillID
		if skillID == "" {
			skillID = skill.ID
		}
		if strings.TrimSpace(skillID) == "" {
			continue
		}
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			name = skillID
		}
		payloads = append(payloads, ai.AttachmentPayload{
			ID:          "skill_" + skillID,
			Name:        name,
			Kind:        "selected_skill",
			SkillID:     skillID,
			Description: skill.Description,
		})
		metadata = append(metadata, chatAttachmentMetadata{
			ID:          "skill_" + skillID,
			Name:        name,
			Kind:        "selected_skill",
			Type:        "selected_skill",
			SkillID:     skillID,
			Description: skill.Description,
		})
	}
	return payloads, metadata, nil
}
