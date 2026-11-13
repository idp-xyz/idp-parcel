package visibilityhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.CustomerNotificationCatalogueRow], error)
	ListClaimItems(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.ClaimItemCatalogueRow], error)
	ListRecoveryMatters(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.RecoveryMatterCatalogueRow], error)
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

// claimsRecoveryRegistryPages 按 registry 取那一册的查询声明：选择器的每个取值各是一册（ADR-0144 决定四），不在表里的取值
// 即集外。
var claimsRecoveryRegistryPages = map[string]*cataloguepage.Catalogue{
	registryCustomerNotification: ports.CustomerNotificationPages,
	registryClaimItem:            ports.ClaimItemPages,
	registryRecoveryMatter:       ports.RecoveryMatterPages,
}

// NewQueryClaimsRecoveryRecordsEndpoint 交回理赔与追偿页各册子的 HTTP 入口
// （GET /claims-recovery-records，票 admin-skeleton-closure-batch/06）。
//
// 查阅不受理索赔、不作资格审核、不定责、不发通知——受理走 /claims（客户提交面，
// 本文件不触碰），资格与结论是内部授权角色的另两个判断（CONTEXT「证据、客户索赔与追偿」一节「收到客户索赔、通过资格审核和确认赔偿责任是不同判断」）。
// 本端点只消费存储读面（ADR-0077 Decision 一），准入复用运营追踪查阅的
// OperationsTrackingIntake，判据同 /exception-triage-records。
//
// **金额零字段**：赔付金额、追回金额属结算与财务上下文（CONTEXT「证据、客户索赔与追偿」一节：「`UC-SA-007` 唯一形成赔付和追偿金额」），
// 各册子的行上连键都不存在，前端模板的金额列由它们的读面回答。
//
// 翻页、排序与筛选照 ADR-0144：registry 选定那一册，按该册声明解码（writePageQueryProblem 头注），坏参数答 400 带
// 理由散文，答复加 page。registry 与其余查询参数一样在 Intake 之后才读：未配置 Intake 对一切请求同答 403（ADR-0055
// 决定三）；配置了的渠道上 registry 缺席或集外答不带理由的坏请求，与迁移前同答。
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
		query, err := intake.IntakeOperationsQuery(request.Context(), request)
		if err != nil {
			writeOperationsIntakeProblem(response, err)
			return
		}
		values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		registry := values.Get("registry")
		catalogue, known := claimsRecoveryRegistryPages[registry]
		if !known {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		pageQuery, err := catalogue.Decode(values)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registryCustomerNotification:
			serveCustomerNotifications(response, request, reader, tenant, query.Limit, pageQuery)
		case registryClaimItem:
			serveClaimItems(response, request, reader, tenant, query.Limit, pageQuery)
		case registryRecoveryMatter:
			serveRecoveryMatters(response, request, reader, tenant, query.Limit, pageQuery)
		}
	})
}

func serveCustomerNotifications(
	response http.ResponseWriter,
	request *http.Request,
	reader ClaimsRecoveryReviewReader,
	tenant domain.TenantID,
	limit int,
	pageQuery cataloguepage.Query,
) {
	page, err := reader.ListCustomerNotifications(request.Context(), tenant, limit, pageQuery)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]customerNotificationBody, 0, len(page.Rows))
	for _, row := range page.Rows {
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
		if manual := row.ManualRecord; manual != nil {
			body.ManualRecord = &manualNotificationBody{
				NotifiedBy: manual.NotifiedBy, Method: manual.Method,
				Original: manual.Original, RecordedAt: catalogueInstant(manual.RecordedAt),
			}
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
		Page:          page.Page(limit),
	})
}

func serveClaimItems(
	response http.ResponseWriter,
	request *http.Request,
	reader ClaimsRecoveryReviewReader,
	tenant domain.TenantID,
	limit int,
	pageQuery cataloguepage.Query,
) {
	page, err := reader.ListClaimItems(request.Context(), tenant, limit, pageQuery)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]claimItemBody, 0, len(page.Rows))
	for _, row := range page.Rows {
		body := claimItemBody{
			Batch:                 row.Batch,
			ItemID:                row.ItemID,
			DocumentNumber:        row.DocumentNumber,
			DocumentNumberAbsence: row.DocumentNumberAbsence,
			Customer:              row.Customer,
			Applicant:             row.Applicant,
			Contract:              row.Contract,
			Target:                row.Target,
			Kind:                  row.Kind,
			Revision:              row.Revision,
			SubmittedAt:           catalogueInstant(row.SubmittedAt),
			Screen:                row.Screen,
			ScreenBasis:           row.ScreenBasis,
			MissingMaterials:      row.MissingMaterials,
			SupplementScope:       row.SupplementScope,
			SupplementNotice:      row.SupplementNotice,
			DeadlineVersions:      row.DeadlineVersions,
			Conclusion:            row.Conclusion,
			PriorConclusion:       row.PriorConclusion,
			Withdrawn:             row.Withdrawn,
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
		Page:    page.Page(limit),
	})
}

func serveRecoveryMatters(
	response http.ResponseWriter,
	request *http.Request,
	reader ClaimsRecoveryReviewReader,
	tenant domain.TenantID,
	limit int,
	pageQuery cataloguepage.Query,
) {
	page, err := reader.ListRecoveryMatters(request.Context(), tenant, limit, pageQuery)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]recoveryMatterBody, 0, len(page.Rows))
	for _, row := range page.Rows {
		body := recoveryMatterBody{
			MatterID:           row.MatterID,
			CaseID:             row.CaseID,
			CaseDocumentNumber: row.CaseDocumentNumber,
			Counterparty:       row.Counterparty,
			Scope:              row.Scope,
			Basis:              row.Basis,
			LegalEntity:        row.LegalEntity,
			Evidence:           row.Evidence,
			OpenedAt:           catalogueInstant(row.OpenedAt),
			Deadline:           catalogueInstant(row.Deadline),
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
		Page:    page.Page(limit),
	})
}

type customerNotificationListResponse struct {
	Outcome       string                     `json:"outcome"`
	Notifications []customerNotificationBody `json:"notifications"`
	Page          cataloguepage.Page         `json:"page"`
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
	// ManualRecord 只在这份通知是人工告知后登记的时在场；整键缺席即经通知通道推送的（或尚未发出）。
	ManualRecord *manualNotificationBody `json:"manualRecord,omitempty"`
}

type manualNotificationBody struct {
	NotifiedBy string `json:"notifiedBy"`
	Method     string `json:"method"`
	Original   string `json:"original"`
	RecordedAt string `json:"recordedAt"`
}

type notificationMilestoneBody struct {
	Milestone  string `json:"milestone"`
	RecordedAt string `json:"recordedAt"`
}

type claimItemListResponse struct {
	Outcome string             `json:"outcome"`
	Items   []claimItemBody    `json:"items"`
	Page    cataloguepage.Page `json:"page"`
}

// claimItemBody 逐字段透出一件客户索赔项的生命周期登记。
//
// screen 两键成对在场表示资格初筛已判；supplement 四键在「等待补充材料」态在场，
// deadlineVersions 计的是期限版本条数（0013 版本链），最新期限即 supplementDeadline。
// conclusion/reviewBy/priorConclusion 是三判的第三判及其复核替代链；withdrawn 是
// 客户撤回登记，撤回不删行。首次索赔期限（首索期限如何裁决）在项行上没有登记格，
// 结构上不存在（票 06 Comments 记明）。材料收讫册（0021）是资格审核的证据面，
// 不在本页三签之列，未透出。
//
// documentNumber / documentNumberAbsence 同异常案件那一行（exceptionCaseBody）。batch 与 itemId 是客户自己的批次号与项标识，
// 单据号不取代它们。
type claimItemBody struct {
	Batch                 string `json:"batch"`
	ItemID                string `json:"itemId"`
	DocumentNumber        string `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string `json:"documentNumberAbsence,omitempty"`
	Customer              string `json:"customer"`
	Applicant             string `json:"applicant,omitempty"`
	Contract              string `json:"contract"`
	Target                string `json:"target"`
	Kind                  string `json:"kind"`
	SubmittedAt           string `json:"submittedAt"`
	Revision              int64  `json:"revision"`
	Screen                string `json:"screen,omitempty"`
	ScreenBasis           string `json:"screenBasis,omitempty"`
	MissingMaterials      string `json:"missingMaterials,omitempty"`
	SupplementScope       string `json:"supplementScope,omitempty"`
	SupplementNotice      string `json:"supplementNotice,omitempty"`
	SupplementDeadline    string `json:"supplementDeadline,omitempty"`
	DeadlineVersions      int64  `json:"deadlineVersions"`
	Conclusion            string `json:"conclusion,omitempty"`
	ConcludedAt           string `json:"concludedAt,omitempty"`
	ReviewBy              string `json:"reviewBy,omitempty"`
	PriorConclusion       string `json:"priorConclusion,omitempty"`
	Withdrawn             bool   `json:"withdrawn"`
	WithdrawnAt           string `json:"withdrawnAt,omitempty"`
}

type recoveryMatterListResponse struct {
	Outcome string               `json:"outcome"`
	Matters []recoveryMatterBody `json:"matters"`
	Page    cataloguepage.Page   `json:"page"`
}

// recoveryMatterBody 逐字段透出一件追偿事项及其两类最新行动节点。
//
// preliminaryNotice / formalAssertion 各取该类行动的最新一节（行动序列 0004 追加，
// attempt 递增），缺席即该类行动尚未发生。对方响应与对外责任结论在事项与行动两表
// 都没有登记格，结构上不存在（票 06 Comments 记明）——追偿的对外结论要等结论类
// 登记进来，不由查阅面替谈判结果下判断。caseDocumentNumber 是关联案件的单据号，没有就缺席。
type recoveryMatterBody struct {
	MatterID           string              `json:"matterId"`
	CaseID             string              `json:"caseId"`
	CaseDocumentNumber string              `json:"caseDocumentNumber,omitempty"`
	Counterparty       string              `json:"counterparty"`
	Scope              string              `json:"scope"`
	Basis              string              `json:"basis"`
	LegalEntity        string              `json:"legalEntity"`
	Evidence           string              `json:"evidence"`
	Deadline           string              `json:"deadline"`
	OpenedAt           string              `json:"openedAt"`
	PreliminaryNotice  *recoveryActionBody `json:"preliminaryNotice,omitempty"`
	FormalAssertion    *recoveryActionBody `json:"formalAssertion,omitempty"`
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
