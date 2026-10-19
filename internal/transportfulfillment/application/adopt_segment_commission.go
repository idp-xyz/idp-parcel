package application

import (
	"context"
	"errors"
	"fmt"
	"time"

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
// 两两不同：段不在册（核段标识，或等控制事实把段立起来）、委托不在册（去提交委托或改引用）、委托在这一段开始之前已取消
// （另起委托）、委托在这一段开始之后才登记取消（两份事实按业务时间相抵：先更正取消或更正控制事实）、服务方不符（指错了
// 委托或服务方）、段没有当前参与（段上的控制事实都被更正撤回了，先处理更正）、段早于委托提交就已开始（两份事实里有一份
// 时刻填错了），折成一格登记方就只能去猜（ADR-0029）。他租户名下的同名段与委托按本租户的键读不回，同落「不在册」那一格。
type CommissionAdoptionOutcome uint8

const (
	CommissionAdoptionOutcomeInvalid CommissionAdoptionOutcome = iota
	CommissionAdopted
	CommissionAdoptionExisting
	CommissionAdoptionConflict
	CommissionAdoptionSegmentNotFound
	CommissionAdoptionCommissionNotFound
	CommissionNotAdoptable
	CommissionAdoptionCancelledAfterStart
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
	case CommissionAdoptionCancelledAfterStart:
		return "COMMISSION_CANCELLED_AFTER_START"
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
	segmentStartBasis  domain.ParticipationBasisReference
	segmentStartedAt   time.Time
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

// Commission 在采用成立时交回委托此刻的样子：已开始，开始依据是已采用各段首个参与里最早的那一条；在`取消晚于开始`时交回
// 在册那一份已取消的委托，登记方拿它的取消时刻与 SegmentStart 对照。
func (result CommissionAdoptionResult) Commission() (ports.TransportCommissionRecord, bool) {
	return result.commission, result.hasCommission
}

// SegmentStart 只在`取消晚于开始`时有值：这一段的首个当前参与——它的入场时刻不晚于委托的取消时刻，两份事实相抵的就是这一对。
func (result CommissionAdoptionResult) SegmentStart() (domain.ParticipationBasisReference, time.Time, bool) {
	return result.segmentStartBasis, result.segmentStartedAt, result.outcome == CommissionAdoptionCancelledAfterStart
}

// CommissionProvider 只在`服务方不符`时有值：委托的服务提供方。登记方据此决定改服务方还是改指委托。
func (result CommissionAdoptionResult) CommissionProvider() (domain.ServiceProviderReference, bool) {
	return result.commissionProvider, result.outcome == CommissionAdoptionProviderMismatch
}

func (result CommissionAdoptionResult) ContinuationReference() string {
	return result.continuation
}

type AdoptSegmentCommissionDeps struct {
	Adoptions        ports.SegmentCommissionAdoptionStore
	Segments         ports.FulfillmentSegmentView
	Commissions      ports.TransportCommissionStore
	StartAdvancement ports.TransportCommissionStartAdvancement
	Clock            ports.Clock
}

type AdoptSegmentCommissionHandler struct {
	deps AdoptSegmentCommissionDeps
}

func NewAdoptSegmentCommissionHandler(deps AdoptSegmentCommissionDeps) *AdoptSegmentCommissionHandler {
	return &AdoptSegmentCommissionHandler{deps: deps}
}

// Adopt 登记一次采用（ADR-0204）：声明过构造门 → 按（租户、段）分已有与冲突 → 段在册 → 委托在册 → 领域判服务方、取消、
// 首个参与与开始先后 → 落连线 → 把委托的开始写成已采用各段首个参与里最早的那一条（尚未开始的标开始，开始晚于这一段的
// 前移，其余不动）。
//
// 已有与冲突判在两项读之前：那一格答的是「这一段采用的是谁」，段与委托此后变成什么样都不改它——委托事后被另一段先开始了，
// 原样重放仍是原连线。
//
// 连线先落、委托后写：同一段的两份采用并发到达时，后到者在落连线那一步就撞上主键，什么都没写；委托与取消或另一段的采用并发
// 时，写开始那一笔的 WHERE 只认还配得上这次写的行，没写上就读回、按此刻的委托重判一遍——重判与首判是同一个领域函数，并发
// 窗口里的答法因此与没有并发时一致。**本编排答`采用成立`以外的任一格时，装配必须整笔回滚**：其余各格都不该留下任何写入，
// 而读回发现委托刚被取消的两格，连线已经落了。
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

	adoption, adopted, change, err := domain.AdoptCommissionForSegment(segment.Segment, commission.Commission, provider)
	if err != nil {
		return refusedCommissionAdoption(err, segment.Segment, commission), nil
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

	current := commission
	// 每一次没写上都说明委托在读与写之间被另一笔改过：首次开始没标上，是另一段先开始了它或它先被取消了；前移没落上，是另一段
	// 已把开始带到不晚于这一次。前者重判后至多再前移一次，后者重判即不动，所以三轮必定收口，再多就是委托册违约。
	for attempt := 0; attempt < maxCommissionStartAttempts; attempt++ {
		next := current
		next.Commission = adopted
		next.RecordedAt = now
		var written bool
		switch change {
		case domain.CommissionStartKept:
			return CommissionAdoptionResult{outcome: CommissionAdopted, adoption: record, hasAdoption: true, commission: current, hasCommission: true}, nil
		case domain.CommissionStartMarked:
			written, err = handler.deps.Commissions.Replace(ctx, next)
		case domain.CommissionStartAdvanced:
			written, err = handler.deps.StartAdvancement.AdvanceStart(ctx, next)
		default:
			return CommissionAdoptionResult{}, fmt.Errorf("%w: start change %d", ErrUnexpectedCommissionSave, change)
		}
		if err != nil {
			return commissionAdoptionUndecided(CommissionAdoptionCommissionStoreUnavailable, command.Segment), nil
		}
		if written {
			return CommissionAdoptionResult{outcome: CommissionAdopted, adoption: record, hasAdoption: true, commission: next, hasCommission: true}, nil
		}
		var found bool
		current, found, err = handler.deps.Commissions.FindByKey(ctx, commissionKey)
		if err != nil {
			return commissionAdoptionUndecided(CommissionAdoptionCommissionStoreUnavailable, command.Segment), nil
		}
		if !found {
			return CommissionAdoptionResult{}, fmt.Errorf("%w: %s", ErrAdoptedCommissionVanished, commissionRef)
		}
		// 先被取消的就采用不成，连线已落，靠装配整笔回滚撤掉；按业务时间分「开始之前取消」与「开始之后才登记取消」两格，与
		// 没有并发时同一个判法。
		_, adopted, change, err = domain.AdoptCommissionForSegment(segment.Segment, current.Commission, provider)
		if err != nil {
			return refusedCommissionAdoption(err, segment.Segment, current), nil
		}
	}
	return CommissionAdoptionResult{}, fmt.Errorf("%w: commission %s start not settled after %d attempts", ErrUnexpectedCommissionSave, commissionRef, maxCommissionStartAttempts)
}

// maxCommissionStartAttempts 是写委托开始的轮数上限，理由见 Adopt 里那一轮循环的注释。
const maxCommissionStartAttempts = 3

// refusedCommissionAdoption 把领域判不过的那一格翻成答复。`取消晚于开始`回显在册的委托与这一段的首个参与：登记方要拿两个
// 时刻对照，才知道该更正取消还是更正控制事实。
func refusedCommissionAdoption(
	err error,
	segment domain.ActualFulfillmentSegment,
	commission ports.TransportCommissionRecord,
) CommissionAdoptionResult {
	switch {
	case errors.Is(err, domain.ErrAdoptionProviderMismatch):
		return CommissionAdoptionResult{outcome: CommissionAdoptionProviderMismatch, commissionProvider: commission.Commission.Provider()}
	case errors.Is(err, domain.ErrCommissionCancelledAfterStart):
		first, _ := segment.FirstEffectiveParticipation()
		return CommissionAdoptionResult{
			outcome:           CommissionAdoptionCancelledAfterStart,
			commission:        commission,
			hasCommission:     true,
			segmentStartBasis: first.EntryBasis(),
			segmentStartedAt:  first.EnteredAt(),
		}
	case errors.Is(err, domain.ErrCommissionNotAdoptable):
		return CommissionAdoptionResult{outcome: CommissionNotAdoptable}
	case errors.Is(err, domain.ErrSegmentWithoutEffectiveParticipation):
		return CommissionAdoptionResult{outcome: CommissionAdoptionSegmentWithoutParticipation}
	case errors.Is(err, domain.ErrSegmentStartedBeforeCommission):
		return CommissionAdoptionResult{outcome: CommissionAdoptionSegmentStartedBeforeCommission}
	default:
		return CommissionAdoptionResult{outcome: CommissionAdoptionNotAccepted}
	}
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
