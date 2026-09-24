package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 版本化网络目录（ADR-0068）的登记用例。它站在写入口之前，只作一件事：**把不完整的
// 登记挡在库外，并把缺处指名**。目录内容属实例半边（PAR-NET-01..15 待提供），所以
// 本用例一个默认值都不补——缺身份就拒，缺版本号就拒，缺业务时区就拒；补一个占位值
// 会把「还没人登记」变成一份能进判断的网络定义。
//
// 受理门与迁移 0008 的 CHECK 逐条同格：库上约束是第二道网不是唯一一道，且撞 CHECK
// 的 INSERT 会把整个环境事务打进中止态（与 VE 登记用例拒撞键同一条理由），登记方
// 得到的只是一段约束名，不是一个说得清补什么的答案。
//
// 七族各一个方法、独立成败：一次登记只登一行，一族被拒不牵连别族——目录修订按笔
// 推进（ADR-0068 Decision 五），批量原子性由进程级入口决定把几笔包进同一个事务。
//
// 事务不由本层开：与本上下文其余用例一致，环境事务由进程级入口给出（先例：
// register_case_configuration.go 与 RegisterPriceCardHandler / RegisterReferenceSeriesHandler），
// 版本行与目录修订因此必然同一笔落地或同一笔回滚。
//
// 写入口的错误原样上抛不折格：NR 目录写入没有出格答案（重复版本号由主键挡、未闭
// 区间并存由部分唯一索引挡，均定于 ADR-0068 Consequences），错误在这里只表示依赖
// 故障或撞上了那两道库上防线——登记与否未知或需换号，都不是本用例能替登记方决定
// 的事。

// RegisterCatalogOutcome 是一次目录登记的应用处理结果。
//
// 只有两格，没有`未决`：登记是租户的管理动作，依赖调不通时没有一个如实的中间答案
// 可记——重试即可，所以那一路交回错误（与 VE 目录登记用例同形）。
type RegisterCatalogOutcome uint8

const (
	RegisterCatalogOutcomeInvalid RegisterCatalogOutcome = iota
	CatalogRegistered
	CatalogRegistrationRefused
)

func (outcome RegisterCatalogOutcome) String() string {
	switch outcome {
	case CatalogRegistered:
		return "REGISTERED"
	case CatalogRegistrationRefused:
		return "REFUSED"
	default:
		return ""
	}
}

// CatalogRefusalReason 指名这一笔登记差在哪一格。逐格分开是因为恢复动作各不相同：
// 缺租户要补管辖，缺身份码要指出登的是哪个对象，缺版本号要登记方给一个，缺业务时区
// 违背「每个节点、网络连接和适用线路必须明确业务时区」要补时区，两端相同违背「相邻
// 节点之间的明确方向」要改端点，零段违背「线路由一个或多个网络连接按明确顺序组成」
// 要给段链，区间倒序要改时间，类别或种类不在封闭集要改词，缺来源违背「调整必须记录
// 来源」要补出处。折成一格会让人去补错东西。
type CatalogRefusalReason uint8

const (
	CatalogRefusalReasonNone CatalogRefusalReason = iota
	CatalogTenantMissing
	CatalogIdentityMissing
	CatalogVersionMissing
	CatalogTimezoneMissing
	CatalogEndpointMissing
	CatalogEndpointsNotDistinct
	CatalogSegmentsMissing
	CatalogSegmentBlank
	CatalogApplicableScopeMissing
	CatalogEffectiveTimeMissing
	CatalogEffectiveRangeReversed
	CatalogTargetKindUnknown
	CatalogAdjustmentKindUnknown
	CatalogSourceMissing
	// CatalogRankingFormUnknown 是路由策略版本声明了族外的排序形态。没声明不在此列——那是
	// 租户还没选，照旧登得进（ADR-0146）。
	CatalogRankingFormUnknown
)

func (reason CatalogRefusalReason) String() string {
	switch reason {
	case CatalogTenantMissing:
		return "TENANT_MISSING"
	case CatalogIdentityMissing:
		return "IDENTITY_MISSING"
	case CatalogVersionMissing:
		return "VERSION_MISSING"
	case CatalogTimezoneMissing:
		return "TIMEZONE_MISSING"
	case CatalogEndpointMissing:
		return "ENDPOINT_MISSING"
	case CatalogEndpointsNotDistinct:
		return "ENDPOINTS_NOT_DISTINCT"
	case CatalogSegmentsMissing:
		return "SEGMENTS_MISSING"
	case CatalogSegmentBlank:
		return "SEGMENT_BLANK"
	case CatalogApplicableScopeMissing:
		return "APPLICABLE_SCOPE_MISSING"
	case CatalogEffectiveTimeMissing:
		return "EFFECTIVE_TIME_MISSING"
	case CatalogEffectiveRangeReversed:
		return "EFFECTIVE_RANGE_REVERSED"
	case CatalogTargetKindUnknown:
		return "TARGET_KIND_UNKNOWN"
	case CatalogAdjustmentKindUnknown:
		return "ADJUSTMENT_KIND_UNKNOWN"
	case CatalogSourceMissing:
		return "SOURCE_MISSING"
	case CatalogRankingFormUnknown:
		return "RANKING_FORM_UNKNOWN"
	default:
		return ""
	}
}

type RegisterCatalogResult struct {
	outcome RegisterCatalogOutcome
	refusal CatalogRefusalReason
}

func (result RegisterCatalogResult) Outcome() RegisterCatalogOutcome {
	return result.outcome
}

// RefusalReason 只在被拒时有值。
func (result RegisterCatalogResult) RefusalReason() CatalogRefusalReason {
	return result.refusal
}

func catalogRegistered() RegisterCatalogResult {
	return RegisterCatalogResult{outcome: CatalogRegistered}
}

func catalogRefused(reason CatalogRefusalReason) RegisterCatalogResult {
	return RegisterCatalogResult{outcome: CatalogRegistrationRefused, refusal: reason}
}

// 七个命令各携带一行登记。租户显式随命令到达（ADR-0003）：目录行只在租户内唯一，
// 跨越租户边界必须在签名上看得见。

type RegisterNodeVersionCommand struct {
	TenantID domain.TenantID
	Node     ports.NodeDefinitionVersion
}

type RegisterConnectionVersionCommand struct {
	TenantID   domain.TenantID
	Connection ports.ConnectionDefinitionVersion
}

type RegisterLineVersionCommand struct {
	TenantID domain.TenantID
	Line     ports.LineDefinitionVersion
}

type RegisterServiceAreaVersionCommand struct {
	TenantID domain.TenantID
	Area     ports.ServiceAreaDefinitionVersion
}

type RegisterServiceCalendarVersionCommand struct {
	TenantID domain.TenantID
	Calendar ports.ServiceCalendarDefinitionVersion
}

type RegisterRouteStrategyVersionCommand struct {
	TenantID domain.TenantID
	Strategy ports.RouteStrategyDefinitionVersion
}

type RegisterAvailabilityAdjustmentCommand struct {
	TenantID   domain.TenantID
	Adjustment ports.AvailabilityAdjustmentStatement
}

// NetworkCatalogRegistration 是版本化网络目录七类定义原语的登记用例。
type NetworkCatalogRegistration struct {
	registry ports.NetworkCatalogRegistry
}

func NewNetworkCatalogRegistration(
	registry ports.NetworkCatalogRegistry,
) (*NetworkCatalogRegistration, error) {
	if registry == nil {
		return nil, fmt.Errorf("network routing application: catalog registry is required")
	}
	return &NetworkCatalogRegistration{registry: registry}, nil
}

func (service *NetworkCatalogRegistration) RegisterNodeVersion(
	ctx context.Context,
	command RegisterNodeVersionCommand,
) (RegisterCatalogResult, error) {
	row := command.Node
	if reason := checkVersionRow(command.TenantID, row.Code, row.Version,
		row.EffectiveFrom, row.EffectiveTo, row.HasEffectiveTo); reason != CatalogRefusalReasonNone {
		return catalogRefused(reason), nil
	}
	if !catalogPresent(row.BusinessTimezone) {
		return catalogRefused(CatalogTimezoneMissing), nil
	}

	if err := service.registry.RegisterNodeVersion(ctx, command.TenantID, row); err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register node version: %w", err)
	}
	return catalogRegistered(), nil
}

func (service *NetworkCatalogRegistration) RegisterConnectionVersion(
	ctx context.Context,
	command RegisterConnectionVersionCommand,
) (RegisterCatalogResult, error) {
	row := command.Connection
	if reason := checkVersionRow(command.TenantID, row.Code, row.Version,
		row.EffectiveFrom, row.EffectiveTo, row.HasEffectiveTo); reason != CatalogRefusalReasonNone {
		return catalogRefused(reason), nil
	}
	switch {
	case !catalogPresent(row.FromNode), !catalogPresent(row.ToNode):
		return catalogRefused(CatalogEndpointMissing), nil
	// 「网络连接必须表达相邻节点之间的明确方向」——两端相同没有方向可言，
	// 与迁移 0008 的 directed_between_two_nodes 同格。
	case strings.TrimSpace(row.FromNode) == strings.TrimSpace(row.ToNode):
		return catalogRefused(CatalogEndpointsNotDistinct), nil
	case !catalogPresent(row.BusinessTimezone):
		return catalogRefused(CatalogTimezoneMissing), nil
	}

	if err := service.registry.RegisterConnectionVersion(ctx, command.TenantID, row); err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register connection version: %w", err)
	}
	return catalogRegistered(), nil
}

func (service *NetworkCatalogRegistration) RegisterLineVersion(
	ctx context.Context,
	command RegisterLineVersionCommand,
) (RegisterCatalogResult, error) {
	row := command.Line
	if reason := checkVersionRow(command.TenantID, row.Code, row.Version,
		row.EffectiveFrom, row.EffectiveTo, row.HasEffectiveTo); reason != CatalogRefusalReasonNone {
		return catalogRefused(reason), nil
	}
	if len(row.Segments) == 0 {
		return catalogRefused(CatalogSegmentsMissing), nil
	}
	// 段链里的空身份是登记方漏填，不是一段「无名连接」——序即语义，缺一环链就断。
	for _, segment := range row.Segments {
		if !catalogPresent(segment) {
			return catalogRefused(CatalogSegmentBlank), nil
		}
	}
	switch {
	case !catalogPresent(row.BusinessTimezone):
		return catalogRefused(CatalogTimezoneMissing), nil
	case !catalogPresent(row.ApplicableScope):
		return catalogRefused(CatalogApplicableScopeMissing), nil
	}

	if err := service.registry.RegisterLineVersion(ctx, command.TenantID, row); err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register line version: %w", err)
	}
	return catalogRegistered(), nil
}

func (service *NetworkCatalogRegistration) RegisterServiceAreaVersion(
	ctx context.Context,
	command RegisterServiceAreaVersionCommand,
) (RegisterCatalogResult, error) {
	row := command.Area
	if reason := checkVersionRow(command.TenantID, row.Code, row.Version,
		row.EffectiveFrom, row.EffectiveTo, row.HasEffectiveTo); reason != CatalogRefusalReasonNone {
		return catalogRefused(reason), nil
	}

	if err := service.registry.RegisterServiceAreaVersion(ctx, command.TenantID, row); err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register service area version: %w", err)
	}
	return catalogRegistered(), nil
}

func (service *NetworkCatalogRegistration) RegisterServiceCalendarVersion(
	ctx context.Context,
	command RegisterServiceCalendarVersionCommand,
) (RegisterCatalogResult, error) {
	row := command.Calendar
	if row.TargetKind.String() == "" {
		return catalogRefused(CatalogTargetKindUnknown), nil
	}
	if reason := checkVersionRow(command.TenantID, row.TargetCode, row.Version,
		row.EffectiveFrom, row.EffectiveTo, row.HasEffectiveTo); reason != CatalogRefusalReasonNone {
		return catalogRefused(reason), nil
	}

	if err := service.registry.RegisterServiceCalendarVersion(ctx, command.TenantID, row); err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register service calendar version: %w", err)
	}
	return catalogRegistered(), nil
}

func (service *NetworkCatalogRegistration) RegisterRouteStrategyVersion(
	ctx context.Context,
	command RegisterRouteStrategyVersionCommand,
) (RegisterCatalogResult, error) {
	row := command.Strategy
	if reason := checkVersionRow(command.TenantID, row.Code, row.Version,
		row.EffectiveFrom, row.EffectiveTo, row.HasEffectiveTo); reason != CatalogRefusalReasonNone {
		return catalogRefused(reason), nil
	}
	// 「新线路版本和路由策略版本必须具有明确生效时间和适用范围」——范围是版本自带
	// 的声明，缺了这一版就没说清自己管哪里。
	if !catalogPresent(row.ApplicableScope) {
		return catalogRefused(CatalogApplicableScopeMissing), nil
	}
	if row.RankingForm != domain.RankingFormUndeclared && row.RankingForm.String() == "" {
		return catalogRefused(CatalogRankingFormUnknown), nil
	}

	if err := service.registry.RegisterRouteStrategyVersion(ctx, command.TenantID, row); err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register route strategy version: %w", err)
	}
	return catalogRegistered(), nil
}

// RegisterAvailabilityAdjustment 登记一条临时可用性调整陈述。调整族的门与稳定六族
// 分开写：它没有生效区间只有生效窗口（EffectiveAt/LiftedAt），选版走历史链不走区间
// ——「稳定网络定义和临时网络可用性调整必须分离」在受理门上的那一半。
func (service *NetworkCatalogRegistration) RegisterAvailabilityAdjustment(
	ctx context.Context,
	command RegisterAvailabilityAdjustmentCommand,
) (RegisterCatalogResult, error) {
	row := command.Adjustment
	switch {
	case !catalogPresent(command.TenantID.String()):
		return catalogRefused(CatalogTenantMissing), nil
	case !catalogPresent(row.Code):
		return catalogRefused(CatalogIdentityMissing), nil
	case row.Version < 1:
		return catalogRefused(CatalogVersionMissing), nil
	case row.TargetKind.String() == "":
		return catalogRefused(CatalogTargetKindUnknown), nil
	case !catalogPresent(row.TargetCode):
		return catalogRefused(CatalogIdentityMissing), nil
	case row.Kind.String() == "":
		return catalogRefused(CatalogAdjustmentKindUnknown), nil
	// 「调整必须记录来源、范围、生效时间和解除时间」——来源缺了，这条陈述就
	// 说不出自己凭什么。
	case !catalogPresent(row.Source):
		return catalogRefused(CatalogSourceMissing), nil
	case row.EffectiveAt.IsZero():
		return catalogRefused(CatalogEffectiveTimeMissing), nil
	case row.HasLiftedAt && !row.LiftedAt.After(row.EffectiveAt):
		return catalogRefused(CatalogEffectiveRangeReversed), nil
	}

	if err := service.registry.RegisterAvailabilityAdjustment(ctx, command.TenantID, row); err != nil {
		return RegisterCatalogResult{}, fmt.Errorf("register availability adjustment: %w", err)
	}
	return catalogRegistered(), nil
}

// checkVersionRow 核稳定六族共用的抬头五件：租户、身份码、版本号、生效时间与区间
// 顺序。生效时间零值即登记方没给——绝对时刻的零值不是一个能用的生效边界，拿它登记
// 等于让这一版从公元元年起就适用（与 VE 目录抬头门同一条理由）。
func checkVersionRow(
	tenant domain.TenantID,
	code string,
	version int32,
	effectiveFrom time.Time,
	effectiveTo time.Time,
	hasEffectiveTo bool,
) CatalogRefusalReason {
	switch {
	case !catalogPresent(tenant.String()):
		return CatalogTenantMissing
	case !catalogPresent(code):
		return CatalogIdentityMissing
	case version < 1:
		return CatalogVersionMissing
	case effectiveFrom.IsZero():
		return CatalogEffectiveTimeMissing
	// [effective_from, effective_to) 是半开区间，两端相等即空区间，与倒序同格拒：
	// 一个不覆盖任何时点的版本不是登记方想登的东西。
	case hasEffectiveTo && !effectiveTo.After(effectiveFrom):
		return CatalogEffectiveRangeReversed
	default:
		return CatalogRefusalReasonNone
	}
}

func catalogPresent(value string) bool {
	return strings.TrimSpace(value) != ""
}
