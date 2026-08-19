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
	// ErrExceptionJourneyNotVisible 表示按信封引用还读不回旅程启动登记。可见性滞后是
	// 续办，重投会改变结果；不当毒丸拒收。
	ErrExceptionJourneyNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: alternate journey is not yet visible")
	// ErrExceptionJourneyRecordInconsistent 表示按键取回的登记指着另一个键、时间为零
	// 或成员为空。仓储不变量已破（ADR-0029），不是等谁。禁止进 WithUndecidedSentinels。
	ErrExceptionJourneyRecordInconsistent = errors.New(
		"visibility exception transportfulfillment adapter: alternate journey disagrees with its key")
	// ErrExceptionJourneyUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrExceptionJourneyUntranslatableAnswer = errors.New(
		"visibility exception transportfulfillment adapter: untranslatable exception journey answer")
	// ErrJourneyProjectionUndecided 与 ErrJourneyProjectionHandoffPending 是 veconsume
	// 同名哨兵的旅程侧别名。交付/揽收/交接文件已占用短名，本文件不得再声明一份。
	ErrJourneyProjectionUndecided      = veconsume.ErrProjectionUndecided
	ErrJourneyProjectionHandoffPending = veconsume.ErrProjectionHandoffPending
)

// ExceptionJourneyFinder 按替代旅程幂等键取回启动登记。由 TF 的 AlternateJourneyStore
// 满足。只取一法：本适配器不写 TF 的库。
type ExceptionJourneyFinder interface {
	FindByKey(
		ctx context.Context,
		key tfports.AlternateJourneyKey,
	) (tfports.AlternateJourneyRecord, bool, error)
}

// JourneyProjectionHandler 是派生编排在旅程适配器侧的窄口。名字带 Journey，避免与
// 同包交付/揽收/交接文件撞符号。
type JourneyProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnExceptionJourneyAdapter 是 veinbox.ExceptionJourneyConsumer 的真实处理方：
// 按四维引用重读 TF 替代/退运旅程，对成员清单逐成员译成已接受源事实命令，交给派生
// 编排（ADR-0066 消费侧循环拆分）。
//
// 跨上下文翻译留在消费方（ADR-0025）。投影只引用源事实，不复制旅程本体；原旅程的
// 中断或继续由原对象自行形成，本适配器不译原旅程的任何状态。
type DeriveOnExceptionJourneyAdapter struct {
	journeys ExceptionJourneyFinder
	derive   JourneyProjectionHandler
}

func NewDeriveOnExceptionJourneyAdapter(
	journeys ExceptionJourneyFinder,
	derive JourneyProjectionHandler,
) (*DeriveOnExceptionJourneyAdapter, error) {
	if journeys == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: journey finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: projection handler is nil")
	}
	return &DeriveOnExceptionJourneyAdapter{journeys: journeys, derive: derive}, nil
}

var _ veinbox.RecordedExceptionJourneyHandler = (*DeriveOnExceptionJourneyAdapter)(nil)

// HandleRecordedExceptionJourney 按信封四维取回旅程，对成员清单逐成员派生投影。
//
// 一封信一笔事务：任一成员未决或意图未交即整封报错回滚、重投从头再跑（头端阻塞是
// ADR-0066 认下的代价，换整封原子）；单成员业务终局（冲突、不接受）入账继续——与
// 单对象信封同一口径，冲突在事实库有行。业务发生时间取旅程 StartedAt，有效时间同
// 发生时间，接收时间取记录 RecordedAt，禁止用信封时间顶业务时间。
func (adapter *DeriveOnExceptionJourneyAdapter) HandleRecordedExceptionJourney(
	ctx context.Context,
	recorded veinbox.RecordedExceptionJourney,
) error {
	key, err := exceptionJourneyKeyFor(recorded)
	if err != nil {
		return err
	}
	record, found, err := adapter.journeys.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrExceptionJourneyNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: original %q purpose %q basis %q",
			ErrExceptionJourneyNotVisible, recorded.Original, recorded.Purpose, recorded.Basis)
	}
	journey := record.Journey
	if record.Key != key ||
		journey.TenantID() != key.TenantID ||
		journey.OriginalJourney() != key.Original ||
		journey.Purpose() != key.Purpose ||
		journey.Basis() != key.Basis {
		return fmt.Errorf("%w: original %q purpose %q basis %q",
			ErrExceptionJourneyRecordInconsistent, recorded.Original, recorded.Purpose, recorded.Basis)
	}
	if journey.Journey().String() == "" || journey.StartedAt().IsZero() || record.RecordedAt.IsZero() {
		return fmt.Errorf("%w: original %q purpose %q basis %q",
			ErrExceptionJourneyRecordInconsistent, recorded.Original, recorded.Purpose, recorded.Basis)
	}
	members := journey.Members()
	if len(members) == 0 {
		return fmt.Errorf("%w: journey %q has no members",
			ErrExceptionJourneyRecordInconsistent, journey.Journey())
	}
	kind, err := kindForJourneyPurpose(journey.Purpose())
	if err != nil {
		return err
	}

	for _, member := range members {
		command, err := exceptionJourneyProjectionCommand(record, member, kind)
		if err != nil {
			return err
		}
		result, err := adapter.derive.Handle(ctx, command)
		if err != nil {
			return err
		}
		if err := veconsume.Consumption(result); err != nil {
			return err
		}
	}
	return nil
}

func exceptionJourneyKeyFor(recorded veinbox.RecordedExceptionJourney) (tfports.AlternateJourneyKey, error) {
	none := tfports.AlternateJourneyKey{}
	tenant, err := tfdomain.NewTenantID(recorded.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrExceptionJourneyUntranslatableAnswer, err)
	}
	original, err := tfdomain.NewJourneyReference(recorded.Original)
	if err != nil {
		return none, fmt.Errorf("%w: original journey: %v", ErrExceptionJourneyUntranslatableAnswer, err)
	}
	purpose, err := journeyPurposeFrom(recorded.Purpose)
	if err != nil {
		return none, err
	}
	basis, err := tfdomain.NewDispositionBasisReference(recorded.Basis)
	if err != nil {
		return none, fmt.Errorf("%w: disposition basis: %v", ErrExceptionJourneyUntranslatableAnswer, err)
	}
	return tfports.AlternateJourneyKey{
		TenantID: tenant,
		Original: original,
		Purpose:  purpose,
		Basis:    basis,
	}, nil
}

// journeyPurposeFrom 把载荷里的目的字面量译回封闭二值。落在集合外是信封坏了，不是等谁。
func journeyPurposeFrom(raw string) (tfdomain.JourneyPurpose, error) {
	switch raw {
	case tfdomain.AlternateJourneyPurpose.String():
		return tfdomain.AlternateJourneyPurpose, nil
	case tfdomain.ReturnJourneyPurpose.String():
		return tfdomain.ReturnJourneyPurpose, nil
	default:
		return tfdomain.JourneyPurposeInvalid, fmt.Errorf("%w: purpose %q",
			ErrExceptionJourneyUntranslatableAnswer, raw)
	}
}

func exceptionJourneyProjectionCommand(
	record tfports.AlternateJourneyRecord,
	member tfdomain.CarriedObjectReference,
	kind vedomain.SourceFactKind,
) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(record.Key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrExceptionJourneyUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(member.String())
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrExceptionJourneyUntranslatableAnswer, err)
	}
	// 成员维进引用不进类型（ADR-0066）：缺成员维，同一封信的第二个成员就撞同键异
	// 内容的来源冲突并被当业务终局静默吞掉。键三维随后，把本事实与同对象在别的
	// 原旅程/目的/依据下的旅程事实错开。
	fact, err := vedomain.NewSourceFactReference(
		"exception-journey/" + member.String() + "/" + record.Key.Original.String() +
			"/" + record.Key.Purpose.String() + "/" + record.Key.Basis.String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrExceptionJourneyUntranslatableAnswer, err)
	}
	// 版本取新旅程引用：旅程启动没有版本演进（无更正入口），同键即同旅程；若同键
	// 将来真出现另一条旅程身份，版本维会把两代分开而不是相互覆盖。
	version, err := vedomain.NewSourceFactVersion(record.Journey.Journey().String())
	if err != nil {
		return none, fmt.Errorf("%w: journey reference: %v", ErrExceptionJourneyUntranslatableAnswer, err)
	}
	occurred := record.Journey.StartedAt()
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

// kindForJourneyPurpose 把封闭二值目的译成两个映射类型字面量。改送与退运语义不同，
// 不能共用一个 exception-journey 类型——目录一行会把两格归进同一里程碑（ADR-0066：
// 分格依据是语义分支，从不是对象身份）。决定来源（普通/监管）是来路不是旅程语义，
// 不进类型；监管来路的核对走 CC 处置执行链。落在封闭集合外是仓储不变量已破。
func kindForJourneyPurpose(purpose tfdomain.JourneyPurpose) (vedomain.SourceFactKind, error) {
	var raw string
	switch purpose {
	case tfdomain.AlternateJourneyPurpose:
		raw = "exception-journey-alternate"
	case tfdomain.ReturnJourneyPurpose:
		raw = "exception-journey-return"
	default:
		return vedomain.SourceFactKind{}, fmt.Errorf("%w: purpose", ErrExceptionJourneyRecordInconsistent)
	}
	kind, err := vedomain.NewSourceFactKind(raw)
	if err != nil {
		return vedomain.SourceFactKind{}, fmt.Errorf("%w: source fact kind: %v",
			ErrExceptionJourneyUntranslatableAnswer, err)
	}
	return kind, nil
}
