package transportfulfillment

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/adoptconsume"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrPickupNotVisible 表示按信封引用还读不回对象级揽收登记。可见性滞后是续办，重投
	// 会改变结果；不当毒丸拒收。
	ErrPickupNotVisible = errors.New(
		"parcel shipment transportfulfillment adapter: offsite pickup registration is not yet visible")
	// ErrPickupRecordInconsistent 表示按键取回的登记指着另一个键或另一个对象——仓储或
	// 数据不变量已破（ADR-0029），不是等谁。它与 ErrPickupNotVisible 分开正是为了别把
	// 永久损坏登记成可续办：混进未决名单，投递会一路重试到上限，而现场要查的是那一行
	// 为什么长成这样。
	ErrPickupRecordInconsistent = errors.New(
		"parcel shipment transportfulfillment adapter: offsite pickup registration disagrees with its key")
	// ErrParcelTargetNotFound 表示这个租户下当前没有可采认的已接受委托声明了该包裹。
	// 委托可能还没落到已接受，重投会改变结果。载运对象是集运单元时也落这一格，见
	// AdoptOnOffsitePickupAdapter 的说明。
	ErrParcelTargetNotFound = errors.New(
		"parcel shipment transportfulfillment adapter: current accepted parcel target not found")
	// ErrAdoptionUndecided 与 ErrAdoptionHandoffPending 是 adoptconsume 同名哨兵的别名：
	// 采用结果如何落成消费两格由 adoptconsume 一处回答，本包只暴露口。
	ErrAdoptionUndecided      = adoptconsume.ErrAdoptionUndecided
	ErrAdoptionHandoffPending = adoptconsume.ErrAdoptionHandoffPending
)

// OffsitePickupFinder 按揽收登记幂等键取回对象级揽收。由 TF 的 OffsitePickupRegistry
// 满足。只取一法：本适配器不写 TF 的库，也不读它的尝试级集合。
type OffsitePickupFinder interface {
	FindByKey(
		ctx context.Context,
		key tfports.OffsitePickupKey,
	) (tfports.OffsitePickupRecord, bool, error)
}

// OffsitePickupAdopter 把已取回的对象级揽收交给采用翻译。真实装配用 OffsitePickupAdapter。
type OffsitePickupAdopter interface {
	AdoptFromOffsitePickup(
		ctx context.Context,
		pickup tfdomain.OffsitePickup,
		target TargetShipment,
	) (psapplication.AdoptNetworkIntakeResult, error)
}

// AdoptOnOffsitePickupAdapter 是 psinbox.OffsitePickupConsumer 的真实处理方：按引用重读
// TF 的对象级揽收登记，用当前已接受投影反查目标委托，再交给 OffsitePickupAdapter。
//
// 跨上下文翻译留在消费方（ADR-0025）。本层不猜 latest，歧义原样上抛。
//
// 载运对象引用按 transport-fulfillment 的领域定义可能指正式包裹身份，**也可能**指集运
// 单元，而事件与记录都不带判别位。今天不替它猜：集运单元号照原样进包裹反查，反查不中
// 就落 ErrParcelTargetNotFound。补上这一格需要 node-operations 暴露集运单元成员反查
// （成员关系属那个上下文），那是另一条票。
type AdoptOnOffsitePickupAdapter struct {
	pickups OffsitePickupFinder
	targets psports.CurrentAcceptedParcelTargetView
	adopt   OffsitePickupAdopter
}

func NewAdoptOnOffsitePickupAdapter(
	pickups OffsitePickupFinder,
	targets psports.CurrentAcceptedParcelTargetView,
	adopt OffsitePickupAdopter,
) (*AdoptOnOffsitePickupAdapter, error) {
	if pickups == nil {
		return nil, fmt.Errorf("parcel shipment transportfulfillment adapter: pickup finder is nil")
	}
	if targets == nil {
		return nil, fmt.Errorf("parcel shipment transportfulfillment adapter: parcel target view is nil")
	}
	if adopt == nil {
		return nil, fmt.Errorf("parcel shipment transportfulfillment adapter: offsite pickup adopter is nil")
	}
	return &AdoptOnOffsitePickupAdapter{pickups: pickups, targets: targets, adopt: adopt}, nil
}

var _ psinbox.RegisteredOffsitePickupHandler = (*AdoptOnOffsitePickupAdapter)(nil)

// HandleRegisteredOffsitePickup 按信封引用取回揽收登记、反查当前已接受目标、转交采用。
//
// 租户从信封带到两次查询：TF FindByKey 与 PS 包裹反查用同一个租户字符串，避免信封租户
// 与记录租户各说各话。
func (adapter *AdoptOnOffsitePickupAdapter) HandleRegisteredOffsitePickup(
	ctx context.Context,
	registered psinbox.RegisteredOffsitePickup,
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
	// 键与本体不符时不采认：取回的揽收若指着另一个对象，采用会挂到另一件包裹上。
	if record.Key != key || record.Pickup.Object() != key.Object ||
		record.Pickup.Attempt() != key.Attempt || record.Pickup.TenantID() != key.TenantID {
		return fmt.Errorf("%w: object %q attempt %q",
			ErrPickupRecordInconsistent, registered.Object, registered.Attempt)
	}

	psTenant, err := psdomain.NewTenantID(registered.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(record.Pickup.Object().String())
	if err != nil {
		return fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}

	target, found, err := adapter.targets.FindCurrentAcceptedByParcel(ctx, psTenant, parcel)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: parcel %q", ErrParcelTargetNotFound, parcel)
	}

	result, err := adapter.adopt.AdoptFromOffsitePickup(ctx, record.Pickup, TargetShipment{
		Identity:          target.Identity(),
		ShipmentRequestID: target.ShipmentRequestID(),
		SubmissionVersion: target.SubmissionVersion(),
	})
	if err != nil {
		return err
	}
	return adoptconsume.Consumption(result)
}

// pickupKeyFor 把信封三维译成 TF 的幂等键。译不出来是引用坏了，不是等谁。
func pickupKeyFor(registered psinbox.RegisteredOffsitePickup) (tfports.OffsitePickupKey, error) {
	none := tfports.OffsitePickupKey{}
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
	return tfports.OffsitePickupKey{TenantID: tenant, Object: object, Attempt: attempt}, nil
}
