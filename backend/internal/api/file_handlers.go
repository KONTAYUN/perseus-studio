package api

import (
	"errors"
	"net/http"

	"webide-backend/internal/workspace"
)

func (s *Server) userWorkspace(r *http.Request) (*workspace.Service, error) {
	user := MustUserFromContext(r.Context())
	return s.factory.Workspace(user.UserID)
}

func (s *Server) writeFileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, workspace.ErrPathExists):
		writeCodedError(w, http.StatusConflict, "path_exists", err.Error())
	case workspace.IsPathError(err):
		writeCodedError(w, http.StatusBadRequest, "invalid_path", err.Error())
	case workspace.IsNotExist(err):
		writeCodedError(w, http.StatusNotFound, "path_not_found", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
