package normalization

import (
	"context"
	"fmt"

	"records/internal/ai"
	"records/internal/models"
	"records/internal/repository"
	"records/pkg/logger"

	"github.com/google/uuid"
)

// Normalizer 归一处理器
type Normalizer struct {
	aiClient ai.Client
	repo     *repository.Repository
	logger   logger.Logger
}

// NewNormalizer 创建归一处理器
func NewNormalizer(aiClient ai.Client, repo *repository.Repository, logger logger.Logger) *Normalizer {
	return &Normalizer{
		aiClient: aiClient,
		repo:     repo,
		logger:   logger,
	}
}

// NormalizeEntities 对会话中的客户/联系人进行归一处理
func (n *Normalizer) NormalizeEntities(ctx context.Context, sessionID uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	// 1. 从对话记录中提取所有客户/联系人提及
	mentions, err := n.extractMentions(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to extract mentions: %w", err)
	}

	if len(mentions) == 0 {
		n.logger.Info("No mentions found in session", "session_id", sessionID)
		return make(map[uuid.UUID]uuid.UUID), nil
	}

	// 2. 获取候选实体（系统中已有的客户/联系人）
	candidates, err := n.getCandidateEntities(ctx, mentions)
	if err != nil {
		return nil, fmt.Errorf("failed to get candidate entities: %w", err)
	}

	// 3. 构建对话上下文
	dialogContext, err := n.buildDialogContext(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to build dialog context: %w", err)
	}

	// 4. 调用 AI 进行归一评分
	request := &models.NormalizationRequest{
		DialogContext:     dialogContext,
		MentionsEntity:    mentions,
		CandidateEntities: candidates,
	}

	results, err := n.aiClient.EntityNormalization(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to perform entity normalization: %w", err)
	}

	// 5. 处理归一结果，生成映射关系
	mergeMap, err := n.processMergeResults(ctx, results, mentions)
	if err != nil {
		return nil, fmt.Errorf("failed to process merge results: %w", err)
	}

	return mergeMap, nil
}

// extractMentions 从对话记录中提取客户/联系人提及
func (n *Normalizer) extractMentions(ctx context.Context, sessionID uuid.UUID) ([]models.EntityMention, error) {
	// 获取会话中所有对话记录
	dialogs, err := n.repo.GetDialogsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	mentions := make([]models.EntityMention, 0)
	customerIDs := make(map[uuid.UUID]bool)

	// 收集所有涉及的客户ID
	for _, dialog := range dialogs {
		if dialog.FocusCustomerID != nil {
			customerIDs[*dialog.FocusCustomerID] = true
		}
	}

	// 为每个客户创建提及记录
	for customerID := range customerIDs {
		customer, err := n.repo.GetCustomer(ctx, customerID)
		if err != nil {
			n.logger.Error("Failed to get customer", "customer_id", customerID, "error", err)
			continue
		}

		if customer == nil {
			continue
		}

		// 添加客户提及
		mentions = append(mentions, models.EntityMention{
			MentionID:  fmt.Sprintf("customer_%s", customerID.String()),
			EntityType: "customer",
			Name:       customer.Name,
		})

		// 添加联系人提及（如果存在）
		if customer.ContactPerson != nil && *customer.ContactPerson != "" {
			mentions = append(mentions, models.EntityMention{
				MentionID:    fmt.Sprintf("contact_%s", customerID.String()),
				EntityType:   "contact",
				Name:         *customer.ContactPerson,
				CustomerName: customer.Name,
			})
		}
	}

	return mentions, nil
}

// getCandidateEntities 获取候选实体
func (n *Normalizer) getCandidateEntities(ctx context.Context, mentions []models.EntityMention) ([]models.CandidateEntity, error) {
	candidates := make([]models.CandidateEntity, 0)

	// 获取所有客户作为候选
	customers, err := n.repo.GetAllCustomers(ctx)
	if err != nil {
		return nil, err
	}

	for _, customer := range customers {
		candidates = append(candidates, models.CandidateEntity{
			EntityID:   customer.ID.String(),
			EntityType: "customer",
			Name:       customer.Name,
		})

		// 添加客户的联系人作为候选
		if customer.ContactPerson != nil && *customer.ContactPerson != "" {
			candidates = append(candidates, models.CandidateEntity{
				EntityID:     fmt.Sprintf("contact_%s", customer.ID.String()),
				EntityType:   "contact",
				Name:         *customer.ContactPerson,
				CustomerName: customer.Name,
				ContactRole:  customer.ContactRole,
				ContactPhone: customer.ContactPhone,
			})
		}
	}

	return candidates, nil
}

// buildDialogContext 构建对话上下文
func (n *Normalizer) buildDialogContext(ctx context.Context, sessionID uuid.UUID) (string, error) {
	// 获取会话中的所有对话记录
	dialogs, err := n.repo.GetDialogsBySession(ctx, sessionID)
	if err != nil {
		return "", err
	}

	// 简化的上下文：只包含客户名称和轮次信息
	context := fmt.Sprintf("Session ID: %s, Total turns: %d", sessionID.String(), len(dialogs))
	return context, nil
}

// processMergeResults 处理归一结果，生成合并映射
func (n *Normalizer) processMergeResults(
	ctx context.Context,
	results []models.NormalizationResult,
	mentions []models.EntityMention,
) (map[uuid.UUID]uuid.UUID, error) {
	mergeMap := make(map[uuid.UUID]uuid.UUID)

	// 按 mention_id 分组结果
	resultsByMention := make(map[string][]models.NormalizationResult)
	for _, result := range results {
		resultsByMention[result.MentionID] = append(resultsByMention[result.MentionID], result)
	}

	// 处理每个提及
	for _, mention := range mentions {
		if mention.EntityType != "customer" {
			continue // 目前只处理客户归一
		}

		mentionResults, exists := resultsByMention[mention.MentionID]
		if !exists || len(mentionResults) == 0 {
			continue
		}

		// 找到最高评分的结果
		var bestResult *models.NormalizationResult
		for i := range mentionResults {
			if bestResult == nil || mentionResults[i].NormalizationScore > bestResult.NormalizationScore {
				bestResult = &mentionResults[i]
			}
		}

		// 只处理高置信度的归一结果
		if bestResult != nil && bestResult.NormalizationLevel == "high" && bestResult.EntityID != nil {
			// 从 mention_id 中提取源客户ID
			var sourceCustomerID uuid.UUID
			fmt.Sscanf(mention.MentionID, "customer_%s", &sourceCustomerID)

			// 解析目标客户ID
			targetCustomerID, err := uuid.Parse(*bestResult.EntityID)
			if err != nil {
				n.logger.Error("Failed to parse target customer ID", "entity_id", *bestResult.EntityID, "error", err)
				continue
			}

			// 如果源和目标不同，添加到合并映射
			if sourceCustomerID != targetCustomerID {
				mergeMap[sourceCustomerID] = targetCustomerID
				n.logger.Info("Customer merge identified",
					"source", sourceCustomerID,
					"target", targetCustomerID,
					"score", bestResult.NormalizationScore)
			}
		}
	}

	return mergeMap, nil
}
