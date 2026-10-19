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
	// ErrMovementFactNotVisible 表示按信封键还读不回移动事实。可见性滞后是续办，重投会改变结果；不当毒丸拒收。
	ErrMovementFactNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: movement fact is not yet visible")
	// ErrMovementSegmentNotVisible 表示事实声明的段那一本册读不动。段在册与否要读得出来才说得清，读不动是依赖的事，续办格。
	ErrMovementSegmentNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: the segment declared by the movement fact cannot be read")
	// ErrMovementFactRecordInconsistent 表示按键取回的记录指着另一个键、时间为零、种类落在封闭三值之外，或段册按键交回了
	// 另一个段。仓储不变量已破（ADR-0029），不是等谁。禁止进 WithUndecidedSentinels。
	ErrMovementFactRecordInconsistent = errors.New(
		"visibility exception transportfulfillment adapter: movement fact disagrees with its key")
	// ErrMovementFactUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrMovementFactUntranslatableAnswer = errors.New(
		"visibility exception transportfulfillment adapter: untranslatable movement fact answer")
	// ErrDepartureGateSignalUndecided 表示阻断信号那一步搁置了：本租户的信号规则读不动，或分诊那一侧停在未决。整封回滚重放，
	// 投影与信号同生共死——只派生投影、把「这次没交进分诊」吞成入账，那一次违规出发就再也不会形成信号。
	ErrDepartureGateSignalUndecided = errors.New(
		"visibility exception transportfulfillment adapter: the departure gate signal is undecided")
	// ErrDepartureGateSignalHandoffPending 表示分诊结论已落库但意图还没交出去。不要进 WithUndecidedSentinels——运维要查的是
	// outbox 下游，不是信号规则册。
	ErrDepartureGateSignalHandoffPending = errors.New(
		"visibility exception transportfulfillment adapter: the departure gate signal triage handoff is still pending")
)

// MovementFactFinder 按（租户、事实、版本）取回移动事实。由 TF 的 MovementFactRegistry 满足。只取一法：本适配器不写 TF 的库。
type MovementFactFinder interface {
	FindByKey(ctx context.Context, key tfports.MovementFactKey) (tfports.MovementFactRecord, bool, error)
}

// MovementSegmentFinder 按键取回事实声明的那一段连同全部参与。由 TF 的 FulfillmentSegments 满足。
type MovementSegmentFinder interface {
	FindByKey(ctx context.Context, key tfports.FulfillmentSegmentKey) (tfports.FulfillmentSegmentRecord, bool, error)
}

// MovementProjectionHandler 是派生编排在移动事实适配器侧的窄口。名字带 Movement，避免与同包各文件撞符号。
type MovementProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DepartureGateSignalRaiser 是阻断信号编排的窄口。规则视图按租户绑定，所以装配处每次按命令上的租户现绑。
type DepartureGateSignalRaiser interface {
	Handle(
		ctx context.Context,
		command veapplication.RaiseDepartureGateSignalCommand,
	) (veapplication.RaiseDepartureGateSignalResult, error)
}

// DeriveOnMovementFactAdapter 是 veinbox.MovementFactConsumer 的真实处理方：按键重读 TF 移动事实，沿它声明的段取出那一刻在控
// 的对象，逐对象派生投影（ADR-0066 消费侧循环拆分）；出发读数里被关务判为阻断的那几件，各形成一次异常信号（ADR-0205 决定五）。
//
// 跨上下文翻译留在消费方（ADR-0025）。投影只引用源事实：不复制门禁读数，也不把出发当成控制转移——移动不结束段，控制只随
// 交接与交付转移。种类词、展开口径与信号的取舍在 ADR-0205 文末补记。
type DeriveOnMovementFactAdapter struct {
	facts    MovementFactFinder
	segments MovementSegmentFinder
	derive   MovementProjectionHandler
	signals  DepartureGateSignalRaiser
}

func NewDeriveOnMovementFactAdapter(
	facts MovementFactFinder,
	segments MovementSegmentFinder,
	derive MovementProjectionHandler,
	signals DepartureGateSignalRaiser,
) (*DeriveOnMovementFactAdapter, error) {
	if facts == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: movement fact finder is nil")
	}
	if segments == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: segment finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: projection handler is nil")
	}
	if signals == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: departure gate signal raiser is nil")
	}
	return &DeriveOnMovementFactAdapter{facts: facts, segments: segments, derive: derive, signals: signals}, nil
}

var _ veinbox.RecordedMovementFactHandler = (*DeriveOnMovementFactAdapter)(nil)

// HandleRecordedMovementFact 按信封键取回移动事实，对段里那一刻在控的对象逐个派生投影，再为阻断的那几件形成信号。
//
// 一封信一笔事务：任一对象未决、意图未交或信号搁置即整封报错回滚、重投从头再跑（ADR-0066 认下的头端阻塞）；单对象业务终局
// （冲突、不接受）入账继续。业务发生时间取事实的 OccurredAt，有效时间同发生时间，接收时间取 RecordedAt，不用信封时间。
func (adapter *DeriveOnMovementFactAdapter) HandleRecordedMovementFact(
	ctx context.Context,
	recorded veinbox.RecordedMovementFact,
) error {
	key, err := movementFactKeyFor(recorded)
	if err != nil {
		return err
	}
	record, found, err := adapter.facts.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMovementFactNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: fact %q version %q", ErrMovementFactNotVisible, recorded.Fact, recorded.Version)
	}
	fact := record.Fact
	if record.Key != key || fact.TenantID() != key.TenantID || fact.Fact() != key.Fact || fact.Version() != key.Version ||
		fact.OccurredAt().IsZero() || record.RecordedAt.IsZero() {
		return fmt.Errorf("%w: fact %q version %q", ErrMovementFactRecordInconsistent, recorded.Fact, recorded.Version)
	}
	kind, err := kindForMovementFact(fact.Kind())
	if err != nil {
		return err
	}

	objects, err := adapter.objectsOf(ctx, fact)
	if err != nil {
		return err
	}
	for _, object := range objects {
		command, err := movementProjectionCommand(record, kind, object)
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
	return adapter.signalBlocked(ctx, fact)
}

// objectsOf 取事实声明的段里、事实发生那一刻在控的对象——与出发门禁读数问关务时同一条口径（ADR-0205 决定四，口径只在
// ActualFulfillmentSegment.ObjectsUnderControlAt 定义一处）。没声明段、段不在册、那一刻段里没有在控对象，都交回空：事实照样
// 入账，只是没有可投的对象。段不在册不等它：事实本就可以先于段登记（ADR-0205 决定四），这封信等不来段；段日后登了也不会
// 回头补投——这一格在补记的越权风险点里。
func (adapter *DeriveOnMovementFactAdapter) objectsOf(
	ctx context.Context,
	fact tfdomain.TransportMovementFact,
) ([]tfdomain.CarriedObjectReference, error) {
	segment, declared := fact.Segment()
	if !declared {
		return nil, nil
	}
	segmentKey := tfports.FulfillmentSegmentKey{TenantID: fact.TenantID(), Segment: segment}
	record, found, err := adapter.segments.FindByKey(ctx, segmentKey)
	if err != nil {
		return nil, fmt.Errorf("%w: segment %q: %v", ErrMovementSegmentNotVisible, segment, err)
	}
	if !found {
		return nil, nil
	}
	if record.Key != segmentKey || record.Segment.Segment() != segment || record.Segment.TenantID() != fact.TenantID() {
		return nil, fmt.Errorf("%w: segment %q", ErrMovementFactRecordInconsistent, segment)
	}
	return record.Segment.ObjectsUnderControlAt(fact.OccurredAt()), nil
}

// signalBlocked 为出发读数里关务判为阻断的每一件形成一次信号（ADR-0205 决定五）。只看逐对象那一格，不看整格：整格是压过的
// 结论，阻断只可能来自某一件，信号也只该落在那一件上。
//
// 更正版不再形成：领域的更正版沿用原登记那一刻的读数（ADR-0205 越权风险点 8），同一次出发、同一份读数，原版已经命中过一次，
// 再命中一次只会把同一件事在分诊里记成两回。
func (adapter *DeriveOnMovementFactAdapter) signalBlocked(ctx context.Context, fact tfdomain.TransportMovementFact) error {
	reading, read := fact.GateReading()
	if !read || reading.Answer() != tfdomain.DepartureGateBlocked {
		return nil
	}
	if _, corrected := fact.Corrects(); corrected {
		return nil
	}
	tenant, err := vedomain.NewTenantID(fact.TenantID().String())
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	for _, finding := range reading.Findings() {
		if finding.Answer() != tfdomain.DepartureGateBlocked {
			continue
		}
		parcel, err := vedomain.NewTrackedParcelReference(finding.Object().String())
		if err != nil {
			return fmt.Errorf("%w: blocked object: %v", ErrMovementFactUntranslatableAnswer, err)
		}
		result, err := adapter.signals.Handle(ctx, veapplication.RaiseDepartureGateSignalCommand{TenantID: tenant, Parcel: parcel})
		if err != nil {
			return err
		}
		switch result.Outcome() {
		case veapplication.DepartureGateSignalRaised:
			if result.HandoffReference() != "" {
				return fmt.Errorf("%w: %s", ErrDepartureGateSignalHandoffPending, result.HandoffReference())
			}
		case veapplication.DepartureGateSignalRuleNotConfigured:
			// 租户没登记这一类信号规则：如实不形成，入账。等不来——规则登记不会回头触发这封信。
		case veapplication.DepartureGateSignalDeferred:
			return fmt.Errorf("%w: object %q", ErrDepartureGateSignalUndecided, finding.Object())
		default:
			return fmt.Errorf("%w: %q", veapplication.ErrUnexpectedDepartureGateSignalOutcome, result.Outcome())
		}
	}
	return nil
}

func movementFactKeyFor(recorded veinbox.RecordedMovementFact) (tfports.MovementFactKey, error) {
	none := tfports.MovementFactKey{}
	tenant, err := tfdomain.NewTenantID(recorded.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	fact, err := tfdomain.NewMovementFactReference(recorded.Fact)
	if err != nil {
		return none, fmt.Errorf("%w: movement fact: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	version, err := tfdomain.NewMovementFactVersion(recorded.Version)
	if err != nil {
		return none, fmt.Errorf("%w: movement fact version: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	return tfports.MovementFactKey{TenantID: tenant, Fact: fact, Version: version}, nil
}

func movementProjectionCommand(
	record tfports.MovementFactRecord,
	kind vedomain.SourceFactKind,
	object tfdomain.CarriedObjectReference,
) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	fact := record.Fact
	tenant, err := vedomain.NewTenantID(fact.TenantID().String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(object.String())
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	// 对象维进引用（ADR-0066 决定二）：一条移动事实在每件对象上各是一份事实；前缀把它与同包交接、揽收、交付错开。
	reference, err := vedomain.NewSourceFactReference("movement-fact/" + object.String() + "/" + fact.Fact().String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	version, err := vedomain.NewSourceFactVersion(fact.Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: movement fact version: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	// 来源事实替代关系由源上下文给出，VE 只登记不裁决：更正在 TF 侧以 Corrects() 回指前版，原样译进 Supersedes，理由同交接那一份。
	var supersedes vedomain.SourceFactVersion
	if predecessor, corrected := fact.Corrects(); corrected {
		if supersedes, err = vedomain.NewSourceFactVersion(predecessor.String()); err != nil {
			return none, fmt.Errorf("%w: superseded movement fact version: %v", ErrMovementFactUntranslatableAnswer, err)
		}
	}
	occurred := fact.OccurredAt()
	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source:      vedomain.SourceTransportFulfillment,
			Parcel:      parcel,
			Fact:        reference,
			Kind:        kind,
			Version:     version,
			Supersedes:  supersedes,
			OccurredAt:  occurred,
			EffectiveAt: occurred,
			ReceivedAt:  record.RecordedAt,
		},
	}, nil
}

// kindForMovementFact 把封闭三值种类译成三个映射类型字面量。出发、移动、到达语义不同，共用一个类型词会让目录一行把三格都
// 归进同一里程碑（与 kindForHandoverVerdict 同一条道理）。读数不进类型词：被阻断的出发也是一次出发，阻断由信号表达，不靠
// 映射目录把它归成另一个里程碑。落在封闭集合外是仓储不变量已破，不是译码失败。
func kindForMovementFact(kind tfdomain.MovementFactKind) (vedomain.SourceFactKind, error) {
	var raw string
	switch kind {
	case tfdomain.DepartureFact:
		raw = "movement-departure"
	case tfdomain.InTransitFact:
		raw = "movement-in-transit"
	case tfdomain.ArrivalFact:
		raw = "movement-arrival"
	default:
		return vedomain.SourceFactKind{}, fmt.Errorf("%w: kind", ErrMovementFactRecordInconsistent)
	}
	sourceKind, err := vedomain.NewSourceFactKind(raw)
	if err != nil {
		return vedomain.SourceFactKind{}, fmt.Errorf("%w: source fact kind: %v", ErrMovementFactUntranslatableAnswer, err)
	}
	return sourceKind, nil
}
