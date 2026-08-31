package settlementhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// OperatingCatalogueReader 是本端点消费的读口：经营结果快照册与成本分摊册两本册子
// 的列表读面。
type OperatingCatalogueReader interface {
	ListOperatingResults(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.OperatingResultCatalogueRow, error)
	ListCostAllocations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.CostAllocationCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ OperatingCatalogueReader = ports.OperatingCatalogueRead(nil)

const (
	outcomeOperatingResultsListed = "OPERATING_RESULTS_LISTED"
	outcomeCostAllocationsListed  = "COST_ALLOCATIONS_LISTED"
)

const (
	registryOperatingResult = "operating-result"
	registryCostAllocation  = "cost-allocation"
)

// NewQuerySettlementOperatingResultsEndpoint 交回经营核算页两本册子的 HTTP 入口
// （GET /settlement-operating-results，票 admin-skeleton-closure-batch/04）。
//
// 两册同属 UC-SA-006（分摊与指标由同一个用例形成、经同一个 OperatingIntent 交下游），
// 因此同页上列；分摊只改变经营归因、不转移原债权债务责任，故它不出现在费用页。
//
// **本端点不下推口径参数，也不代算任何指标**：毛利只能派生不能直接修改，派生它的门
// 在写口（重建时复验组成与毛利是否相符）；读口再算一遍就成了第二处定义，且两处一旦
// 不一致，页面上看到的会是读口那个没人验过的数。空册就是空册——经营口径的数字造不得
// （见 spec 事实基线）。
func NewQuerySettlementOperatingResultsEndpoint(
	intake CatalogueQueryIntake,
	reader OperatingCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		registry := request.URL.Query().Get("registry")
		if registry != registryOperatingResult && registry != registryCostAllocation {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registryOperatingResult:
			serveOperatingResults(response, request, reader, tenant, query.Limit)
		case registryCostAllocation:
			serveCostAllocations(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveOperatingResults(
	response http.ResponseWriter,
	request *http.Request,
	reader OperatingCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListOperatingResults(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]operatingResultBody, 0, len(rows))
	for _, row := range rows {
		components := make([]operatingComponentBody, 0, len(row.Components))
		for _, component := range row.Components {
			components = append(components, operatingComponentBody{
				Source: component.Source,
				Effect: component.Effect,
				Amount: minorAmount(component.AmountMinor),
			})
		}
		bodies = append(bodies, operatingResultBody{
			Scope:      row.Scope,
			Period:     row.Period,
			Basis:      row.Basis,
			Currency:   row.Currency,
			Margin:     minorAmount(row.MarginMinor),
			Version:    row.Version,
			AsOf:       rfc3339(row.AsOf),
			Corrects:   row.Corrects,
			RecordedAt: rfc3339(row.RecordedAt),
			Components: components,
		})
	}
	writeJSON(response, http.StatusOK, operatingResultListResponse{
		Outcome: outcomeOperatingResultsListed,
		Results: bodies,
	})
}

func serveCostAllocations(
	response http.ResponseWriter,
	request *http.Request,
	reader OperatingCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListCostAllocations(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]costAllocationBody, 0, len(rows))
	for _, row := range rows {
		portions := make([]allocationPortionBody, 0, len(row.Portions))
		for _, portion := range row.Portions {
			portions = append(portions, allocationPortionBody{
				Target: portion.Target,
				Amount: minorAmount(portion.AmountMinor),
			})
		}
		bodies = append(bodies, costAllocationBody{
			Allocation:        row.Allocation,
			Source:            row.Source,
			SourceAmount:      minorAmount(row.SourceMinor),
			Currency:          row.Currency,
			Rule:              row.Rule,
			UnallocatedAmount: minorAmount(row.UnallocatedMinor),
			Version:           row.Version,
			AllocatedAt:       rfc3339(row.AllocatedAt),
			Corrects:          row.Corrects,
			RecordedAt:        rfc3339(row.RecordedAt),
			Portions:          portions,
		})
	}
	writeJSON(response, http.StatusOK, costAllocationListResponse{
		Outcome:     outcomeCostAllocationsListed,
		Allocations: bodies,
	})
}

type operatingResultListResponse struct {
	Outcome string                `json:"outcome"`
	Results []operatingResultBody `json:"results"`
}

// operatingResultBody 逐字段透出一份经营结果快照：口径三件（分析范围、账期、基准）、
// 币种、组成、毛利、计算版本与截至时点。
//
// margin 照库上那一列透出，不重算（理由见构造函数）。负毛利即经营损失，不另立一键
// ——同一个数按正负分两键会让「零」落进两键都不占的缝里。
type operatingResultBody struct {
	Scope      string                   `json:"scope"`
	Period     string                   `json:"period"`
	Basis      string                   `json:"basis"`
	Currency   string                   `json:"currency"`
	Margin     string                   `json:"margin"`
	Version    string                   `json:"version"`
	AsOf       string                   `json:"asOf"`
	Corrects   string                   `json:"corrects,omitempty"`
	RecordedAt string                   `json:"recordedAt"`
	Components []operatingComponentBody `json:"components"`
}

// operatingComponentBody 逐字段透出一个组成项：来源金额身份、对指标的封闭二向
// （INCREASES / DECREASES）与金额。
//
// 组成逐项透出而不只给毛利：CONTEXT 硬要求审核应付与供应商费用贷项按各自借贷方向
// 分别计入一次，「当前有效审核应付」不得被解释为已经静默净含贷项——净额把这条要求
// 抹掉之后，页面上再也看不出它有没有被遵守。
type operatingComponentBody struct {
	Source string `json:"source"`
	Effect string `json:"effect"`
	Amount string `json:"amount"`
}

type costAllocationListResponse struct {
	Outcome     string               `json:"outcome"`
	Allocations []costAllocationBody `json:"allocations"`
}

// costAllocationBody 逐字段透出一次成本分摊。
//
// unallocatedAmount 照实透出：已分摊与未分摊之和严格等于来源金额是写口的不变量，
// 未分摊余额是第一类结果不是尾差——没有合格对象或分母为零时来源金额整笔留在这里等
// 新依据，本端点把它显式摆出来，不折进份额里凑平。portions 为空数组即「全额未分摊」。
type costAllocationBody struct {
	Allocation        string                  `json:"allocation"`
	Source            string                  `json:"source"`
	SourceAmount      string                  `json:"sourceAmount"`
	Currency          string                  `json:"currency"`
	Rule              string                  `json:"rule"`
	UnallocatedAmount string                  `json:"unallocatedAmount"`
	Version           string                  `json:"version"`
	AllocatedAt       string                  `json:"allocatedAt"`
	Corrects          string                  `json:"corrects,omitempty"`
	RecordedAt        string                  `json:"recordedAt"`
	Portions          []allocationPortionBody `json:"portions"`
}

// allocationPortionBody 逐字段透出归因到某个分析对象的一份份额。
type allocationPortionBody struct {
	Target string `json:"target"`
	Amount string `json:"amount"`
}
