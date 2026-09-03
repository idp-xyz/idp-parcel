package transportfulfillment

import (
	"context"
	"errors"
	"fmt"
	"time"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// externalCarrierTrackingKind 是 TF 为这一类事实命名的类型词，定义在 transport-fulfillment
// 的 CONTEXT「外部承运轨迹事实」词条。它是里程碑映射的键之一；状态词不进类型词——同一个词在
// 两家源可以含义相反，映射登记册按（源上下文，事实类型）建键这条不变量不为此松动。
const externalCarrierTrackingKind = "external-carrier-tracking"

var (
	// ErrExternalTrackingNotVisible 表示按引用还读不回那一版。可见性滞后是续办，重投会改变结果。
	ErrExternalTrackingNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: external carrier tracking fact is not yet visible")
	// ErrExternalTrackingRecordInconsistent 表示按键取回的登记指着另一个键、时间缺席，或**有效
	// 时间仍是待判断**。后者尤其不是等谁：TF 的交接口对待判断版本响亮拒绝入队，一份待判断的引用
	// 到了这里只可能是仓储或装配出了本上下文任何路径都造不出的状态（ADR-0029）。禁止进
	// WithUndecidedSentinels。
	ErrExternalTrackingRecordInconsistent = errors.New(
		"visibility exception transportfulfillment adapter: external carrier tracking fact disagrees with its key or is still pending judgment")
	// ErrExternalTrackingUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrExternalTrackingUntranslatableAnswer = errors.New(
		"visibility exception transportfulfillment adapter: untranslatable external tracking answer")
	// ErrExternalTrackingProjectionUndecided 与 ErrExternalTrackingProjectionHandoffPending 是
	// veconsume 同名哨兵的别名：派生结果如何落成消费两格由 veconsume 一处回答。
	ErrExternalTrackingProjectionUndecided      = veconsume.ErrProjectionUndecided
	ErrExternalTrackingProjectionHandoffPending = veconsume.ErrProjectionHandoffPending
)

// ExternalTrackingFactFinder 按（租户+事实+版本）取回那一代登记。由 TF 的 ExternalTrackingFacts
// 满足。只取一法：本适配器不写 TF 的库。
type ExternalTrackingFactFinder interface {
	FindByKey(ctx context.Context, key tfports.ExternalTrackingFactKey) (tfports.ExternalTrackingFactRecord, bool, error)
}

// DeriveOnExternalCarrierTrackingAdapter 是 veinbox.ExternalTrackingConsumer 的真实处理方：按引用
// 重读 TF 外部承运轨迹事实，译成已接受源事实命令，再交给派生编排。
//
// 跨上下文翻译留在消费方（ADR-0025）。三个时间原样过桥、各归各位：OccurredAt 是源给的发生时间，
// EffectiveAt 是 TF 判断过的有效时间，ReceivedAt 是 TF 接到素材的时间——本适配器不拿任何一个
// 顶替另一个（ADR-0102）。原始状态词不过桥：它不是类型词，解释它是另一次判断、另一张票；
// 映射目录里没有这一类型时，DeriveProjectionHandler 如实留为未归类。
type DeriveOnExternalCarrierTrackingAdapter struct {
	facts  ExternalTrackingFactFinder
	derive ProjectionHandler
}

func NewDeriveOnExternalCarrierTrackingAdapter(
	facts ExternalTrackingFactFinder,
	derive ProjectionHandler,
) (*DeriveOnExternalCarrierTrackingAdapter, error) {
	if facts == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: external tracking finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: projection handler is nil")
	}
	return &DeriveOnExternalCarrierTrackingAdapter{facts: facts, derive: derive}, nil
}

var _ veinbox.JudgedExternalTrackingHandler = (*DeriveOnExternalCarrierTrackingAdapter)(nil)

// HandleJudgedExternalTracking 按信封引用取回那一版并派生投影。信封只做唤醒指针：三个时间全部
// 取自事实本体，禁止用信封 OccurredAt/RecordedAt 顶业务时间。
func (adapter *DeriveOnExternalCarrierTrackingAdapter) HandleJudgedExternalTracking(
	ctx context.Context,
	judged veinbox.JudgedExternalTracking,
) error {
	key, err := externalTrackingKeyFor(judged)
	if err != nil {
		return err
	}
	record, found, err := adapter.facts.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrExternalTrackingNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: fact %q version %q", ErrExternalTrackingNotVisible, judged.Fact, judged.Version)
	}
	fact := record.Fact
	if record.Key != key || fact.TenantID() != key.TenantID || fact.Fact() != key.Fact || fact.Version() != key.Version {
		return fmt.Errorf("%w: fact %q version %q", ErrExternalTrackingRecordInconsistent, judged.Fact, judged.Version)
	}
	effectiveAt, isJudged := fact.EffectiveAt()
	if !isJudged || fact.OccurredAt().IsZero() || fact.ReceivedAt().IsZero() {
		return fmt.Errorf("%w: fact %q version %q", ErrExternalTrackingRecordInconsistent, judged.Fact, judged.Version)
	}

	command, err := externalTrackingProjectionCommand(fact, effectiveAt)
	if err != nil {
		return err
	}
	result, err := adapter.derive.Handle(ctx, command)
	if err != nil {
		return err
	}
	return veconsume.Consumption(result)
}

func externalTrackingProjectionCommand(
	fact tfdomain.ExternalCarrierTrackingFact,
	effectiveAt time.Time,
) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(fact.TenantID().String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrExternalTrackingUntranslatableAnswer, err)
	}
	// 载运对象引用按 TF 的领域定义可能指正式包裹身份，也可能指集运单元。今天不替它猜：对象号
	// 照原样进追踪包裹引用，与交付、交接两路同一条做法。
	parcel, err := vedomain.NewTrackedParcelReference(fact.Object().String())
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrExternalTrackingUntranslatableAnswer, err)
	}
	// 事实引用带 external-carrier-tracking/ 前缀：同 Source 下交付、揽收、交接各有自己的前缀段，
	// 裸键会让几路事实撞车。
	reference, err := vedomain.NewSourceFactReference(externalCarrierTrackingKind + "/" + fact.Fact().String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrExternalTrackingUntranslatableAnswer, err)
	}
	version, err := vedomain.NewSourceFactVersion(fact.Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: version: %v", ErrExternalTrackingUntranslatableAnswer, err)
	}
	kind, err := vedomain.NewSourceFactKind(externalCarrierTrackingKind)
	if err != nil {
		return none, fmt.Errorf("%w: source fact kind: %v", ErrExternalTrackingUntranslatableAnswer, err)
	}
	// 替代关系由源上下文给出，VE 只登记不裁决：TF 的版本链上每一版回指前版（源更正或本仓判断），
	// 这里原样译进 Supersedes。前版若是待判断、从未到过 VE，指名一个不在场的前身无害——
	// CurrentlyEffective 按在场集合派生。
	var supersedes vedomain.SourceFactVersion
	if prior, has := fact.Supersedes(); has {
		supersedes, err = vedomain.NewSourceFactVersion(prior.String())
		if err != nil {
			return none, fmt.Errorf("%w: superseded version: %v", ErrExternalTrackingUntranslatableAnswer, err)
		}
	}
	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source:      vedomain.SourceTransportFulfillment,
			Parcel:      parcel,
			Fact:        reference,
			Kind:        kind,
			Version:     version,
			Supersedes:  supersedes,
			OccurredAt:  fact.OccurredAt(),
			EffectiveAt: effectiveAt,
			ReceivedAt:  fact.ReceivedAt(),
		},
	}, nil
}

func externalTrackingKeyFor(judged veinbox.JudgedExternalTracking) (tfports.ExternalTrackingFactKey, error) {
	none := tfports.ExternalTrackingFactKey{}
	tenant, err := tfdomain.NewTenantID(judged.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrExternalTrackingUntranslatableAnswer, err)
	}
	fact, err := tfdomain.NewExternalTrackingFactReference(judged.Fact)
	if err != nil {
		return none, fmt.Errorf("%w: fact: %v", ErrExternalTrackingUntranslatableAnswer, err)
	}
	version, err := tfdomain.NewExternalTrackingFactVersion(judged.Version)
	if err != nil {
		return none, fmt.Errorf("%w: version: %v", ErrExternalTrackingUntranslatableAnswer, err)
	}
	return tfports.ExternalTrackingFactKey{TenantID: tenant, Fact: fact, Version: version}, nil
}
