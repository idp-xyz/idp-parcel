package visibilityhttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// VisibilityCatalogueReader 是目录查阅端点消费的读口：六类规则与策略目录，加异常披露
// 规则与冲突信号规则两册（票 ve-disclosure-policy-view/03）。
type VisibilityCatalogueReader interface {
	ListMilestoneMappings(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.MilestoneMappingCatalogueRow, error)
	ListTriageRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.TriageRuleCatalogueRow, error)
	ListNotificationPolicies(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.NotificationPolicyCatalogueRow, error)
	ListClaimEligibilities(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ClaimEligibilityCatalogueRow, error)
	ListClaimAuthorizations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ClaimAuthorizationCatalogueRow, error)
	ListDisclosurePolicies(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.DisclosurePolicyCatalogueRow, error)
	ListExceptionDisclosureRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ExceptionDisclosureRuleCatalogueRow, error)
	ListConflictSignalRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ConflictSignalRuleCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本适配器不新造查询语义，只消费票
// admin-web-page-wiring-frontier/02 钉住的那一个列表读面。
var _ VisibilityCatalogueReader = ports.CatalogueListRead(nil)

// outcomeVisibilityCataloguesListed 是本端点唯一的业务成格，kind 随响应回显；空册也是
// 这一格（ADR-0077 Decision 四，空册本身就是内容，不折成未配置）。
const outcomeVisibilityCataloguesListed = "VISIBILITY_CATALOGUES_LISTED"

// 目录种类的封闭集（?kind= 分派，形状循 /commercial-policies）。前六格与写入口
// CatalogRegistry 的六个 Register 方法逐一对上，后两格对单立的两个登记口——种类命名
// 册子，与端口方法同词根。异常披露规则（EXCEPTION_DISCLOSURE_RULE，0023）与披露策略
// （DISCLOSURE_POLICY，0012）是相邻的两本册，词里的「规则」与「策略」就是分册的记号，
// 不得互相顶替。
const (
	kindMilestoneMapping        = "MILESTONE_MAPPING"
	kindTriageRule              = "TRIAGE_RULE"
	kindNotificationPolicy      = "NOTIFICATION_POLICY"
	kindClaimEligibility        = "CLAIM_ELIGIBILITY"
	kindClaimAuthorization      = "CLAIM_AUTHORIZATION"
	kindDisclosurePolicy        = "DISCLOSURE_POLICY"
	kindExceptionDisclosureRule = "EXCEPTION_DISCLOSURE_RULE"
	kindConflictSignalRule      = "CONFLICT_SIGNAL_RULE"
)

// NewQueryVisibilityCataloguesEndpoint 交回 VE 规则与策略目录查阅的 HTTP 入口
// （GET /visibility-catalogues?kind=，票 admin-web-page-wiring-frontier/02；两册规则目录
// 随票 ve-disclosure-policy-view/03 加入同一端点，多两个 kind，不另立路径）。
//
// 准入复用运营追踪查阅的 OperationsTrackingIntake，不新立一路：目录查阅与投影查阅
// 同属租户内运营读面，授权边界同为租户（ADR-0078 的隔离读注入对两者同形适用）。
// kind 缺席或集外按坏请求拒：六种册子的行形状互不相同，替调用方选一种就是猜。kind
// 的在场与取值属传输形状（与方法检查同级，先于 Intake），读它不构成读业务内容——
// 未配置 Intake 对全部种类同答 403，分支选择不泄露任何东西。
func NewQueryVisibilityCataloguesEndpoint(
	intake OperationsTrackingIntake,
	reader VisibilityCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		kind := request.URL.Query().Get("kind")
		switch kind {
		case kindMilestoneMapping, kindTriageRule, kindNotificationPolicy,
			kindClaimEligibility, kindClaimAuthorization, kindDisclosurePolicy,
			kindExceptionDisclosureRule, kindConflictSignalRule:
		default:
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		query, err := intake.IntakeOperationsQuery(request.Context(), request)
		if err != nil {
			writeOperationsIntakeProblem(response, err)
			return
		}
		tenant := query.Scope.Tenant()

		switch kind {
		case kindMilestoneMapping:
			serveMilestoneMappingCatalogues(response, request, reader, tenant, query.Limit)
		case kindTriageRule:
			serveTriageRuleCatalogues(response, request, reader, tenant, query.Limit)
		case kindNotificationPolicy:
			serveNotificationPolicyCatalogues(response, request, reader, tenant, query.Limit)
		case kindClaimEligibility:
			serveClaimEligibilityCatalogues(response, request, reader, tenant, query.Limit)
		case kindClaimAuthorization:
			serveClaimAuthorizationCatalogues(response, request, reader, tenant, query.Limit)
		case kindDisclosurePolicy:
			serveDisclosurePolicyCatalogues(response, request, reader, tenant, query.Limit)
		case kindExceptionDisclosureRule:
			serveExceptionDisclosureRuleCatalogues(response, request, reader, tenant, query.Limit)
		case kindConflictSignalRule:
			serveConflictSignalRuleCatalogues(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveMilestoneMappingCatalogues(
	response http.ResponseWriter,
	request *http.Request,
	reader VisibilityCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListMilestoneMappings(request.Context(), tenant, limit)
	if err != nil {
		// 读不回是答案未形成，不是「空册」——伪装成后者会让一次该重试的故障变成一份
		// 看起来如实的空册。各 kind 分支同一条理由，下同。
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]milestoneMappingCatalogueBody, 0, len(rows))
	for _, row := range rows {
		body := milestoneMappingCatalogueBody{
			Version:       row.Version,
			EffectiveFrom: catalogueInstant(row.EffectiveFrom),
			ApprovedBy:    row.ApprovedBy,
			Entries:       make([]milestoneMappingEntryBody, 0, len(row.Entries)),
		}
		if row.HasEffectiveTo {
			body.EffectiveTo = catalogueInstant(row.EffectiveTo)
		}
		for _, entry := range row.Entries {
			body.Entries = append(body.Entries, milestoneMappingEntryBody{
				Source:    entry.Source,
				FactKind:  entry.FactKind,
				Milestone: entry.Milestone,
			})
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, milestoneMappingListResponse{
		Outcome:    outcomeVisibilityCataloguesListed,
		Kind:       kindMilestoneMapping,
		Catalogues: bodies,
	})
}

func serveTriageRuleCatalogues(
	response http.ResponseWriter,
	request *http.Request,
	reader VisibilityCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListTriageRules(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]triageRuleCatalogueBody, 0, len(rows))
	for _, row := range rows {
		body := triageRuleCatalogueBody{
			Version:       row.Version,
			EffectiveFrom: catalogueInstant(row.EffectiveFrom),
			ApprovedBy:    row.ApprovedBy,
			Entries:       make([]triageRuleEntryBody, 0, len(row.Entries)),
		}
		if row.HasEffectiveTo {
			body.EffectiveTo = catalogueInstant(row.EffectiveTo)
		}
		for _, entry := range row.Entries {
			body.Entries = append(body.Entries, triageRuleEntryBody{
				SignalKind: entry.SignalKind,
				Confidence: entry.Confidence,
				Outcome:    entry.Outcome,
			})
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, triageRuleListResponse{
		Outcome:    outcomeVisibilityCataloguesListed,
		Kind:       kindTriageRule,
		Catalogues: bodies,
	})
}

func serveNotificationPolicyCatalogues(
	response http.ResponseWriter,
	request *http.Request,
	reader VisibilityCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListNotificationPolicies(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]notificationPolicyBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, notificationPolicyBody{
			Policy:        row.Policy,
			Channel:       row.Channel,
			DeadlineAfter: row.DeadlineAfter,
			Obligation:    row.Obligation,
			ApprovedBy:    row.ApprovedBy,
		})
	}
	writeJSON(response, http.StatusOK, notificationPolicyListResponse{
		Outcome:    outcomeVisibilityCataloguesListed,
		Kind:       kindNotificationPolicy,
		Catalogues: bodies,
	})
}

func serveClaimEligibilityCatalogues(
	response http.ResponseWriter,
	request *http.Request,
	reader VisibilityCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListClaimEligibilities(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]claimEligibilityBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, claimEligibilityBody{
			Contract:     row.Contract,
			Version:      row.Version,
			ApprovedBy:   row.ApprovedBy,
			CoveredKinds: append([]string{}, row.CoveredKinds...),
		})
	}
	writeJSON(response, http.StatusOK, claimEligibilityListResponse{
		Outcome:    outcomeVisibilityCataloguesListed,
		Kind:       kindClaimEligibility,
		Catalogues: bodies,
	})
}

func serveClaimAuthorizationCatalogues(
	response http.ResponseWriter,
	request *http.Request,
	reader VisibilityCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListClaimAuthorizations(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]claimAuthorizationBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, claimAuthorizationBody{
			Customer:   row.Customer,
			Version:    row.Version,
			ApprovedBy: row.ApprovedBy,
			Applicants: append([]string{}, row.Applicants...),
		})
	}
	writeJSON(response, http.StatusOK, claimAuthorizationListResponse{
		Outcome:    outcomeVisibilityCataloguesListed,
		Kind:       kindClaimAuthorization,
		Catalogues: bodies,
	})
}

func serveDisclosurePolicyCatalogues(
	response http.ResponseWriter,
	request *http.Request,
	reader VisibilityCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListDisclosurePolicies(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]disclosurePolicyCatalogueBody, 0, len(rows))
	for _, row := range rows {
		body := disclosurePolicyCatalogueBody{
			Version:       row.Version,
			EffectiveFrom: catalogueInstant(row.EffectiveFrom),
			ApprovedBy:    row.ApprovedBy,
			Entries:       make([]disclosurePolicyEntryBody, 0, len(row.Entries)),
		}
		if row.HasEffectiveTo {
			body.EffectiveTo = catalogueInstant(row.EffectiveTo)
		}
		for _, entry := range row.Entries {
			body.Entries = append(body.Entries, disclosurePolicyEntryBody{
				Customer:   entry.Customer,
				Milestones: disclosureCellBodyOf(entry.Milestones),
				ETA:        disclosureCellBodyOf(entry.ETA),
				Final:      disclosureCellBodyOf(entry.Final),
				Note:       disclosureCellBodyOf(entry.Note),
			})
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, disclosurePolicyListResponse{
		Outcome:    outcomeVisibilityCataloguesListed,
		Kind:       kindDisclosurePolicy,
		Catalogues: bodies,
	})
}

func serveExceptionDisclosureRuleCatalogues(
	response http.ResponseWriter,
	request *http.Request,
	reader VisibilityCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListExceptionDisclosureRules(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]exceptionDisclosureRuleCatalogueBody, 0, len(rows))
	for _, row := range rows {
		body := exceptionDisclosureRuleCatalogueBody{
			Version:       row.Version,
			EffectiveFrom: catalogueInstant(row.EffectiveFrom),
			ApprovedBy:    row.ApprovedBy,
			Entries:       make([]exceptionDisclosureRuleEntryBody, 0, len(row.Entries)),
		}
		if row.HasEffectiveTo {
			body.EffectiveTo = catalogueInstant(row.EffectiveTo)
		}
		for _, entry := range row.Entries {
			body.Entries = append(body.Entries, exceptionDisclosureRuleEntryBody{
				Customer:    entry.Customer,
				SignalKind:  entry.SignalKind,
				Confidence:  entry.Confidence,
				Disclosable: entry.Disclosable,
				AutoRelease: entry.AutoRelease,
				Content:     entry.Content,
			})
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, exceptionDisclosureRuleListResponse{
		Outcome:    outcomeVisibilityCataloguesListed,
		Kind:       kindExceptionDisclosureRule,
		Catalogues: bodies,
	})
}

func serveConflictSignalRuleCatalogues(
	response http.ResponseWriter,
	request *http.Request,
	reader VisibilityCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListConflictSignalRules(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]conflictSignalRuleBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, conflictSignalRuleBody{
			SignalKind:   row.SignalKind,
			Version:      row.Version,
			Confidence:   row.Confidence,
			ApprovedBy:   row.ApprovedBy,
			RegisteredAt: catalogueInstant(row.RegisteredAt),
		})
	}
	writeJSON(response, http.StatusOK, conflictSignalRuleListResponse{
		Outcome:    outcomeVisibilityCataloguesListed,
		Kind:       kindConflictSignalRule,
		Catalogues: bodies,
	})
}

// catalogueInstant 是目录区间时刻的传输词形。本包投影查阅同用 RFC3339Nano，两处读面
// 对同类时刻不摆两种词形。
func catalogueInstant(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

// 六种册子各自的响应与行体。字段名不共享一套泛化壳，理由同 /commercial-policies：
// 六种行形状互不相同，共享壳要么空出五份字段，要么把强类型折成 any——kind 回显加
// 各自成形的 catalogues 数组，调用方按 kind 择形状。

type milestoneMappingListResponse struct {
	Outcome    string                          `json:"outcome"`
	Kind       string                          `json:"kind"`
	Catalogues []milestoneMappingCatalogueBody `json:"catalogues"`
}

// milestoneMappingCatalogueBody 是一版映射连同整版条目。effectiveTo 缺席即当前版
// （未闭区间）——列表读口用显式布尔分辨，传输层折成缺席字段，两义不混。
type milestoneMappingCatalogueBody struct {
	Version       string                      `json:"version"`
	EffectiveFrom string                      `json:"effectiveFrom"`
	EffectiveTo   string                      `json:"effectiveTo,omitempty"`
	ApprovedBy    string                      `json:"approvedBy"`
	Entries       []milestoneMappingEntryBody `json:"entries"`
}

type milestoneMappingEntryBody struct {
	Source    string `json:"source"`
	FactKind  string `json:"factKind"`
	Milestone string `json:"milestone"`
}

type triageRuleListResponse struct {
	Outcome    string                    `json:"outcome"`
	Kind       string                    `json:"kind"`
	Catalogues []triageRuleCatalogueBody `json:"catalogues"`
}

type triageRuleCatalogueBody struct {
	Version       string                `json:"version"`
	EffectiveFrom string                `json:"effectiveFrom"`
	EffectiveTo   string                `json:"effectiveTo,omitempty"`
	ApprovedBy    string                `json:"approvedBy"`
	Entries       []triageRuleEntryBody `json:"entries"`
}

type triageRuleEntryBody struct {
	SignalKind string `json:"signalKind"`
	Confidence string `json:"confidence"`
	Outcome    string `json:"outcome"`
}

type notificationPolicyListResponse struct {
	Outcome    string                   `json:"outcome"`
	Kind       string                   `json:"kind"`
	Catalogues []notificationPolicyBody `json:"catalogues"`
}

// notificationPolicyBody 无版本与区间字段：这份目录的版本化由披露策略引用本身承担
// （0010），deadlineAfter 是 interval 的文本词形（相对量，绝对截止点在目录上不存在）。
type notificationPolicyBody struct {
	Policy        string `json:"policy"`
	Channel       string `json:"channel"`
	DeadlineAfter string `json:"deadlineAfter"`
	Obligation    string `json:"obligation"`
	ApprovedBy    string `json:"approvedBy"`
}

type claimEligibilityListResponse struct {
	Outcome    string                 `json:"outcome"`
	Kind       string                 `json:"kind"`
	Catalogues []claimEligibilityBody `json:"catalogues"`
}

type claimEligibilityBody struct {
	Contract     string   `json:"contract"`
	Version      string   `json:"version"`
	ApprovedBy   string   `json:"approvedBy"`
	CoveredKinds []string `json:"coveredKinds"`
}

type claimAuthorizationListResponse struct {
	Outcome    string                   `json:"outcome"`
	Kind       string                   `json:"kind"`
	Catalogues []claimAuthorizationBody `json:"catalogues"`
}

// claimAuthorizationBody 的 applicants 允许为空数组：目录行在场而名单为空是「此账户
// 当前不授权任何人代提」的显式决定（0018），与「还没登记」（整行不在 catalogues 里）
// 不是一回事——页面文案必须分开说，别把已作出的授权决定读丢。
type claimAuthorizationBody struct {
	Customer   string   `json:"customer"`
	Version    string   `json:"version"`
	ApprovedBy string   `json:"approvedBy"`
	Applicants []string `json:"applicants"`
}

type disclosurePolicyListResponse struct {
	Outcome    string                          `json:"outcome"`
	Kind       string                          `json:"kind"`
	Catalogues []disclosurePolicyCatalogueBody `json:"catalogues"`
}

type disclosurePolicyCatalogueBody struct {
	Version       string                      `json:"version"`
	EffectiveFrom string                      `json:"effectiveFrom"`
	EffectiveTo   string                      `json:"effectiveTo,omitempty"`
	ApprovedBy    string                      `json:"approvedBy"`
	Entries       []disclosurePolicyEntryBody `json:"entries"`
}

type disclosurePolicyEntryBody struct {
	Customer   string             `json:"customer"`
	Milestones disclosureCellBody `json:"milestones"`
	ETA        disclosureCellBody `json:"eta"`
	Final      disclosureCellBody `json:"final"`
	Note       disclosureCellBody `json:"note"`
}

// disclosureCellBody 一维一格：content 只在 SHOWN 时在场（0012 的 shape 约束），
// 缺席字段照实转写读口的空串。
type disclosureCellBody struct {
	State   string `json:"state"`
	Content string `json:"content,omitempty"`
}

func disclosureCellBodyOf(cell ports.DisclosureDimensionCell) disclosureCellBody {
	return disclosureCellBody{State: cell.State, Content: cell.Content}
}

type exceptionDisclosureRuleListResponse struct {
	Outcome    string                                 `json:"outcome"`
	Kind       string                                 `json:"kind"`
	Catalogues []exceptionDisclosureRuleCatalogueBody `json:"catalogues"`
}

// exceptionDisclosureRuleCatalogueBody 是一版异常披露规则连同整版条目（0023）。它与
// disclosurePolicyCatalogueBody 抬头同形、条目不同形：那边按客户答四维态，这边按客户 ×
// 信号 × 可信度答三件——两本相邻的册各自成形，不共享条目壳。
type exceptionDisclosureRuleCatalogueBody struct {
	Version       string                             `json:"version"`
	EffectiveFrom string                             `json:"effectiveFrom"`
	EffectiveTo   string                             `json:"effectiveTo,omitempty"`
	ApprovedBy    string                             `json:"approvedBy"`
	Entries       []exceptionDisclosureRuleEntryBody `json:"entries"`
}

// exceptionDisclosureRuleEntryBody 的 content 只在 disclosable 时在场（0023 的成对约束），
// 缺席字段照实转写读口的空串——与披露策略条目的 content 同一条传输纪律。
type exceptionDisclosureRuleEntryBody struct {
	Customer    string `json:"customer"`
	SignalKind  string `json:"signalKind"`
	Confidence  string `json:"confidence"`
	Disclosable bool   `json:"disclosable"`
	AutoRelease bool   `json:"autoRelease"`
	Content     string `json:"content,omitempty"`
}

type conflictSignalRuleListResponse struct {
	Outcome    string                   `json:"outcome"`
	Kind       string                   `json:"kind"`
	Catalogues []conflictSignalRuleBody `json:"catalogues"`
}

// conflictSignalRuleBody 无生效区间字段：这份目录一租户一条、换版是治理动作（0025 头注），
// 行上只有库落下的登记时刻；registeredAt 与区间时刻同用 catalogueInstant 的词形。
type conflictSignalRuleBody struct {
	SignalKind   string `json:"signalKind"`
	Version      string `json:"version"`
	Confidence   string `json:"confidence"`
	ApprovedBy   string `json:"approvedBy"`
	RegisteredAt string `json:"registeredAt"`
}
