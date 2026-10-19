// request_sell_evaluation_on_parcel_moment.go 编排 ADR-0190 决定二、三的发起半边：一件包裹的计费时点到来时，按包裹走到
// 结算账户、查这一户采用的计费时点，相符才按自然键登记一份 SELL 评价请求，并在同一事务经 Outbox 交出只带
// （租户、请求标识）的信封（决定一）。选卡与形成不在这里：前者归 parcel-pricing，后者在评价已记录之后由 SELL 消费门做。
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SellEvaluationRequestOutcome 是一次「计费时点到来」的应用答案。`已存在`是业务答案不是失败：同一件包裹在所采时点
// 只发起一次（决定二），再来一次交回先到者。`未配置`与`包裹无商业解析`都不发起、调用方入账不重投，两格分开是因为
// 恢复动作不同：前者等租户登记，后者要去小包托运那一侧查这件包裹为什么不在任何已接受委托上。`未决`是依赖故障，重试。
type SellEvaluationRequestOutcome uint8

const (
	SellEvaluationRequestOutcomeInvalid SellEvaluationRequestOutcome = iota
	SellEvaluationRequested
	SellEvaluationRequestExisting
	SellEvaluationRequestUnconfigured
	SellEvaluationRequestParcelUnresolved
	SellEvaluationRequestNotAccepted
	SellEvaluationRequestUndecided
)

func (outcome SellEvaluationRequestOutcome) String() string {
	switch outcome {
	case SellEvaluationRequested:
		return "SELL_EVALUATION_REQUESTED"
	case SellEvaluationRequestExisting:
		return "EXISTING_SELL_EVALUATION_REQUEST"
	case SellEvaluationRequestUnconfigured:
		return "SELL_EVALUATION_REQUEST_UNCONFIGURED"
	case SellEvaluationRequestParcelUnresolved:
		return "PARCEL_HAS_NO_COMMERCIAL_RESOLUTION"
	case SellEvaluationRequestNotAccepted:
		return "SELL_EVALUATION_REQUEST_NOT_ACCEPTED"
	case SellEvaluationRequestUndecided:
		return "SELL_EVALUATION_REQUEST_UNDECIDED"
	default:
		return ""
	}
}

// SellEvaluationRequestUnconfiguredReason 指名`未配置`停在链的哪一格。各格都是租户取值：结算政策是解析键要不要的事，
// 账户与计费时点采用各是一本册。「采用了别的时点」也答`未配置`：这一户在这一拍上没有登记过要计费，不发起正是它
// 登的那一行的意思。
type SellEvaluationRequestUnconfiguredReason uint8

const (
	SellEvaluationRequestUnconfiguredReasonNone SellEvaluationRequestUnconfiguredReason = iota
	SellRequestSettlementPolicyNotAdopted
	SellRequestSettlementAccountNotRegistered
	SellRequestBillingMomentNotAdopted
	SellRequestBillingMomentDiffers
)

func (reason SellEvaluationRequestUnconfiguredReason) String() string {
	switch reason {
	case SellRequestSettlementPolicyNotAdopted:
		return "SETTLEMENT_POLICY_NOT_ADOPTED"
	case SellRequestSettlementAccountNotRegistered:
		return "SETTLEMENT_ACCOUNT_NOT_REGISTERED"
	case SellRequestBillingMomentNotAdopted:
		return "SELL_BILLING_MOMENT_NOT_ADOPTED"
	case SellRequestBillingMomentDiffers:
		return "SELL_BILLING_MOMENT_DIFFERS"
	default:
		return ""
	}
}

// SellEvaluationRequestUndecidedReason 指名`未决`停在哪一口。
type SellEvaluationRequestUndecidedReason uint8

const (
	SellEvaluationRequestUndecidedReasonNone SellEvaluationRequestUndecidedReason = iota
	SellRequestCommercialResolutionUnavailable
	SellRequestSettlementTermsUnavailable
	SellRequestSettlementAccountUnavailable
	SellRequestBillingMomentAdoptionUnavailable
	SellRequestRegistryUnavailable
	SellRequestIdentityUnavailable
	SellRequestHandoffUnavailable
)

func (reason SellEvaluationRequestUndecidedReason) String() string {
	switch reason {
	case SellRequestCommercialResolutionUnavailable:
		return "PARCEL_COMMERCIAL_RESOLUTION_UNAVAILABLE"
	case SellRequestSettlementTermsUnavailable:
		return "ADOPTED_SETTLEMENT_TERMS_UNAVAILABLE"
	case SellRequestSettlementAccountUnavailable:
		return "SETTLEMENT_ACCOUNT_REGISTER_UNAVAILABLE"
	case SellRequestBillingMomentAdoptionUnavailable:
		return "SELL_BILLING_MOMENT_ADOPTION_UNAVAILABLE"
	case SellRequestRegistryUnavailable:
		return "SELL_EVALUATION_REQUEST_REGISTRY_UNAVAILABLE"
	case SellRequestIdentityUnavailable:
		return "SELL_EVALUATION_REQUEST_IDENTITY_UNAVAILABLE"
	case SellRequestHandoffUnavailable:
		return "SELL_EVALUATION_REQUEST_HANDOFF_UNAVAILABLE"
	default:
		return ""
	}
}

// ErrUnexpectedSellEvaluationRequestSave 说明请求册交回了封闭集合以外的写入结果。
var ErrUnexpectedSellEvaluationRequestSave = errors.New("settlement accounting: unexpected sell evaluation request save outcome")

// RequestSellEvaluationOnParcelMomentCommand 是一拍包裹计费时点：哪件包裹、哪一种内置时点、凭哪件来源事实、以哪一刻
// 为计价基准。结算账户、结算币种与计价范围都不在命令上——它们由链与采用册答，调用方带得了它们就有了第二套口径。
type RequestSellEvaluationOnParcelMomentCommand struct {
	TenantID       domain.TenantID
	Parcel         domain.ParcelReference
	Moment         domain.ParcelBillingMoment
	Source         domain.SourceFactReference
	PricingBasisAt time.Time
}

type RequestSellEvaluationOnParcelMomentResult struct {
	outcome      SellEvaluationRequestOutcome
	unconfigured SellEvaluationRequestUnconfiguredReason
	undecided    SellEvaluationRequestUndecidedReason
	record       ports.SellEvaluationRequestRecord
	hasRecord    bool
}

func (result RequestSellEvaluationOnParcelMomentResult) Outcome() SellEvaluationRequestOutcome {
	return result.outcome
}

// UnconfiguredReason 只在`未配置`时非零。
func (result RequestSellEvaluationOnParcelMomentResult) UnconfiguredReason() SellEvaluationRequestUnconfiguredReason {
	return result.unconfigured
}

// UndecidedReason 只在`未决`时非零。
func (result RequestSellEvaluationOnParcelMomentResult) UndecidedReason() SellEvaluationRequestUndecidedReason {
	return result.undecided
}

// Request 在已发起与`已存在`两格交回册上那一份；`已存在`交回的是先到者。
func (result RequestSellEvaluationOnParcelMomentResult) Request() (ports.SellEvaluationRequestRecord, bool) {
	return result.record, result.hasRecord
}

// RequestSellEvaluationOnParcelMomentDeps 各口都必备：链的三段缺一段就走不到账户，采用册缺了就只能默认一种时点，
// 而请求册、铸造口与交接口是「登记并同事务交信封」的三件。
type RequestSellEvaluationOnParcelMomentDeps struct {
	Resolutions ports.ParcelCommercialResolutionView
	Terms       ports.AdoptedSettlementTermsView
	Accounts    ports.SettlementAccountView
	Adoptions   ports.SellBillingMomentAdoptionView
	Registry    ports.SellEvaluationRequestRegistry
	Identity    ports.SellEvaluationRequestIdentityFactory
	Downstream  ports.SellEvaluationRequestHandoff
	Clock       ports.Clock
}

type RequestSellEvaluationOnParcelMomentHandler struct {
	deps RequestSellEvaluationOnParcelMomentDeps
}

// NewRequestSellEvaluationOnParcelMomentHandler 构造期拒 nil，缺件包 ErrNilDependency 并点名，理由同
// NewRequestBuyEvaluationHandler。
func NewRequestSellEvaluationOnParcelMomentHandler(
	deps RequestSellEvaluationOnParcelMomentDeps,
) (*RequestSellEvaluationOnParcelMomentHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"parcel commercial resolution view", deps.Resolutions == nil},
		{"adopted settlement terms view", deps.Terms == nil},
		{"settlement account view", deps.Accounts == nil},
		{"sell billing moment adoption view", deps.Adoptions == nil},
		{"sell evaluation request registry", deps.Registry == nil},
		{"sell evaluation request identity factory", deps.Identity == nil},
		{"sell evaluation request handoff", deps.Downstream == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &RequestSellEvaluationOnParcelMomentHandler{deps: deps}, nil
}

// Handle 走一拍：受理 → 包裹到结算账户 → 采用册 → 按自然键找先到者 → 铸造 → 登记 → 交信封。
//
// 本编排只该跑在调用方的事务里：登记与交信封同生同灭（决定一）。所以交接失败不像 BUY 那只编排那样留续办引用、照答
// 已发起，而是答`未决`让调用方整笔回滚重来——留续办引用的形状在事务里会让一份请求行提交而信封缺席，没有任何东西再
// 推它。信封被框架确定性拒收时整笔交回 error（errors.Is ports.ErrSellEvaluationRequestHandoffRejected）：重来永远同一个
// 结果，归`未决`会重试到失败预算耗尽，把永久损坏登成可续办（ADR-0029）。
func (handler *RequestSellEvaluationOnParcelMomentHandler) Handle(
	ctx context.Context,
	command RequestSellEvaluationOnParcelMomentCommand,
) (RequestSellEvaluationOnParcelMomentResult, error) {
	if command.TenantID.String() == "" || command.Parcel.String() == "" || command.Moment.String() == "" ||
		command.Source.String() == "" || command.PricingBasisAt.IsZero() {
		return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestNotAccepted}, nil
	}

	account, currency, halted := handler.settlementAccountOf(ctx, command)
	if halted != nil {
		return *halted, nil
	}

	adoption, adopted, err := handler.deps.Adoptions.FindSellBillingMomentAdoption(ctx, command.TenantID, account)
	if err != nil {
		return sellRequestUndecided(SellRequestBillingMomentAdoptionUnavailable), nil
	}
	if !adopted {
		return sellRequestUnconfigured(SellRequestBillingMomentNotAdopted), nil
	}
	if adoption.Moment() != command.Moment {
		return sellRequestUnconfigured(SellRequestBillingMomentDiffers), nil
	}

	naturalKey := domain.SellEvaluationRequestNaturalKey{Parcel: command.Parcel, Moment: command.Moment}
	existing, found, err := handler.deps.Registry.FindByNaturalKey(ctx, command.TenantID, naturalKey)
	if err != nil {
		return sellRequestUndecided(SellRequestRegistryUnavailable), nil
	}
	if found {
		return handler.existingRequest(ctx, existing)
	}

	id, err := handler.deps.Identity.MintSellEvaluationRequestID(ctx)
	if err != nil {
		return sellRequestUndecided(SellRequestIdentityUnavailable), nil
	}
	// 结算币种取账户键上那一格：账户行是不可变元组（ADR-0158），按这把键找到的那一行，币种就是键上这一格，不必为它
	// 再读一次账户行。
	request, err := domain.SubmitSellEvaluationRequest(domain.SellEvaluationRequestSpec{
		ID:                 id,
		Parcel:             command.Parcel,
		Account:            account,
		SettlementCurrency: currency,
		PricingScope:       adoption.PricingScope(),
		Moment:             command.Moment,
		Source:             command.Source,
		PricingBasisAt:     command.PricingBasisAt,
	})
	if err != nil {
		return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestNotAccepted}, nil
	}
	record := ports.SellEvaluationRequestRecord{
		Key:        ports.SellEvaluationRequestKey{TenantID: command.TenantID, Request: id},
		Request:    request,
		RecordedAt: handler.deps.Clock.Now(),
		State:      domain.SellRequestSubmitted,
	}
	saved, err := handler.deps.Registry.Save(ctx, record)
	if err != nil {
		return sellRequestUndecided(SellRequestRegistryUnavailable), nil
	}
	switch saved {
	case ports.EvaluationRequestSaved:
		return handler.handOff(ctx, record, SellEvaluationRequested)
	case ports.EvaluationRequestAlreadyRequested:
		winner, found, err := handler.deps.Registry.FindByNaturalKey(ctx, command.TenantID, naturalKey)
		if err != nil || !found {
			return sellRequestUndecided(SellRequestRegistryUnavailable), nil
		}
		return handler.existingRequest(ctx, winner)
	default:
		return RequestSellEvaluationOnParcelMomentResult{}, fmt.Errorf("%w: %d", ErrUnexpectedSellEvaluationRequestSave, saved)
	}
}

// settlementAccountOf 走「包裹 → 结算账户」那条链，交回账户与它的结算币种；停在哪一格就交回那一格的结果。
// 方向取应收：SELL·客户费用是向货主收的钱，同一组法人、相对方、币种与政策下的应付行不是这一问的答案。
func (handler *RequestSellEvaluationOnParcelMomentHandler) settlementAccountOf(
	ctx context.Context,
	command RequestSellEvaluationOnParcelMomentCommand,
) (domain.SettlementAccountID, domain.CurrencyCode, *RequestSellEvaluationOnParcelMomentResult) {
	halt := func(result RequestSellEvaluationOnParcelMomentResult) (domain.SettlementAccountID, domain.CurrencyCode, *RequestSellEvaluationOnParcelMomentResult) {
		return domain.SettlementAccountID{}, domain.CurrencyCode{}, &result
	}

	resolution, resolved, err := handler.deps.Resolutions.LoadParcelCommercialResolution(ctx, command.TenantID, command.Parcel)
	if err != nil {
		return halt(sellRequestUndecided(SellRequestCommercialResolutionUnavailable))
	}
	if !resolved {
		return halt(RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestParcelUnresolved})
	}

	terms, adopted, err := handler.deps.Terms.LoadAdoptedSettlementTerms(ctx, command.TenantID, resolution)
	if err != nil {
		return halt(sellRequestUndecided(SellRequestSettlementTermsUnavailable))
	}
	if !adopted {
		return halt(sellRequestUnconfigured(SellRequestSettlementPolicyNotAdopted))
	}
	key, err := terms.AccountKey(domain.ChargeReceivable)
	if err != nil {
		return halt(RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestNotAccepted})
	}

	account, registered, err := handler.deps.Accounts.Find(ctx, command.TenantID, key)
	if err != nil {
		return halt(sellRequestUndecided(SellRequestSettlementAccountUnavailable))
	}
	if !registered {
		return halt(sellRequestUnconfigured(SellRequestSettlementAccountNotRegistered))
	}
	return account, key.Currency(), nil
}

// existingRequest 按先到者作答并再交一次同一封信封：Outbox 按认领键吞掉第二次；万一先到者的信封没随它落下（调用方
// 没把两件放进同一事务），重放就是补上它的那一拍。
func (handler *RequestSellEvaluationOnParcelMomentHandler) existingRequest(
	ctx context.Context,
	record ports.SellEvaluationRequestRecord,
) (RequestSellEvaluationOnParcelMomentResult, error) {
	return handler.handOff(ctx, record, SellEvaluationRequestExisting)
}

func (handler *RequestSellEvaluationOnParcelMomentHandler) handOff(
	ctx context.Context,
	record ports.SellEvaluationRequestRecord,
	outcome SellEvaluationRequestOutcome,
) (RequestSellEvaluationOnParcelMomentResult, error) {
	err := handler.deps.Downstream.HandOffSellEvaluationRequest(ctx, ports.SellEvaluationRequestIntent{Record: record})
	if errors.Is(err, ports.ErrSellEvaluationRequestHandoffRejected) {
		return RequestSellEvaluationOnParcelMomentResult{}, fmt.Errorf("request sell evaluation %s: %w", record.Key.Request, err)
	}
	if err != nil {
		return sellRequestUndecided(SellRequestHandoffUnavailable), nil
	}
	return RequestSellEvaluationOnParcelMomentResult{outcome: outcome, record: record, hasRecord: true}, nil
}

func sellRequestUnconfigured(reason SellEvaluationRequestUnconfiguredReason) RequestSellEvaluationOnParcelMomentResult {
	return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestUnconfigured, unconfigured: reason}
}

func sellRequestUndecided(reason SellEvaluationRequestUndecidedReason) RequestSellEvaluationOnParcelMomentResult {
	return RequestSellEvaluationOnParcelMomentResult{outcome: SellEvaluationRequestUndecided, undecided: reason}
}
