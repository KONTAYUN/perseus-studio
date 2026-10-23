package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"webide-backend/internal/store"
)

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	skillsSvc, err := s.factory.Skills(user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := skillsSvc.ListForUser(r.Context(), user.UserID, user.Role)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": skillsForResponse(user.Role, items)})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleSkillRoutes(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	userSkillsSvc, err := s.factory.Skills(user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	if parts[0] == "upload" {
		if r.Method != http.MethodPost || len(parts) != 1 {
			http.NotFound(w, r)
			return
		}
		if !s.requireAdminOrWriteForbidden(w, r) {
			return
		}
		s.handleSkillUpload(w, r)
		return
	}
	if parts[0] == "install-selected" {
		if r.Method != http.MethodPost || len(parts) != 1 {
			http.NotFound(w, r)
			return
		}
		if !s.requireAdminOrWriteForbidden(w, r) {
			return
		}
		s.handleSkillInstallSelected(w, r)
		return
	}

	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			item, err := userSkillsSvc.GetForUser(r.Context(), user.UserID, user.Role, id)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			ownerSkillsSvc, err := s.factory.Skills(item.UserID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			tree, _ := ownerSkillsSvc.DirectoryTreeForUser(r.Context(), user.UserID, user.Role, id)
			writeJSON(w, http.StatusOK, map[string]any{"skill": skillForResponse(user.Role, item), "tree": tree})
		case http.MethodPatch:
			if !s.requireAdminOrWriteForbidden(w, r) {
				return
			}
			existing, err := userSkillsSvc.GetForUser(r.Context(), user.UserID, user.Role, id)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			ownerSkillsSvc, err := s.factory.Skills(existing.UserID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			var req store.SkillInput
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json body")
				return
			}
			item, err := ownerSkillsSvc.Update(r.Context(), existing.UserID, id, req)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, item)
		case http.MethodDelete:
			if !s.requireAdminOrWriteForbidden(w, r) {
				return
			}
			existing, err := userSkillsSvc.GetForUser(r.Context(), user.UserID, user.Role, id)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			ownerSkillsSvc, err := s.factory.Skills(existing.UserID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if err := ownerSkillsSvc.DeleteAny(r.Context(), id); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": id})
		default:
			http.NotFound(w, r)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "read" {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		item, err := userSkillsSvc.GetForUser(r.Context(), user.UserID, user.Role, id)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		ownerSkillsSvc, err := s.factory.Skills(item.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result, err := ownerSkillsSvc.ReadSkillForUser(r.Context(), user.UserID, user.Role, id, false)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleSkillUpload(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	skillsSvc, err := s.factory.Skills(user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart body")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		writeError(w, http.StatusBadRequest, "只支持上传 .zip 格式的 Skill")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(file, (26<<20)+1))
	if len(raw) > 26<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "Skill archive exceeds 26 MiB")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}
	result, err := skillsSvc.InstallZip(r.Context(), user.UserID, raw, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := http.StatusCreated
	if len(result.Candidates) > 0 {
		status = http.StatusAccepted
	}
	writeJSON(w, status, result)
}

func (s *Server) handleSkillInstallSelected(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	skillsSvc, err := s.factory.Skills(user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req struct {
		UploadID     string   `json:"upload_id"`
		CandidateIDs []string `json:"candidate_ids"`
		Enabled      *bool    `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	items, err := skillsSvc.InstallSelected(r.Context(), user.UserID, req.UploadID, req.CandidateIDs, enabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"installed": items})
}
