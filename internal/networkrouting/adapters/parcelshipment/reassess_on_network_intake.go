package parcelshipment

import (
	"context"
	"errors"
	"fmt"
	"time"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrReassessmentUndecided 说明这次复核停在未决：复核编排只对重投推得动的那几格答未决
// （ADR-0235），重投推不动的答`未判`、入账，不走到这里。它上抛让消费门整体回滚重投。
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

// AcceptedParcelLookup 按（租户，包裹）读回当前已接受委托在判断键上的各维与接受时刻。真实装配是
// AcceptedCarriedGeoSource，与交接那一路同一份读法。
type AcceptedParcelLookup interface {
	LoadAcceptedCarriedGeo(
		ctx context.Context,
		tenant nrdomain.TenantID,
		parcel nrdomain.DeclaredParcelID,
	) (AcceptedCarriedGeo, bool, error)
}

// ReassessOnNetworkIntakeAdapter 把 PS 采用结果信封接到 NR 的复核编排，是
// nrinbox.NetworkIntakeConsumer 的真实处理方。
//
// 它与 ReassessOnIntakeAdapter 分成两层：那一层拥有 ADR-0025 的翻译缝（采用记录逐维
// 译成复核触发），本层只做信封那一侧的事——按引用重新取回记录、读回这件委托接受的
// 时刻、把复核结论折成消费门认得的两格。翻译不在这里重写一遍，两处各译一次迟早分家。
type ReassessOnNetworkIntakeAdapter struct {
	adoptions AdoptedIntakeSource
	accepted  AcceptedParcelLookup
	reassess  *ReassessOnIntakeAdapter
}

func NewReassessOnNetworkIntakeAdapter(
	adoptions AdoptedIntakeSource,
	accepted AcceptedParcelLookup,
	reassess *ReassessOnIntakeAdapter,
) (*ReassessOnNetworkIntakeAdapter, error) {
	if adoptions == nil {
		return nil, fmt.Errorf("network routing parcelshipment adapter: adoption source is nil")
	}
	if accepted == nil {
		return nil, fmt.Errorf("network routing parcelshipment adapter: accepted parcel lookup is nil")
	}
	if reassess == nil {
		return nil, fmt.Errorf("network routing parcelshipment adapter: reassessment adapter is nil")
	}
	return &ReassessOnNetworkIntakeAdapter{adoptions: adoptions, accepted: accepted, reassess: reassess}, nil
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

	// `不采用`没有要复核的收寄事实，不为它去读委托；接受时刻留空，翻译层按不采用答。
	var acceptedAt time.Time
	if record.Adopted {
		acceptedAt, err = adapter.acceptedAtOf(ctx, record)
		if err != nil {
			return err
		}
	}
	result, err := adapter.reassess.TriggerReassessment(ctx, record, acceptedAt,
		nrports.RequestCarriedContent{Geo: geoFromCarried(intake.Geo)})
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

// acceptedAtOf 读回采用记录指着的那份已接受委托的接受时刻，复核拿它作业务时间下限（ADR-0279）。采用信封不带它，PS 也不在
// 采用时比业务时间与接受：这一格由 NR 在复核时比，PS 采用时拒在所有权上不对，也盖不到交接那一路。
//
// 采用以「当前已接受委托声明了这件包裹」为前提（ADR-0060），已接受是终态、基线一经固定不再改，所以读不着、读到另一份委托
// 或另一条基线都是不变量破了，响亮上抛、不进未决哨兵；读不动才是重投推得动的那一格。
func (adapter *ReassessOnNetworkIntakeAdapter) acceptedAtOf(
	ctx context.Context,
	record psports.IntakeAdoptionRecord,
) (time.Time, error) {
	tenant, err := nrdomain.NewTenantID(record.Key.TenantID.String())
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := nrdomain.NewDeclaredParcelID(record.Key.Parcel.String())
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	accepted, found, err := adapter.accepted.LoadAcceptedCarriedGeo(ctx, tenant, parcel)
	if err != nil {
		return time.Time{}, err
	}
	if !found {
		return time.Time{}, fmt.Errorf("%w: adopted intake of parcel %q has no current accepted request declaring it",
			ErrAcceptedRequestInconsistent, record.Key.Parcel)
	}
	if accepted.ShipmentRequestID.String() != record.ShipmentRequestID.String() ||
		accepted.AcceptanceBaseline.String() != record.Intake.Baseline().String() {
		return time.Time{}, fmt.Errorf("%w: adopted intake names request %q baseline %q, accepted parcel reads %q baseline %q",
			ErrAcceptedRequestInconsistent, record.ShipmentRequestID, record.Intake.Baseline(),
			accepted.ShipmentRequestID, accepted.AcceptanceBaseline)
	}
	return accepted.AcceptedAt, nil
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
	case nrapplication.ReassessTriggerNotJudged:
		// 入账未判，原因已由复核编排落进未判登记（ADR-0235）。重投推不动它，让它占着小包托运的分区，只会
		// 把同一封信上的别的各路与同一分区之后的信封一起拖到 ABANDONED。
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
