package api

import (
	"net/http"
	"webide-backend/internal/workspace"
)

func (s *Server) userWorkspace(r *http.Request) (*workspace.Service, error) {
	user := MustUserFromContext(r.Context())
	return s.factory.Workspace(user.UserID)
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
