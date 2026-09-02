package nodeopshttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ReviewCatalogueReader 是本端点消费的读口：收寄册、待识别实物册与集运单元册三本
// 册子的列表读面（管理台 node-operations-review 页）。
type ReviewCatalogueReader interface {
	ListReceptions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ReceptionCatalogueRow, error)
	ListUnidentifiedItems(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.UnidentifiedItemCatalogueRow, error)
	ListConsolidationUnits(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ConsolidationUnitCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ ReviewCatalogueReader = ports.ReviewCatalogueRead(nil)

const (
	outcomeReceptionsListed         = "RECEPTIONS_LISTED"
	outcomeUnidentifiedItemsListed  = "UNIDENTIFIED_ITEMS_LISTED"
	outcomeConsolidationUnitsListed = "CONSOLIDATION_UNITS_LISTED"
)

const (
	registryReception         = "reception"
	registryUnidentifiedItem  = "unidentified-item"
	registryConsolidationUnit = "consolidation-unit"
)

// NewQueryNodeOperationsRecordsEndpoint 交回节点作业查阅页三本册子的 HTTP 入口
// （GET /node-operations-records，票 admin-skeleton-closure-batch/05）。
//
// 本页是治理查阅面，不是一线作业端（ADR-0021）：实时现场作业走设备渠道的命令端点，
// 本端点零登记零编辑动作，只上列已登记的作业事实。页面五区里实际测量与节点侧交接
// 证据两区在存储上还没有登记册，本端点的册名封闭集刻意没有那两格——没有表就没有
// 读法，答一份恒空的册子会把「无处可登」演成「登记册为空」（票 05 Comments 记明）。
//
// **不下推身份裁决或处置参数**：待识别实物的身份确认由 parcel-shipment 以版本化
// 关联形成，查阅面收下「确认身份」参数就等于让目录读口长出第二种「处置」语义。
func NewQueryNodeOperationsRecordsEndpoint(
	intake CatalogueQueryIntake,
	reader ReviewCatalogueReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		registry := request.URL.Query().Get("registry")
		if registry != registryReception &&
			registry != registryUnidentifiedItem &&
			registry != registryConsolidationUnit {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		tenant := query.Scope.Tenant()

		switch registry {
		case registryReception:
			serveReceptions(response, request, reader, tenant, query.Limit)
		case registryUnidentifiedItem:
			serveUnidentifiedItems(response, request, reader, tenant, query.Limit)
		case registryConsolidationUnit:
			serveConsolidationUnits(response, request, reader, tenant, query.Limit)
		}
	})
}

func serveReceptions(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListReceptions(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]receptionCatalogueBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, receptionCatalogueBody{
			SourceID:             row.SourceID,
			Kind:                 row.Kind,
			Unit:                 row.Unit,
			Node:                 row.Node,
			DeliveredBy:          row.DeliveredBy,
			ReceivedAt:           rfc3339(row.ReceivedAt),
			ControlKind:          row.ControlKind,
			ControlEstablishedAt: rfc3339(row.ControlEstablishedAt),
			ControlReleasedBy:    row.ControlReleasedBy,
			ControlReleasedAt:    optionalInstant(row.ControlReleasedAt),
			ServiceMarkers:       append([]string{}, row.ServiceMarkers...),
			RecordedAt:           rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, receptionCatalogueListResponse{
		Outcome:    outcomeReceptionsListed,
		Receptions: bodies,
	})
}

func serveUnidentifiedItems(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListUnidentifiedItems(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]unidentifiedItemBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, unidentifiedItemBody{
			SourceID:         row.SourceID,
			Unit:             row.Unit,
			Node:             row.Node,
			Candidates:       append([]string{}, row.Candidates...),
			IdentityConflict: row.IdentityConflict,
			ReceivedAt:       rfc3339(row.ReceivedAt),
			Association:      row.Association,
			RecordedAt:       rfc3339(row.RecordedAt),
		})
	}
	writeJSON(response, http.StatusOK, unidentifiedItemListResponse{
		Outcome: outcomeUnidentifiedItemsListed,
		Items:   bodies,
	})
}

func serveConsolidationUnits(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListConsolidationUnits(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]consolidationUnitBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, consolidationUnitBody{
			UnitID:                row.UnitID,
			Asset:                 row.Asset,
			Phase:                 row.Phase,
			MemberCount:           row.MemberCount,
			SealCount:             row.SealCount,
			OpenedSourceID:        row.OpenedSourceID,
			OpenedBy:              row.OpenedBy,
			LatestSeal:            row.LatestSeal,
			LatestSealSourceID:    row.LatestSealSourceID,
			LatestSealPerformedBy: row.LatestSealPerformedBy,
			LatestSealedAt:        optionalInstant(row.LatestSealedAt),
			ClosedAt:              optionalInstant(row.ClosedAt),
		})
	}
	writeJSON(response, http.StatusOK, consolidationUnitListResponse{
		Outcome: outcomeConsolidationUnitsListed,
		Units:   bodies,
	})
}

type receptionCatalogueListResponse struct {
	Outcome    string                   `json:"outcome"`
	Receptions []receptionCatalogueBody `json:"receptions"`
}

// receptionCatalogueBody 逐字段透出一行收寄登记。
//
// kind 是收寄判断的封闭词（INTAKE_FORMED / PENDING_IDENTIFICATION）：本册只列带
// 收寄与控制在场的两格，词原样透出不折成布尔——「已形成」与「待识别」都是收寄，
// 折成一个「是否收寄」会把身份未定这层真话抹掉。
//
// control 三件照登记转写：controlKind 是控制建立来源的封闭词（NODE_INTAKE /
// HANDOVER_IN）；released 两件成对缺席表示实物仍在节点控制中——转出只能由权威交接
// 引用建立（domain.PhysicalControl），本读面不代判「在库/出库」一类派生状态词。
type receptionCatalogueBody struct {
	SourceID             string   `json:"sourceId"`
	Kind                 string   `json:"kind"`
	Unit                 string   `json:"unit"`
	Node                 string   `json:"node"`
	DeliveredBy          string   `json:"deliveredBy"`
	ReceivedAt           string   `json:"receivedAt"`
	ControlKind          string   `json:"controlKind"`
	ControlEstablishedAt string   `json:"controlEstablishedAt"`
	ControlReleasedBy    string   `json:"controlReleasedBy,omitempty"`
	ControlReleasedAt    string   `json:"controlReleasedAt,omitempty"`
	ServiceMarkers       []string `json:"serviceMarkers"`
	RecordedAt           string   `json:"recordedAt"`
}

type unidentifiedItemListResponse struct {
	Outcome string                 `json:"outcome"`
	Items   []unidentifiedItemBody `json:"items"`
}

// unidentifiedItemBody 逐字段透出一行待识别实物登记。
//
// candidates 与 identityConflict 照登记转写；association 是登记时已有的正式包裹
// 关联引用，缺席即整键不出现——身份确认由 parcel-shipment 以版本化关联形成，识别
// 成功不回写本行（识别不删除原实物记录），所以缺席说的是「登记那一刻还没有」，
// 不是「至今没有」。unit 是节点签发的作业实物标识，即页面「内部作业标签」列。
type unidentifiedItemBody struct {
	SourceID         string   `json:"sourceId"`
	Unit             string   `json:"unit"`
	Node             string   `json:"node"`
	Candidates       []string `json:"candidates"`
	IdentityConflict bool     `json:"identityConflict"`
	ReceivedAt       string   `json:"receivedAt"`
	Association      string   `json:"association,omitempty"`
	RecordedAt       string   `json:"recordedAt"`
}

type consolidationUnitListResponse struct {
	Outcome string                  `json:"outcome"`
	Units   []consolidationUnitBody `json:"units"`
}

// consolidationUnitBody 逐字段透出一个集运单元实例。
//
// phase 是三相封闭词（OPEN / SEALED / CLOSED）。memberCount 是当前成员**数**不是
// 成员清单：成员逐件属写模型与容纳索引，列面只说多少。latestSeal 两件取最近一次
// 封装快照，尚未封装过则成对缺席；sealCount 把「从未封装」与「重新封装过」分开。
// 没有形成时间键——单元行上只有库面簿记时刻，不是业务事实，代填会把簿记演成作业。
//
// openedSourceId 与 openedBy 是 UC-NO-003 结果契约要求保存的`来源`与`执行方`，取自
// 开启那一次作业，恒在场（库面该列 NOT NULL）。latestSealSourceId 与
// latestSealPerformedBy 是最近一次封装的同两件，与 latestSeal 同缺同在。
//
// 来源身份透出而不只透执行方，是因为**分辨导入进来的事实与设备扫描的事实靠的是它**：
// 执行方两条路上可以是同一个人，来源身份不会。少了它，一线过渡期导入的封签在页面上与
// 现场扫描的封签长得一模一样。
//
// 移入、移出、开封与关闭各自的执行方不在本体：一个单元会有许多次那样的作业，列面每格
// 只放得下一个值。要逐次看得读来源事实登记，那是另一本册子。
type consolidationUnitBody struct {
	UnitID                string `json:"unitId"`
	Asset                 string `json:"asset"`
	Phase                 string `json:"phase"`
	MemberCount           int64  `json:"memberCount"`
	SealCount             int64  `json:"sealCount"`
	OpenedSourceID        string `json:"openedSourceId"`
	OpenedBy              string `json:"openedBy"`
	LatestSeal            string `json:"latestSeal,omitempty"`
	LatestSealSourceID    string `json:"latestSealSourceId,omitempty"`
	LatestSealPerformedBy string `json:"latestSealPerformedBy,omitempty"`
	LatestSealedAt        string `json:"latestSealedAt,omitempty"`
	ClosedAt              string `json:"closedAt,omitempty"`
}
