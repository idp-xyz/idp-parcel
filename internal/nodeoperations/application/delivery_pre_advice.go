package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 送站预报的提交与撤回编排（UC-NO-006 应用流程步骤 2；ADR-0212 决定二）。认证、准入与载荷形状在首方客户渠道那一层答完（步骤 1），
// 到这里的命令已带信封给出的租户、客户账户与来源。事务由进程级入口给出：预报册的写口无事务即拒。

// ErrDeliveryPreAdviceRejected 说明命令形不成一份预报或撤回：号码为空或重复、缺来源请求键一类。重发同样的内容不会变好，在线口答
// 坏载荷。
var ErrDeliveryPreAdviceRejected = errors.New("node operations: delivery pre-advice command rejected")

// ErrDeliveryPreAdviceNotFound 说明撤回指名的预报在本租户看不见：不存在，或不属信封里那个客户账户。两种同答，不泄露别的客户账户的
// 预报在不在（UC-NO-006「权限、隔离与审计」）。
var ErrDeliveryPreAdviceNotFound = errors.New("node operations: delivery pre-advice not visible")

// SubmitDeliveryPreAdviceCommand 携带一次预报提交。租户、客户账户、客户端（Channel）与来源请求键取自信封；其余是客户声明的内容。
// 没有标识与形成时刻：那是本上下文签的。DeliveringParty 零值即没给线索。
type SubmitDeliveryPreAdviceCommand struct {
	TenantID          domain.TenantID
	CustomerAccount   domain.CustomerAccountReference
	Channel           string
	RequestKey        string
	Node              domain.NodeReference
	ExpectedArrivalAt time.Time
	Numbers           []domain.PreAdvisedParcelNumber
	DeliveringParty   domain.DeliveringPartyReference
}

// WithdrawDeliveryPreAdviceCommand 携带一次预报撤回。CustomerAccount 是信封里那个客户账户，只撤它的预报。
type WithdrawDeliveryPreAdviceCommand struct {
	TenantID        domain.TenantID
	CustomerAccount domain.CustomerAccountReference
	Channel         string
	RequestKey      string
	PreAdvice       domain.DeliveryPreAdviceID
}

// DeliveryPreAdviceOutcome 是两口的封闭答案（UC-NO-006 结果语义契约），各格都是形成了的答案。
type DeliveryPreAdviceOutcome uint8

const (
	DeliveryPreAdviceOutcomeInvalid DeliveryPreAdviceOutcome = iota
	// DeliveryPreAdviceRegistered：送站预报已登记，标识已签发。
	DeliveryPreAdviceRegistered
	// DeliveryPreAdviceWithdrawn：送站预报已撤回，此后不再成立范围。
	DeliveryPreAdviceWithdrawn
	// DeliveryPreAdviceExistingResult：同一接入请求原样重投，回显在册那一份；撤回口上这份预报已被另一接入请求撤回也答这一格。
	DeliveryPreAdviceExistingResult
	// DeliveryPreAdviceSourceConflict：同一接入请求换了内容，回显在册那一份，什么也没落。
	DeliveryPreAdviceSourceConflict
)

func (outcome DeliveryPreAdviceOutcome) String() string {
	switch outcome {
	case DeliveryPreAdviceRegistered:
		return "DELIVERY_PRE_ADVICE_REGISTERED"
	case DeliveryPreAdviceWithdrawn:
		return "DELIVERY_PRE_ADVICE_WITHDRAWN"
	case DeliveryPreAdviceExistingResult:
		return "EXISTING_RESULT"
	case DeliveryPreAdviceSourceConflict:
		return "SOURCE_CONFLICT"
	default:
		return ""
	}
}

// DeliveryPreAdviceSubmissionResult 是提交的答案与它所指的那一份预报：新落的，或在册的。
type DeliveryPreAdviceSubmissionResult struct {
	outcome   DeliveryPreAdviceOutcome
	preAdvice domain.DeliveryPreAdvice
}

// NewDeliveryPreAdviceSubmissionResult 给传输层的替身造答案用；编排自己也经它出答案。
func NewDeliveryPreAdviceSubmissionResult(outcome DeliveryPreAdviceOutcome, preAdvice domain.DeliveryPreAdvice) DeliveryPreAdviceSubmissionResult {
	return DeliveryPreAdviceSubmissionResult{outcome: outcome, preAdvice: preAdvice}
}

func (result DeliveryPreAdviceSubmissionResult) Outcome() DeliveryPreAdviceOutcome {
	return result.outcome
}
func (result DeliveryPreAdviceSubmissionResult) PreAdvice() domain.DeliveryPreAdvice {
	return result.preAdvice
}

// DeliveryPreAdviceWithdrawalResult 是撤回的答案与它所指的那一份撤回：新落的，或在册的。
type DeliveryPreAdviceWithdrawalResult struct {
	outcome    DeliveryPreAdviceOutcome
	withdrawal domain.DeliveryPreAdviceWithdrawal
}

// NewDeliveryPreAdviceWithdrawalResult 给传输层的替身造答案用；编排自己也经它出答案。
func NewDeliveryPreAdviceWithdrawalResult(outcome DeliveryPreAdviceOutcome, withdrawal domain.DeliveryPreAdviceWithdrawal) DeliveryPreAdviceWithdrawalResult {
	return DeliveryPreAdviceWithdrawalResult{outcome: outcome, withdrawal: withdrawal}
}

func (result DeliveryPreAdviceWithdrawalResult) Outcome() DeliveryPreAdviceOutcome {
	return result.outcome
}
func (result DeliveryPreAdviceWithdrawalResult) Withdrawal() domain.DeliveryPreAdviceWithdrawal {
	return result.withdrawal
}

type DeliveryPreAdviceDeps struct {
	Register ports.DeliveryPreAdviceRegister
	IDs      ports.DeliveryPreAdviceIdentityFactory
	Clock    ports.Clock
}

type DeliveryPreAdviceHandler struct {
	deps DeliveryPreAdviceDeps
}

// NewDeliveryPreAdviceHandler 构造期拒缺件：漏装的册到第一份预报才暴露，会被当成一次依赖故障。
func NewDeliveryPreAdviceHandler(deps DeliveryPreAdviceDeps) (*DeliveryPreAdviceHandler, error) {
	if deps.Register == nil || deps.IDs == nil || deps.Clock == nil {
		return nil, errors.New("node operations: delivery pre-advice needs a register, an ID factory and a clock")
	}
	return &DeliveryPreAdviceHandler{deps: deps}, nil
}

// Submit 形成一份预报并登记。重放与冲突按来源请求键认（UC-NO-006「一致性、幂等与并发」）：册按（租户，客户端，来源请求键）回显在册
// 那一份，本编排比内容摘要——同一份答已有结果，另一份答来源冲突，册上那一份都不被顶替。标识先签后落：撞上在册那一份时这一次签的
// 号作废，客户拿到的永远是第一次签的那个。
func (handler *DeliveryPreAdviceHandler) Submit(ctx context.Context, command SubmitDeliveryPreAdviceCommand) (DeliveryPreAdviceSubmissionResult, error) {
	_, digest, err := domain.CanonicalizeDeliveryPreAdviceSubmissionPayload(command.Node, command.ExpectedArrivalAt, command.Numbers, command.DeliveringParty)
	if err != nil {
		return DeliveryPreAdviceSubmissionResult{}, fmt.Errorf("%w: %v", ErrDeliveryPreAdviceRejected, err)
	}
	source, err := customerRequestSource(command.Channel, command.RequestKey, digest)
	if err != nil {
		return DeliveryPreAdviceSubmissionResult{}, err
	}
	id, err := handler.deps.IDs.NextDeliveryPreAdviceID(ctx)
	if err != nil {
		return DeliveryPreAdviceSubmissionResult{}, fmt.Errorf("submit delivery pre-advice: %w", err)
	}
	preAdvice, err := domain.FormDeliveryPreAdvice(domain.DeliveryPreAdviceSpec{
		TenantID:          command.TenantID,
		ID:                id,
		CustomerAccount:   command.CustomerAccount,
		Node:              command.Node,
		ExpectedArrivalAt: command.ExpectedArrivalAt,
		Numbers:           command.Numbers,
		DeliveringParty:   command.DeliveringParty,
		Source:            source,
		FormedAt:          handler.deps.Clock.Now(),
	})
	if err != nil {
		return DeliveryPreAdviceSubmissionResult{}, fmt.Errorf("%w: %v", ErrDeliveryPreAdviceRejected, err)
	}
	recorded, outcome, err := handler.deps.Register.RecordDeliveryPreAdvice(ctx, preAdvice)
	if err != nil {
		return DeliveryPreAdviceSubmissionResult{}, fmt.Errorf("submit delivery pre-advice: %w", err)
	}
	switch outcome {
	case ports.DeliveryScopeAppended:
		return NewDeliveryPreAdviceSubmissionResult(DeliveryPreAdviceRegistered, recorded), nil
	case ports.DeliveryScopeAlreadyRecorded:
		replay, err := replayOutcome(recorded.Source().ContentDigest, digest)
		if err != nil {
			return DeliveryPreAdviceSubmissionResult{}, fmt.Errorf("submit delivery pre-advice: %w", err)
		}
		return NewDeliveryPreAdviceSubmissionResult(replay, recorded), nil
	default:
		// 预报册的提交口只该答上面两格；答别的格是实现坏了，不是业务答案。
		return DeliveryPreAdviceSubmissionResult{}, fmt.Errorf("submit delivery pre-advice: unexpected register outcome %q", outcome)
	}
}

// Withdraw 撤回信封里那个客户账户的一份预报。册只在预报属这个客户账户时落撤回；别的账户的与不存在的同答看不见。撞键时按回显的
// 那一份分格：是这一个接入请求落下的，比内容摘要分已有结果与来源冲突；是另一个接入请求落下的，这份预报已撤回、撤回是终态，答已有
// 结果并回显在册那一份——两次撤回的内容都只是这份预报，同一个结论。
func (handler *DeliveryPreAdviceHandler) Withdraw(ctx context.Context, command WithdrawDeliveryPreAdviceCommand) (DeliveryPreAdviceWithdrawalResult, error) {
	_, digest, err := domain.CanonicalizeDeliveryPreAdviceWithdrawalPayload(command.PreAdvice)
	if err != nil {
		return DeliveryPreAdviceWithdrawalResult{}, fmt.Errorf("%w: %v", ErrDeliveryPreAdviceRejected, err)
	}
	source, err := customerRequestSource(command.Channel, command.RequestKey, digest)
	if err != nil {
		return DeliveryPreAdviceWithdrawalResult{}, err
	}
	withdrawal, err := domain.FormDeliveryPreAdviceWithdrawal(domain.DeliveryPreAdviceWithdrawalSpec{
		TenantID:        command.TenantID,
		PreAdvice:       command.PreAdvice,
		CustomerAccount: command.CustomerAccount,
		Source:          source,
		WithdrawnAt:     handler.deps.Clock.Now(),
	})
	if err != nil {
		return DeliveryPreAdviceWithdrawalResult{}, fmt.Errorf("%w: %v", ErrDeliveryPreAdviceRejected, err)
	}
	recorded, outcome, err := handler.deps.Register.RecordDeliveryPreAdviceWithdrawal(ctx, withdrawal)
	if err != nil {
		return DeliveryPreAdviceWithdrawalResult{}, fmt.Errorf("withdraw delivery pre-advice: %w", err)
	}
	switch outcome {
	case ports.DeliveryScopeAppended:
		return NewDeliveryPreAdviceWithdrawalResult(DeliveryPreAdviceWithdrawn, recorded), nil
	case ports.DeliveryScopeTargetNotFound:
		return DeliveryPreAdviceWithdrawalResult{}, ErrDeliveryPreAdviceNotFound
	case ports.DeliveryScopeAlreadyRecorded:
		if recorded.Source().Channel != source.Channel || recorded.Source().RequestKey != source.RequestKey {
			return NewDeliveryPreAdviceWithdrawalResult(DeliveryPreAdviceExistingResult, recorded), nil
		}
		replay, err := replayOutcome(recorded.Source().ContentDigest, digest)
		if err != nil {
			return DeliveryPreAdviceWithdrawalResult{}, fmt.Errorf("withdraw delivery pre-advice: %w", err)
		}
		return NewDeliveryPreAdviceWithdrawalResult(replay, recorded), nil
	default:
		return DeliveryPreAdviceWithdrawalResult{}, fmt.Errorf("withdraw delivery pre-advice: unexpected register outcome %q", outcome)
	}
}

// customerRequestSource 把信封给的客户端与来源请求键配上本次内容摘要。两格空白是装配错了（信封铸造不出空键），按拒形答，不拿空串
// 去册上认重放。
func customerRequestSource(channel, requestKey, digest string) (domain.CustomerRequestSource, error) {
	if strings.TrimSpace(channel) == "" || strings.TrimSpace(requestKey) == "" {
		return domain.CustomerRequestSource{}, fmt.Errorf("%w: the envelope names no client or no source request key", ErrDeliveryPreAdviceRejected)
	}
	return domain.CustomerRequestSource{Channel: channel, RequestKey: requestKey, ContentDigest: digest}, nil
}

// replayOutcome 按在册摘要分重放与来源冲突。在册摘要的形状认不出时不形成答案：读不懂这份记录与读不出来，同样是此刻判断不了。
func replayOutcome(stored, current string) (DeliveryPreAdviceOutcome, error) {
	switch domain.CompareVersionedDigest(stored, current) {
	case domain.SamePayload:
		return DeliveryPreAdviceExistingResult, nil
	case domain.DifferentPayload:
		return DeliveryPreAdviceSourceConflict, nil
	default:
		return DeliveryPreAdviceOutcomeInvalid, fmt.Errorf("recorded content digest %q has a shape this line cannot compare", stored)
	}
}
