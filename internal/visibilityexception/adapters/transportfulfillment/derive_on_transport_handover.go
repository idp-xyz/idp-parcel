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
	// ErrHandoverNotVisible 表示按信封引用还读不回交接判断登记。可见性滞后是续办，重投
	// 会改变结果；不当毒丸拒收。
	ErrHandoverNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: transport handover is not yet visible")
	// ErrHandoverRecordInconsistent 表示按键取回的登记指着另一个键、时间为零，或裁决
	// 落在封闭三值之外。仓储不变量已破（ADR-0029），不是等谁。禁止进 WithUndecidedSentinels。
	ErrHandoverRecordInconsistent = errors.New(
		"visibility exception transportfulfillment adapter: transport handover disagrees with its key")
	// ErrHandoverUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrHandoverUntranslatableAnswer = errors.New(
		"visibility exception transportfulfillment adapter: untranslatable transport handover answer")
	// ErrHandoverProjectionUndecided 与 ErrHandoverProjectionHandoffPending 是
	// veconsume 同名哨兵的交接侧别名。交付/揽收文件已占用短名，本文件不得再声明一份。
	ErrHandoverProjectionUndecided      = veconsume.ErrProjectionUndecided
	ErrHandoverProjectionHandoffPending = veconsume.ErrProjectionHandoffPending
)

// TransportHandoverFinder 按交接判断幂等键取回登记。由 TF 的 TransportHandoverRegistry
// 满足。只取一法：本适配器不写 TF 的库。
type TransportHandoverFinder interface {
	FindByKey(
		ctx context.Context,
		key tfports.TransportHandoverKey,
	) (tfports.TransportHandoverRecord, bool, error)
}

// HandoverProjectionHandler 是派生编排在交接适配器侧的窄口。名字带 Handover，避免
// 与同包交付/揽收文件撞符号。
type HandoverProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnTransportHandoverAdapter 是 veinbox.TransportHandoverConsumer 的真实处理方：
// 按四维引用重读 TF 交接判断，译成已接受源事实命令，再交给派生编排。
//
// 跨上下文翻译留在消费方（ADR-0025）。投影只引用源事实，不复制权威交接，也不把交接
// 投影当成 node-operations 的控制转出。
type DeriveOnTransportHandoverAdapter struct {
	handovers TransportHandoverFinder
	derive    HandoverProjectionHandler
}

func NewDeriveOnTransportHandoverAdapter(
	handovers TransportHandoverFinder,
	derive HandoverProjectionHandler,
) (*DeriveOnTransportHandoverAdapter, error) {
	if handovers == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: handover finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: projection handler is nil")
	}
	return &DeriveOnTransportHandoverAdapter{handovers: handovers, derive: derive}, nil
}

var _ veinbox.RegisteredTransportHandoverHandler = (*DeriveOnTransportHandoverAdapter)(nil)

// HandleRegisteredTransportHandover 按信封四维取回交接并派生投影。
//
// 信封只做唤醒指针：业务发生时间取 Handover.JudgedAt，有效时间同发生时间，接收时间
// 取记录 RecordedAt，禁止用信封 OccurredAt/RecordedAt 顶业务时间。FindByKey 必须带
// 版本——更正是新版本新登记，按三维键读「当前版」会把更正与原判断叠成一次查找。
func (adapter *DeriveOnTransportHandoverAdapter) HandleRegisteredTransportHandover(
	ctx context.Context,
	registered veinbox.RegisteredTransportHandover,
) error {
	key, err := handoverKeyFor(registered)
	if err != nil {
		return err
	}
	record, found, err := adapter.handovers.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHandoverNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: object %q scope %q version %q",
			ErrHandoverNotVisible, registered.Object, registered.Scope, registered.Version)
	}
	if record.Key != key ||
		record.Handover.Object() != key.Object ||
		record.Handover.Scope() != key.Scope ||
		record.Handover.Version() != key.Version ||
		record.Handover.TenantID() != key.TenantID {
		return fmt.Errorf("%w: object %q scope %q version %q",
			ErrHandoverRecordInconsistent, registered.Object, registered.Scope, registered.Version)
	}
	if record.Handover.JudgedAt().IsZero() || record.RecordedAt.IsZero() {
		return fmt.Errorf("%w: object %q scope %q version %q",
			ErrHandoverRecordInconsistent, registered.Object, registered.Scope, registered.Version)
	}

	command, err := handoverProjectionCommand(record)
	if err != nil {
		return err
	}
	result, err := adapter.derive.Handle(ctx, command)
	if err != nil {
		return err
	}
	return veconsume.Consumption(result)
}

func handoverKeyFor(registered veinbox.RegisteredTransportHandover) (tfports.TransportHandoverKey, error) {
	none := tfports.TransportHandoverKey{}
	tenant, err := tfdomain.NewTenantID(registered.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrHandoverUntranslatableAnswer, err)
	}
	object, err := tfdomain.NewCarriedObjectReference(registered.Object)
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrHandoverUntranslatableAnswer, err)
	}
	scope, err := tfdomain.NewHandoverScopeReference(registered.Scope)
	if err != nil {
		return none, fmt.Errorf("%w: handover scope: %v", ErrHandoverUntranslatableAnswer, err)
	}
	version, err := tfdomain.NewHandoverResultVersion(registered.Version)
	if err != nil {
		return none, fmt.Errorf("%w: handover version: %v", ErrHandoverUntranslatableAnswer, err)
	}
	return tfports.TransportHandoverKey{
		TenantID: tenant,
		Object:   object,
		Scope:    scope,
		Version:  version,
	}, nil
}

func handoverProjectionCommand(record tfports.TransportHandoverRecord) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(record.Handover.TenantID().String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrHandoverUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(record.Handover.Object().String())
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrHandoverUntranslatableAnswer, err)
	}
	// 前缀把交接事实键与同包揽收/交付错开；范围进引用，因为同一对象可有多次交接。
	fact, err := vedomain.NewSourceFactReference(
		"transport-handover/" + record.Handover.Object().String() + "/" + record.Handover.Scope().String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrHandoverUntranslatableAnswer, err)
	}
	version, err := vedomain.NewSourceFactVersion(record.Handover.Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: handover version: %v", ErrHandoverUntranslatableAnswer, err)
	}
	kind, err := kindForHandoverVerdict(record.Handover.Verdict())
	if err != nil {
		return none, err
	}
	occurred := record.Handover.JudgedAt()
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

// kindForHandoverVerdict 把封闭三值裁决译成三个映射类型字面量。已交接、拒收、待确认
// 语义不同，不能共用一个 transport-handover 类型——否则目录一行会把三格都归进同一
// 里程碑。落在封闭集合外是仓储不变量已破，不是译码失败。
func kindForHandoverVerdict(verdict tfdomain.HandoverVerdict) (vedomain.SourceFactKind, error) {
	var raw string
	switch verdict {
	case tfdomain.ObjectHandedOver:
		raw = "handover-handed-over"
	case tfdomain.HandoverRefused:
		raw = "handover-refused"
	case tfdomain.HandoverPendingConfirmation:
		raw = "handover-pending-confirmation"
	default:
		return vedomain.SourceFactKind{}, fmt.Errorf("%w: verdict", ErrHandoverRecordInconsistent)
	}
	kind, err := vedomain.NewSourceFactKind(raw)
	if err != nil {
		return vedomain.SourceFactKind{}, fmt.Errorf("%w: source fact kind: %v", ErrHandoverUntranslatableAnswer, err)
	}
	return kind, nil
}
