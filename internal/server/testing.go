package server

import "net/http"

func (s *Server) HandleWebSocketForTest(w http.ResponseWriter, r *http.Request) {
	s.handleWebSocket(w, r)
}
