package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"webide-backend/internal/workspace"
)

func (s *Server) userWorkspace(r *http.Request) (*workspace.Service, error) {
	user := MustUserFromContext(r.Context())
	return s.factory.Workspace(user.UserID)
}

func (s *Server) handleFilesTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tree, err := files.Tree()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tree})
}

func (s *Server) handleFilesSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req searchFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	items, err := files.Search(req.Query, req.Limit)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleFilesRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := files.ReadFile(r.URL.Query().Get("path"))
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart body")
		return
	}

	targetPath := r.FormValue("path")
	overwrite := r.FormValue("overwrite") == "true"
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}

	var result workspace.FileWriteResult
	if overwrite {
		result, err = files.WriteBytes(r.Context(), targetPath, content, "upload")
	} else {
		result, err = files.CreateBytes(r.Context(), targetPath, content)
	}
	if err != nil {
		s.writeFileError(w, err)
		return
	}

	status := http.StatusCreated
	if overwrite {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) handleFilesWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req writeFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.WriteFile(r.Context(), req.Path, req.Content, "write")
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req writeFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.CreateFile(r.Context(), req.Path, req.Content)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleFilesMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req pathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.MkdirNew(req.Path)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleFilesRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req renameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.Rename(req.OldPath, req.NewPath)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req reorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.ReorderPath(req.Path, req.TargetParentPath, req.PreviousPath, req.NextPath)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req pathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.MoveToTrash(req.Path)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items, err := files.TrashItems()
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleFilesTrashRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req restoreTrashRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.RestoreTrashItem(req.Path, req.Overwrite)
	if err != nil {
		if errors.Is(err, workspace.ErrPathExists) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":    "原路径已存在，是否覆盖？",
				"conflict": true,
				"path":     req.Path,
			})
			return
		}
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesTrashDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req pathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := files.DeleteTrashItem(req.Path)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFilesEmptyTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	files, err := s.userWorkspace(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := files.EmptyTrash()
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) writeFileError(w http.ResponseWriter, err error) {
	switch {
	case workspace.IsPathError(err):
		writeError(w, http.StatusBadRequest, err.Error())
	case workspace.IsNotExist(err):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
