package player

import (
	"net/http"

	"github.com/Tieumi221E/Jusplay/internal/ai"
)

// The AI backends' state for the page:
//
//	GET  api/ai            status, configuration (no keys) and download state
//
// Changes go through the capabilities (ai.config, ai.download, ai.cancel).
func (s *Server) aiAPI(w http.ResponseWriter, r *http.Request, p string) {
	s.ai.mu.Lock()
	e := s.ai.e
	s.ai.mu.Unlock()
	if e == nil {
		http.Error(w, "no AI support", http.StatusNotFound)
		return
	}
	switch {
	case p == "api/ai" && r.Method == http.MethodGet:
		out := s.aiStatus()
		out["config"] = e.Config()
		out["download"] = e.Download()
		out["recommended"] = ai.Recommended
		writeJSON(w, out)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}
