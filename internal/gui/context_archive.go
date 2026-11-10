package gui

import (
	"encoding/json"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
	"net/http"
	"strconv"
)

func writeGone(rw http.ResponseWriter, why string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Cache-Control", "no-store")
	rw.WriteHeader(http.StatusNotFound)
	json.NewEncoder(rw).Encode(map[string]string{"archive": why})
}

func contextReplyAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/context/reply", func(rw http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil || id <= 0 {
			http.Error(rw, "invalid route id", http.StatusBadRequest)
			return
		}
		out := gateway.PromptReply{ID: id, From: "memory"}
		var (
			from provider.Protocol
			body []byte
			cut  bool
			have bool
		)
		if gw := served.Load(); gw != nil {
			from, body, cut, have = gw.ReplyBody(id)
		}
		if !have {
			writeGone(rw, "off")
			return
		}
		pieces, cut, ok := gateway.ReplyPieces(from, body, cut)
		if !ok {
			writeGone(rw, "unread")
			return
		}
		out.Pieces, out.Cut = pieces, cut
		rw.Header().Set("Cache-Control", "no-store")
		writeJSON(rw, out)
	})
}
