package customshttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// GateConditionCatalogueReader 是本端点消费的读口：门禁条件目录与认定两表的列表读面。
// 查阅不触发判断、决定或披露——接存储读面，不接应用编排（ADR-0077 Decision 一）。
type GateConditionCatalogueReader interface {
	ListGateConditions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.GateConditionCatalogueEntry, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ GateConditionCatalogueReader = ports.GateConditionCatalogueRead(nil)

// 业务结果单格：门禁条件册只有一本，端点不设 registry 分派（封闭集为一时参数只会
// 造出一个恒定值）。空册如实答空列表走 2xx 成格（ADR-0077 Decision 四）。
const outcomeGateConditionsListed = "GATE_CONDITIONS_LISTED"

// NewQueryGateConditionsEndpoint 交回门禁条件册查阅的 HTTP 入口
// （GET /customs-gate-conditions，票 admin-web-page-wiring-frontier/06）。
//
// 独立端点而不并进 /customs-case-registers 的分派：那个端点的三册归 customs-cases
// 一张页面（其处理器注释明写门禁两表不进该分派），本册归 customs-restrictions 页
// ——kind/registry 分派对应「一页里的页签」，各立入口对应「各自独立的页」（判据
// 同票 05 引的 /commercial-customer-contracts 先例）。
//
// 逐认定原样上列，不折门禁五值结论：折叠（FoldGateConclusion）是门禁编排的判断
// 语义，查阅面转述登记册本身。空 findings 数组是「此动作在此边界本就不受门禁」的
// 如实一格（领域折为不适用）；「目录未登记 → 未决」表现为整份目录不在 gates 里
// ——两格含义与关闭义务那对相反（0008 自注），页面文案必须各说各话。
func NewQueryGateConditionsEndpoint(
	intake CatalogueQueryIntake,
	reader GateConditionCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		entries, err := reader.ListGateConditions(
			request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]gateConditionCatalogueBody, 0, len(entries))
		for _, entry := range entries {
			findings := make([]gateFindingBody, 0, len(entry.Findings))
			for _, finding := range entry.Findings {
				findings = append(findings, gateFindingBody{
					Precondition: finding.Precondition.String(),
					State:        finding.State.String(),
				})
			}
			bodies = append(bodies, gateConditionCatalogueBody{
				Scope:        entry.Scope.String(),
				Action:       entry.Action.String(),
				Boundary:     entry.Boundary.String(),
				RegisteredAt: entry.RegisteredAt.UTC().Format(time.RFC3339Nano),
				Findings:     findings,
			})
		}
		writeJSON(response, http.StatusOK, gateConditionListResponse{
			Outcome: outcomeGateConditionsListed,
			Gates:   bodies,
		})
	})
}

type gateConditionListResponse struct {
	Outcome string                       `json:"outcome"`
	Gates   []gateConditionCatalogueBody `json:"gates"`
}

// gateConditionCatalogueBody 是一份门禁目录连同全部已登记认定。findings 空数组是
// 「此动作在此边界本就不受门禁」的如实一格；「目录未登记 → 未决」表现为整份目录
// 不在 gates 里。两格含义与关闭义务那对相反——那边空清单是「无义务项」的中性事实、
// 未登记是不可关；这边空清单是放行侧的「不受管」，未登记才无从复核（0008 自注）。
type gateConditionCatalogueBody struct {
	Scope        string            `json:"scope"`
	Action       string            `json:"action"`
	Boundary     string            `json:"boundary"`
	RegisteredAt string            `json:"registeredAt"`
	Findings     []gateFindingBody `json:"findings"`
}

// gateFindingBody 逐字段透出一项前置条件认定。state 封闭三值（MET / UNMET /
// CONFLICTING）——刻意没有「未知」格，集外取值在读口上抛，不在传输层折第四格。
type gateFindingBody struct {
	Precondition string `json:"precondition"`
	State        string `json:"state"`
}
