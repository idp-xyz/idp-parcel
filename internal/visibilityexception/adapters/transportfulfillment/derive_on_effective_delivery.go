// Package transportfulfillment 是 visibility-exception 消费 transport-fulfillment
// 结果的适配器（ADR-0025 消费方侧）。它只翻译不判断：交付记录译成已接受源事实命令，
// 归类与派生由 DeriveProjectionHandler 回答。交付投影不是 parcel-shipment 终局。
package transportfulfillment

import (
	"context"
	"errors"
	"fmt"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var (
	// ErrDeliveryNotVisible 表示按信封引用还读不回有效交付登记。可见性滞后是续办，重投
	// 会改变结果；不当毒丸拒收。
	ErrDeliveryNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: effective delivery is not yet visible")
	// ErrDeliveryRecordInconsistent 表示按键取回的登记指着另一个键，或三时间缺席。
	// 仓储不变量已破（ADR-0029），不是等谁。禁止进 WithUndecidedSentinels。
	ErrDeliveryRecordInconsistent = errors.New(
		"visibility exception transportfulfillment adapter: effective delivery disagrees with its key")
	// ErrUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrUntranslatableAnswer = errors.New(
		"visibility exception transportfulfillment adapter: untranslatable answer")
	// ErrProjectionUndecided 与 ErrProjectionHandoffPending 是 veconsume 同名哨兵的
	// 别名：派生结果如何落成消费两格由 veconsume 一处回答，本包只暴露口。
	ErrProjectionUndecided      = veconsume.ErrProjectionUndecided
	ErrProjectionHandoffPending = veconsume.ErrProjectionHandoffPending
)

// EffectiveDeliveryFinder 按有效交付幂等键取回登记。由 TF 的 EffectiveDeliveryStore
// 满足。只取一法：本适配器不写 TF 的库。
type EffectiveDeliveryFinder interface {
	FindByKey(
		ctx context.Context,
		key tfports.EffectiveDeliveryKey,
	) (tfports.EffectiveDeliveryRecord, bool, error)
}

// ProjectionHandler 是派生编排在本适配器侧的窄口。真实装配交给
// *application.DeriveProjectionHandler。
type ProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnEffectiveDeliveryAdapter 是 veinbox.EffectiveDeliveryConsumer 的真实处理方：
// 按引用重读 TF 有效交付登记，译成已接受源事实命令，再交给派生编排。
//
// 跨上下文翻译留在消费方（ADR-0025）。投影只引用源事实，不复制有效交付，也不把交付
// 投影当成 parcel-shipment 终局。
//
// 载运对象引用按 transport-fulfillment 的领域定义可能指正式包裹身份，也可能指集运
// 单元。今天不替它猜：对象号照原样进追踪包裹引用。
type DeriveOnEffectiveDeliveryAdapter struct {
	deliveries EffectiveDeliveryFinder
	derive     ProjectionHandler
}

func NewDeriveOnEffectiveDeliveryAdapter(
	deliveries EffectiveDeliveryFinder,
	derive ProjectionHandler,
) (*DeriveOnEffectiveDeliveryAdapter, error) {
	if deliveries == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: delivery finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: projection handler is nil")
	}
	return &DeriveOnEffectiveDeliveryAdapter{deliveries: deliveries, derive: derive}, nil
}

var _ veinbox.RegisteredEffectiveDeliveryHandler = (*DeriveOnEffectiveDeliveryAdapter)(nil)

// HandleRegisteredEffectiveDelivery 按信封引用取回有效交付并派生投影。
//
// 信封只做唤醒指针：业务发生时间取 Delivery.OccurredAt，有效时间同发生时间，接收
// 时间取记录 RecordedAt，禁止用信封 OccurredAt/RecordedAt 顶业务时间。只按键取当前
// 版——结果版本在事件 ID 里区分两代入队，不从 ID 回解析去查旧行。
func (adapter *DeriveOnEffectiveDeliveryAdapter) HandleRegisteredEffectiveDelivery(
	ctx context.Context,
	registered veinbox.RegisteredEffectiveDelivery,
) error {
	key, err := deliveryKeyFor(registered)
	if err != nil {
		return err
	}
	record, found, err := adapter.deliveries.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDeliveryNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: object %q attempt %q",
			ErrDeliveryNotVisible, registered.Object, registered.Attempt)
	}
	if record.Key != key || record.Delivery.Object() != key.Object ||
		record.Delivery.Attempt() != key.Attempt || record.Delivery.TenantID() != key.TenantID {
		return fmt.Errorf("%w: object %q attempt %q",
			ErrDeliveryRecordInconsistent, registered.Object, registered.Attempt)
	}
	if record.Delivery.OccurredAt().IsZero() || record.RecordedAt.IsZero() {
		return fmt.Errorf("%w: object %q attempt %q",
			ErrDeliveryRecordInconsistent, registered.Object, registered.Attempt)
	}

	command, err := projectionCommand(record)
	if err != nil {
		return err
	}
	result, err := adapter.derive.Handle(ctx, command)
	if err != nil {
		return err
	}
	return veconsume.Consumption(result)
}

func projectionCommand(record tfports.EffectiveDeliveryRecord) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(record.Delivery.TenantID().String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(record.Delivery.Object().String())
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	// 事实引用必须带 effective-delivery/ 前缀：同包揽收投影会用同一 Source 与同一
	// （对象+尝试）三维，裸键会让两路事实撞车。
	fact, err := vedomain.NewSourceFactReference(
		"effective-delivery/" + record.Delivery.Object().String() + "/" + record.Delivery.Attempt().String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrUntranslatableAnswer, err)
	}
	version, err := vedomain.NewSourceFactVersion(record.Delivery.Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: delivery version: %v", ErrUntranslatableAnswer, err)
	}
	kind, err := vedomain.NewSourceFactKind("effective-delivery")
	if err != nil {
		return none, fmt.Errorf("%w: source fact kind: %v", ErrUntranslatableAnswer, err)
	}
	occurred := record.Delivery.OccurredAt()
	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source:      vedomain.SourceTransportFulfillment,
			Parcel:      parcel,
			Fact:        fact,
			Kind:        kind,
			Version:     version,
			OccurredAt:  occurred,
			EffectiveAt: occurred,
			ReceivedAt:  record.RecordedAt,
		},
	}, nil
}

func deliveryKeyFor(registered veinbox.RegisteredEffectiveDelivery) (tfports.EffectiveDeliveryKey, error) {
	none := tfports.EffectiveDeliveryKey{}
	tenant, err := tfdomain.NewTenantID(registered.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	object, err := tfdomain.NewCarriedObjectReference(registered.Object)
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	attempt, err := tfdomain.NewAttemptReference(registered.Attempt)
	if err != nil {
		return none, fmt.Errorf("%w: attempt: %v", ErrUntranslatableAnswer, err)
	}
	return tfports.EffectiveDeliveryKey{TenantID: tenant, Object: object, Attempt: attempt}, nil
}
