package transportfulfillment

import (
	"context"
	"errors"
	"fmt"
	"time"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrCarrierPickupNotVisible 表示按信封所指的（租户 + 事实 + 版本）还读不回那一代收寄。可见性滞后是续办，
	// 重投会改变结果；不当毒丸拒收。
	ErrCarrierPickupNotVisible = errors.New(
		"parcel shipment transportfulfillment adapter: carrier first effective pickup is not yet visible")
	// ErrCarrierPickupRecordInconsistent 表示按键取回的登记指着另一个键、本体与键各说各话、信封的载运对象与
	// 本体不符，或取回的是一版待确认——TF 对待确认版本响亮拒绝入队（ADR-0135 决定七），到这里是提供方交接口
	// 的装配缺陷，不是等谁。它与 ErrCarrierPickupNotVisible 分开正是为了别把永久损坏登记成可续办（ADR-0029）；
	// 生产装配不得把它进 WithUndecidedSentinels。
	ErrCarrierPickupRecordInconsistent = errors.New(
		"parcel shipment transportfulfillment adapter: carrier first effective pickup disagrees with its key")
	// ErrVoidedCarrierPickupRederivationUndecided 是票 label-channel/25「要裁的」2 那一格的显式未决：TF 交出
	// **失效版本**（依据被源更正为不再表达收寄，ADR-0135 决定六），而已据前版形成的非取消终局怎么重派生归
	// PS owner、尚未裁定。未裁之前按「未确认规则保持显式未决」办——不吸收、不重派生、不判、零写入；owner 裁定
	// 后（ADR-0117 同来源更正的采用版本链是可循的形）在本适配器接上重派生，这一格随之消失。
	//
	// 它是未决哨兵而不是不一致：恢复动作是「等 owner 裁定并接上」，重投在那之后会改变结果。
	ErrVoidedCarrierPickupRederivationUndecided = errors.New(
		"parcel shipment transportfulfillment adapter: rederivation on a voided carrier first effective pickup is not yet ruled")
)

// CarrierFirstEffectivePickupFinder 按（租户 + 事实 + 版本）取回**指名那一代**收寄。由 TF 的
// CarrierFirstEffectivePickups 满足。只取一法：本适配器不写 TF 的库，也不问链尾——信封每份代表一代，按当前版读
// 会把更正之前入队的那一份也读成更正后那一代（票 label-channel/24 的教训，ADR-0135 决定七）。
type CarrierFirstEffectivePickupFinder interface {
	FindByKey(
		ctx context.Context,
		key tfports.CarrierFirstEffectivePickupKey,
	) (tfports.CarrierFirstEffectivePickupRecord, bool, error)
}

// ParcelLabelFinalJudge 是面单渠道服务终局判断各路共用的处理方核的口（ADR-0134 决定二）：按（租户 + 包裹）反查
// 目标委托、折命令、判断、把 LabelServiceFinalOutcome 译成消费结论。生产装配接 labelfinal.ParcelJudgmentCore；本包只多出「取回 TF 事实、
// 核有效时间、折引用」那一段，核与翻译表不另写第二份。
type ParcelLabelFinalJudge interface {
	JudgeParcel(
		ctx context.Context,
		tenantRaw string,
		parcelRaw string,
		pickup psdomain.CarrierFirstEffectivePickupSpec,
	) error
}

// JudgeOnCarrierFirstEffectivePickupAdapter 是 psinbox.CarrierFirstEffectivePickupConsumer 的真实处理方：按信封
// 所指版本取回 TF 的实际承运商首次有效收寄、核键与本体一致，再把（事实、版本、业务发生时间）折成
// CarrierFirstEffectivePickupSpec 交共用核。
//
// 跨上下文翻译留在消费方（ADR-0025）。PS 不判「这条证据算不算收寄」——那是 TF 已判过的（TF CONTEXT「外部承运
// 轨迹事实」Rules）；这里只引用它判出的事实与业务发生时间，一列不复制。EffectiveAt 取事实上的业务发生时间
// （ADR-0135 决定三），不是本上下文读到它的时间。
//
// 载运对象按 transport-fulfillment 的领域定义可能指正式包裹身份，**也可能**指集运单元，事实与信封都不带判别位。
// 今天不替它猜：对象串原样进包裹反查（在核里），反查不中落 labelfinal.ErrParcelTargetNotFound。
type JudgeOnCarrierFirstEffectivePickupAdapter struct {
	pickups CarrierFirstEffectivePickupFinder
	judge   ParcelLabelFinalJudge
}

func NewJudgeOnCarrierFirstEffectivePickupAdapter(
	pickups CarrierFirstEffectivePickupFinder,
	judge ParcelLabelFinalJudge,
) (*JudgeOnCarrierFirstEffectivePickupAdapter, error) {
	if pickups == nil {
		return nil, fmt.Errorf("parcel shipment transportfulfillment adapter: carrier pickup finder is nil")
	}
	if judge == nil {
		return nil, fmt.Errorf("parcel shipment transportfulfillment adapter: label final judge is nil")
	}
	return &JudgeOnCarrierFirstEffectivePickupAdapter{pickups: pickups, judge: judge}, nil
}

var _ psinbox.RegisteredCarrierFirstEffectivePickupHandler = (*JudgeOnCarrierFirstEffectivePickupAdapter)(nil)

// HandleRegisteredCarrierFirstEffectivePickup 按信封所指版本取回收寄、核一致、按结果分格：已形成 → 折引用交核判；
// 失效 → 显式未决（要裁的 2）；待确认 → 不一致。租户串从信封带到取回与反查两次查询，避免信封租户与记录租户
// 各说各话。
func (adapter *JudgeOnCarrierFirstEffectivePickupAdapter) HandleRegisteredCarrierFirstEffectivePickup(
	ctx context.Context,
	registered psinbox.RegisteredCarrierFirstEffectivePickup,
) error {
	key, err := carrierPickupKeyFor(registered)
	if err != nil {
		return err
	}
	record, found, err := adapter.pickups.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCarrierPickupNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: fact %q version %q", ErrCarrierPickupNotVisible, registered.Fact, registered.Version)
	}
	pickup := record.Pickup
	// 键与本体不符时不采认：取回的收寄若指着另一代或另一个对象，终局会挂到另一件包裹或另一个版本上。
	if record.Key != key || pickup.TenantID() != key.TenantID || pickup.Fact() != key.Fact || pickup.Version() != key.Version ||
		(registered.Object != "" && pickup.Object().String() != registered.Object) {
		return fmt.Errorf("%w: fact %q version %q", ErrCarrierPickupRecordInconsistent, registered.Fact, registered.Version)
	}

	switch pickup.Result() {
	case tfdomain.CarrierPickupVoided:
		return fmt.Errorf("%w: fact %q version %q", ErrVoidedCarrierPickupRederivationUndecided, registered.Fact, registered.Version)
	case tfdomain.CarrierPickupFormed:
		// 已形成的版本必带业务发生时间（TF 构造门守着）；缺了是本体坏了，不是等谁。
		occurredAt, present := pickup.OccurredAt()
		if !present {
			return fmt.Errorf("%w: formed pickup %q version %q carries no occurrence time",
				ErrCarrierPickupRecordInconsistent, registered.Fact, registered.Version)
		}
		spec, err := firstEffectivePickupSpecFor(key, occurredAt)
		if err != nil {
			return err
		}
		return adapter.judge.JudgeParcel(ctx, registered.TenantID, pickup.Object().String(), spec)
	default:
		// 待确认不提供、不入队（ADR-0135 决定七）；封闭集合外同理——两者都不是可以据以判终局的东西。
		return fmt.Errorf("%w: fact %q version %q is %q, not a formed pickup",
			ErrCarrierPickupRecordInconsistent, registered.Fact, registered.Version, pickup.Result())
	}
}

// carrierPickupKeyFor 把信封三维译成 TF 的键。译不出来是引用坏了，不是等谁。
func carrierPickupKeyFor(registered psinbox.RegisteredCarrierFirstEffectivePickup) (tfports.CarrierFirstEffectivePickupKey, error) {
	none := tfports.CarrierFirstEffectivePickupKey{}
	tenant, err := tfdomain.NewTenantID(registered.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	fact, err := tfdomain.NewCarrierFirstEffectivePickupReference(registered.Fact)
	if err != nil {
		return none, fmt.Errorf("%w: carrier pickup fact: %v", ErrUntranslatableAnswer, err)
	}
	version, err := tfdomain.NewCarrierFirstEffectivePickupVersion(registered.Version)
	if err != nil {
		return none, fmt.Errorf("%w: carrier pickup version: %v", ErrUntranslatableAnswer, err)
	}
	return tfports.CarrierFirstEffectivePickupKey{TenantID: tenant, Fact: fact, Version: version}, nil
}

// firstEffectivePickupSpecFor 把 TF 的（事实，版本，业务发生时间）折成本上下文的只读引用三件。引用本体在编排里经
// ReferenceCarrierFirstEffectivePickup 立起，这里只交规格。
func firstEffectivePickupSpecFor(
	key tfports.CarrierFirstEffectivePickupKey,
	occurredAt time.Time,
) (psdomain.CarrierFirstEffectivePickupSpec, error) {
	none := psdomain.CarrierFirstEffectivePickupSpec{}
	fact, err := psdomain.NewCarrierFirstEffectivePickupFactReference(key.Fact.String())
	if err != nil {
		return none, fmt.Errorf("%w: carrier pickup fact: %v", ErrUntranslatableAnswer, err)
	}
	version, err := psdomain.NewCarrierFirstEffectivePickupFactVersion(key.Version.String())
	if err != nil {
		return none, fmt.Errorf("%w: carrier pickup version: %v", ErrUntranslatableAnswer, err)
	}
	return psdomain.CarrierFirstEffectivePickupSpec{Fact: fact, Version: version, EffectiveAt: occurredAt}, nil
}
