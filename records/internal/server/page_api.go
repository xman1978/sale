package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"records/internal/auth"
	"records/internal/models"
	"records/internal/repository"

	"github.com/google/uuid"
)

// pageAPI 响应格式（与 page 前端期望一致）
type pageAPIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

// createRecordRequest POST /api/records 请求体
type createRecordRequest struct {
	CustomerID    string  `json:"customer_id"`
	CustomerName  string  `json:"customer_name"`
	FollowContent string  `json:"follow_content"`
	FollowTime    string  `json:"follow_time"`
	FollowMethod  string  `json:"follow_method"`
	ContactPerson string  `json:"contact_person"`
	ContactRole   *string `json:"contact_role"`
	FollowGoal    string  `json:"follow_goal"`
	FollowResult  string  `json:"follow_result"`
	RiskContent   *string `json:"risk_content"`
	NextPlan      string  `json:"next_plan"`
}

// updateRecordRequest PUT /api/records/:id 请求体
type updateRecordRequest struct {
	FollowMethod  string  `json:"follow_method"`
	ContactPerson string  `json:"contact_person"`
	ContactRole   *string `json:"contact_role"`
	FollowGoal    string  `json:"follow_goal"`
	FollowResult  string  `json:"follow_result"`
	RiskContent   *string `json:"risk_content"`
	NextPlan      string  `json:"next_plan"`
}

// pageUserIDFromRequest 从请求获取已认证用户 ID。优先验证 JWT；无 JWT 或失效时，若允许则回退到 x-user-id。
// 返回 (userID, true) 表示已认证；(_, false) 表示需返回 401。
func (s *Server) pageUserIDFromRequest(r *http.Request) (string, bool) {
	secret := s.config.Server.JWTSecret
	if secret != "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			uid, err := auth.Validate(secret, tokenString)
			if err == nil && !isInvalidUserID(uid) {
				return uid, true
			}
		}
		if s.config.Server.AllowXUserIDFallback {
			uid := r.Header.Get("x-user-id")
			if uid != "" && !isInvalidUserID(uid) {
				return uid, true
			}
		}
		return "", false
	}
	// 未配置 JWT 时回退到 x-user-id（不推荐，存在冒充风险）
	uid := r.Header.Get("x-user-id")
	if uid == "" || isInvalidUserID(uid) {
		return "", false
	}
	return uid, true
}

// isInvalidUserID 判断是否为无效/已废弃的用户 ID（如 demo_user）
func isInvalidUserID(uid string) bool {
	return uid == "demo_user"
}

// pageAPIRootHandler 处理 {api_prefix}/records（GET 列表、POST 新建）
func (s *Server) pageAPIRootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != s.apiPrefix()+"/records" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.pageRecordsHandler(w, r)
	case http.MethodPost:
		s.pageRecordsCreateHandler(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// pageAPISubHandler 处理 /api/records/:id（PUT 更新、DELETE 删除）
func (s *Server) pageAPISubHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut, http.MethodDelete:
		s.pageRecordsByIDHandler(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) writePageJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) pageRecordsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.pageUserIDFromRequest(r)
	if !ok {
		s.writePageJSON(w, http.StatusUnauthorized, pageAPIResponse{Success: false, Message: "未登录或登录已过期"})
		return
	}
	s.logger.Info("pageRecordsHandler", "user_id", userID, "auth_header", r.Header.Get("Authorization") != "", "x_user_id", r.Header.Get("x-user-id"))
	repo := repository.New(s.db)
	// 打开跟进记录页时，若 users 表中无该用户，则根据飞书用户信息创建；userID 为 union_id
	if _, err := s.ensureUserExists(r.Context(), userID, false); err != nil {
		s.logger.Error("Ensure user exists from Feishu failed", "error", err, "user_id", userID)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "获取用户信息失败"})
		return
	}
	records, err := repo.ListFollowRecordsForPage(r.Context(), userID)
	if err != nil {
		s.logger.Error("List follow records for page failed", "error", err)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "查询记录失败"})
		return
	}
	s.logger.Info("pageRecordsHandler: query result", "user_id", userID, "record_count", len(records))
	if len(records) == 0 {
		if ids, err := repo.GetDistinctUserIDsInFollowRecords(r.Context()); err == nil {
			s.logger.Info("pageRecordsHandler: distinct user_ids in follow_records (for debug)", "user_ids", ids)
		}
	}

	data := make([]map[string]interface{}, len(records))
	for i, rec := range records {
		data[i] = followRecordToPageMap(&rec.FollowRecord, rec.CustomerIDStr)
	}
	s.writePageJSON(w, http.StatusOK, pageAPIResponse{Success: true, Data: data})
}

func (s *Server) pageRecordsCreateHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.pageUserIDFromRequest(r)
	if !ok {
		s.writePageJSON(w, http.StatusUnauthorized, pageAPIResponse{Success: false, Message: "未登录或登录已过期"})
		return
	}
	var req createRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writePageJSON(w, http.StatusBadRequest, pageAPIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	followTime, err := time.Parse(time.RFC3339, req.FollowTime)
	if err != nil {
		followTime = time.Now()
	}

	repo := repository.New(s.db)
	if _, err := s.ensureUserExists(r.Context(), userID, false); err != nil {
		s.logger.Error("Ensure user exists failed", "error", err, "user_id", userID)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "创建记录失败"})
		return
	}
	record, err := repo.CreateFollowRecordForPage(r.Context(),
		userID, req.CustomerID, req.CustomerName, req.FollowContent, followTime,
		req.FollowMethod, req.ContactPerson, req.ContactRole,
		req.FollowGoal, req.FollowResult, req.RiskContent, req.NextPlan)
	if err != nil {
		s.logger.Error("Create follow record for page failed", "error", err)
		s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "创建记录失败"})
		return
	}

	s.writePageJSON(w, http.StatusOK, pageAPIResponse{Success: true, Data: followRecordToPageMap(record, record.CustomerID.String())})
}

func (s *Server) pageRecordsByIDHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, s.apiPrefix()+"/records/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	id, err := uuid.Parse(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	repo := repository.New(s.db)

	switch r.Method {
	case http.MethodPut:
		var req updateRecordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writePageJSON(w, http.StatusBadRequest, pageAPIResponse{Success: false, Message: "无效的请求体"})
			return
		}

		userID, authOk := s.pageUserIDFromRequest(r)
		if !authOk {
			s.writePageJSON(w, http.StatusUnauthorized, pageAPIResponse{Success: false, Message: "未登录或登录已过期"})
			return
		}
		record, err := repo.GetFollowRecordByID(r.Context(), id)
		if err != nil || record == nil {
			s.writePageJSON(w, http.StatusNotFound, pageAPIResponse{Success: false, Message: "记录不存在"})
			return
		}
		if record.UserID != userID {
			s.writePageJSON(w, http.StatusForbidden, pageAPIResponse{Success: false, Message: "无权限操作此记录"})
			return
		}

		record.FollowMethod = &req.FollowMethod
		record.ContactPerson = &req.ContactPerson
		record.ContactRole = req.ContactRole
		record.FollowGoal = &req.FollowGoal
		record.FollowResult = &req.FollowResult
		record.RiskContent = req.RiskContent
		record.NextPlan = &req.NextPlan

		if err := repo.UpdateFollowRecord(r.Context(), record); err != nil {
			s.logger.Error("Update follow record failed", "error", err, "id", id)
			s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "更新记录失败"})
			return
		}

		s.writePageJSON(w, http.StatusOK, pageAPIResponse{Success: true, Data: followRecordToPageMap(record, record.CustomerID.String())})

	case http.MethodDelete:
		userID, authOk := s.pageUserIDFromRequest(r)
		if !authOk {
			s.writePageJSON(w, http.StatusUnauthorized, pageAPIResponse{Success: false, Message: "未登录或登录已过期"})
			return
		}
		record, _ := repo.GetFollowRecordByID(r.Context(), id)
		if record != nil && record.UserID != userID {
			s.writePageJSON(w, http.StatusForbidden, pageAPIResponse{Success: false, Message: "无权限操作此记录"})
			return
		}
		ok, err := repo.DeleteFollowRecord(r.Context(), id, userID)
		if err != nil {
			s.logger.Error("Delete follow record failed", "error", err, "id", id)
			s.writePageJSON(w, http.StatusInternalServerError, pageAPIResponse{Success: false, Message: "删除记录失败"})
			return
		}
		if !ok {
			s.writePageJSON(w, http.StatusNotFound, pageAPIResponse{Success: false, Message: "记录不存在"})
			return
		}
		s.writePageJSON(w, http.StatusOK, pageAPIResponse{Success: true, Message: "删除成功"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// followRecordToPageMap 将 FollowRecord 转为前端期望的 map（snake_case，id 为 UUID 字符串）
func followRecordToPageMap(r *models.FollowRecord, customerIDStr string) map[string]interface{} {
	m := map[string]interface{}{
		"id":            r.ID.String(),
		"customer_id":   customerIDStr,
		"customer_name": r.CustomerName,
		"follow_time":   r.FollowTime.Format(time.RFC3339),
		"created_at":    r.CreatedAt.Format(time.RFC3339),
	}
	if r.FollowContent != nil {
		m["follow_content"] = *r.FollowContent
	} else {
		m["follow_content"] = ""
	}
	if r.FollowMethod != nil {
		m["follow_method"] = *r.FollowMethod
	} else {
		m["follow_method"] = "线上"
	}
	if r.ContactPerson != nil {
		m["contact_person"] = *r.ContactPerson
	} else {
		m["contact_person"] = ""
	}
	if r.ContactRole != nil {
		m["contact_role"] = *r.ContactRole
	} else {
		m["contact_role"] = nil
	}
	if r.FollowGoal != nil {
		m["follow_goal"] = *r.FollowGoal
	} else {
		m["follow_goal"] = ""
	}
	if r.FollowResult != nil {
		m["follow_result"] = *r.FollowResult
	} else {
		m["follow_result"] = ""
	}
	if r.RiskContent != nil {
		m["risk_content"] = *r.RiskContent
	} else {
		m["risk_content"] = nil
	}
	if r.NextPlan != nil {
		m["next_plan"] = *r.NextPlan
	} else {
		m["next_plan"] = ""
	}
	return m
}
