package server

import (
	"encoding/json"
	"net/http"
	"time"

	"records/internal/hotwords"
)

// hotwordsStatsHandler GET {api_prefix}/hotwords/stats 返回当日热词统计
func (s *Server) hotwordsStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	data, err := hotwords.BuildPageDataByDate(r.Context(), s.db, today)
	if err != nil {
		s.logger.Error("hotwords BuildPageDataByDate failed", "error", err)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "获取热词统计失败"})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp := pageAPIResponse{Success: true, Data: data}
	_ = json.NewEncoder(w).Encode(resp)
}

// hotwordsRunDatesHandler GET {api_prefix}/hotwords/run_dates 返回日期列表（仅当日，热词只展示当天统计）
func (s *Server) hotwordsRunDatesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now()
	todayStr := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp := pageAPIResponse{Success: true, Data: map[string]interface{}{"dates": []string{todayStr}}}
	_ = json.NewEncoder(w).Encode(resp)
}
