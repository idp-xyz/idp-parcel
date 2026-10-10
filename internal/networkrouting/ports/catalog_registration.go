package ports

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// 本文件是版本化网络目录（ADR-0068）的登记边界：七类定义原语的登记行形状与写入口。
// 行类型自持久化适配器上移到端口层，是因为登记用例（应用层）要以它们表达受理门，
// 而应用层不得依赖适配器。读侧端口在 catalog_read.go：选版读口供证据视图的目录
// 折叠（ADR-0148 决定六部分停用 ADR-0068 决定六之后，证据视图读本目录），运营查阅
// 上列按族列版本行原文、不选版不折叠。

// CatalogTargetKind 是日历与可用性调整的适用对象类别，封闭三类（CONTEXT：针对节点、
// 网络连接或线路）。
type CatalogTargetKind uint8

const (
	CatalogTargetKindInvalid CatalogTargetKind = iota
	TargetNode
	TargetConnection
	TargetLine
)

func (kind CatalogTargetKind) String() string {
	switch kind {
	case TargetNode:
		return "NODE"
	case TargetConnection:
		return "CONNECTION"
	case TargetLine:
		return "LINE"
	default:
		return ""
	}
}

// CatalogTargetKindFrom 逐格译回封闭三类。库上 CHECK 守着集合，default 兜两头：读侧
// 是「CHECK 被后续迁移放宽而 Go 侧没跟上」——那时报错，不吸收成某一格；写侧是登记方
// 给了集合外的词——由登记口在入库前拒绝。
func CatalogTargetKindFrom(raw string) (CatalogTargetKind, error) {
	switch raw {
	case "NODE":
		return TargetNode, nil
	case "CONNECTION":
		return TargetConnection, nil
	case "LINE":
		return TargetLine, nil
	default:
		return CatalogTargetKindInvalid, fmt.Errorf("未知适用对象类别 %q", raw)
	}
}

// AvailabilityAdjustmentKind 是调整种类，封闭四格（CONTEXT：临时停运、关闭、恢复或
// 适用范围调整）。
type AvailabilityAdjustmentKind uint8

const (
	AvailabilityAdjustmentKindInvalid AvailabilityAdjustmentKind = iota
	AdjustmentSuspension
	AdjustmentClosure
	AdjustmentResumption
	AdjustmentScopeAdjustment
)

func (kind AvailabilityAdjustmentKind) String() string {
	switch kind {
	case AdjustmentSuspension:
		return "SUSPENSION"
	case AdjustmentClosure:
		return "CLOSURE"
	case AdjustmentResumption:
		return "RESUMPTION"
	case AdjustmentScopeAdjustment:
		return "SCOPE_ADJUSTMENT"
	default:
		return ""
	}
}

// AvailabilityAdjustmentKindFrom 与 CatalogTargetKindFrom 同一条纪律。
func AvailabilityAdjustmentKindFrom(raw string) (AvailabilityAdjustmentKind, error) {
	switch raw {
	case "SUSPENSION":
		return AdjustmentSuspension, nil
	case "CLOSURE":
		return AdjustmentClosure, nil
	case "RESUMPTION":
		return AdjustmentResumption, nil
	case "SCOPE_ADJUSTMENT":
		return AdjustmentScopeAdjustment, nil
	default:
		return AvailabilityAdjustmentKindInvalid, fmt.Errorf("未知调整种类 %q", raw)
	}
}

// NodeDefinitionVersion 是一个物流节点在某时点的适用版本行。
//
// HasEffectiveTo 为假即未闭区间（当前版本）。用显式布尔而不是零值判断：零时刻是一个
// 合法的绝对时刻，拿它兼作「没有终点」会让补历史的区间登不进来。
//
// 稳定定义各族（临时调整族除外，它只记来源）的版本行各带一格 Basis：这一版的登记依据
// （domain.CatalogBasisReference）。选版读口 LoadDefinitionsAt 不取它——折叠证据用不到依据，读它的是
// 运营查阅上列。
type NodeDefinitionVersion struct {
	Code             string
	Version          int32
	BusinessTimezone string
	EffectiveFrom    time.Time
	EffectiveTo      time.Time
	HasEffectiveTo   bool
	Basis            domain.CatalogBasisReference
}

// ConnectionDefinitionVersion 是一条有向网络连接的适用版本行。
type ConnectionDefinitionVersion struct {
	Code             string
	Version          int32
	FromNode         string
	ToNode           string
	BusinessTimezone string
	EffectiveFrom    time.Time
	EffectiveTo      time.Time
	HasEffectiveTo   bool
	Basis            domain.CatalogBasisReference
}

// LineDefinitionVersion 是一条线路的适用版本行；Segments 是连接身份的有序数组。逐段成本依据是另一张表上
// 的行，随登记命令到达（RegisterLineVersionCommand.CostBases），不在本行类型里。
type LineDefinitionVersion struct {
	Code             string
	Version          int32
	Segments         []string
	BusinessTimezone string
	ApplicableScope  string
	EffectiveFrom    time.Time
	EffectiveTo      time.Time
	HasEffectiveTo   bool
	Basis            domain.CatalogBasisReference
}

// ServiceAreaDefinitionVersion 是一个服务区域的适用版本行：版本、有效区间与覆盖。覆盖文法首版两种形态
// （整个国家 / 地区，或国家 / 地区加一组邮编前缀），形态归产品、取值归租户（ADR-0148 决定二）。
type ServiceAreaDefinitionVersion struct {
	Code           string
	Version        int32
	EffectiveFrom  time.Time
	EffectiveTo    time.Time
	HasEffectiveTo bool
	// 覆盖与节点角色（ADR-0148 决定二、五）。HasCoverage 为假即这版没登覆盖——本格落地之前的存量版本，或只登了
	// 身份与有效期的版本；折叠时它不解析任何地址。PostalPrefixes 为空即整国家 / 地区覆盖；两组节点是这版区域
	// 用来收寄（始发）与交付（尾程注入）的节点身份。
	HasCoverage      bool
	CoverageCountry  string
	PostalPrefixes   []string
	OriginNodes      []string
	DestinationNodes []string
	Basis            domain.CatalogBasisReference
}

// ServiceCalendarDefinitionVersion 是某适用对象的服务日历适用版本行。
// 截单、处理时长与衔接缓冲都是这一版的内容（ADR-0175）。指针为空是没登记这一格，与登记了 0 不同。
type ServiceCalendarDefinitionVersion struct {
	TargetKind        CatalogTargetKind
	TargetCode        string
	Version           int32
	EffectiveFrom     time.Time
	EffectiveTo       time.Time
	HasEffectiveTo    bool
	CutoffLocalMinute *int
	ProcessingMinutes *int
	BufferMinutes     *int
	Basis             domain.CatalogBasisReference
}

// AvailabilityAdjustmentStatement 是一条临时调整陈述——历史链上的一个版本行
// （形成、变化或解除各成一行）。LoadDefinitionsAt 交回其中的**当前陈述**（历史链
// 最大版本且生效窗口覆盖 asOf）；登记口与运营查阅上列经手的是任意版本行。
type AvailabilityAdjustmentStatement struct {
	Code        string
	Version     int32
	TargetKind  CatalogTargetKind
	TargetCode  string
	Kind        AvailabilityAdjustmentKind
	Source      string
	EffectiveAt time.Time
	LiftedAt    time.Time
	HasLiftedAt bool
}

// RouteStrategyDefinitionVersion 是一个路由策略的适用版本行。RankingForm 是这一版声明的内置
// 排序形态（ADR-0146），零值即这一版没有声明。冻结形态与剩余段数限额同一版声明，缺一格即未声明。
// 比较币种与所引价格政策（ADR-0148 决定四）是租户取值，缺一格即未声明——没登比较币种时取数侧
// 只在同币种下合成，异币种进未决格，不写死任何默认币种。
type RouteStrategyDefinitionVersion struct {
	Code            string
	Version         int32
	ApplicableScope string
	RankingForm     domain.RankingForm
	// FreezeForm 零值且限额为 nil 即这一版没有声明冻结形态。
	FreezeForm                  domain.FreezeForm
	FreezeRemainingSegmentLimit *int
	// AutoRerouteForm 零值或阈值为 nil 即这一版没有声明自动改路。阈值是租户取值。
	AutoRerouteForm                      domain.AutoRerouteForm
	AutoRerouteImprovementThresholdMinor *int
	// HasComparisonCurrency / HasComparisonPricePolicy 为假即这一版没登这两格（不是空串）。
	HasComparisonCurrency    bool
	ComparisonCurrency       string
	HasComparisonPricePolicy bool
	ComparisonPricePolicy    string
	EffectiveFrom            time.Time
	EffectiveTo              time.Time
	HasEffectiveTo           bool
	Basis                    domain.CatalogBasisReference
}

// LineCostBasisKind 是线路段成本依据的封闭两类（票 routing-first-cut/10，ADR-0148 决定四）：
// 外包段引 parcel-pricing 的 BUY 价卡方案，自营段引 party-commercial 的内部价格政策版本。
type LineCostBasisKind uint8

const (
	LineCostBasisKindInvalid LineCostBasisKind = iota
	SupplierBuyPlanBasis
	InternalPolicyBasis
)

func (kind LineCostBasisKind) String() string {
	switch kind {
	case SupplierBuyPlanBasis:
		return "SUPPLIER_BUY_PLAN"
	case InternalPolicyBasis:
		return "INTERNAL_POLICY"
	default:
		return ""
	}
}

// LineCostBasisKindFrom 逐格译回封闭两类，default 兜两头：读侧是「CHECK 被后续迁移放宽而 Go 侧
// 没跟上」——那时报错；写侧是登记方给了集合外的词——由登记口在入库前拒绝。
func LineCostBasisKindFrom(raw string) (LineCostBasisKind, error) {
	switch raw {
	case "SUPPLIER_BUY_PLAN":
		return SupplierBuyPlanBasis, nil
	case "INTERNAL_POLICY":
		return InternalPolicyBasis, nil
	default:
		return LineCostBasisKindInvalid, fmt.Errorf("未知线路段成本依据种类 %q", raw)
	}
}

// LineSegmentCostBasis 是线路段链上一个位置的候选成本依据引用。SegmentIndex 对段链数组下标
// （序即语义，与 LineDefinitionVersion.Segments 同一数组）；Reference 是引用串——外包段是方案
// 「id/version」，自营段是政策版本「对象身份/版本标签」，正文分别归被引上下文，这里只持引用。
type LineSegmentCostBasis struct {
	SegmentIndex int
	Kind         LineCostBasisKind
	Reference    string
}

// NetworkCatalogRegistry 是版本化网络目录七类定义原语的写入口（ADR-0068）。
//
// 目录**内容**属实例半边（PAR-NET-01..15 待提供），本端口只建门：不带任何默认定义，
// 缺件时不替登记方补值——补一个占位值会把「还没人登记」变成一次有依据的判断。
//
// 所有方法都在调用方的事务内执行（RequireExecutor 语义）：版本行与目录修订必须同一
// 事务推进（ADR-0068 Decision 五），环境事务由进程级登记口给出。
//
// 写入结果只有成功与错误两格，没有 VE 目录写入口那样的出格答案，三道防线各有归属
// （均定于 ADR-0068，本端口不重判）：同身份重复版本号由主键挡，未闭区间的并存由
// 部分唯一索引挡，已闭区间的重叠由读口以 ErrAmbiguousNetworkCatalog 兜。
type NetworkCatalogRegistry interface {
	RegisterNodeVersion(
		ctx context.Context,
		tenant domain.TenantID,
		row NodeDefinitionVersion,
	) error
	RegisterConnectionVersion(
		ctx context.Context,
		tenant domain.TenantID,
		row ConnectionDefinitionVersion,
	) error
	RegisterLineVersion(
		ctx context.Context,
		tenant domain.TenantID,
		row LineDefinitionVersion,
	) error
	// RegisterLineCostBases 追加一条线路版本的逐段成本依据（票 routing-first-cut/10）。它是线路
	// 版本的内容半，只伴随既有版本行登记：目录修订仍由版本行负责推进，这里只保证依据行与
	// 版本行同一事务（ADR-0068 Decision 五）。
	RegisterLineCostBases(
		ctx context.Context,
		tenant domain.TenantID,
		lineCode string,
		version int32,
		bases []LineSegmentCostBasis,
	) error
	RegisterServiceAreaVersion(
		ctx context.Context,
		tenant domain.TenantID,
		row ServiceAreaDefinitionVersion,
	) error
	RegisterServiceCalendarVersion(
		ctx context.Context,
		tenant domain.TenantID,
		row ServiceCalendarDefinitionVersion,
	) error
	RegisterRouteStrategyVersion(
		ctx context.Context,
		tenant domain.TenantID,
		row RouteStrategyDefinitionVersion,
	) error
	RegisterAvailabilityAdjustment(
		ctx context.Context,
		tenant domain.TenantID,
		row AvailabilityAdjustmentStatement,
	) error
}
