package sainbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// —— 计价没形成评价 ——

// evaluationRequestUnformedConsumerName 是本消费者在 inbox 键上的稳定名。
const evaluationRequestUnformedConsumerName = "settlement-accounting/record-evaluation-request-unformed"

// EvaluationRequestUnformedEventType 是本消费者认的事件类型：计价对一份评价请求答不出评价时交出的信封（ADR-0229 决定三）。
// 字符串由消费方自己写出，不 import 生产方（ADR-0025）；两串相等由 cmd/parcel-dispatch 用生产方真交接口入队的用例钉住。
const EvaluationRequestUnformedEventType eventing.EventType = "parcel-pricing.evaluation-request.unformed"

// UnformedEvaluationRequest 是译码后的原词：租户、请求标识、方向、停在哪一格。方向与停因是计价那一侧的词，由处理方
// （adapters/parcelpricing）译成本上下文的词。
type UnformedEvaluationRequest struct {
	TenantID  string
	Request   string
	Direction string
	Cause     string
}

// UnformedEvaluationRequestHandler 是本消费者转交的处理方。
type UnformedEvaluationRequestHandler interface {
	HandleUnformedEvaluationRequest(ctx context.Context, unformed UnformedEvaluationRequest) error
}

// EvaluationRequestUnformedConsumer 把「评价未形成」信封推进消费门。
type EvaluationRequestUnformedConsumer struct {
	gate *inboxconsume.Gate[UnformedEvaluationRequest]
}

func NewEvaluationRequestUnformedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler UnformedEvaluationRequestHandler,
) (*EvaluationRequestUnformedConsumer, error) {
	if transactor == nil || store == nil || handler == nil {
		return nil, fmt.Errorf("settlement accounting inbox: evaluation request unformed consumer needs transactor, store and handler")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[UnformedEvaluationRequest]{
		Transactor:     transactor,
		Store:          store,
		Name:           evaluationRequestUnformedConsumerName,
		EventType:      EvaluationRequestUnformedEventType,
		Decode:         decodeUnformedEvaluationRequest,
		Handle:         handler.HandleUnformedEvaluationRequest,
		UnexpectedType: "settlement accounting inbox",
		HandleVerb:     "handle unformed evaluation request",
	})
	if err != nil {
		return nil, err
	}
	return &EvaluationRequestUnformedConsumer{gate: gate}, nil
}

func (consumer *EvaluationRequestUnformedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeUnformedEvaluationRequest 译载荷，四键缺一即毒丸。detail 一键给人读，不进处理。
func decodeUnformedEvaluationRequest(payload []byte) (UnformedEvaluationRequest, error) {
	var body struct {
		TenantID  string `json:"tenantId"`
		Request   string `json:"evaluationRequestId"`
		Direction string `json:"direction"`
		Cause     string `json:"cause"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return UnformedEvaluationRequest{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Request == "" || body.Direction == "" || body.Cause == "" {
		return UnformedEvaluationRequest{}, fmt.Errorf("%w: missing unformed evaluation request fields", ErrPoisonEnvelope)
	}
	return UnformedEvaluationRequest(body), nil
}

// —— 参数已登记 ——

// ErrEvaluationResumptionUndecided 是续办这一路的未决哨兵：至少一件续办停在依赖答不出上（队列、某本册、铸造口或交信封），
// 整封重投时已续办的那几件因为有了下一代或有了请求而不再被取，没续办成的再来一次。生产装配把它登进这几路的哨兵名单。
var ErrEvaluationResumptionUndecided = errors.New("settlement accounting inbox: evaluation resumption is undecided")

// EvaluationParameterRegistration 是续办门认的一类「参数已登记」信封：事件类型、在 inbox 键上的名字、载荷里允许出现的
// 登记种类。三类信封（结算账户已登记、结算的评价参数已登记、计价的参数已登记）各装一扇门、共用一套续办纪律。
type EvaluationParameterRegistration struct {
	ConsumerName string
	EventType    eventing.EventType
	Kinds        []domain.EvaluationParameterKind
}

// 三类信封的门。事件类型由消费方自己写出（ADR-0025），两串漂开撞 dispatch.no_subscriber，载荷漂开由 cmd/parcel-dispatch
// 用生产方真交接口入队的用例揪红。
var (
	// SettlementAccountRegisteredResumption 收「结算账户已登记」。同一类信封 parcel-shipment 也收，那一路续办的是接受前
	// 财务控制；两路各记各的 inbox。
	SettlementAccountRegisteredResumption = EvaluationParameterRegistration{
		ConsumerName: "settlement-accounting/resume-evaluations-on-settlement-account-registration",
		EventType:    "settlement-accounting.settlement-account.registered",
		Kinds:        []domain.EvaluationParameterKind{domain.ParameterSettlementAccount},
	}
	// EvaluationParameterRegisteredResumption 收结算自己那几本评价参数册（SELL 计费时点采用、费用项目映射、BUY 触发册）
	// 的登记。
	EvaluationParameterRegisteredResumption = EvaluationParameterRegistration{
		ConsumerName: "settlement-accounting/resume-evaluations-on-evaluation-parameter-registration",
		EventType:    "settlement-accounting.evaluation-parameter.registered",
		Kinds: []domain.EvaluationParameterKind{
			domain.ParameterSellBillingMomentAdoption, domain.ParameterFeeItemMapping, domain.ParameterBuyEvaluationTrigger,
		},
	}
	// PricingParameterRegisteredResumption 收计价的登记（价卡、序列与目录的在用版本）。
	PricingParameterRegisteredResumption = EvaluationParameterRegistration{
		ConsumerName: "settlement-accounting/resume-evaluations-on-pricing-parameter-registration",
		EventType:    "parcel-pricing.pricing-parameter.registered",
		Kinds: []domain.EvaluationParameterKind{
			domain.ParameterPriceCard, domain.ParameterReferenceSeries, domain.ParameterReferenceCatalogue,
		},
	}
)

// stalledIdentity 认一件等着的东西是谁：种类与两段键，不含停因与位置——同一件重跑后换了停因或停下时刻，仍是同一件。
type stalledIdentity struct {
	kind          ports.StalledEvaluationKind
	first, second string
}

func identityOf(item ports.StalledEvaluation) stalledIdentity {
	return stalledIdentity{kind: item.Kind, first: item.Position.FirstKey, second: item.Position.SecondKey}
}

// StalledEvaluationResumer 是续办一件的编排，生产装配接 saapplication.ResumeStalledEvaluationHandler。
type StalledEvaluationResumer interface {
	Handle(
		ctx context.Context,
		command saapplication.ResumeStalledEvaluationCommand,
	) (saapplication.ResumeStalledEvaluationResult, error)
}

// EvaluationParameterRegisteredConsumer 是一扇续办门：一封「参数已登记」到来，按租户取回停在这一类登记解得开的停因上的
// 那几件，逐件各开一笔事务续办一次（ADR-0229 决定五）。门每次现建，理由同 parcel-shipment 那几扇登记续办门：处理函数
// 要拿到进事务之前的 ctx 去开各件自己的顶层事务。
type EvaluationParameterRegisteredConsumer struct {
	registration EvaluationParameterRegistration
	transactor   bentoapp.Transactor
	store        *inbox.Store
	queue        ports.StalledEvaluationQueue
	resumer      StalledEvaluationResumer
	pageSize     int
}

func NewEvaluationParameterRegisteredConsumer(
	registration EvaluationParameterRegistration,
	transactor bentoapp.Transactor,
	store *inbox.Store,
	queue ports.StalledEvaluationQueue,
	resumer StalledEvaluationResumer,
	pageSize int,
) (*EvaluationParameterRegisteredConsumer, error) {
	if registration.ConsumerName == "" || registration.EventType == "" || len(registration.Kinds) == 0 {
		return nil, fmt.Errorf("settlement accounting inbox: evaluation parameter registration is incomplete")
	}
	if transactor == nil || store == nil || queue == nil || resumer == nil {
		return nil, fmt.Errorf("settlement accounting inbox: evaluation resumption consumer needs transactor, store, queue and resumer")
	}
	if pageSize <= 0 {
		return nil, fmt.Errorf("settlement accounting inbox: evaluation resumption page size must be positive, got %d", pageSize)
	}
	return &EvaluationParameterRegisteredConsumer{
		registration: registration,
		transactor:   transactor,
		store:        store,
		queue:        queue,
		resumer:      resumer,
		pageSize:     pageSize,
	}, nil
}

// registeredParameter 是译码后的一次登记：租户、种类、价卡带的计价范围，以及凭的依据（由信封标识造）。
type registeredParameter struct {
	tenant domain.TenantID
	kind   domain.EvaluationParameterKind
	scope  string
	basis  domain.ResumeBasis
}

func (consumer *EvaluationParameterRegisteredConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	gate, err := inboxconsume.New(inboxconsume.Spec[registeredParameter]{
		Transactor: consumer.transactor,
		Store:      consumer.store,
		Name:       consumer.registration.ConsumerName,
		EventType:  consumer.registration.EventType,
		Decode: func(payload []byte) (registeredParameter, error) {
			return consumer.decode(envelope.ID, payload)
		},
		Handle: func(_ context.Context, registered registeredParameter) error {
			return consumer.resumeWaiting(ctx, registered)
		},
		UnexpectedType: "settlement accounting inbox",
		HandleVerb:     "resume evaluations on parameter registration",
	})
	if err != nil {
		return err
	}
	return gate.Consume(ctx, envelope)
}

// decode 译载荷：tenantId 与 registrationKind 两键必备，pricingScope 只有价卡带。种类必须是这扇门认的那几种——别的种类出现
// 在这类信封上说明发布侧的契约漂了，重投长不出对的种类，是毒丸。
func (consumer *EvaluationParameterRegisteredConsumer) decode(id eventing.EventID, payload []byte) (registeredParameter, error) {
	var body struct {
		TenantID         string `json:"tenantId"`
		RegistrationKind string `json:"registrationKind"`
		PricingScope     string `json:"pricingScope"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return registeredParameter{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	tenant, err := domain.NewTenantID(body.TenantID)
	if err != nil {
		return registeredParameter{}, fmt.Errorf("%w: tenant: %v", ErrPoisonEnvelope, err)
	}
	kind, err := domain.EvaluationParameterKindFromName(body.RegistrationKind)
	if err != nil {
		return registeredParameter{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	accepted := false
	for _, allowed := range consumer.registration.Kinds {
		accepted = accepted || allowed == kind
	}
	if !accepted {
		return registeredParameter{}, fmt.Errorf("%w: kind %s does not travel on %s", ErrPoisonEnvelope, kind, consumer.registration.EventType)
	}
	basis, err := domain.NewParameterResumeBasis(kind, string(id))
	if err != nil {
		return registeredParameter{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	return registeredParameter{tenant: tenant, kind: kind, scope: body.PricingScope, basis: basis}, nil
}

// resumeWaiting 按页取等着的件，逐件在各自的顶层事务里续办一次。结局分三类收：落定（续办了、仍停着、早续办过、不在等）、
// 未决（记下，等本封重投）、硬错误（矛盾、装配缺件，记下并继续办其余）。硬错误压过未决上抛——它要人看，折进未决会重投到
// 预算耗尽。按位置往后翻页、不足一页即到底：办过却没写回的件（不在等、硬错误、未决）仍满足队列的口径，每页重读队首会被
// 它们占满。续办中途换了位置的件（发起停办重跑后又停下）可能在后面的页里再出现一次，记过的不再办。
func (consumer *EvaluationParameterRegisteredConsumer) resumeWaiting(ctx context.Context, registered registeredParameter) error {
	causes := domain.EvaluationStopCausesResumedBy(registered.kind)
	if len(causes) == 0 {
		return nil
	}
	query := ports.StalledEvaluationQuery{Causes: causes, ExcludeBasis: registered.basis}
	if registered.kind == domain.ParameterPriceCard {
		query.PricingScope = registered.scope
	}
	attempted := map[stalledIdentity]bool{}
	var (
		total, settled int
		undecided      []error
		failures       []error
	)
	for {
		items, err := consumer.queue.ListStalled(ctx, registered.tenant, query, consumer.pageSize)
		if err != nil {
			return fmt.Errorf("%w: list stalled evaluations: %v", ErrEvaluationResumptionUndecided, err)
		}
		for _, item := range items {
			query.After = item.Position
			if attempted[identityOf(item)] {
				continue
			}
			attempted[identityOf(item)] = true
			total++
			var result saapplication.ResumeStalledEvaluationResult
			err := consumer.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
				resumed, err := consumer.resumer.Handle(txCtx, saapplication.ResumeStalledEvaluationCommand{
					TenantID: registered.tenant,
					Item:     item,
					Basis:    registered.basis,
					Causes:   causes,
				})
				if err != nil {
					return err
				}
				result = resumed
				if resumed.Outcome() == saapplication.StalledEvaluationUndecided {
					return fmt.Errorf("%w: %s %s: %s: %v", ErrEvaluationResumptionUndecided,
						item.Kind, item.Cause, resumed.UndecidedStage(), resumed.UndecidedCause())
				}
				return nil
			})
			switch {
			case errors.Is(err, ErrEvaluationResumptionUndecided):
				undecided = append(undecided, err)
			case err != nil:
				failures = append(failures, fmt.Errorf("%s %s: %w", item.Kind, item.Cause, err))
			case result.Outcome() == saapplication.StalledEvaluationNotAccepted:
				failures = append(failures, fmt.Errorf("%s %s: resumption not accepted", item.Kind, item.Cause))
			default:
				settled++
			}
		}
		if len(items) < consumer.pageSize {
			break
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d stalled evaluations failed to resume (%d settled, %d undecided); first: %w",
			len(failures), total, settled, len(undecided), failures[0])
	}
	if len(undecided) > 0 {
		return fmt.Errorf("%w: %d of %d stalled evaluations stayed undecided (%d settled); first: %v",
			ErrEvaluationResumptionUndecided, len(undecided), total, settled, undecided[0])
	}
	return nil
}
