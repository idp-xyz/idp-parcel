package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrReassessmentUndecided 说明这次复核停在未决（复核登记册或路由历史读不回、证据
// 未配置、适用性看不清、取号依赖故障）。它上抛让消费门整体回滚重投。
//
// 与 ErrRouteHandoffUndecided 分设而不共用：两条链等的依赖不同，运维读到失败码要能
// 分出该去查初始路由那条还是复核这条。两者在装配点都登记进未决哨兵翻译——不翻的话
// 失败码会塌成 dispatch.publish_failed，把「消费方等依赖」误报成传输故障。
var ErrReassessmentUndecided = errors.New(
	"network routing parcelshipment adapter: route reassessment is undecided")

// AdoptedIntakeSource 是适配器内部协作者：按采用键整行取回采用结果。它由
// parcel-shipment 的采用仓储满足（*pspostgres.IntakeAdoptions 的同名方法）——本适配器
// 只声明自己要什么，不导入对方的端口包以外的东西。
type AdoptedIntakeSource interface {
	FindByKey(
		ctx context.Context,
		key psports.IntakeAdoptionKey,
	) (psports.IntakeAdoptionRecord, bool, error)
}

// ReassessOnNetworkIntakeAdapter 把 PS 采用结果信封接到 NR 的复核编排，是
// nrinbox.NetworkIntakeConsumer 的真实处理方。
//
// 它与 ReassessOnIntakeAdapter 分成两层：那一层拥有 ADR-0025 的翻译缝（采用记录逐维
// 译成复核触发），本层只做信封那一侧的两件事——按引用重新取回记录、把复核结论折成
// 消费门认得的两格。翻译不在这里重写一遍，两处各译一次迟早分家。
type ReassessOnNetworkIntakeAdapter struct {
	adoptions AdoptedIntakeSource
	reassess  *ReassessOnIntakeAdapter
}

func NewReassessOnNetworkIntakeAdapter(
	adoptions AdoptedIntakeSource,
	reassess *ReassessOnIntakeAdapter,
) (*ReassessOnNetworkIntakeAdapter, error) {
	if adoptions == nil {
		return nil, fmt.Errorf("network routing parcelshipment adapter: adoption source is nil")
	}
	if reassess == nil {
		return nil, fmt.Errorf("network routing parcelshipment adapter: reassessment adapter is nil")
	}
	return &ReassessOnNetworkIntakeAdapter{adoptions: adoptions, reassess: reassess}, nil
}

var _ nrinbox.NetworkIntakeHandler = (*ReassessOnNetworkIntakeAdapter)(nil)

// HandleAdoptedNetworkIntake 按引用取回采用记录再复核。
//
// 它整个跑在消费门的事务里：复核登记、适用性写入与判断库读写都用同一执行器加入同一
// 事务，所以「处理成功与消费入账同一事务」原样成立。
func (adapter *ReassessOnNetworkIntakeAdapter) HandleAdoptedNetworkIntake(
	ctx context.Context,
	intake nrinbox.AdoptedNetworkIntake,
) error {
	key, err := adoptionKeyOf(intake)
	if err != nil {
		return err
	}
	record, found, err := adapter.adoptions.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("load intake adoption: %w", err)
	}
	if !found {
		// 采用结果与它的意图在 PS 侧同一事务落库，读不着通常是可见性滞后。回滚重投会
		// 再来一次；当成终局跳过则会丢掉这次复核。
		return fmt.Errorf("%w: intake adoption %q/%q is not visible",
			ErrEnvelopeContradictsAuthority, intake.Parcel, intake.Version)
	}

	result, err := adapter.reassess.TriggerReassessment(ctx, record)
	if err != nil {
		// `不采用`是提供方的终局业务答案：没有责任起点成立，路由没有要复核的收寄事实。
		// 入账收工——让它回滚重投会把一份合法的`不采用`永远卡在队列里。
		if errors.Is(err, ErrRefusalDoesNotTrigger) {
			return nil
		}
		return err
	}
	return reassessmentToConsumption(result)
}

// adoptionKeyOf 把信封的四维引用译回采用键。来源类型逐格翻译、default 报错不吸收
// （ADR-0025）：那一维决定控制依据在 NR 侧落成哪一格，吸收成缺省会让复核挂在另一种
// 控制证据上。
func adoptionKeyOf(intake nrinbox.AdoptedNetworkIntake) (psports.IntakeAdoptionKey, error) {
	none := psports.IntakeAdoptionKey{}
	tenant, err := psdomain.NewTenantID(intake.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(intake.Parcel)
	if err != nil {
		return none, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	version, err := psdomain.NewSourceResultVersion(intake.Version)
	if err != nil {
		return none, fmt.Errorf("%w: source result version: %v", ErrUntranslatableAnswer, err)
	}
	kind, err := intakeSourceKindOf(intake.Kind)
	if err != nil {
		return none, err
	}
	return psports.IntakeAdoptionKey{
		TenantID: tenant,
		Parcel:   parcel,
		Kind:     kind,
		Version:  version,
	}, nil
}

func intakeSourceKindOf(raw string) (psdomain.IntakeSourceKind, error) {
	switch raw {
	case psdomain.NodeIntakeSource.String():
		return psdomain.NodeIntakeSource, nil
	case psdomain.OffsitePickupSource.String():
		return psdomain.OffsitePickupSource, nil
	default:
		return psdomain.IntakeSourceKindInvalid, fmt.Errorf(
			"%w: intake source kind %q", ErrUntranslatableAnswer, raw)
	}
}

// reassessmentToConsumption 把复核结论折成消费门认得的两格：nil 是「这份投递处理完了」，
// error 是「回滚重投」。折叠规则同 outcomeToConsumption——按「重投会不会改变结果」分。
func reassessmentToConsumption(result nrapplication.ReassessRouteResult) error {
	switch result.Outcome() {
	case nrapplication.ReassessedStillApplicable,
		nrapplication.ReassessedPlanLapsed,
		nrapplication.ReassessedFirstPlanFormed,
		nrapplication.ReassessedRerouted:
		// 四种结论都已越过提交边界落了库，是终局答案。失效与改路尤其不能因为「听起来
		// 像坏消息」就重投：复核复的就是这件事。
		return nil
	case nrapplication.ReassessExistingResult:
		// 同一触发已处理过，读回原结果：幂等，重投不会变。
		return nil
	case nrapplication.ReassessTriggerConflict:
		// 同一关联登记过另一份内容：原触发与原结果不被覆盖，重投也不会变。
		return nil
	case nrapplication.ReassessUndecided:
		return fmt.Errorf("%w: %s", ErrReassessmentUndecided, result.UndecidedReason())
	case nrapplication.ReassessTriggerNotAccepted:
		// 触发原料立不起来。原料全部由本包从采用记录译出，所以这一格是适配器缺陷而不是
		// 外来坏数据——响亮上抛，不要静默入账。
		return fmt.Errorf("%w: the trigger built from the adoption record was refused",
			ErrUntranslatableAnswer)
	default:
		return fmt.Errorf("%w: unexpected reassessment outcome %q",
			ErrUntranslatableAnswer, result.Outcome())
	}
}
