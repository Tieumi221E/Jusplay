package player

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Tieumi221E/Jusplay/internal/ai"
)

// The AI backends' settings and the recommended models' download:
//
//	GET  api/ai            status, configuration (no keys) and download state
//	PUT  api/ai/config     a changed configuration (ai.PublicConfig)
//	POST api/ai/download   fetch the missing recommended files (only when the user asks)
//	POST api/ai/download/cancel
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
	case p == "api/ai/config" && r.Method == http.MethodPut:
		var c ai.PublicConfig
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&c); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := e.SetConfig(c); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		s.logf("ai: configuration changed (recognition %s, translation %s)", c.ASR.Kind, c.MT.Kind)
		w.WriteHeader(http.StatusNoContent)
	case p == "api/ai/download" && r.Method == http.MethodPost:
		if err := e.StartDownload(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.logf("ai: download of the recommended models started")
		w.WriteHeader(http.StatusNoContent)
	case p == "api/ai/download/cancel" && r.Method == http.MethodPost:
		e.CancelDownload()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}
