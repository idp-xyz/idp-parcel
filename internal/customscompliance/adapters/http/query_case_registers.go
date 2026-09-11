package customshttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// CaseRegisterCatalogueReader 是本端点消费的读口：案件配置三本册子（就绪判断、提交
// 授权、关闭义务目录）的列表读面。查阅不触发判断、决定或披露——接存储读面，不接应用
// 编排，与 /customs-compliance-rules 同一条分界（ADR-0077 Decision 一）。
type CaseRegisterCatalogueReader interface {
	ListReadinessJudgments(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]domain.ReadinessJudgment, error)
	ListSubmissionAuthorities(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]domain.SubmissionAuthorization, error)
	ListClosureObligations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ClosureObligationCatalogueEntry, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ CaseRegisterCatalogueReader = ports.CaseRegisterCatalogueRead(nil)

// 业务结果的封闭集合：三本册子各占一格。空册如实答空列表走 2xx 成格，不折成未配置
// （ADR-0077 Decision 四）。
const (
	outcomeReadinessJudgmentsListed    = "READINESS_JUDGMENTS_LISTED"
	outcomeSubmissionAuthoritiesListed = "SUBMISSION_AUTHORITIES_LISTED"
	outcomeClosureObligationsListed    = "CLOSURE_OBLIGATIONS_LISTED"
)

// registry 查询参数的封闭集（未知值坏请求），判据同 /customs-compliance-rules。
const (
	registryReadiness           = "readiness"
	registrySubmissionAuthority = "submission-authority"
	registryClosureObligation   = "closure-obligation"
)

// NewQueryCaseRegistersEndpoint 交回案件配置册查阅的 HTTP 入口
// （GET /customs-case-registers，票 admin-web-page-wiring-frontier/05）。
//
// 三本册子是 customs-cases 一张页面的三格查阅面，共用一个端点按 `registry` 分派——
// 与 /commercial-customer-contracts 各立入口的先例不冲突，分界照那票的第二层理由：
// kind/registry 分派的是「一页里的页签」，各立入口对应「各自独立的页」。门禁两表归
// customs-restrictions 页，不进本分派（票 06 另立）。
//
// 关闭义务的上列**不下推业务截点参数**（票 05 内裁的一小格）：端点契约不收 cutoff，
// 全部已登记义务项连同适用区间原样上列，区间判读留给读者。按截点盘点是关闭核对的
// 判断输入，那是点读 LoadObligationItems 伺候的另一个调用面——查阅面收下截点参数，
// 就等于让目录读口长出第二种「盘点」语义，而它不该有判断语义。
func NewQueryCaseRegistersEndpoint(
	intake CatalogueQueryIntake,
	reader CaseRegisterCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		registry := request.URL.Query().Get("registry")
		if registry != registryReadiness &&
			registry != registrySubmissionAuthority &&
			registry != registryClosureObligation {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registryReadiness:
			serveReadinessJudgments(response, request, reader, tenant, query.Limit)
		case registrySubmissionAuthority:
			serveSubmissionAuthorities(response, request, reader, tenant, query.Limit)
		case registryClosureObligation:
			serveClosureObligations(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveReadinessJudgments(
	response http.ResponseWriter,
	request *http.Request,
	reader CaseRegisterCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	judgments, err := reader.ListReadinessJudgments(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]readinessJudgmentBody, 0, len(judgments))
	for _, judgment := range judgments {
		body := readinessJudgmentBody{
			Unit:     judgment.Unit().String(),
			Basis:    judgment.Basis().String(),
			JudgedAt: judgment.JudgedAt().UTC().Format(time.RFC3339Nano),
		}
		if cause, at, revoked := judgment.Revocation(); revoked {
			body.RevokedBy = cause
			body.RevokedAt = at.UTC().Format(time.RFC3339Nano)
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, readinessJudgmentListResponse{
		Outcome:   outcomeReadinessJudgmentsListed,
		Judgments: bodies,
	})
}

func serveSubmissionAuthorities(
	response http.ResponseWriter,
	request *http.Request,
	reader CaseRegisterCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	authorizations, err := reader.ListSubmissionAuthorities(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]submissionAuthorityBody, 0, len(authorizations))
	for _, authorization := range authorizations {
		body := submissionAuthorityBody{
			Unit:      authorization.Unit().String(),
			Authority: authorization.Authority().String(),
			GrantedAt: authorization.GrantedAt().UTC().Format(time.RFC3339Nano),
		}
		if cause, at, revoked := authorization.Revocation(); revoked {
			body.RevokedBy = cause
			body.RevokedAt = at.UTC().Format(time.RFC3339Nano)
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, submissionAuthorityListResponse{
		Outcome:     outcomeSubmissionAuthoritiesListed,
		Authorities: bodies,
	})
}

func serveClosureObligations(
	response http.ResponseWriter,
	request *http.Request,
	reader CaseRegisterCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	entries, err := reader.ListClosureObligations(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]closureObligationCatalogueBody, 0, len(entries))
	for _, entry := range entries {
		items := make([]closureObligationItemBody, 0, len(entry.Items))
		for _, registration := range entry.Items {
			item := closureObligationItemBody{
				Obligation:  registration.Item.Obligation,
				Scope:       registration.Item.Scope,
				State:       registration.Item.State.String(),
				Basis:       registration.Item.Basis,
				HandedTo:    registration.Item.HandedTo,
				AppliesFrom: registration.AppliesFrom.UTC().Format(time.RFC3339Nano),
			}
			// 终点零值即尚无终点：缺席是真话不是缺陷（判据同解释规则的开放版）。
			if !registration.AppliesUntil.IsZero() {
				item.AppliesUntil = registration.AppliesUntil.UTC().Format(time.RFC3339Nano)
			}
			items = append(items, item)
		}
		bodies = append(bodies, closureObligationCatalogueBody{
			Case:         entry.Case.String(),
			RegisteredAt: entry.RegisteredAt.UTC().Format(time.RFC3339Nano),
			Items:        items,
		})
	}
	writeJSON(response, http.StatusOK, closureObligationListResponse{
		Outcome:    outcomeClosureObligationsListed,
		Catalogues: bodies,
	})
}

type readinessJudgmentListResponse struct {
	Outcome   string                  `json:"outcome"`
	Judgments []readinessJudgmentBody `json:"judgments"`
}

type submissionAuthorityListResponse struct {
	Outcome     string                    `json:"outcome"`
	Authorities []submissionAuthorityBody `json:"authorities"`
}

type closureObligationListResponse struct {
	Outcome    string                           `json:"outcome"`
	Catalogues []closureObligationCatalogueBody `json:"catalogues"`
}

// readinessJudgmentBody 逐字段透出一条就绪判断。撤销两列同现同缺（领域 Revoke 两者
// 缺一构造不出）：缺席即仍有效，在场即已失效且原判断原样保留——调用方判「已失效」
// 看这对字段，不用第二个布尔（0006 自注：失效不得谎报成未配置）。
type readinessJudgmentBody struct {
	Unit      string `json:"unit"`
	Basis     string `json:"basis"`
	JudgedAt  string `json:"judgedAt"`
	RevokedBy string `json:"revokedBy,omitempty"`
	RevokedAt string `json:"revokedAt,omitempty"`
}

// submissionAuthorityBody 与就绪同形的另一条轨（CONTEXT 244：分别形成和失效）。两册
// 分端点分派、分格转写，页面上分列两栏——不得合成一个「可提交」标记（CONTEXT 硬句
// 164，票 05 形状约束一）。
type submissionAuthorityBody struct {
	Unit      string `json:"unit"`
	Authority string `json:"authority"`
	GrantedAt string `json:"grantedAt"`
	RevokedBy string `json:"revokedBy,omitempty"`
	RevokedAt string `json:"revokedAt,omitempty"`
}

type closureObligationItemBody struct {
	Obligation string `json:"obligation"`
	Scope      string `json:"scope"`
	State      string `json:"state"`
	Basis      string `json:"basis"`
	// handedTo 只在承接项在场（CONTEXT「来源责任方、接收责任方、接受决定及权限」：承接必须指名接收责任方；库 CHECK
	// 双向配对），非承接项缺席。
	HandedTo string `json:"handedTo,omitempty"`
	// 适用区间原样透出（appliesUntil 缺席即尚无终点）。截点判读归读者：本端点不收
	// cutoff，见文件头裁决。
	AppliesFrom  string `json:"appliesFrom"`
	AppliesUntil string `json:"appliesUntil,omitempty"`
}

// closureObligationCatalogueBody 是一份义务目录连同全部已登记义务项。items 空数组是
// 「目录已登记、当前无义务项」的如实一格；「目录未登记 → 未决」表现为整份目录不在
// catalogues 里——页面要给这两态不同的说法，判据与票 01 的 contentRegistered 同形
// （0008 自注：两格含义相反）。
type closureObligationCatalogueBody struct {
	Case         string                      `json:"case"`
	RegisteredAt string                      `json:"registeredAt"`
	Items        []closureObligationItemBody `json:"items"`
}
