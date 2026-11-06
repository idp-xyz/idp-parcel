package ports

import (
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是本上下文各列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/06）：可排维与筛选维按
// 答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列在 postgres 读面里。缺省序保迁移前的次序（该票 spec「缺省序保
// 今天的次序」），决胜键一律与排序维同向（决定三）。
//
// /node-operations-records 按 registry 分派六册，registry 在六册上都登为选择器（决定四）：端点按它挑册、在解码之前拒认不出的册名，
// 取值进游标摘要——一册的游标拿到另一册去用答 400。
//
// 按实物或按交接事件计量的几册（头注里点名）是 ADR-0144 越权风险点「精确 `total`」最先撞上的一类：总数仍给精确值，超出租户级规模
// 时回该记录改成 null。

// registrySelector 是 /node-operations-records 挑册的那个查询键。
const registrySelector = "registry"

// ReceptionPages 是收寄登记册（registry=reception）的声明。
//
// 缺省序是登记时刻倒序，与迁移前同一口径；行标识是来源标识，即收寄行主键去掉租户。kind 放出作筛选维，词表只取本册列的两格
// （形成与待识别）：另几种判断走向没有收寄与控制，不在本册，筛它们只会答空。q 覆盖来源标识与收寄里的作业实物、节点、交付方。
// 本册按实物计量。
var ReceptionPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "node-operations-receptions",
	Sorts:       []cataloguepage.SortDimension{{Name: "recordedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "kind", Vocabulary: []string{RecordIntakeFormed.String(), RecordPendingIdentification.String()}},
	},
	Selectors: []string{registrySelector},
})

// UnidentifiedItemPages 是待识别实物册（registry=unidentified-item 的 items 那一半）的声明。
//
// 缺省序是登记时刻倒序，与迁移前同一口径；行标识是来源标识。identityCheck 放出作筛选维，词表取自 IdentityCheck：未核对的那几行
// 要人工重核，复核页要筛得出它们；来源保全之前的行不带核对态，按任一词筛都不在其中。q 覆盖来源标识、作业实物、节点与来源里的外部
// 标识观察。本册按实物计量。
var UnidentifiedItemPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "node-operations-unidentified-items",
	Sorts:       []cataloguepage.SortDimension{{Name: "recordedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "identityCheck", Vocabulary: cataloguepage.Words[IdentityCheck]()},
	},
	Selectors: []string{registrySelector},
})

// IdentityReviewUnitPages 是按实物列的待识别实物页（registry=identity-review-unit；registry=unidentified-item 的 units 那一半是它
// 不带条件的第一页）的声明。
//
// 本册的行是聚合：一件有过待识别那一格的作业实物一行。缺省序是它待识别各版里最近一次登记时刻（latestRecordedAt）倒序，与迁移前
// 同一口径；这一维是读面在公用表表达式里导出的列，键集落在导出列上。行标识是作业实物——聚合的分组键，册上没有别的主键。一件实物
// 此刻冲突与否、处置过没有都由它各链的当前有效版本派生，派生的不作筛选维。q 只覆盖作业实物。
//
// 导出列会前移：一件实物又登一版待识别，它的 latestRecordedAt 就换成那一版的登记时刻，在册上的位置跟着挪。倒序翻页时，还没翻到的
// 那件因此挤到已翻过的那头，这一趟可能漏掉它；已翻过的那件仍在已翻过的那头，不重出。正序翻页时反过来：已翻过的那件跳到游标之后，
// 这一趟可能再出现一次；还没翻到的那件只往后挪，不漏。
var IdentityReviewUnitPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "node-operations-identity-review-units",
	Sorts:       []cataloguepage.SortDimension{{Name: "latestRecordedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "latestRecordedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{registrySelector},
})

// ConsolidationUnitPages 是集运单元册（registry=consolidation-unit）的声明。
//
// 缺省序是单元标识正序，不是登记时刻倒序：单元行上没有业务时刻（形成时间无登记格，库面簿记时刻不是业务事实），迁移前就按标识
// 稳定排序。可排维因此只有标识这一维，它同时是行标识。phase 放出作筛选维，词表是库面三相（domain.ConsolidationPhase*）。q 覆盖
// 单元标识、载具与最近一次封签。
var ConsolidationUnitPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "node-operations-consolidation-units",
	Sorts:       []cataloguepage.SortDimension{{Name: "unitId", Kind: cataloguepage.Text}},
	DefaultSort: cataloguepage.Sort{Field: "unitId"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "phase", Vocabulary: []string{
			domain.ConsolidationPhaseOpen, domain.ConsolidationPhaseSealed, domain.ConsolidationPhaseClosed,
		}},
	},
	Selectors: []string{registrySelector},
})

// ConsolidationMemberPages 是集运成员关系册（registry=consolidation-member）的声明。
//
// 缺省序是落册时刻倒序，与迁移前同一口径；行标识是来源标识，即作业事实行主键去掉租户。action 放出作筛选维，词表只取本册列的
// 移入与移出两格。q 覆盖来源标识、单元与成员。本册按实物计量（一件实物每进出一次袋一行）。
var ConsolidationMemberPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "node-operations-consolidation-members",
	Sorts:       []cataloguepage.SortDimension{{Name: "recordedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "action", Vocabulary: []string{domain.AddMemberAction.String(), domain.RemoveMemberAction.String()}},
	},
	Selectors: []string{registrySelector},
})

// PhysicalControlTransitionPages 是节点控制转移册（registry=physical-control）的声明。
//
// 缺省序是记录时刻倒序，与迁移前同一口径；行标识是（交接对象、交接范围、交接版本、方向），即转移行主键去掉租户。outcome 放出作
// 筛选维，词表取自 domain.ControlTransitionOutcome：没推进的那几行各有各的去查处，人要筛得出它们。q 覆盖交接键三段、作业实物与
// 节点。本册按交接事件计量。
var PhysicalControlTransitionPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "node-operations-physical-control-transitions",
	Sorts:       []cataloguepage.SortDimension{{Name: "recordedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt", Descending: true},
	Identity: []cataloguepage.ValueKind{
		cataloguepage.Text, cataloguepage.Text, cataloguepage.Text, cataloguepage.Text,
	},
	Filters: []cataloguepage.FilterDimension{
		{Name: "outcome", Vocabulary: cataloguepage.Words[domain.ControlTransitionOutcome]()},
	},
	Selectors: []string{registrySelector},
})

// DeliveryPreAdvicePages 是送站预报列表（GET /node-operations-delivery-pre-advices）的声明。
//
// 缺省序是预计送达倒序，不是登记时刻倒序：迁移前就按预计送达排，列表的时间区间筛的也是这一格，人按到站的先后看预报。行标识是预报
// 标识。node、documentNumber 与预计送达区间两端是端点既有的参数，保持原义登为选择器（决定四），取值进游标摘要。撤回与否由撤回册上
// 有没有那一行派生，不作筛选维。q 覆盖预报标识、单据号、客户账户与节点。
var DeliveryPreAdvicePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "node-operations-delivery-pre-advices",
	Sorts:       []cataloguepage.SortDimension{{Name: "expectedArrivalAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "expectedArrivalAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"node", "documentNumber", "expectedArrivalFrom", "expectedArrivalUntil"},
})

// DeliveryHandoverNotePages 是送站交接单列表（GET /node-operations-delivery-handover-notes）的声明。
//
// 缺省序是开立时刻倒序，与迁移前同一口径；交接单没有另立登记时刻，开立就是它进册的时刻。行标识是交接单标识。node、documentNumber
// 与开立区间两端是端点既有的参数，登为选择器，理由同预报。关闭与范围成立都是派生（关闭册上有没有那一行、声称的账户是否全部确认），
// 不作筛选维。q 覆盖交接单标识、单据号、节点与交付方线索。
var DeliveryHandoverNotePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "node-operations-delivery-handover-notes",
	Sorts:       []cataloguepage.SortDimension{{Name: "openedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "openedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"node", "documentNumber", "openedFrom", "openedUntil"},
})
