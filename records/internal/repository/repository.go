package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"records/internal/models"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func New(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

type txKey struct{}

type execer interface {
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error)
}

func (r *Repository) getExecer(ctx context.Context) execer {
	if tx, ok := ctx.Value(txKey{}).(*sqlx.Tx); ok && tx != nil {
		return tx
	}
	return r.db
}

func (r *Repository) GetUser(ctx context.Context, userID string) (*models.User, error) {
	var user models.User
	executor := r.getExecer(ctx)
	err := executor.GetContext(ctx, &user, "SELECT id, name, phone, status, orgname, avatar_url, start_lark FROM users WHERE id = $1", userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get user by id=%s: %w", userID, err)
	}
	return &user, nil
}

func (r *Repository) CreateUser(ctx context.Context, user *models.User) error {
	query := `INSERT INTO users (id, name, phone, status, orgname, avatar_url) VALUES (:id, :name, :phone, :status, :orgname, :avatar_url)`
	executor := r.getExecer(ctx)
	_, err := executor.NamedExecContext(ctx, query, user)
	if err != nil {
		return fmt.Errorf("create user id=%s: %w", user.ID, err)
	}
	return nil
}

func (r *Repository) UpdateUser(ctx context.Context, user *models.User) error {
	query := `UPDATE users SET name = :name, phone = :phone, status = :status, orgname = :orgname, avatar_url = :avatar_url, updated_at = NOW() WHERE id = :id`
	executor := r.getExecer(ctx)
	_, err := executor.NamedExecContext(ctx, query, user)
	if err != nil {
		return fmt.Errorf("update user id=%s: %w", user.ID, err)
	}
	return nil
}

// EnsureUserExists 确保用户存在，若不存在则创建占位用户
func (r *Repository) EnsureUserExists(ctx context.Context, userID string) error {
	user, err := r.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if user != nil {
		return nil
	}
	u := &models.User{ID: userID, Name: userID, OrgName: "", Status: 0}
	return r.CreateUser(ctx, u)
}

// EnsureUserFromOAuth 根据 OAuth 用户信息创建或更新 users 表（orgname 默认空）
func (r *Repository) EnsureUserFromOAuth(ctx context.Context, userID, name string, avatarURL *string) error {
	user, err := r.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		u := &models.User{
			ID:       userID,
			Name:     name,
			OrgName:  "",
			Status:   0,
			AvatarURL: avatarURL,
		}
		return r.CreateUser(ctx, u)
	}
	user.Name = name
	user.AvatarURL = avatarURL
	return r.UpdateUser(ctx, user)
}

func (r *Repository) UpdateUserStartLark(ctx context.Context, userID string) error {
	query := `UPDATE users SET start_lark = NOW() WHERE id = $1`
	executor := r.getExecer(ctx)
	_, err := executor.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("update user start lark id=%s: %w", userID, err)
	}
	return nil
}

func (r *Repository) GetCustomer(ctx context.Context, customerID uuid.UUID) (*models.Customer, error) {
	var customer models.Customer
	executor := r.getExecer(ctx)
	err := executor.GetContext(ctx, &customer, "SELECT id, name, contact_person, contact_phone, contact_role FROM customers WHERE id = $1", customerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get customer id=%s: %w", customerID, err)
	}
	return &customer, nil
}

func (r *Repository) GetCustomerByName(ctx context.Context, name string) (*models.Customer, error) {
	var customer models.Customer
	executor := r.getExecer(ctx)
	err := executor.GetContext(ctx, &customer, "SELECT id, name, contact_person, contact_phone, contact_role FROM customers WHERE name = $1", name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get customer by name=%s: %w", name, err)
	}
	return &customer, nil
}

func (r *Repository) GetAllCustomers(ctx context.Context) ([]*models.Customer, error) {
	var customers []*models.Customer
	executor := r.getExecer(ctx)
	err := executor.SelectContext(ctx, &customers, `SELECT id, name, contact_person, contact_phone, contact_role FROM customers ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("get all customers: %w", err)
	}
	return customers, nil
}

func (r *Repository) CreateCustomer(ctx context.Context, customer *models.Customer) error {
	query := `INSERT INTO customers (id, name, contact_person, contact_phone, contact_role) VALUES (:id, :name, :contact_person, :contact_phone, :contact_role)`
	executor := r.getExecer(ctx)
	_, err := executor.NamedExecContext(ctx, query, customer)
	if err != nil {
		return fmt.Errorf("create customer name=%s: %w", customer.Name, err)
	}
	return nil
}

func (r *Repository) UpdateCustomer(ctx context.Context, customer *models.Customer) error {
	query := `UPDATE customers SET name = :name, contact_person = :contact_person, contact_phone = :contact_phone, contact_role = :contact_role, updated_at = NOW() WHERE id = :id`
	executor := r.getExecer(ctx)
	_, err := executor.NamedExecContext(ctx, query, customer)
	if err != nil {
		return fmt.Errorf("update customer id=%s: %w", customer.ID, err)
	}
	return nil
}

func (r *Repository) GetActiveSession(ctx context.Context, userID string) (*models.Session, error) {
	var session models.Session
	query := `SELECT id, user_id, status, ended_at FROM sessions WHERE user_id = $1 AND status != $2 AND ended_at IS NULL ORDER BY created_at DESC LIMIT 1`
	executor := r.getExecer(ctx)
	err := executor.GetContext(ctx, &session, query, userID, models.StatusExit)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get active session for user=%s: %w", userID, err)
	}
	return &session, nil
}

func (r *Repository) CreateSession(ctx context.Context, session *models.Session) error {
	query := `INSERT INTO sessions (id, user_id, status) VALUES (:id, :user_id, :status)`
	executor := r.getExecer(ctx)
	_, err := executor.NamedExecContext(ctx, query, session)
	if err != nil {
		return fmt.Errorf("create session id=%s: %w", session.ID, err)
	}
	return nil
}

func (r *Repository) UpdateSession(ctx context.Context, session *models.Session) error {
	query := `UPDATE sessions SET status = :status, ended_at = :ended_at, updated_at = NOW() WHERE id = :id`
	executor := r.getExecer(ctx)
	_, err := executor.NamedExecContext(ctx, query, session)
	if err != nil {
		return fmt.Errorf("update session id=%s: %w", session.ID, err)
	}
	return nil
}

func (r *Repository) UpdateSessionWithOptimisticLock(ctx context.Context, session *models.Session, expectedUpdatedAt time.Time) error {
	query := `UPDATE sessions SET status = $1, ended_at = $2, updated_at = NOW() WHERE id = $3 AND updated_at = $4`
	executor := r.getExecer(ctx)
	result, err := executor.ExecContext(ctx, query, session.Status, session.EndedAt, session.ID, expectedUpdatedAt)
	if err != nil {
		return fmt.Errorf("update session with optimistic lock id=%s: %w", session.ID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("session %s has been modified by another process", session.ID)
	}

	return nil
}

func (r *Repository) GetLatestDialog(ctx context.Context, sessionID uuid.UUID) (*models.Dialog, error) {
	var dialog models.Dialog
	query := `SELECT id, session_id, state, status, turn_index, focus_customer_id, is_first_focus, semantic_relevance, pending_updates, runtime_snapshot, turn_content, created_at FROM dialogs WHERE session_id = $1 ORDER BY turn_index DESC LIMIT 1`
	executor := r.getExecer(ctx)
	err := executor.GetContext(ctx, &dialog, query, sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest dialog session=%s: %w", sessionID, err)
	}
	return &dialog, nil
}

// GetLatestFocusCustomerIDFromDialogs 从对话表中获取最新一条具有 focus_customer_id 的记录，用于恢复丢失的 focus
func (r *Repository) GetLatestFocusCustomerIDFromDialogs(ctx context.Context, sessionID uuid.UUID) (*uuid.UUID, error) {
	var focusID uuid.UUID
	query := `SELECT focus_customer_id FROM dialogs WHERE session_id = $1 AND focus_customer_id IS NOT NULL ORDER BY turn_index DESC LIMIT 1`
	executor := r.getExecer(ctx)
	err := executor.GetContext(ctx, &focusID, query, sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest focus from dialogs session=%s: %w", sessionID, err)
	}
	return &focusID, nil
}

func (r *Repository) CreateDialog(ctx context.Context, dialog *models.Dialog) error {
	query := `INSERT INTO dialogs (id, session_id, state, status, turn_index, focus_customer_id, is_first_focus, semantic_relevance, pending_updates, runtime_snapshot, turn_content) VALUES (:id, :session_id, :state, :status, :turn_index, :focus_customer_id, :is_first_focus, :semantic_relevance, :pending_updates, :runtime_snapshot, :turn_content)`
	executor := r.getExecer(ctx)
	_, err := executor.NamedExecContext(ctx, query, dialog)
	if err != nil {
		return fmt.Errorf("create dialog session=%s turn=%d: %w", dialog.SessionID, dialog.TurnIndex, err)
	}
	return nil
}

func (r *Repository) GetDialogsBySession(ctx context.Context, sessionID uuid.UUID) ([]*models.Dialog, error) {
	var dialogs []*models.Dialog
	query := `SELECT id, session_id, state, status, turn_index, focus_customer_id, is_first_focus, semantic_relevance, pending_updates, runtime_snapshot, turn_content, created_at FROM dialogs WHERE session_id = $1 ORDER BY turn_index ASC`
	executor := r.getExecer(ctx)
	err := executor.SelectContext(ctx, &dialogs, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get dialogs by session=%s: %w", sessionID, err)
	}
	return dialogs, nil
}

// GetSessionConversationHistory 获取会话的原始对话历史（不含当前轮），格式为「User: …\nAssistant: …」供大模型理解上下文
// beforeTurnIndex 不包含该 turn_index 及之后的轮次
func (r *Repository) GetSessionConversationHistory(ctx context.Context, sessionID uuid.UUID, beforeTurnIndex int) (string, error) {
	dialogs, err := r.GetDialogsBySession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	var parts []string
	for _, d := range dialogs {
		if d.TurnIndex >= beforeTurnIndex {
			break
		}
		if d.TurnContent != nil && *d.TurnContent != "" {
			parts = append(parts, *d.TurnContent)
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n"), nil
}

// DeleteDialogsBySession 删除指定会话的所有 dialog 记录（OUTPUTTING 完成后调用，释放存储）
// 并发：按 session_id 删除，利用 idx_dialogs_session_id 索引，不同会话无锁竞争
func (r *Repository) DeleteDialogsBySession(ctx context.Context, sessionID uuid.UUID) error {
	executor := r.getExecer(ctx)
	_, err := executor.ExecContext(ctx, `DELETE FROM dialogs WHERE session_id = $1`, sessionID)
	if err != nil {
		return fmt.Errorf("delete dialogs session=%s: %w", sessionID, err)
	}
	return nil
}

// DeleteSession 删除指定会话记录（OUTPUTTING 完成后调用，需在 DeleteDialogsBySession 之后执行以满足 FK 约束）
func (r *Repository) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	executor := r.getExecer(ctx)
	_, err := executor.ExecContext(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		return fmt.Errorf("delete session id=%s: %w", sessionID, err)
	}
	return nil
}

func (r *Repository) GetLatestFollowRecord(ctx context.Context, customerID uuid.UUID) (*models.FollowRecord, error) {
	var record models.FollowRecord
	query := `SELECT id, user_id, customer_id, customer_name, contact_person, contact_phone, contact_role, follow_time, follow_method, follow_content, follow_goal, follow_result, risk_content, next_plan, created_at FROM follow_records WHERE customer_id = $1 ORDER BY follow_time DESC LIMIT 1`
	executor := r.getExecer(ctx)
	err := executor.GetContext(ctx, &record, query, customerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest follow record customer=%s: %w", customerID, err)
	}
	return &record, nil
}

func (r *Repository) CreateFollowRecord(ctx context.Context, record *models.FollowRecord) error {
	query := `INSERT INTO follow_records (id, user_id, customer_id, customer_name, contact_person, contact_phone, contact_role, follow_time, follow_method, follow_content, follow_goal, follow_result, risk_content, next_plan) VALUES (:id, :user_id, :customer_id, :customer_name, :contact_person, :contact_phone, :contact_role, :follow_time, :follow_method, :follow_content, :follow_goal, :follow_result, :risk_content, :next_plan)`
	executor := r.getExecer(ctx)
	_, err := executor.NamedExecContext(ctx, query, record)
	if err != nil {
		return fmt.Errorf("create follow record customer=%s: %w", record.CustomerID, err)
	}
	return nil
}

func (r *Repository) UpdateFollowRecord(ctx context.Context, record *models.FollowRecord) error {
	query := `UPDATE follow_records SET customer_name = :customer_name, contact_person = :contact_person, contact_phone = :contact_phone, contact_role = :contact_role, follow_time = :follow_time, follow_method = :follow_method, follow_content = :follow_content, follow_goal = :follow_goal, follow_result = :follow_result, risk_content = :risk_content, next_plan = :next_plan, updated_at = NOW() WHERE id = :id`
	executor := r.getExecer(ctx)
	_, err := executor.NamedExecContext(ctx, query, record)
	if err != nil {
		return fmt.Errorf("update follow record id=%s: %w", record.ID, err)
	}
	return nil
}

func (r *Repository) GetSessionFollowRecords(ctx context.Context, sessionID uuid.UUID) ([]*models.FollowRecord, error) {
	var records []*models.FollowRecord
	query := `SELECT fr.id, fr.user_id, fr.customer_id, fr.customer_name, fr.contact_person, fr.contact_phone, fr.contact_role, fr.follow_time, fr.follow_method, fr.follow_content, fr.follow_goal, fr.follow_result, fr.risk_content, fr.next_plan, fr.created_at FROM follow_records fr JOIN dialogs d ON d.focus_customer_id = fr.customer_id WHERE d.session_id = $1 ORDER BY fr.follow_time DESC`
	executor := r.getExecer(ctx)
	err := executor.SelectContext(ctx, &records, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session follow records session=%s: %w", sessionID, err)
	}
	return records, nil
}

func (r *Repository) GetCustomerFollowRecords(ctx context.Context, customerID uuid.UUID) ([]*models.FollowRecord, error) {
	var records []*models.FollowRecord
	query := `SELECT id, user_id, customer_id, customer_name, contact_person, contact_phone, contact_role, follow_time, follow_method, follow_content, follow_goal, follow_result, risk_content, next_plan, created_at FROM follow_records WHERE customer_id = $1 ORDER BY follow_time DESC`
	executor := r.getExecer(ctx)
	err := executor.SelectContext(ctx, &records, query, customerID)
	if err != nil {
		return nil, fmt.Errorf("get customer follow records customer=%s: %w", customerID, err)
	}
	return records, nil
}

func (r *Repository) WithTx(ctx context.Context, fn func(context.Context) error) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		} else if err != nil {
			tx.Rollback()
		}
	}()

	txCtx := context.WithValue(ctx, txKey{}, tx)
	err = fn(txCtx)
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// Page API 相关方法（统一使用 follow_records）

// FollowRecordWithCustomerID 用于 page API 返回，customer_id_str 即 customers.id
type FollowRecordWithCustomerID struct {
	models.FollowRecord
	CustomerIDStr string `db:"customer_id_str"`
}

// GetDistinctUserIDsInFollowRecords 返回 follow_records 表中所有不同的 user_id（用于调试）
func (r *Repository) GetDistinctUserIDsInFollowRecords(ctx context.Context) ([]string, error) {
	var rows []struct {
		UserID string `db:"user_id"`
	}
	query := `SELECT DISTINCT user_id FROM follow_records ORDER BY user_id`
	executor := r.getExecer(ctx)
	if err := executor.SelectContext(ctx, &rows, query); err != nil {
		return nil, fmt.Errorf("get distinct user_ids: %w", err)
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.UserID
	}
	return ids, nil
}

func (r *Repository) ListFollowRecordsForPage(ctx context.Context, userID string) ([]*FollowRecordWithCustomerID, error) {
	var records []*FollowRecordWithCustomerID
	// userID 为空时返回空列表（与 page 行为一致）
	if userID == "" {
		return records, nil
	}
	query := `SELECT fr.id, fr.user_id, fr.customer_id, fr.customer_name, fr.contact_person, fr.contact_phone, fr.contact_role,
		fr.follow_time, fr.follow_method, fr.follow_content, fr.follow_goal, fr.follow_result, fr.risk_content, fr.next_plan, fr.created_at,
		c.id::text AS customer_id_str
		FROM follow_records fr
		JOIN customers c ON fr.customer_id = c.id
		WHERE fr.user_id = $1
		ORDER BY fr.follow_time DESC`
	executor := r.getExecer(ctx)
	err := executor.SelectContext(ctx, &records, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list follow records for page: %w", err)
	}
	return records, nil
}

func (r *Repository) CreateFollowRecordForPage(ctx context.Context, userID, customerIDStr, customerName, followContent string, followTime time.Time, followMethod, contactPerson string, contactRole *string, followGoal, followResult string, riskContent *string, nextPlan string) (*models.FollowRecord, error) {
	// 若 customerIDStr 为有效 UUID 且客户存在则复用，否则创建新客户
	var customer *models.Customer
	if id, err := uuid.Parse(customerIDStr); err == nil {
		customer, _ = r.GetCustomer(ctx, id)
	}
	if customer == nil {
		customer = &models.Customer{
			ID:   uuid.New(),
			Name: customerName,
		}
		if err := r.CreateCustomer(ctx, customer); err != nil {
			return nil, fmt.Errorf("create customer for page: %w", err)
		}
	}

	fm := followMethod
	if fm == "" {
		fm = "线上"
	}
	record := &models.FollowRecord{
		ID:            uuid.New(),
		UserID:        userID,
		CustomerID:    customer.ID,
		CustomerName:  customerName,
		ContactPerson: &contactPerson,
		ContactRole:   contactRole,
		FollowTime:    followTime,
		FollowMethod:  &fm,
		FollowContent: &followContent,
		FollowGoal:    &followGoal,
		FollowResult:  &followResult,
		RiskContent:   riskContent,
		NextPlan:      &nextPlan,
	}
	if err := r.CreateFollowRecord(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

func (r *Repository) GetFollowRecordByID(ctx context.Context, id uuid.UUID) (*models.FollowRecord, error) {
	var record models.FollowRecord
	query := `SELECT id, user_id, customer_id, customer_name, contact_person, contact_phone, contact_role, follow_time, follow_method, follow_content, follow_goal, follow_result, risk_content, next_plan, created_at FROM follow_records WHERE id = $1`
	executor := r.getExecer(ctx)
	err := executor.GetContext(ctx, &record, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get follow record id=%s: %w", id, err)
	}
	return &record, nil
}

func (r *Repository) DeleteFollowRecord(ctx context.Context, id uuid.UUID, userID string) (bool, error) {
	query := `DELETE FROM follow_records WHERE id = $1 AND user_id = $2`
	executor := r.getExecer(ctx)
	result, err := executor.ExecContext(ctx, query, id, userID)
	if err != nil {
		return false, fmt.Errorf("delete follow record id=%s: %w", id, err)
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}
