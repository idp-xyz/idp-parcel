package ports

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// 本文件是版本化网络目录（ADR-0068）的登记边界：七类定义原语的登记行形状与写入口。
// 行类型自持久化适配器上移到端口层，是因为登记用例（应用层）要以它们表达受理门，
// 而应用层不得依赖适配器；读侧的快照与选版**刻意不设端口**——解析层存在之前三个
// 证据视图不读本目录（ADR-0068 Decision 六），把读口抬成端口等于向三口发出邀请，
// 正是护栏要挡的方向。

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
type NodeDefinitionVersion struct {
	Code             string
	Version          int32
	BusinessTimezone string
	EffectiveFrom    time.Time
	EffectiveTo      time.Time
	HasEffectiveTo   bool
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
}

// LineDefinitionVersion 是一条线路的适用版本行；Segments 是连接身份的有序数组。
type LineDefinitionVersion struct {
	Code             string
	Version          int32
	Segments         []string
	BusinessTimezone string
	ApplicableScope  string
	EffectiveFrom    time.Time
	EffectiveTo      time.Time
	HasEffectiveTo   bool
}

// ServiceAreaDefinitionVersion 是一个服务区域的适用版本行。覆盖内容列未定
// （开放集，等 PAR-NET-14 的形态），版本机制先行。
type ServiceAreaDefinitionVersion struct {
	Code           string
	Version        int32
	EffectiveFrom  time.Time
	EffectiveTo    time.Time
	HasEffectiveTo bool
}

// ServiceCalendarDefinitionVersion 是某适用对象的服务日历适用版本行。
type ServiceCalendarDefinitionVersion struct {
	TargetKind     CatalogTargetKind
	TargetCode     string
	Version        int32
	EffectiveFrom  time.Time
	EffectiveTo    time.Time
	HasEffectiveTo bool
}

// AvailabilityAdjustmentStatement 是一个临时调整在某时点生效中的当前陈述（该调整
// 历史链上的最大版本，且其生效窗口覆盖 asOf）。
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

// RouteStrategyDefinitionVersion 是一个路由策略的适用版本行。规则正文属 PAR-NET-14，
// 这里只有版本、范围与有效区间。
type RouteStrategyDefinitionVersion struct {
	Code            string
	Version         int32
	ApplicableScope string
	EffectiveFrom   time.Time
	EffectiveTo     time.Time
	HasEffectiveTo  bool
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
