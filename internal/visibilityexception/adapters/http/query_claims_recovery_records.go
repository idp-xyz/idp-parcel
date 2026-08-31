package visibilityhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ClaimsRecoveryReviewReader 是理赔与追偿页消费的读口：客户异常通知册、客户索赔
// 项册、追偿事项册（管理台 claims-recovery 页三页签，票 admin-skeleton-closure-batch/06）。
type ClaimsRecoveryReviewReader interface {
	ListCustomerNotifications(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CustomerNotificationCatalogueRow, error)
	ListClaimItems(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ClaimItemCatalogueRow, error)
	ListRecoveryMatters(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.RecoveryMatterCatalogueRow, error)
}

var _ ClaimsRecoveryReviewReader = ports.ClaimsRecoveryReviewRead(nil)

const (
	outcomeCustomerNotificationsListed = "CUSTOMER_NOTIFICATIONS_LISTED"
	outcomeClaimItemsListed            = "CLAIM_ITEMS_LISTED"
	outcomeRecoveryMattersListed       = "RECOVERY_MATTERS_LISTED"
)

const (
	registryCustomerNotification = "customer-notification"
	registryClaimItem            = "claim-item"
	registryRecoveryMatter       = "recovery-matter"
)

// NewQueryClaimsRecoveryRecordsEndpoint 交回理赔与追偿页三本册子的 HTTP 入口
// （GET /claims-recovery-records，票 admin-skeleton-closure-batch/06）。
//
// 查阅不受理索赔、不作资格审核、不定责、不发通知——受理走 /claims（客户提交面，
// 本文件不触碰），资格与结论是内部授权角色的另两个判断（CONTEXT「三判分步」）。
// 本端点只消费存储读面（ADR-0077 Decision 一），准入复用运营追踪查阅的
// OperationsTrackingIntake，判据同 /exception-triage-records。
//
// **金额零字段**：赔付金额、追回金额属结算与财务上下文（CONTEXT 边界「赔付金额的
// 计算」），三本册子的行上连键都不存在，前端模板的金额列由它们的读面回答。
func NewQueryClaimsRecoveryRecordsEndpoint(
	intake OperationsTrackingIntake,
	reader ClaimsRecoveryReviewReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		registry := request.URL.Query().Get("registry")
		if registry != registryCustomerNotification &&
			registry != registryClaimItem &&
			registry != registryRecoveryMatter {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, err := intake.IntakeOperationsQuery(request.Context(), request)
		if err != nil {
			writeOperationsIntakeProblem(response, err)
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registryCustomerNotification:
			serveCustomerNotifications(response, request, reader, tenant, query.Limit)
		case registryClaimItem:
			serveClaimItems(response, request, reader, tenant, query.Limit)
		case registryRecoveryMatter:
			serveRecoveryMatters(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveCustomerNotifications(
	response http.ResponseWriter,
	request *http.Request,
	reader ClaimsRecoveryReviewReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListCustomerNotifications(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]customerNotificationBody, 0, len(rows))
	for _, row := range rows {
		body := customerNotificationBody{
			NotificationID: row.NotificationID,
			Customer:       row.Customer,
			Episode:        row.Episode,
			Policy:         row.Policy,
			Content:        row.Content,
			Channel:        row.Channel,
			Obligation:     row.Obligation,
			DecidedAt:      catalogueInstant(row.DecidedAt),
			Deadline:       catalogueInstant(row.Deadline),
		}
		// 空里程碑交回空数组：还没有任何送达进展也是如实答案，不是缺字段。
		milestones := make([]notificationMilestoneBody, 0, len(row.Milestones))
		for _, node := range row.Milestones {
			milestones = append(milestones, notificationMilestoneBody{
				Milestone:  node.Milestone,
				RecordedAt: catalogueInstant(node.RecordedAt),
			})
		}
		body.Milestones = milestones
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, customerNotificationListResponse{
		Outcome:       outcomeCustomerNotificationsListed,
		Notifications: bodies,
	})
}

func serveClaimItems(
	response http.ResponseWriter,
	request *http.Request,
	reader ClaimsRecoveryReviewReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListClaimItems(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]claimItemBody, 0, len(rows))
	for _, row := range rows {
		body := claimItemBody{
			Batch:            row.Batch,
			ItemID:           row.ItemID,
			Customer:         row.Customer,
			Applicant:        row.Applicant,
			Contract:         row.Contract,
			Target:           row.Target,
			Kind:             row.Kind,
			Revision:         row.Revision,
			SubmittedAt:      catalogueInstant(row.SubmittedAt),
			Screen:           row.Screen,
			ScreenBasis:      row.ScreenBasis,
			MissingMaterials: row.MissingMaterials,
			SupplementScope:  row.SupplementScope,
			SupplementNotice: row.SupplementNotice,
			DeadlineVersions: row.DeadlineVersions,
			Conclusion:       row.Conclusion,
			PriorConclusion:  row.PriorConclusion,
			Withdrawn:        row.Withdrawn,
		}
		if row.SupplementDeadline != nil {
			body.SupplementDeadline = catalogueInstant(*row.SupplementDeadline)
		}
		if row.ConcludedAt != nil {
			body.ConcludedAt = catalogueInstant(*row.ConcludedAt)
		}
		if row.ReviewBy != nil {
			body.ReviewBy = catalogueInstant(*row.ReviewBy)
		}
		if row.WithdrawnAt != nil {
			body.WithdrawnAt = catalogueInstant(*row.WithdrawnAt)
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, claimItemListResponse{
		Outcome: outcomeClaimItemsListed,
		Items:   bodies,
	})
}

func serveRecoveryMatters(
	response http.ResponseWriter,
	request *http.Request,
	reader ClaimsRecoveryReviewReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListRecoveryMatters(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]recoveryMatterBody, 0, len(rows))
	for _, row := range rows {
		body := recoveryMatterBody{
			MatterID:     row.MatterID,
			CaseID:       row.CaseID,
			Counterparty: row.Counterparty,
			Scope:        row.Scope,
			Basis:        row.Basis,
			LegalEntity:  row.LegalEntity,
			Evidence:     row.Evidence,
			OpenedAt:     catalogueInstant(row.OpenedAt),
			Deadline:     catalogueInstant(row.Deadline),
		}
		if row.PreliminaryNotice != nil {
			body.PreliminaryNotice = recoveryActionBodyOf(*row.PreliminaryNotice)
		}
		if row.FormalAssertion != nil {
			body.FormalAssertion = recoveryActionBodyOf(*row.FormalAssertion)
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, recoveryMatterListResponse{
		Outcome: outcomeRecoveryMattersListed,
		Matters: bodies,
	})
}

type customerNotificationListResponse struct {
	Outcome       string                     `json:"outcome"`
	Notifications []customerNotificationBody `json:"notifications"`
}

// customerNotificationBody 逐字段透出一份客户异常通知义务及其送达里程碑。
//
// milestones 是义务行上的追加序列（0004 里程碑列），逐节点透出、不折成「最新状态」
// 单值：义务是否履行要看序列有没有到达送达节点，折并会把「发出但未送达」演成完成。
type customerNotificationBody struct {
	NotificationID string                      `json:"notificationId"`
	Customer       string                      `json:"customer"`
	Episode        string                      `json:"episode"`
	DecidedAt      string                      `json:"decidedAt"`
	Policy         string                      `json:"policy"`
	Content        string                      `json:"content"`
	Deadline       string                      `json:"deadline"`
	Channel        string                      `json:"channel"`
	Obligation     string                      `json:"obligation"`
	Milestones     []notificationMilestoneBody `json:"milestones"`
}

type notificationMilestoneBody struct {
	Milestone  string `json:"milestone"`
	RecordedAt string `json:"recordedAt"`
}

type claimItemListResponse struct {
	Outcome string          `json:"outcome"`
	Items   []claimItemBody `json:"items"`
}

// claimItemBody 逐字段透出一件客户索赔项的生命周期登记。
//
// screen 两键成对在场表示资格初筛已判；supplement 四键在「等待补充材料」态在场，
// deadlineVersions 计的是期限版本条数（0013 版本链），最新期限即 supplementDeadline。
// conclusion/reviewBy/priorConclusion 是三判的第三判及其复核替代链；withdrawn 是
// 客户撤回登记，撤回不删行。首次索赔期限（首索期限如何裁决）在项行上没有登记格，
// 结构上不存在（票 06 Comments 记明）。材料收讫册（0021）是资格审核的证据面，
// 不在本页三签之列，未透出。
type claimItemBody struct {
	Batch              string `json:"batch"`
	ItemID             string `json:"itemId"`
	Customer           string `json:"customer"`
	Applicant          string `json:"applicant,omitempty"`
	Contract           string `json:"contract"`
	Target             string `json:"target"`
	Kind               string `json:"kind"`
	SubmittedAt        string `json:"submittedAt"`
	Revision           int64  `json:"revision"`
	Screen             string `json:"screen,omitempty"`
	ScreenBasis        string `json:"screenBasis,omitempty"`
	MissingMaterials   string `json:"missingMaterials,omitempty"`
	SupplementScope    string `json:"supplementScope,omitempty"`
	SupplementNotice   string `json:"supplementNotice,omitempty"`
	SupplementDeadline string `json:"supplementDeadline,omitempty"`
	DeadlineVersions   int64  `json:"deadlineVersions"`
	Conclusion         string `json:"conclusion,omitempty"`
	ConcludedAt        string `json:"concludedAt,omitempty"`
	ReviewBy           string `json:"reviewBy,omitempty"`
	PriorConclusion    string `json:"priorConclusion,omitempty"`
	Withdrawn          bool   `json:"withdrawn"`
	WithdrawnAt        string `json:"withdrawnAt,omitempty"`
}

type recoveryMatterListResponse struct {
	Outcome string               `json:"outcome"`
	Matters []recoveryMatterBody `json:"matters"`
}

// recoveryMatterBody 逐字段透出一件追偿事项及其两类最新行动节点。
//
// preliminaryNotice / formalAssertion 各取该类行动的最新一节（行动序列 0004 追加，
// attempt 递增），缺席即该类行动尚未发生。对方响应与对外责任结论在事项与行动两表
// 都没有登记格，结构上不存在（票 06 Comments 记明）——追偿的对外结论要等结论类
// 登记进来，不由查阅面替谈判结果下判断。
type recoveryMatterBody struct {
	MatterID          string              `json:"matterId"`
	CaseID            string              `json:"caseId"`
	Counterparty      string              `json:"counterparty"`
	Scope             string              `json:"scope"`
	Basis             string              `json:"basis"`
	LegalEntity       string              `json:"legalEntity"`
	Evidence          string              `json:"evidence"`
	Deadline          string              `json:"deadline"`
	OpenedAt          string              `json:"openedAt"`
	PreliminaryNotice *recoveryActionBody `json:"preliminaryNotice,omitempty"`
	FormalAssertion   *recoveryActionBody `json:"formalAssertion,omitempty"`
}

type recoveryActionBody struct {
	Milestone  string `json:"milestone"`
	Attempt    int64  `json:"attempt"`
	OccurredAt string `json:"occurredAt"`
}

func recoveryActionBodyOf(cell ports.RecoveryActionCell) *recoveryActionBody {
	return &recoveryActionBody{
		Milestone:  cell.Milestone,
		Attempt:    cell.Attempt,
		OccurredAt: catalogueInstant(cell.OccurredAt),
	}
}
