package customshttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// CatalogueQuery 是一次已授权的主数据目录查阅（ADR-0077）：合规规则库
// （/customs-compliance-rules）与案件配置册（/customs-case-registers）两个端点共用
// 同一个查询形状与同一路准入。作用域来自认证与授权结果，授权边界只有租户——没有
// 客户维；页大小由接入面按渠道契约裁决——两样都不采信调用方自报。
type CatalogueQuery struct {
	Scope domain.OperationsQueryScope
	Limit int
}

// CatalogueQueryIntake 把一次已认证的运营查阅请求翻译成查询。
//
// 它是接口而非解析代码，理由与 ResultIntake 相同：请求方身份与租户必须同时核对，
// 运营接入面的认证归操作者渠道（ADR-0100），其真 Intake 未就位；采信自报租户会穿透 ADR-0003 的隔离
// 边界。未决期间本包不带任何实现，包括「开发用」的采信头部版本。
type CatalogueQueryIntake interface {
	IntakeCatalogueQuery(ctx context.Context, request *http.Request) (CatalogueQuery, error)
}

// RuleCatalogueReader 是本端点消费的读口。查阅不触发判断、决定或披露——所以这里接
// 存储读面，不接应用编排，与 /shipment-request-views 同一条分界（ADR-0077 Decision 一）。
type RuleCatalogueReader interface {
	ListCaseRequirementRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CaseRequirementRuleEntry, error)
	ListInterpretationRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.InterpretationRuleEntry, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义，只消费 ADR-0077
// 钉住的那一个读面。
var _ RuleCatalogueReader = ports.RuleCatalogueRead(nil)

// 业务结果的封闭集合：两本册子各占一格。空目录如实答空列表走 2xx 成格，不折成未配置
// （ADR-0077 Decision 四：空目录的续办是操作员去登记口登记，未配置的续办是接入方去
// 配置渠道——恢复动作不同，判据同 ADR-0029）。
const (
	outcomeCaseRequirementRulesListed = "CASE_REQUIREMENT_RULES_LISTED"
	outcomeInterpretationRulesListed  = "INTERPRETATION_RULES_LISTED"
)

// registry 查询参数的封闭集（未知值坏请求）。值指名登记册，不是内容过滤器——按内容
// 维过滤的查询语义今天不存在，长出来时在本上下文票内扩，不动本分派（ADR-0077）。
const (
	registryCaseRequirement = "case-requirement"
	registryInterpretation  = "interpretation"
)

// NewQueryComplianceRulesEndpoint 交回合规规则库查阅的 HTTP 入口
// （GET /customs-compliance-rules，ADR-0077）。
//
// 两本规则登记册共用一个端点，按 `registry` 查询参数分派：参数在场与否、取值在不在
// 封闭集内属传输形状（与方法检查同级，先于 Intake），读它不构成读业务内容——未配置
// Intake 对全部分支同答 403，分支选择不泄露任何东西。缺席按坏请求拒：替调用方默认
// 一本册子就是替它猜。
func NewQueryComplianceRulesEndpoint(
	intake CatalogueQueryIntake,
	reader RuleCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		registry := request.URL.Query().Get("registry")
		if registry != registryCaseRequirement && registry != registryInterpretation {
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
		case registryCaseRequirement:
			serveCaseRequirementRules(response, request, reader, tenant, query.Limit)
		case registryInterpretation:
			serveInterpretationRules(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveCaseRequirementRules(
	response http.ResponseWriter,
	request *http.Request,
	reader RuleCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rules, err := reader.ListCaseRequirementRules(request.Context(), tenant, limit)
	if err != nil {
		// 读不回是答案未形成，不是「空册」——伪装成后者会让一次该重试的故障变成终局。
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	// 空列表交回空数组而不是 null：调用方判「没有行」不该先判「有没有字段」。
	bodies := make([]caseRequirementRuleBody, 0, len(rules))
	for _, rule := range rules {
		bodies = append(bodies, caseRequirementRuleBody{
			Jurisdiction: rule.Jurisdiction.String(),
			Direction:    rule.Direction.String(),
			Procedure:    rule.Procedure.String(),
			Required:     rule.Judgment.Required,
			Basis:        rule.Judgment.Basis,
		})
	}
	writeJSON(response, http.StatusOK, caseRequirementRuleListResponse{
		Outcome: outcomeCaseRequirementRulesListed,
		Rules:   bodies,
	})
}

func serveInterpretationRules(
	response http.ResponseWriter,
	request *http.Request,
	reader RuleCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rules, err := reader.ListInterpretationRules(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]interpretationRuleBody, 0, len(rules))
	for _, rule := range rules {
		body := interpretationRuleBody{
			Layer:        rule.Layer.String(),
			Jurisdiction: rule.Jurisdiction.String(),
			Rule:         rule.Rule.String(),
			AppliesFrom:  rule.AppliesFrom.UTC().Format(time.RFC3339Nano),
		}
		// 终点零值即尚无终点（开放版）：缺席是真话不是缺陷，不为区间完整补一个
		// 编造的「无限远」时刻。
		if !rule.AppliesUntil.IsZero() {
			body.AppliesUntil = rule.AppliesUntil.UTC().Format(time.RFC3339Nano)
		}
		bodies = append(bodies, body)
	}
	writeJSON(response, http.StatusOK, interpretationRuleListResponse{
		Outcome: outcomeInterpretationRulesListed,
		Rules:   bodies,
	})
}

func writeCatalogueIntakeProblem(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrAccessChannelNotConfigured) {
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
		return
	}
	if errors.Is(err, ErrMalformedRequest) {
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
		return
	}
	writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
}

type caseRequirementRuleListResponse struct {
	Outcome string                    `json:"outcome"`
	Rules   []caseRequirementRuleBody `json:"rules"`
}

type interpretationRuleListResponse struct {
	Outcome string                   `json:"outcome"`
	Rules   []interpretationRuleBody `json:"rules"`
}

// caseRequirementRuleBody 逐字段透出建案要求规则：监管范围三维、判断与依据。依据对
// 两个取值都在场——「不要求」也说得出依据，这正是它与「没登记」的分界；运营查阅不经
// 披露删减，租户内没有跨账户存在性可泄。
type caseRequirementRuleBody struct {
	Jurisdiction string `json:"jurisdiction"`
	Direction    string `json:"direction"`
	Procedure    string `json:"procedure"`
	Required     bool   `json:"required"`
	Basis        string `json:"basis"`
}

// interpretationRuleBody 逐字段透出解释规则版本：选择键三维、法定生效区间与规则引用
// （ADR-0070 问一甲的登记面）。appliesUntil 缺席即开放版。
type interpretationRuleBody struct {
	Layer        string `json:"layer"`
	Jurisdiction string `json:"jurisdiction"`
	Rule         string `json:"rule"`
	AppliesFrom  string `json:"appliesFrom"`
	AppliesUntil string `json:"appliesUntil,omitempty"`
}
