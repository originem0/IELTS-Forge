package main

import "net/http"

func registerLibrarySummaryAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/library/summary", func(w http.ResponseWriter, r *http.Request) {
		result, err := disk.indexedSummary(r.Header.Get("X-ELP-Directory"))
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, result)
	})
}
