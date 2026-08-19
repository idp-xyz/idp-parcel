// Package parcelshipment 是 visibility-exception 消费 parcel-shipment 的适配器
// （ADR-0025 消费方侧）。它只翻译不判断：终局采用记录译成已接受源事实命令，归类与
// 派生由 DeriveProjectionHandler 回答；包裹反查答复译成货主客户账户引用，采认与
// 委托归属由 PS 回答。
//
// 终局投影是四路投影里唯一一条来源为 parcel-shipment 的：终局是 PS 拥有的事实，
// 不是履约侧事实。有效交付登记是它的**上游来源**，已有自己的投影账，两者各记各的。
package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var (
	// ErrFinalNotVisible 表示按信封引用还读不回终局采用登记。可见性滞后是续办，重投
	// 会改变结果；不当毒丸拒收。
	ErrFinalNotVisible = errors.New(
		"visibility exception parcelshipment adapter: final outcome is not yet visible")
	// ErrFinalRecordInconsistent 表示按键取回的登记指着另一个键，或采认时刻为零。
	// 仓储不变量已破（ADR-0029），不是等谁。禁止进 WithUndecidedSentinels。
	ErrFinalRecordInconsistent = errors.New(
		"visibility exception parcelshipment adapter: final outcome disagrees with its key")
	// ErrFinalUntranslatableAnswer 表示引用译不成领域标识。引用坏了，不是等谁。
	ErrFinalUntranslatableAnswer = errors.New(
		"visibility exception parcelshipment adapter: untranslatable final outcome answer")
	// ErrFinalProjectionUndecided 与 ErrFinalProjectionHandoffPending 是 veconsume
	// 同名哨兵的终局侧别名。
	ErrFinalProjectionUndecided      = veconsume.ErrProjectionUndecided
	ErrFinalProjectionHandoffPending = veconsume.ErrProjectionHandoffPending
)

// FinalOutcomeFinder 按终局采用幂等键取回登记。由 PS 的 FinalOutcomeStore 满足。
// 只取一法：本适配器不写 PS 的库。
type FinalOutcomeFinder interface {
	FindByKey(
		ctx context.Context,
		key psports.FinalAdoptionKey,
	) (psports.FinalOutcomeRecord, bool, error)
}

// FinalProjectionHandler 是派生编排在终局适配器侧的窄口。
type FinalProjectionHandler interface {
	Handle(
		ctx context.Context,
		command veapplication.DeriveProjectionCommand,
	) (veapplication.DeriveProjectionResult, error)
}

// DeriveOnFinalOutcomeAdapter 是 veinbox.FinalOutcomeConsumer 的真实处理方：按四维
// 引用重读 PS 终局采用判断，译成已接受源事实命令，再交给派生编排。
//
// 跨上下文翻译留在消费方（ADR-0025）。投影只引用源事实，不复制终局，也不回写终局。
type DeriveOnFinalOutcomeAdapter struct {
	finals FinalOutcomeFinder
	derive FinalProjectionHandler
}

func NewDeriveOnFinalOutcomeAdapter(
	finals FinalOutcomeFinder,
	derive FinalProjectionHandler,
) (*DeriveOnFinalOutcomeAdapter, error) {
	if finals == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: final outcome finder is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception parcelshipment adapter: projection handler is nil")
	}
	return &DeriveOnFinalOutcomeAdapter{finals: finals, derive: derive}, nil
}

var _ veinbox.FormedFinalOutcomeHandler = (*DeriveOnFinalOutcomeAdapter)(nil)

// HandleFormedFinalOutcome 按信封四维取回终局并派生投影。
//
// 两支都派生：不采用同样是已提交的终局判断，追踪要据它知道有一份来源没被采认（PS 侧
// FormParcelFinalHandler 的 handOff 注释就是这么定的）。省掉它会让迟到来源在追踪上
// 无声消失。
//
// FindByKey 必须带版本——更正走新版本新登记，按三维读「当前版」会把更正与原判断叠成
// 一次查找，重派生场景下静默拿错版本。
func (adapter *DeriveOnFinalOutcomeAdapter) HandleFormedFinalOutcome(
	ctx context.Context,
	formed veinbox.FormedFinalOutcome,
) error {
	key, err := finalKeyFor(formed)
	if err != nil {
		return err
	}
	record, found, err := adapter.finals.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFinalNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: parcel %q kind %q version %q",
			ErrFinalNotVisible, formed.Parcel, formed.Kind, formed.Version)
	}
	if record.Key != key {
		return fmt.Errorf("%w: parcel %q kind %q version %q",
			ErrFinalRecordInconsistent, formed.Parcel, formed.Kind, formed.Version)
	}
	// 采认时刻同时是本事实的业务时间，为零就没有可用的时间维——不是等谁，是那一行坏了。
	if record.AdoptedAt.IsZero() {
		return fmt.Errorf("%w: parcel %q kind %q version %q",
			ErrFinalRecordInconsistent, formed.Parcel, formed.Kind, formed.Version)
	}

	command, err := finalProjectionCommand(record)
	if err != nil {
		return err
	}
	result, err := adapter.derive.Handle(ctx, command)
	if err != nil {
		return err
	}
	return veconsume.Consumption(result)
}

// finalKeyFor 把信封四维译回采用键。种类是封闭枚举，反解析用 PS 领域包导出的那一份
// ——字符串形式在那里定义，本包再抄一份 switch 会让日后新增一个责任结果种类时无人
// 提醒。
func finalKeyFor(formed veinbox.FormedFinalOutcome) (psports.FinalAdoptionKey, error) {
	none := psports.FinalAdoptionKey{}
	tenant, err := psdomain.NewTenantID(formed.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrFinalUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(formed.Parcel)
	if err != nil {
		return none, fmt.Errorf("%w: parcel: %v", ErrFinalUntranslatableAnswer, err)
	}
	kind, err := psdomain.NewResponsibilityOutcomeKind(formed.Kind)
	if err != nil {
		return none, fmt.Errorf("%w: responsibility outcome kind: %v", ErrFinalUntranslatableAnswer, err)
	}
	version, err := psdomain.NewResponsibilityOutcomeVersion(formed.Version)
	if err != nil {
		return none, fmt.Errorf("%w: responsibility outcome version: %v", ErrFinalUntranslatableAnswer, err)
	}
	return psports.FinalAdoptionKey{
		TenantID: tenant,
		Parcel:   parcel,
		Kind:     kind,
		Version:  version,
	}, nil
}

func finalProjectionCommand(record psports.FinalOutcomeRecord) (veapplication.DeriveProjectionCommand, error) {
	none := veapplication.DeriveProjectionCommand{}
	tenant, err := vedomain.NewTenantID(record.Key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrFinalUntranslatableAnswer, err)
	}
	parcel, err := vedomain.NewTrackedParcelReference(record.Key.Parcel.String())
	if err != nil {
		return none, fmt.Errorf("%w: parcel: %v", ErrFinalUntranslatableAnswer, err)
	}
	// 前缀把终局事实键与同包揽收/交付/交接错开；责任结果种类进引用，因为同一包裹可有
	// 多条彼此独立的终局流（各自的来源责任不同）。版本不进引用——它已经是版本维，
	// 一维一职。
	fact, err := vedomain.NewSourceFactReference(
		"final-outcome/" + record.Key.Parcel.String() + "/" + record.Key.Kind.String())
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrFinalUntranslatableAnswer, err)
	}
	version, err := vedomain.NewSourceFactVersion(record.Key.Version.String())
	if err != nil {
		return none, fmt.Errorf("%w: responsibility outcome version: %v", ErrFinalUntranslatableAnswer, err)
	}
	kind, err := kindForFinalRecord(record)
	if err != nil {
		return none, err
	}

	// 采认判定本身就是这份事实的业务时刻——同 JudgedAt 之于交接，不是拿处理时间顶替
	// 业务时间。AdoptedAt 由 PS 应用层的 Clock 端口记录，不是库默认时间戳。
	occurred := record.AdoptedAt
	// 有效时间两支分开取，这是四路里第一条 EffectiveAt 可以不等于 OccurredAt 的：
	// 已采认支的源侧给得出第三时间（被采用责任结果的业务时间），翻译一步把它塌成采认
	// 时刻等于丢掉源真值；不采用支没有 Final 对象因而给不出，只能同 OccurredAt。
	effective := occurred
	if record.Finalized {
		effective = record.Final.EffectiveAt()
	}
	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source:      vedomain.SourceParcelShipment,
			Parcel:      parcel,
			Fact:        fact,
			Kind:        kind,
			Version:     version,
			OccurredAt:  occurred,
			EffectiveAt: effective,
			ReceivedAt:  record.AdoptedAt,
		},
	}, nil
}

// kindForFinalRecord 把采用/不采用两格译成两个映射类型字面量。已采认与未采认语义
// 相反，不能共用一个 final-outcome 类型——否则目录一行会把两格都归进同一里程碑
// （同 kindForHandoverVerdict 的理由）。
//
// 分格止于 Finalized：终局本体上的 FinalKindReference 属实例半边（租户合同定义
// 终局类型目录），把它炼进字面量等于让机制半边依赖一份还不存在的实例值。它作为不透明
// 身份串留在终局本体里，不进这里。
func kindForFinalRecord(record psports.FinalOutcomeRecord) (vedomain.SourceFactKind, error) {
	raw := "final-outcome-not-adopted"
	if record.Finalized {
		raw = "final-outcome-adopted"
	}
	kind, err := vedomain.NewSourceFactKind(raw)
	if err != nil {
		return vedomain.SourceFactKind{}, fmt.Errorf("%w: source fact kind: %v", ErrFinalUntranslatableAnswer, err)
	}
	return kind, nil
}
