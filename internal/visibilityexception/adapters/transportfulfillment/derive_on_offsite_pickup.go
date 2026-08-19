// Package transportfulfillment 是 visibility-exception 消费 transport-fulfillment
// 对象级结果的适配器（ADR-0025 消费方侧）。它只翻译不判断：揽收登记译成已接受源事实
// 命令，归类与派生由 DeriveProjectionHandler 回答。揽收与交付各文件自洽，不设共享.go。
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
	// ErrPickupNotVisible 表示按信封引用还读不回对象级揽收登记。可见性滞后是续办，重投
	// 会改变结果；不当毒丸拒收。
	ErrPickupNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: offsite pickup registration is not yet visible")
	// ErrPickupRecordInconsistent 表示按键取回的登记指着另一个键、另一个对象，或时间为零。
	// 仓储不变量已破（ADR-0029），不是等谁。禁止进 WithUndecidedSentinels。
	ErrPickupRecordInconsistent = errors.New(
		"visibility exception transportfulfillment adapter: offsite pickup registration disagrees with its key")
	// ErrPickupUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrPickupUntranslatableAnswer = errors.New(
		"visibility exception transportfulfillment adapter: untranslatable offsite pickup answer")
	// ErrPickupProjectionUndecided 与 ErrPickupProjectionHandoffPending 是 veconsume
	// 同名哨兵的揽收侧别名：派生结果如何落成消费两格由 veconsume 一处回答，本文件只暴露口。
	// 交付文件不要再声明一份同名别名——两边会在同包撞符号。
	ErrPickupProjectionUndecided      = veconsume.ErrProjectionUndecided
	ErrPickupProjectionHandoffPending = veconsume.ErrProjectionHandoffPending
)

// OffsitePickupFinder 按揽收登记幂等键取回对象级揽收。由 TF 的 OffsitePickupRegistry
// 满足。只取一法：本适配器不写 TF 的库，也不读它的尝试级集合。
type OffsitePickupFinder interface {
	FindByKey(
		ctx context.Context,
		key tfports.OffsitePickupKey,
	) (tfports.OffsitePickupRecord, bool, error)
}

// PickupProjectionHandler 是派生编排在揽收适配器侧的窄口。真实装配交给
// *application.DeriveProjectionHandler。名字带 Pickup，避免与同包交付文件撞符号。
type PickupProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnOffsitePickupAdapter 是 veinbox.OffsitePickupConsumer 的真实处理方：按引用
// 重读 TF 对象级揽收登记，译成已接受源事实命令，再交给派生编排。
//
// 跨上下文翻译留在消费方（ADR-0025）。投影只引用源事实，不复制场外揽收。
//
// 载运对象引用按 transport-fulfillment 的领域定义可能指正式包裹身份，也可能指集运
// 单元，而事件与记录都不带判别位。今天不替它猜、不跳过：Object 原样当包裹引用。
type DeriveOnOffsitePickupAdapter struct {
	pickups OffsitePickupFinder
	derive  PickupProjectionHandler
}

func NewDeriveOnOffsitePickupAdapter(
	pickups OffsitePickupFinder,
	derive PickupProjectionHandler,
) (*DeriveOnOffsitePickupAdapter, error) {
	if pickups == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: pickup finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: projection handler is nil")
	}
	return &DeriveOnOffsitePickupAdapter{pickups: pickups, derive: derive}, nil
}

var _ veinbox.RegisteredOffsitePickupHandler = (*DeriveOnOffsitePickupAdapter)(nil)

// HandleRegisteredOffsitePickup 按信封引用取回揽收并派生投影。
//
// 信封只做唤醒指针：业务发生时间取 Pickup.OccurredAt，有效时间同发生时间，接收时间
// 取记录 RecordedAt，禁止用信封 OccurredAt/RecordedAt 顶业务时间。
func (adapter *DeriveOnOffsitePickupAdapter) HandleRegisteredOffsitePickup(
	ctx context.Context,
	registered veinbox.RegisteredOffsitePickup,
) error {
	key, err := pickupKeyFor(registered)
	if err != nil {
		return err
	}
	record, found, err := adapter.pickups.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPickupNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: object %q attempt %q",
			ErrPickupNotVisible, registered.Object, registered.Attempt)
	}
	if record.Key != key || record.Pickup.Object() != key.Object ||
		record.Pickup.Attempt() != key.Attempt || record.Pickup.TenantID() != key.TenantID {
		return fmt.Errorf("%w: object %q attempt %q",
			ErrPickupRecordInconsistent, registered.Object, registered.Attempt)
	}
	if record.Pickup.OccurredAt().IsZero() || record.RecordedAt.IsZero() {
		return fmt.Errorf("%w: object %q attempt %q",
			ErrPickupRecordInconsistent, registered.Object, registered.Attempt)
	}

	command, err := pickupProjectionCommand(registered, record)
	if err != nil {
		return err
	}
	result, err := adapter.derive.Handle(ctx, command)
	if err != nil {
		return err
	}
	return veconsume.Consumption(result)
}

// pickupKeyFor 把信封三维译成 TF 的幂等键。译不出来是引用坏了，不是等谁。
func pickupKeyFor(registered veinbox.RegisteredOffsitePickup) (tfports.OffsitePickupKey, error) {
	none := tfports.OffsitePickupKey{}
	tenant, err := tfdomain.NewTenantID(registered.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrPickupUntranslatableAnswer, err)
	}
	object, err := tfdomain.NewCarriedObjectReference(registered.Object)
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrPickupUntranslatableAnswer, err)
	}
	attempt, err := tfdomain.NewAttemptReference(registered.Attempt)
	if err != nil {
		return none, fmt.Errorf("%w: attempt: %v", ErrPickupUntranslatableAnswer, err)
	}
	return tfports.OffsitePickupKey{TenantID: tenant, Object: object, Attempt: attempt}, nil
}

func pickupProjectionCommand(
	registered veinbox.RegisteredOffsitePickup,
	record tfports.OffsitePickupRecord,
) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(registered.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrPickupUntranslatableAnswer, err)
	}
	// 集运单元不猜、不跳过：Object 原样当包裹引用。
	parcel, err := vedomain.NewTrackedParcelReference(record.Pickup.Object().String())
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrPickupUntranslatableAnswer, err)
	}
	// 前缀把揽收事实键与同包交付（同为对象+尝试）错开，避免同源同键冲突。
	fact, err := vedomain.NewSourceFactReference(
		"offsite-pickup/" + record.Pickup.Object().String() + "/" + record.Pickup.Attempt().String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrPickupUntranslatableAnswer, err)
	}
	version, err := vedomain.NewSourceFactVersion(record.Pickup.Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: pickup version: %v", ErrPickupUntranslatableAnswer, err)
	}
	occurred := record.Pickup.OccurredAt()
	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source:      vedomain.SourceTransportFulfillment,
			Parcel:      parcel,
			Fact:        fact,
			Version:     version,
			OccurredAt:  occurred,
			EffectiveAt: occurred,
			ReceivedAt:  record.RecordedAt,
		},
	}, nil
}
