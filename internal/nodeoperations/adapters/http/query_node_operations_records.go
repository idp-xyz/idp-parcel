package nodeopshttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ReviewCatalogueReader 是本端点消费的读口：节点作业查阅各本册子的列表读面，一册一个方法，
// 册名见下方 registry 常量（管理台 node-operations-review 页按册名取）。
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
	ListConsolidationMembers(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ConsolidationMemberCatalogueRow, error)
	ListIdentityReviewUnits(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.IdentityReviewUnitRow, error)
	ListPhysicalControlTransitions(ctx context.Context, tenant domain.TenantID, limit int) ([]ports.PhysicalControlTransitionCatalogueRow, error)
}

// 编译期锁缝：读口形状与端口保持一致——本端点不新造查询语义。
var _ ReviewCatalogueReader = ports.ReviewCatalogueRead(nil)

const (
	outcomeReceptionsListed           = "RECEPTIONS_LISTED"
	outcomeUnidentifiedItemsListed    = "UNIDENTIFIED_ITEMS_LISTED"
	outcomeConsolidationUnitsListed   = "CONSOLIDATION_UNITS_LISTED"
	outcomeConsolidationMembersListed = "CONSOLIDATION_MEMBERS_LISTED"
)

const (
	registryReception           = "reception"
	registryUnidentifiedItem    = "unidentified-item"
	registryConsolidationUnit   = "consolidation-unit"
	registryConsolidationMember = "consolidation-member"
)

// NewQueryNodeOperationsRecordsEndpoint 交回节点作业查阅页各本册子的 HTTP 入口
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
			registry != registryConsolidationUnit &&
			registry != registryConsolidationMember &&
			registry != registryPhysicalControl {
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
		case registryConsolidationMember:
			serveConsolidationMembers(response, request, reader, tenant, query.Limit)
		case registryPhysicalControl:
			servePhysicalControlTransitions(response, request, reader, tenant, query.Limit)
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
		body := receptionCatalogueBody{
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
			ServiceResultCheck:   row.ServiceResultCheck.String(),
			IdentityCheck:        row.IdentityCheck.String(),
			RecordedAt:           rfc3339(row.RecordedAt),
		}
		body.deliveryScopeCheckFields = deliveryScopeCheckFieldsOf(row.DeliveryScopeCheck)
		bodies = append(bodies, body)
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
		body := unidentifiedItemBody{
			SourceID:         row.SourceID,
			Unit:             row.Unit,
			Node:             row.Node,
			Candidates:       append([]string{}, row.Candidates...),
			IdentityConflict: row.IdentityConflict,
			ReceivedAt:       rfc3339(row.ReceivedAt),
			Association:      row.Association,
			RecordedAt:       rfc3339(row.RecordedAt),
			JudgmentVersion:  row.JudgmentVersion,
			SourcePreserved:  row.SourcePreserved,
			SucceededBy:      row.SucceededBy,
			IdentityCheck:    row.IdentityCheck.String(),
		}
		body.IdentityConflictCause = row.IdentityConflictCause.String()
		body.deliveryScopeCheckFields = deliveryScopeCheckFieldsOf(row.DeliveryScopeCheck)
		body.DeliveryScopeBasis, body.AssignedHandoverNote = deliveryScopeBasisBodyOf(row.DeliveryScopeBasis)
		if row.SourcePreserved {
			mark := row.Mark
			body.ExternalMark = &mark
		}
		bodies = append(bodies, body)
	}
	units, err := identityReviewUnitBodies(request, reader, tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	writeJSON(response, http.StatusOK, unidentifiedItemListResponse{
		Outcome: outcomeUnidentifiedItemsListed,
		Items:   bodies,
		Units:   units,
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
//
// serviceResultCheck 与收寄写侧答复 receptionResponse 同名同值：CHECKED / UNREADABLE，取
// ports.ServiceResultCheck 的名字，传输层不另起词——设备与管理台读到的是同一个词。只有 INTAKE_FORMED 行带，
// 待识别行整键不出现：照本册 controlReleased* 与待识别册 association 的惯例，「这一格不适用」用缺席表达，不用
// 空串或 null 兼表。封闭词原样透出不折成布尔，理由同 kind。它是信息格不是业务判别：UNREADABLE 那行照样是
// INTAKE_FORMED，这一格告诉看的人那行 serviceMarkers 的 [] 不代表没有取消或终局；少了它，「已查、无标记」与
// 「读不动」两行逐字相同（ADR-0186）。
//
// identityCheck 与收寄写侧答复同名同值：CHECKED / UNCHECKED，取 ports.IdentityCheck 的名字，只有待识别行带、其余整键不出现，
// 缺席惯例同 serviceResultCheck。它让未核对的待识别与查无此号的那一行在册面上分得开（ADR-0199 决定五）。
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
	ServiceResultCheck   string   `json:"serviceResultCheck,omitempty"`
	IdentityCheck        string   `json:"identityCheck,omitempty"`
	RecordedAt           string   `json:"recordedAt"`
	// 交付范围核对态各键与收寄答复同名同值（deliveryScopeCheckFields；ADR-0212 决定五「收寄答复与复核目录都带它」），这一版没记时整键缺席。
	deliveryScopeCheckFields
}

// units 是同一页按实物列的那一半（票 no-reception-identity/06）：items 逐行列待识别，分不出哪一版仍是当前有效版本、冲突处置过没有；
// units 把一件实物各条链的当前有效版本列在一起，处置版带处置文档。两者同一次查询答出，页面不必为处置动作另发一次请求。
type unidentifiedItemListResponse struct {
	Outcome string                   `json:"outcome"`
	Items   []unidentifiedItemBody   `json:"items"`
	Units   []identityReviewUnitBody `json:"units"`
}

// unidentifiedItemBody 逐字段透出一行待识别实物登记。
//
// candidates 与 identityConflict 照登记转写；association 是登记时已有的正式包裹
// 关联引用，缺席即整键不出现——身份确认由 parcel-shipment 以版本化关联形成，识别
// 成功不回写本行（识别不删除原实物记录），所以缺席说的是「登记那一刻还没有」，
// 不是「至今没有」。unit 是节点签发的作业实物标识，即页面「内部作业标签」列。
//
// judgmentVersion 是人工重核指名用的那一版（ADR-0199 决定三）。sourcePreserved 恒在场：来源保全之前的行答 false，
// externalMark 随之整键不出现——那一格号码从未留存，空串会被读成「设备没报号」；来源在时 externalMark 照来源文档原样带号，
// 空号也带空串。succeededBy 只在这一版已被接续时出现，带直接接续它的那一版。identityCheck 同收寄册那一格：UNCHECKED 那一行的
// candidates 是 []，说的是没问到，不是查无此号（ADR-0199 决定五）。identityConflictCause 只在这一行记了冲突来由时出现，取
// ports.IdentityConflictCause 的原名（ADR-0211 决定一）：AMBIGUITY_ONLY 那一行在声明消失之后可再重核，其余与缺席的只经处置解开。
type unidentifiedItemBody struct {
	SourceID         string   `json:"sourceId"`
	Unit             string   `json:"unit"`
	Node             string   `json:"node"`
	Candidates       []string `json:"candidates"`
	IdentityConflict bool     `json:"identityConflict"`
	ReceivedAt       string   `json:"receivedAt"`
	Association      string   `json:"association,omitempty"`
	RecordedAt       string   `json:"recordedAt"`
	JudgmentVersion  string   `json:"judgmentVersion,omitempty"`
	SourcePreserved  bool     `json:"sourcePreserved"`
	ExternalMark     *string  `json:"externalMark,omitempty"`
	SucceededBy      string   `json:"succeededBy,omitempty"`
	IdentityCheck    string   `json:"identityCheck,omitempty"`
	// identityConflictCause 的读法见类型头注。
	IdentityConflictCause string `json:"identityConflictCause,omitempty"`
	// 交付范围核对态各键同收寄册那一格；deliveryScopeBasis 是这一版来源里上报的依据引用，与收寄载荷同一个 {kind, id} 形，来源没带即
	// 整键缺席；assignedHandoverNote 是这件实物此刻最后归入的交接单，没归入过即整键缺席。两样分开给的理由在 ports 那一格的注释。
	deliveryScopeCheckFields
	DeliveryScopeBasis   *deliveryScopeBasisDocument `json:"deliveryScopeBasis,omitempty"`
	AssignedHandoverNote string                      `json:"assignedHandoverNote,omitempty"`
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

func serveConsolidationMembers(
	response http.ResponseWriter,
	request *http.Request,
	reader ReviewCatalogueReader,
	tenant domain.TenantID,
	limit int,
) {
	rows, err := reader.ListConsolidationMembers(request.Context(), tenant, limit)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	bodies := make([]consolidationMemberBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, consolidationMemberBody{
			SourceID:         row.SourceID,
			Action:           row.Action,
			UnitID:           row.UnitID,
			Member:           row.Member,
			PerformedBy:      row.PerformedBy,
			Evidence:         row.Evidence,
			OccurredAt:       rfc3339(row.OccurredAt),
			RecordedAt:       rfc3339(row.RecordedAt),
			DirectionalBasis: directionalBasisBodyOf(row.DirectionalBasis),
		})
	}
	writeJSON(response, http.StatusOK, consolidationMemberListResponse{
		Outcome: outcomeConsolidationMembersListed,
		Members: bodies,
	})
}

type consolidationMemberListResponse struct {
	Outcome string                    `json:"outcome"`
	Members []consolidationMemberBody `json:"members"`
}

// consolidationMemberBody 逐字段透出一次移入或移出的来源事实。sourceId 原样带 FTI/ 标记：导入事实与设备事实靠它
// 分（ADR-0089 决定四）。occurredAt 是现场的业务时间，recordedAt 是落册时刻，两者都给。
//
// directionalBasis 是移入事实落册时同一笔记下的方向依据记录（票 no-directional-basis/01）。它为 null 有两种读法，
// 按 action 分：移出行本来就不带；移入行为 null 读作`未记录`——那条事实落册时还不记方向依据——**不读作依据齐全**。
// 记录说的是 judgedAt 那一刻所知，不是装袋那一刻；偏离的判断归 network-routing、异常归 visibility-exception，
// 本读面只上列，不带任何处置、确认或解除动作。
type consolidationMemberBody struct {
	SourceID         string                `json:"sourceId"`
	Action           string                `json:"action"`
	UnitID           string                `json:"unitId"`
	Member           string                `json:"member"`
	PerformedBy      string                `json:"performedBy"`
	Evidence         string                `json:"evidence"`
	OccurredAt       string                `json:"occurredAt"`
	RecordedAt       string                `json:"recordedAt"`
	DirectionalBasis *directionalBasisBody `json:"directionalBasis"`
}

// directionalBasisBody 三格同一个形：answer 是封闭词 ESTABLISHED / MISSING / NOT_CONFIGURED / UNREADABLE /
// NOT_JUDGED，reasons 只在 MISSING 时非空；引用随格——身份成立带所依收寄，服务结果缺少带取消决定或终局。
type directionalBasisBody struct {
	JudgedAt      string                   `json:"judgedAt"`
	Identity      directionalBasisCellBody `json:"identity"`
	ServiceResult directionalBasisCellBody `json:"serviceResult"`
	Routing       directionalBasisCellBody `json:"routing"`
}

type directionalBasisCellBody struct {
	Answer         string   `json:"answer"`
	Reasons        []string `json:"reasons"`
	Association    string   `json:"association,omitempty"`
	IntakeSourceID string   `json:"intakeSourceId,omitempty"`
	IntakeVersion  string   `json:"intakeVersion,omitempty"`
	Cancellation   string   `json:"cancellation,omitempty"`
	FinalKind      string   `json:"finalKind,omitempty"`
	FinalVersion   string   `json:"finalVersion,omitempty"`
}

func directionalBasisBodyOf(record domain.DirectionalBasisRecord) *directionalBasisBody {
	if !record.Recorded() {
		return nil
	}
	identity := record.Identity()
	serviceResult := record.ServiceResult()
	body := &directionalBasisBody{
		JudgedAt:      rfc3339(record.JudgedAt()),
		Identity:      directionalBasisCellBodyOf(identity.Cell()),
		ServiceResult: directionalBasisCellBodyOf(serviceResult.Cell()),
		Routing:       directionalBasisCellBodyOf(record.Routing().Cell()),
	}
	if association, established := identity.Association(); established {
		body.Identity.Association = association.String()
		body.Identity.IntakeSourceID = identity.IntakeSourceID()
		body.Identity.IntakeVersion = identity.IntakeVersion().String()
	}
	body.ServiceResult.Cancellation = serviceResult.Cancellation()
	body.ServiceResult.FinalKind, body.ServiceResult.FinalVersion = serviceResult.Final()
	return body
}

func directionalBasisCellBodyOf(cell domain.DirectionalBasisCell) directionalBasisCellBody {
	body := directionalBasisCellBody{Answer: cell.Answer().String(), Reasons: []string{}}
	for _, reason := range cell.Reasons() {
		body.Reasons = append(body.Reasons, reason.String())
	}
	return body
}
