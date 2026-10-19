package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrUnexpectedAdoptionSave 说明采用册交回了封闭集合以外的写入结果。
	ErrUnexpectedAdoptionSave = errors.New("transport fulfillment: unexpected segment commission adoption save outcome")
	// ErrAdoptedCommissionVanished 说明标开始那一笔没落上、读回时委托却不在册了。委托只增不删，读不回是库里的数据违约，
	// 不是登记方能改的输入。
	ErrAdoptedCommissionVanished = errors.New("transport fulfillment: the adopted commission is no longer registered")
)

// CommissionAdoptionOutcome 是「履约段采用运输委托」的应用处理结果（ADR-0204）。前置条件不满足的各格都是业务答案，续办
// 两两不同：段不在册（核段标识，或等控制事实把段立起来）、委托不在册（去提交委托或改引用）、委托已取消（另起委托）、服务
// 方不符（指错了委托或服务方）、段没有当前参与（段上的控制事实都被更正撤回了，先处理更正）、段早于委托提交就已开始（两份
// 事实里有一份时刻填错了），折成一格登记方就只能去猜（ADR-0029）。他租户名下的同名段与委托按本租户的键读不回，同落「不在
// 册」那一格。
type CommissionAdoptionOutcome uint8

const (
	CommissionAdoptionOutcomeInvalid CommissionAdoptionOutcome = iota
	CommissionAdopted
	CommissionAdoptionExisting
	CommissionAdoptionConflict
	CommissionAdoptionSegmentNotFound
	CommissionAdoptionCommissionNotFound
	CommissionNotAdoptable
	CommissionAdoptionProviderMismatch
	CommissionAdoptionSegmentWithoutParticipation
	CommissionAdoptionSegmentStartedBeforeCommission
	CommissionAdoptionNotAccepted
	CommissionAdoptionUndecided
)

func (outcome CommissionAdoptionOutcome) String() string {
	switch outcome {
	case CommissionAdopted:
		return "COMMISSION_ADOPTED"
	case CommissionAdoptionExisting:
		return "EXISTING_ADOPTION"
	case CommissionAdoptionConflict:
		return "ADOPTION_CONFLICT"
	case CommissionAdoptionSegmentNotFound:
		return "SEGMENT_NOT_FOUND"
	case CommissionAdoptionCommissionNotFound:
		return "COMMISSION_NOT_FOUND"
	case CommissionNotAdoptable:
		return "COMMISSION_NOT_ADOPTABLE"
	case CommissionAdoptionProviderMismatch:
		return "PROVIDER_MISMATCH"
	case CommissionAdoptionSegmentWithoutParticipation:
		return "SEGMENT_WITHOUT_EFFECTIVE_PARTICIPATION"
	case CommissionAdoptionSegmentStartedBeforeCommission:
		return "SEGMENT_STARTED_BEFORE_COMMISSION"
	case CommissionAdoptionNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case CommissionAdoptionUndecided:
		return "ADOPTION_UNDECIDED"
	default:
		return ""
	}
}

// CommissionAdoptionUndecidedReason 指名采用停在哪一步等谁。
type CommissionAdoptionUndecidedReason uint8

const (
	CommissionAdoptionUndecidedReasonNone CommissionAdoptionUndecidedReason = iota
	CommissionAdoptionStoreUnavailable
	CommissionAdoptionSegmentStoreUnavailable
	CommissionAdoptionCommissionStoreUnavailable
)

func (reason CommissionAdoptionUndecidedReason) String() string {
	switch reason {
	case CommissionAdoptionStoreUnavailable:
		return "ADOPTION_STORE_UNAVAILABLE"
	case CommissionAdoptionSegmentStoreUnavailable:
		return "SEGMENT_STORE_UNAVAILABLE"
	case CommissionAdoptionCommissionStoreUnavailable:
		return "COMMISSION_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// AdoptSegmentCommissionCommand 携带一次采用声明：哪一段、在哪一份委托下、由哪一家履约。服务方由登记方声明、与委托的服务
// 提供方核对，不从委托抄过来——抄过来就核不出指错了委托。
type AdoptSegmentCommissionCommand struct {
	TenantID   domain.TenantID
	Segment    string
	Commission string
	Provider   string
}

type CommissionAdoptionResult struct {
	outcome            CommissionAdoptionOutcome
	reason             CommissionAdoptionUndecidedReason
	adoption           ports.SegmentCommissionAdoptionRecord
	hasAdoption        bool
	commission         ports.TransportCommissionRecord
	hasCommission      bool
	commissionProvider domain.ServiceProviderReference
	continuation       string
}

func (result CommissionAdoptionResult) Outcome() CommissionAdoptionOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result CommissionAdoptionResult) UndecidedReason() CommissionAdoptionUndecidedReason {
	return result.reason
}

// Adoption 在采用成立、已有采用与采用冲突时交回在册那一份连线——冲突时是段上已有的那一份，不是这次送来的。
func (result CommissionAdoptionResult) Adoption() (ports.SegmentCommissionAdoptionRecord, bool) {
	return result.adoption, result.hasAdoption
}

// Commission 只在采用成立时交回委托此刻的样子：已开始，开始依据是首个采用它的那一段的首个参与。
func (result CommissionAdoptionResult) Commission() (ports.TransportCommissionRecord, bool) {
	return result.commission, result.hasCommission
}

// CommissionProvider 只在`服务方不符`时有值：委托的服务提供方。登记方据此决定改服务方还是改指委托。
func (result CommissionAdoptionResult) CommissionProvider() (domain.ServiceProviderReference, bool) {
	return result.commissionProvider, result.outcome == CommissionAdoptionProviderMismatch
}

func (result CommissionAdoptionResult) ContinuationReference() string {
	return result.continuation
}

type AdoptSegmentCommissionDeps struct {
	Adoptions   ports.SegmentCommissionAdoptionStore
	Segments    ports.FulfillmentSegmentView
	Commissions ports.TransportCommissionStore
	Clock       ports.Clock
}

type AdoptSegmentCommissionHandler struct {
	deps AdoptSegmentCommissionDeps
}

func NewAdoptSegmentCommissionHandler(deps AdoptSegmentCommissionDeps) *AdoptSegmentCommissionHandler {
	return &AdoptSegmentCommissionHandler{deps: deps}
}

// Adopt 登记一次采用（ADR-0204）：声明过构造门 → 按（租户、段）分已有与冲突 → 段在册 → 委托在册 → 领域判服务方、取消、
// 首个参与与开始先后 → 落连线 → 委托尚未开始时以段的首个参与标为已开始。
//
// 已有与冲突判在两项读之前：那一格答的是「这一段采用的是谁」，段与委托此后变成什么样都不改它——委托事后被另一段先开始了，
// 原样重放仍是原连线。
//
// 连线先落、委托后标：同一段的两份采用并发到达时，后到者在落连线那一步就撞上主键，什么都没写；委托与取消并发时，标开始那
// 一笔的 WHERE 只认尚未取消的行，没标上就读回再判。**本编排答`采用成立`以外的任一格时，装配必须整笔回滚**：其余各格都不
// 该留下任何写入，而读回发现委托刚被取消的那一格，连线已经落了。
//
// 采用不入队：今天没有它的消费者（tf-chain-plan「跨票通则」X3）；运输收费发生项登记要它时按段回读（ports 的头注）。
func (handler *AdoptSegmentCommissionHandler) Adopt(
	ctx context.Context,
	command AdoptSegmentCommissionCommand,
) (CommissionAdoptionResult, error) {
	segmentRef, err := domain.NewFulfillmentSegmentReference(command.Segment)
	if err != nil {
		return CommissionAdoptionResult{outcome: CommissionAdoptionNotAccepted}, nil
	}
	commissionRef, err := domain.NewTransportCommissionReference(command.Commission)
	if err != nil {
		return CommissionAdoptionResult{outcome: CommissionAdoptionNotAccepted}, nil
	}
	provider, err := domain.NewServiceProviderReference(command.Provider)
	if err != nil {
		return CommissionAdoptionResult{outcome: CommissionAdoptionNotAccepted}, nil
	}
	digest, err := domain.CanonicalizeSegmentCommissionAdoptionPayload(command.Commission, command.Provider)
	if err != nil {
		return CommissionAdoptionResult{}, err
	}

	key := ports.SegmentCommissionAdoptionKey{TenantID: command.TenantID, Segment: segmentRef}
	existing, found, err := handler.deps.Adoptions.FindBySegment(ctx, key)
	if err != nil {
		return commissionAdoptionUndecided(CommissionAdoptionStoreUnavailable, command.Segment), nil
	}
	if found {
		return answerFromStoredCommissionAdoption(existing, digest, command.Segment), nil
	}

	segment, found, err := handler.deps.Segments.FindByKey(ctx, ports.FulfillmentSegmentKey{TenantID: command.TenantID, Segment: segmentRef})
	if err != nil {
		return commissionAdoptionUndecided(CommissionAdoptionSegmentStoreUnavailable, command.Segment), nil
	}
	if !found {
		return CommissionAdoptionResult{outcome: CommissionAdoptionSegmentNotFound}, nil
	}
	commissionKey := ports.TransportCommissionKey{TenantID: command.TenantID, Commission: commissionRef}
	commission, found, err := handler.deps.Commissions.FindByKey(ctx, commissionKey)
	if err != nil {
		return commissionAdoptionUndecided(CommissionAdoptionCommissionStoreUnavailable, command.Segment), nil
	}
	if !found {
		return CommissionAdoptionResult{outcome: CommissionAdoptionCommissionNotFound}, nil
	}

	adoption, adopted, startedNow, err := domain.AdoptCommissionForSegment(segment.Segment, commission.Commission, provider)
	switch {
	case errors.Is(err, domain.ErrAdoptionProviderMismatch):
		return CommissionAdoptionResult{outcome: CommissionAdoptionProviderMismatch, commissionProvider: commission.Commission.Provider()}, nil
	case errors.Is(err, domain.ErrCommissionNotAdoptable):
		return CommissionAdoptionResult{outcome: CommissionNotAdoptable}, nil
	case errors.Is(err, domain.ErrSegmentWithoutEffectiveParticipation):
		return CommissionAdoptionResult{outcome: CommissionAdoptionSegmentWithoutParticipation}, nil
	case errors.Is(err, domain.ErrSegmentStartedBeforeCommission):
		return CommissionAdoptionResult{outcome: CommissionAdoptionSegmentStartedBeforeCommission}, nil
	case err != nil:
		return CommissionAdoptionResult{outcome: CommissionAdoptionNotAccepted}, nil
	}

	now := handler.deps.Clock.Now()
	record := ports.SegmentCommissionAdoptionRecord{Key: key, ContentDigest: digest, Adoption: adoption, RecordedAt: now}
	saved, err := handler.deps.Adoptions.Save(ctx, record)
	if err != nil {
		return commissionAdoptionUndecided(CommissionAdoptionStoreUnavailable, command.Segment), nil
	}
	switch saved {
	case ports.AdoptionSaved:
	case ports.AdoptionAlreadyRegistered:
		// 同一段并发首交的输家照样按摘要分已有与冲突：先到者采用的可能是另一份委托，读回赢家就答「已有」会把冲突吞掉。
		winner, found, err := handler.deps.Adoptions.FindBySegment(ctx, key)
		if err != nil || !found {
			return commissionAdoptionUndecided(CommissionAdoptionStoreUnavailable, command.Segment), nil
		}
		return answerFromStoredCommissionAdoption(winner, digest, command.Segment), nil
	default:
		return CommissionAdoptionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedAdoptionSave, saved)
	}

	if !startedNow {
		return CommissionAdoptionResult{outcome: CommissionAdopted, adoption: record, hasAdoption: true, commission: commission, hasCommission: true}, nil
	}
	started := commission
	started.Commission = adopted
	started.RecordedAt = now
	marked, err := handler.deps.Commissions.Replace(ctx, started)
	if err != nil {
		return commissionAdoptionUndecided(CommissionAdoptionCommissionStoreUnavailable, command.Segment), nil
	}
	if marked {
		return CommissionAdoptionResult{outcome: CommissionAdopted, adoption: record, hasAdoption: true, commission: started, hasCommission: true}, nil
	}
	// 读到时尚未开始也未取消，标的那一刻却没标上：另一笔在中间先开始或先取消了它。先开始的是别的段先采用，这一段照常采用、
	// 不改写那一次开始；先取消的就采用不成，连线已落，靠装配整笔回滚撤掉。
	current, found, err := handler.deps.Commissions.FindByKey(ctx, commissionKey)
	if err != nil {
		return commissionAdoptionUndecided(CommissionAdoptionCommissionStoreUnavailable, command.Segment), nil
	}
	if !found {
		return CommissionAdoptionResult{}, fmt.Errorf("%w: %s", ErrAdoptedCommissionVanished, commissionRef)
	}
	if _, cancelled := current.Commission.Cancelled(); cancelled {
		return CommissionAdoptionResult{outcome: CommissionNotAdoptable}, nil
	}
	if _, _, started := current.Commission.TransportStarted(); started {
		return CommissionAdoptionResult{outcome: CommissionAdopted, adoption: record, hasAdoption: true, commission: current, hasCommission: true}, nil
	}
	return CommissionAdoptionResult{}, fmt.Errorf("%w: commission %s neither started nor cancelled after a refused start", ErrUnexpectedCommissionSave, commissionRef)
}

// answerFromStoredCommissionAdoption 按段上已有的那一份作答：同一对（委托、服务方）是重放，异是冲突——一段至多采用一份委托，不按最后
// 到达顶替；认不出摘要形状答未决。两格都回显在册那一份。
func answerFromStoredCommissionAdoption(stored ports.SegmentCommissionAdoptionRecord, digest, subject string) CommissionAdoptionResult {
	switch domain.CompareStoredDigest(stored.ContentDigest, digest, "") {
	case domain.SamePayload:
		return CommissionAdoptionResult{outcome: CommissionAdoptionExisting, adoption: stored, hasAdoption: true}
	case domain.DifferentPayload:
		return CommissionAdoptionResult{outcome: CommissionAdoptionConflict, adoption: stored, hasAdoption: true}
	default:
		return commissionAdoptionUndecided(CommissionAdoptionStoreUnavailable, subject)
	}
}

func commissionAdoptionUndecided(reason CommissionAdoptionUndecidedReason, subject string) CommissionAdoptionResult {
	return CommissionAdoptionResult{
		outcome:      CommissionAdoptionUndecided,
		reason:       reason,
		continuation: commissionContinuation("SEGMENT_COMMISSION_ADOPTION", reason.String(), subject),
	}
}
