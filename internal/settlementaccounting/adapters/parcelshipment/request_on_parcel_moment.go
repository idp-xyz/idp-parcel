package parcelshipment

import (
	"context"
	"errors"
	"fmt"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var (
	// ErrIntakeAdoptionNotVisible 表示按信封所指的采用键还读不回那一份采用记录。采用记录与信封在提供方同一事务落库，
	// 同库同进程下读不回只剩可见性滞后，重投会改变结果。生产装配把它登进 WithUndecidedSentinels。
	ErrIntakeAdoptionNotVisible = errors.New(
		"settlement accounting parcelshipment adapter: intake adoption is not yet visible")
	// ErrFinalOutcomeNotVisible 是终局那一路的同一格。
	ErrFinalOutcomeNotVisible = errors.New(
		"settlement accounting parcelshipment adapter: final outcome is not yet visible")
	// ErrSellEvaluationRequestUndecided 表示发起编排停在自己的未决上（链上某一口、采用册、请求册、铸造口或交接口不可用）。
	// 同样是续办，同样进 WithUndecidedSentinels；编排的未决原因留在错误正文里，运维据它去查那一口。
	ErrSellEvaluationRequestUndecided = errors.New(
		"settlement accounting parcelshipment adapter: requesting the sell evaluation is undecided")
	// ErrAdoptionRecordInconsistent 表示按键取回的记录指着另一个键、或业务时刻为零。提供方仓储不变量已破，不是等谁；
	// 禁止进 WithUndecidedSentinels（ADR-0029）。
	ErrAdoptionRecordInconsistent = errors.New(
		"settlement accounting parcelshipment adapter: adoption record disagrees with its key")
	// ErrUnexpectedSellEvaluationRequestOutcome 表示编排交回了封闭集合以外的结果。静默入账等于替编排作判断。
	ErrUnexpectedSellEvaluationRequestOutcome = errors.New(
		"settlement accounting parcelshipment adapter: unexpected sell evaluation request outcome")
)

// SellEvaluationRequester 是发起编排的那一口。真实装配接 saapplication.RequestSellEvaluationOnParcelMomentHandler。
type SellEvaluationRequester interface {
	Handle(
		ctx context.Context,
		command saapplication.RequestSellEvaluationOnParcelMomentCommand,
	) (saapplication.RequestSellEvaluationOnParcelMomentResult, error)
}

// IntakeAdoptionSource 按采用键整行取回采用记录，由 PS 的 *pspostgres.IntakeAdoptions 满足；只声明读的这一口。
type IntakeAdoptionSource interface {
	FindByKey(ctx context.Context, key psports.IntakeAdoptionKey) (psports.IntakeAdoptionRecord, bool, error)
}

// FinalOutcomeSource 按终局采用键整行取回终局记录，由 PS 的 *pspostgres.FinalOutcomes 满足；只声明读的这一口。
type FinalOutcomeSource interface {
	FindByKey(ctx context.Context, key psports.FinalAdoptionKey) (psports.FinalOutcomeRecord, bool, error)
}

// RequestOnNetworkIntakeAdapter 是 sainbox.NetworkIntakeRecordedConsumer 的真实处理方：按信封四维回读采用记录，
// 只有`采用`那一格算`有效网络收寄已采用`这一拍，译成包裹计费时点交发起编排（ADR-0190 决定二）。
type RequestOnNetworkIntakeAdapter struct {
	adoptions IntakeAdoptionSource
	requests  SellEvaluationRequester
}

func NewRequestOnNetworkIntakeAdapter(
	adoptions IntakeAdoptionSource,
	requests SellEvaluationRequester,
) (*RequestOnNetworkIntakeAdapter, error) {
	if adoptions == nil {
		return nil, fmt.Errorf("settlement accounting parcelshipment adapter: intake adoption source is nil")
	}
	if requests == nil {
		return nil, fmt.Errorf("settlement accounting parcelshipment adapter: sell evaluation requester is nil")
	}
	return &RequestOnNetworkIntakeAdapter{adoptions: adoptions, requests: requests}, nil
}

var _ sainbox.RecordedNetworkIntakeHandler = (*RequestOnNetworkIntakeAdapter)(nil)

// HandleRecordedNetworkIntake 回读 → 不采用的略过 → 交编排 → 把结果落成消费两格。
//
// 计价基准时点取这一份采用的责任起点（被采用来源的实际发生时间），不取采认时刻也不取信封时刻：责任起点是包裹在
// 网络里开始计费的那一刻，处理时间替代不了它。同一件包裹的更正采用会再来一封，自然键（包裹、时点）让它答`已存在`，
// 不重算（ADR-0190 越权风险点 5）。
func (adapter *RequestOnNetworkIntakeAdapter) HandleRecordedNetworkIntake(
	ctx context.Context,
	recorded sainbox.RecordedNetworkIntake,
) error {
	key, err := intakeAdoptionKeyOf(recorded)
	if err != nil {
		return err
	}
	record, found, err := adapter.adoptions.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrIntakeAdoptionNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: parcel %q kind %q version %q",
			ErrIntakeAdoptionNotVisible, recorded.Parcel, recorded.Kind, recorded.Version)
	}
	if record.Key != key {
		return fmt.Errorf("%w: intake adoption parcel %q kind %q version %q",
			ErrAdoptionRecordInconsistent, recorded.Parcel, recorded.Kind, recorded.Version)
	}
	if !record.Adopted {
		// `不采用`没有责任起点，不是`有效网络收寄已采用`。它是提供方的终局答案，入账收工。
		return nil
	}
	start := record.Intake.ResponsibilityStart()
	if start.IsZero() {
		return fmt.Errorf("%w: adopted intake for parcel %q has no responsibility start",
			ErrAdoptionRecordInconsistent, recorded.Parcel)
	}
	command, err := parcelMomentCommand(key.TenantID, key.Parcel, sadomain.BillingMomentNetworkIntakeAdopted,
		"network-intake/"+key.Parcel.String()+"/"+key.Kind.String()+"/"+key.Version.String(), start)
	if err != nil {
		return err
	}
	result, err := adapter.requests.Handle(ctx, command)
	if err != nil {
		return err
	}
	return sellRequestConsumption(result)
}

// RequestOnFinalOutcomeAdapter 是 sainbox.FinalOutcomeFormedConsumer 的真实处理方：按信封四维回读终局记录，只有已终局
// 的「有效交付」算`有效交付已终局`这一拍。
type RequestOnFinalOutcomeAdapter struct {
	finals   FinalOutcomeSource
	requests SellEvaluationRequester
}

func NewRequestOnFinalOutcomeAdapter(
	finals FinalOutcomeSource,
	requests SellEvaluationRequester,
) (*RequestOnFinalOutcomeAdapter, error) {
	if finals == nil {
		return nil, fmt.Errorf("settlement accounting parcelshipment adapter: final outcome source is nil")
	}
	if requests == nil {
		return nil, fmt.Errorf("settlement accounting parcelshipment adapter: sell evaluation requester is nil")
	}
	return &RequestOnFinalOutcomeAdapter{finals: finals, requests: requests}, nil
}

var _ sainbox.FormedFinalOutcomeHandler = (*RequestOnFinalOutcomeAdapter)(nil)

// HandleFormedFinalOutcome 回读 → 只认已终局的有效交付 → 交编排 → 落成消费两格。
//
// 「有效交付」认的是终局所采用的责任结果种类（小包托运的封闭集合），不认终局类型引用：后者是合同规则给的开放引用，
// 归商业规则的实例半边，拿它判会把一个租户的命名当成产品口径。退运完成、服务终止、监管处置与面单渠道的终局都不是
// 这一拍（ADR-0190 越权风险点 8）。计价基准时点取终局的生效时点——被采用责任结果的业务时间，不是采认时刻。
func (adapter *RequestOnFinalOutcomeAdapter) HandleFormedFinalOutcome(
	ctx context.Context,
	formed sainbox.FormedFinalOutcome,
) error {
	key, err := finalAdoptionKeyOf(formed)
	if err != nil {
		return err
	}
	record, found, err := adapter.finals.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrFinalOutcomeNotVisible, err)
	}
	if !found {
		return fmt.Errorf("%w: parcel %q kind %q version %q",
			ErrFinalOutcomeNotVisible, formed.Parcel, formed.Kind, formed.Version)
	}
	if record.Key != key {
		return fmt.Errorf("%w: final outcome parcel %q kind %q version %q",
			ErrAdoptionRecordInconsistent, formed.Parcel, formed.Kind, formed.Version)
	}
	if !record.Finalized || record.Key.Kind != psdomain.EffectiveDeliveryOutcome {
		return nil
	}
	effective := record.Final.EffectiveAt()
	if effective.IsZero() {
		return fmt.Errorf("%w: final outcome for parcel %q has no effective time",
			ErrAdoptionRecordInconsistent, formed.Parcel)
	}
	command, err := parcelMomentCommand(key.TenantID, key.Parcel, sadomain.BillingMomentEffectiveDeliveryFinalized,
		"final-outcome/"+key.Parcel.String()+"/"+key.Kind.String()+"/"+key.Version.String(), effective)
	if err != nil {
		return err
	}
	result, err := adapter.requests.Handle(ctx, command)
	if err != nil {
		return err
	}
	return sellRequestConsumption(result)
}

// parcelMomentCommand 把提供方的键译成发起命令。来源事实引用取采用键：前缀把收寄采用与终局两种键错开，种类与版本都
// 进引用——同一件包裹会有不同来源类型、不同版本的多份采用，缺一维就指不回是哪一份。租户不进引用，它在请求册键上。
func parcelMomentCommand(
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
	moment sadomain.ParcelBillingMoment,
	source string,
	basis time.Time,
) (saapplication.RequestSellEvaluationOnParcelMomentCommand, error) {
	none := saapplication.RequestSellEvaluationOnParcelMomentCommand{}
	saTenant, err := sadomain.NewTenantID(tenant.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	saParcel, err := sadomain.NewParcelReference(parcel.String())
	if err != nil {
		return none, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	fact, err := sadomain.NewSourceFactReference(source)
	if err != nil {
		return none, fmt.Errorf("%w: source fact: %v", ErrUntranslatableAnswer, err)
	}
	return saapplication.RequestSellEvaluationOnParcelMomentCommand{
		TenantID:       saTenant,
		Parcel:         saParcel,
		Moment:         moment,
		Source:         fact,
		PricingBasisAt: basis,
	}, nil
}

// sellRequestConsumption 把编排结果落成消费两格，按「重投会不会改变结果」分：已发起、已存在、未配置、包裹无商业解析
// 都是终局答案，入账不重投（未配置等租户登记，补登之后也不回补，ADR-0190 越权风险点 3）；未决重投；未受理在这里
// 是本包译出的命令被拒，属适配器缺陷，响亮上抛不入账。
func sellRequestConsumption(result saapplication.RequestSellEvaluationOnParcelMomentResult) error {
	switch result.Outcome() {
	case saapplication.SellEvaluationRequested,
		saapplication.SellEvaluationRequestExisting,
		saapplication.SellEvaluationRequestUnconfigured,
		saapplication.SellEvaluationRequestParcelUnresolved:
		return nil
	case saapplication.SellEvaluationRequestUndecided:
		return fmt.Errorf("%w: %s", ErrSellEvaluationRequestUndecided, result.UndecidedReason())
	case saapplication.SellEvaluationRequestNotAccepted:
		return fmt.Errorf("%w: the command built from the adoption record was refused", ErrUntranslatableAnswer)
	default:
		return fmt.Errorf("%w: %q", ErrUnexpectedSellEvaluationRequestOutcome, result.Outcome())
	}
}

// intakeAdoptionKeyOf 把信封四维译回采用键。来源类型按提供方封闭集合逐格比对，集外报错不吸收（ADR-0025）。
func intakeAdoptionKeyOf(recorded sainbox.RecordedNetworkIntake) (psports.IntakeAdoptionKey, error) {
	none := psports.IntakeAdoptionKey{}
	tenant, err := psdomain.NewTenantID(recorded.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(recorded.Parcel)
	if err != nil {
		return none, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	version, err := psdomain.NewSourceResultVersion(recorded.Version)
	if err != nil {
		return none, fmt.Errorf("%w: source result version: %v", ErrUntranslatableAnswer, err)
	}
	for _, kind := range []psdomain.IntakeSourceKind{psdomain.NodeIntakeSource, psdomain.OffsitePickupSource} {
		if kind.String() == recorded.Kind {
			return psports.IntakeAdoptionKey{TenantID: tenant, Parcel: parcel, Kind: kind, Version: version}, nil
		}
	}
	return none, fmt.Errorf("%w: intake source kind %q", ErrUntranslatableAnswer, recorded.Kind)
}

// finalAdoptionKeyOf 把信封四维译回终局采用键。责任结果种类用提供方导出的反解析，字符串形式在哪定义就在哪反解。
func finalAdoptionKeyOf(formed sainbox.FormedFinalOutcome) (psports.FinalAdoptionKey, error) {
	none := psports.FinalAdoptionKey{}
	tenant, err := psdomain.NewTenantID(formed.TenantID)
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(formed.Parcel)
	if err != nil {
		return none, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	kind, err := psdomain.NewResponsibilityOutcomeKind(formed.Kind)
	if err != nil {
		return none, fmt.Errorf("%w: responsibility outcome kind: %v", ErrUntranslatableAnswer, err)
	}
	version, err := psdomain.NewResponsibilityOutcomeVersion(formed.Version)
	if err != nil {
		return none, fmt.Errorf("%w: responsibility outcome version: %v", ErrUntranslatableAnswer, err)
	}
	return psports.FinalAdoptionKey{TenantID: tenant, Parcel: parcel, Kind: kind, Version: version}, nil
}
