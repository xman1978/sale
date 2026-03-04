package server

import (
	"net/http"
	"strings"
	"time"

	"records/internal/repository"
)

// managerUsersHandler 处理 GET {apiP}/manager/users
func (s *Server) managerUsersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != s.apiPrefix()+"/manager/users" {
		http.NotFound(w, r)
		return
	}

	userID, ok := s.pageUserIDFromRequest(r)
	if !ok {
		s.writePageJSON(w, http.StatusUnauthorized, pageAPIResponse{Success: false, Message: "未登录或登录已过期"})
		return
	}
	repo := repository.New(s.db)
	isManager, err := repo.IsManager(r.Context(), userID)
	if err != nil {
		s.logger.Error("IsManager failed", "error", err, "user_id", userID)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "权限检查失败"})
		return
	}
	if !isManager {
		s.writePageJSON(w, http.StatusForbidden, pageAPIResponse{Success: false, Message: "无权限查看"})
		return
	}

	list, err := repo.ListUsersForManager(r.Context(), userID)
	if err != nil {
		s.logger.Error("ListUsersForManager failed", "error", err)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "获取用户列表失败"})
		return
	}
	data := make([]map[string]interface{}, len(list))
	for i, u := range list {
		var lastRecordAt interface{} = nil
		if u.LastRecordAt != nil {
			lastRecordAt = u.LastRecordAt.Format(time.RFC3339)
		}
		data[i] = map[string]interface{}{"user_id": u.UserID, "name": u.Name, "record_count": u.RecordCount, "last_record_at": lastRecordAt}
	}
	s.writePageJSON(w, http.StatusOK, pageAPIResponse{Success: true, Data: data})
}

// managerUsersSubHandler 处理 GET {apiP}/manager/users/{user_id}/groups 与 /manager/users/{user_id}/records
func (s *Server) managerUsersSubHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	apiP := s.apiPrefix()
	path := strings.TrimPrefix(r.URL.Path, apiP+"/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	// 期望: manager, users, {user_id}, groups|records
	if len(parts) < 4 || parts[0] != "manager" || parts[1] != "users" {
		http.NotFound(w, r)
		return
	}
	targetUserID := parts[2]
	action := parts[3]

	userID, ok := s.pageUserIDFromRequest(r)
	if !ok {
		s.writePageJSON(w, http.StatusUnauthorized, pageAPIResponse{Success: false, Message: "未登录或登录已过期"})
		return
	}
	repo := repository.New(s.db)
	isManager, err := repo.IsManager(r.Context(), userID)
	if err != nil {
		s.logger.Error("IsManager failed", "error", err, "user_id", userID)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "权限检查失败"})
		return
	}
	if !isManager {
		s.writePageJSON(w, http.StatusForbidden, pageAPIResponse{Success: false, Message: "无权限查看"})
		return
	}

	scopeIDs, err := repo.GetManagerScopeUserIDs(r.Context(), userID)
	if err != nil {
		s.logger.Error("GetManagerScopeUserIDs failed", "error", err)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "获取范围失败"})
		return
	}
	allowed := scopeIDs == nil // nil 表示全部
	if !allowed {
		for _, id := range scopeIDs {
			if id == targetUserID {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		s.writePageJSON(w, http.StatusForbidden, pageAPIResponse{Success: false, Message: "无权限查看该用户"})
		return
	}

	switch action {
	case "groups":
		list, err := repo.ListCustomerFollowGroupsForManager(r.Context(), targetUserID)
		if err != nil {
			s.logger.Error("ListCustomerFollowGroupsForManager failed", "error", err)
			s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "获取分组失败"})
			return
		}
		data := make([]map[string]interface{}, len(list))
		for i, g := range list {
			var lastRecordAt interface{} = nil
			if g.LastRecordAt != nil {
				lastRecordAt = g.LastRecordAt.Format(time.RFC3339)
			}
			data[i] = map[string]interface{}{"customer_name": g.CustomerName, "last_record_at": lastRecordAt}
		}
		s.writePageJSON(w, http.StatusOK, pageAPIResponse{Success: true, Data: data})
		return
	case "records":
		customerName := r.URL.Query().Get("customer_name")
		followContent := r.URL.Query().Get("follow_content")
		if customerName == "" {
			s.writePageJSON(w, http.StatusBadRequest, pageAPIResponse{Success: false, Message: "缺少 customer_name"})
			return
		}
		list, err := repo.ListFollowRecordsForManager(r.Context(), targetUserID, customerName, followContent)
		if err != nil {
			s.logger.Error("ListFollowRecordsForManager failed", "error", err)
			s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "获取记录失败"})
			return
		}
		data := make([]map[string]interface{}, len(list))
		for i, rec := range list {
			data[i] = followRecordToPageMap(&rec.FollowRecord, rec.CustomerIDStr)
		}
		s.writePageJSON(w, http.StatusOK, pageAPIResponse{Success: true, Data: data})
		return
	default:
		http.NotFound(w, r)
	}
}
