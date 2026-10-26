package transportfulfillment

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrDeliveryNotVisible 表示按信封引用还读不回有效交付登记。可见性滞后是续办，重投
	// 会改变结果；不当毒丸拒收。
	ErrDeliveryNotVisible = errors.New(
		"parcel shipment transportfulfillment adapter: effective delivery is not yet visible")
	// ErrDeliveryRecordInconsistent 表示按键取回的登记指着另一个键或另一个对象——仓储或
	// 数据不变量已破（ADR-0029），不是等谁。它与 ErrDeliveryNotVisible 分开正是为了别把
	// 永久损坏登记成可续办：混进未决名单，投递会一路重试到上限，而现场要查的是那一行
	// 为什么长成这样。CONS-FINAL-B 不得把它进 WithUndecidedSentinels。
	ErrDeliveryRecordInconsistent = errors.New(
		"parcel shipment transportfulfillment adapter: effective delivery disagrees with its key")
	// ErrFinalUndecided 与 ErrFinalHandoffPending 是 finalconsume 同名哨兵的别名：终局
	// 结果如何落成消费两格由 finalconsume 一处回答，本包只暴露口。
	ErrFinalUndecided      = finalconsume.ErrFinalUndecided
	ErrFinalHandoffPending = finalconsume.ErrFinalHandoffPending
)

// EffectiveDeliveryFinder 按有效交付幂等键取回登记。由 TF 的 EffectiveDeliveryStore
// 满足。只取一法：本适配器不写 TF 的库。
type EffectiveDeliveryFinder interface {
	FindByKey(
		ctx context.Context,
		key tfports.EffectiveDeliveryKey,
	) (tfports.EffectiveDeliveryRecord, bool, error)
}

// EffectiveDeliveryAdopter 把已取回的有效交付交给终局翻译。真实装配用
// DeliveryOutcomeAdapter。
type EffectiveDeliveryAdopter interface {
	AdoptFromEffectiveDelivery(
		ctx context.Context,
		delivery tfdomain.EffectiveDelivery,
		target TargetShipment,
	) (psapplication.FormParcelFinalResult, error)
}

// AdoptOnEffectiveDeliveryAdapter 是 psinbox.EffectiveDeliveryConsumer 的真实处理方：
// 按引用重读 TF 的有效交付登记，用当前已接受投影反查目标委托，再交给
// DeliveryOutcomeAdapter。
//
// 跨上下文翻译留在消费方（ADR-0025）。本层不猜 latest，歧义原样上抛。
//
// 载运对象引用按 transport-fulfillment 的领域定义可能指正式包裹身份，**也可能**指集运
// 单元，而事件与记录都不带判别位。反查包裹之前先问节点作业的单元册（units）：对象在交付
// 发生那一刻是集运单元，这一封收工——单元不是包裹，没有委托，不形成终局，也不留本上下文
// 的记录，只有消费门的 inbox 行；整袋交付是不是袋里每件包裹的有效交付另由单元控制规则定，
// 此刻不投到成员上。这一问先于终局编排里的取消边界锁与取消视图，所以不改变它们的锁序。判据
// 只是单元册那一答（同串的包裹号按单元认），不按串形或前缀猜：不在册上的照旧反查，反查不中
// 仍落 ErrParcelTargetNotFound。单元册读不动落 ErrConsolidationUnitNotVisible（续办），答了
// 却译不回落 ErrConsolidationUnitAnswerInconsistent（确定性，不进未决名单）。
type AdoptOnEffectiveDeliveryAdapter struct {
	deliveries EffectiveDeliveryFinder
	targets    psports.CurrentAcceptedParcelTargetView
	adopt      EffectiveDeliveryAdopter
	units      psports.ConsolidationUnitView
}

func NewAdoptOnEffectiveDeliveryAdapter(
	deliveries EffectiveDeliveryFinder,
	targets psports.CurrentAcceptedParcelTargetView,
	adopt EffectiveDeliveryAdopter,
	units psports.ConsolidationUnitView,
) (*AdoptOnEffectiveDeliveryAdapter, error) {
	if deliveries == nil {
		return nil, fmt.Errorf("parcel shipment transportfulfillment adapter: delivery finder is nil")
	}
	if targets == nil {
		return nil, fmt.Errorf("parcel shipment transportfulfillment adapter: parcel target view is nil")
	}
	if adopt == nil {
		return nil, fmt.Errorf("parcel shipment transportfulfillment adapter: effective delivery adopter is nil")
	}
	if units == nil {
		return nil, fmt.Errorf("parcel shipment transportfulfillment adapter: consolidation unit view is nil")
	}
	return &AdoptOnEffectiveDeliveryAdapter{deliveries: deliveries, targets: targets, adopt: adopt, units: units}, nil
}

var _ psinbox.RegisteredEffectiveDeliveryHandler = (*AdoptOnEffectiveDeliveryAdapter)(nil)

// HandleRegisteredEffectiveDelivery 按信封引用取回有效交付、反查当前已接受目标、转交终局。
//
// 租户从信封带到两次查询：TF FindByKey 与 PS 包裹反查用同一个租户字符串，避免信封租户
// 与记录租户各说各话。
func (adapter *AdoptOnEffectiveDeliveryAdapter) HandleRegisteredEffectiveDelivery(
	ctx context.Context,
	registered psinbox.RegisteredEffectiveDelivery,
) error {
	key, err := deliveryKeyFor(registered)
	if err != nil {
		return err
	}
	// 只按键取当前版。结果版本在事件 ID 里区分两代入队，不从 ID 回解析去查旧行。
	record, found, err := adapter.deliveries.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDeliveryNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: object %q attempt %q",
			ErrDeliveryNotVisible, registered.Object, registered.Attempt)
	}
	// 键与本体不符时不采认：取回的交付若指着另一个对象，终局会挂到另一件包裹上。
	if record.Key != key || record.Delivery.Object() != key.Object ||
		record.Delivery.Attempt() != key.Attempt || record.Delivery.TenantID() != key.TenantID {
		return fmt.Errorf("%w: object %q attempt %q",
			ErrDeliveryRecordInconsistent, registered.Object, registered.Attempt)
	}

	psTenant, err := psdomain.NewTenantID(registered.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(record.Delivery.Object().String())
	if err != nil {
		return fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}

	unit, err := carriedObjectIsConsolidationUnit(ctx, adapter.units, psTenant, parcel, record.Delivery.OccurredAt())
	if err != nil {
		return err
	}
	if unit {
		return nil
	}

	target, found, err := adapter.targets.FindCurrentAcceptedByParcel(ctx, psTenant, parcel)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: parcel %q", ErrParcelTargetNotFound, parcel)
	}

	result, err := adapter.adopt.AdoptFromEffectiveDelivery(ctx, record.Delivery, TargetShipment{
		Identity:          target.Identity(),
		ShipmentRequestID: target.ShipmentRequestID(),
	})
	if err != nil {
		return err
	}
	return finalconsume.Consumption(result)
}

// deliveryKeyFor 把信封三维译成 TF 的幂等键。译不出来是引用坏了，不是等谁。
func deliveryKeyFor(registered psinbox.RegisteredEffectiveDelivery) (tfports.EffectiveDeliveryKey, error) {
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
