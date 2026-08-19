// Package networkrouting 是 visibility-exception 消费 network-routing 已接受事实的
// 翻译层：按信封键重读 NR 权威本体，译成已接受源事实命令交给派生编排。跨上下文翻译
// 留在消费方（ADR-0025）。
package networkrouting

import (
	"context"
	"errors"
	"fmt"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var (
	// ErrInitialRouteNotVisible 表示按信封引用还读不回初始路由判断。可见性滞后是续办，
	// 重投会改变结果；不当毒丸拒收。
	ErrInitialRouteNotVisible = errors.New(
		"visibility exception networkrouting adapter: initial route judgment is not yet visible")
	// ErrInitialRouteRecordInconsistent 表示按键取回的记录指着另一个键、两支结论都带
	// 或都缺、或时间为零。仓储不变量已破（「无路由不得用空计划表达」的消费面），不是
	// 等谁。禁止进 WithUndecidedSentinels。
	ErrInitialRouteRecordInconsistent = errors.New(
		"visibility exception networkrouting adapter: initial route record disagrees with its key")
	// ErrInitialRouteUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrInitialRouteUntranslatableAnswer = errors.New(
		"visibility exception networkrouting adapter: untranslatable initial route answer")
	// ErrInitialRouteProjectionUndecided 与 ErrInitialRouteProjectionHandoffPending 是
	// veconsume 同名哨兵的初始路由侧别名。带 InitialRoute 前缀：后续 NR 事实（如改路）
	// 落进本包时不与短名相撞。
	ErrInitialRouteProjectionUndecided      = veconsume.ErrProjectionUndecided
	ErrInitialRouteProjectionHandoffPending = veconsume.ErrProjectionHandoffPending
)

// InitialRouteFinder 按初始路由判断键取回记录。由 NR 的 InitialRouteStore 满足。
// 只取一法：本适配器不写 NR 的库。
type InitialRouteFinder interface {
	FindByKey(
		ctx context.Context,
		key nrdomain.InitialRouteJudgmentKey,
	) (nrports.InitialRouteRecord, bool, error)
}

// InitialRouteProjectionHandler 是派生编排在初始路由适配器侧的窄口。
type InitialRouteProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnInitialRouteAdapter 是 veinbox.InitialRouteConsumer 的真实处理方：按六维
// 引用重读 NR 初始路由判断，译成已接受源事实命令，再交给派生编排。
//
// 投影只引用源事实，不复制权威判断——计划段链、候选依据都留在 NR，这里只带键、类型
// 与三个时间。
type DeriveOnInitialRouteAdapter struct {
	routes InitialRouteFinder
	derive InitialRouteProjectionHandler
}

func NewDeriveOnInitialRouteAdapter(
	routes InitialRouteFinder,
	derive InitialRouteProjectionHandler,
) (*DeriveOnInitialRouteAdapter, error) {
	if routes == nil {
		return nil, fmt.Errorf("visibility exception networkrouting adapter: initial route finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception networkrouting adapter: projection handler is nil")
	}
	return &DeriveOnInitialRouteAdapter{routes: routes, derive: derive}, nil
}

var _ veinbox.FormedInitialRouteHandler = (*DeriveOnInitialRouteAdapter)(nil)

// HandleFormedInitialRoute 按信封六维取回判断并派生投影。
//
// 信封只做唤醒指针：业务发生时间取判断本体 JudgedAt，接收时间取记录 RecordedAt，
// 禁止用信封 OccurredAt/RecordedAt 顶替。FindByKey 必须带全部六维——主键含
// customer_account_id，缺一维永远命不中。
func (adapter *DeriveOnInitialRouteAdapter) HandleFormedInitialRoute(
	ctx context.Context,
	formed veinbox.FormedInitialRoute,
) error {
	key, err := initialRouteKeyFor(formed)
	if err != nil {
		return err
	}
	record, found, err := adapter.routes.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInitialRouteNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: parcel %q purpose %q baseline %q",
			ErrInitialRouteNotVisible, formed.Parcel, formed.Purpose, formed.Baseline)
	}
	if err := initialRouteRecordConsistent(key, record); err != nil {
		return err
	}

	command, err := initialRouteProjectionCommand(record)
	if err != nil {
		return err
	}
	result, err := adapter.derive.Handle(ctx, command)
	if err != nil {
		return err
	}
	return veconsume.Consumption(result)
}

func initialRouteKeyFor(formed veinbox.FormedInitialRoute) (nrdomain.InitialRouteJudgmentKey, error) {
	none := nrdomain.InitialRouteJudgmentKey{}
	tenant, err := nrdomain.NewTenantID(formed.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrInitialRouteUntranslatableAnswer, err)
	}
	customer, err := nrdomain.NewCustomerAccountID(formed.CustomerAccountID)
	if err != nil {
		return none, fmt.Errorf("%w: customer account: %v", ErrInitialRouteUntranslatableAnswer, err)
	}
	shipment, err := nrdomain.NewShipmentRequestID(formed.Shipment)
	if err != nil {
		return none, fmt.Errorf("%w: shipment request: %v", ErrInitialRouteUntranslatableAnswer, err)
	}
	baseline, err := nrdomain.NewAcceptanceBaselineReference(formed.Baseline)
	if err != nil {
		return none, fmt.Errorf("%w: acceptance baseline: %v", ErrInitialRouteUntranslatableAnswer, err)
	}
	parcel, err := nrdomain.NewDeclaredParcelID(formed.Parcel)
	if err != nil {
		return none, fmt.Errorf("%w: declared parcel: %v", ErrInitialRouteUntranslatableAnswer, err)
	}
	purpose, err := nrdomain.NewServicePurpose(formed.Purpose)
	if err != nil {
		return none, fmt.Errorf("%w: service purpose: %v", ErrInitialRouteUntranslatableAnswer, err)
	}
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:           tenant,
		CustomerAccountID:  customer,
		ShipmentRequestID:  shipment,
		AcceptanceBaseline: baseline,
		DeclaredParcelID:   parcel,
		ServicePurpose:     purpose,
	}, nil
}

// initialRouteRecordConsistent 复验取回记录与键、结论形状与时间。「计划或无路由二居
// 其一」是 ports.InitialRouteRecord 的硬句——两个都带或都缺不是可等待的滞后，是坏写入。
func initialRouteRecordConsistent(
	key nrdomain.InitialRouteJudgmentKey,
	record nrports.InitialRouteRecord,
) error {
	inconsistent := fmt.Errorf("%w: parcel %q purpose %q baseline %q",
		ErrInitialRouteRecordInconsistent,
		key.DeclaredParcelID.String(), key.ServicePurpose.String(), key.AcceptanceBaseline.String())
	if record.Key != key || record.HasPlan == record.HasNoRoute || record.RecordedAt.IsZero() {
		return inconsistent
	}
	if record.HasPlan && (record.Plan.Key() != key || record.Plan.JudgedAt().IsZero()) {
		return inconsistent
	}
	if record.HasNoRoute && (record.NoRoute.Key() != key || record.NoRoute.JudgedAt().IsZero()) {
		return inconsistent
	}
	return nil
}

func initialRouteProjectionCommand(record nrports.InitialRouteRecord) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	key := record.Key
	tenant, err := vedomain.NewTenantID(key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrInitialRouteUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(key.DeclaredParcelID.String())
	if err != nil {
		return none, fmt.Errorf("%w: declared parcel: %v", ErrInitialRouteUntranslatableAnswer, err)
	}
	// 前缀把初始路由事实与 NETWORK_ROUTING 源下其它事实的 (source,fact,version) 错开；
	// 目的进引用，因为同一包裹可按不同服务目的各成一份判断。
	fact, err := vedomain.NewSourceFactReference(
		"initial-route/" + key.DeclaredParcelID.String() + "/" + key.ServicePurpose.String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrInitialRouteUntranslatableAnswer, err)
	}
	// 版本维用接受基线：判断键本身不带版本序列，「同一接受基线恰一个当前有效结果」
	// 意味着基线换版即新判断新事实版本。
	version, err := vedomain.NewSourceFactVersion(key.AcceptanceBaseline.String())
	if err != nil {
		return none, fmt.Errorf("%w: acceptance baseline: %v", ErrInitialRouteUntranslatableAnswer, err)
	}

	// 两支结论各自成事实类型（对应存储面封闭结论集合 ROUTE_FORMED/NO_CURRENT_ROUTE）：
	// 计划已形成与无当前路由语义相反，共用一个类型会让映射目录一行把两格归进同一里程碑。
	// 计划有自己的生效边界（EffectiveFrom）；无路由判断没有，有效时间同发生时间。
	var (
		raw       string
		occurred  time.Time
		effective time.Time
	)
	if record.HasPlan {
		raw = "initial-route-formed"
		occurred = record.Plan.JudgedAt()
		effective = record.Plan.EffectiveFrom()
	} else {
		raw = "initial-route-no-current-route"
		occurred = record.NoRoute.JudgedAt()
		effective = occurred
	}
	kind, err := vedomain.NewSourceFactKind(raw)
	if err != nil {
		return none, fmt.Errorf("%w: source fact kind: %v", ErrInitialRouteUntranslatableAnswer, err)
	}

	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source:      vedomain.SourceNetworkRouting,
			Parcel:      parcel,
			Fact:        fact,
			Kind:        kind,
			Version:     version,
			OccurredAt:  occurred,
			EffectiveAt: effective,
			ReceivedAt:  record.RecordedAt,
		},
	}, nil
}
