package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 解除履约段采用（ADR-0232 决定一至三；票 demo-chain-gaps/57）：持「解除履约段采用」授予的操作者撤回「这一段在这份委托下履约」那条
// 声明。只在这一段还没有按这份委托形成发生项时允许——已形成的那几项先经发生项失效口失效，解除才放行：解除之后那几项的采购上下文就
// 说不出出处了，而它们已经交给结算形成了预期成本。委托的开始随之撤回到已提交，或按其余已采用段最早的首个参与重新派生。

var (
	// ErrUnexpectedWithdrawalSave 说明解除册交回了封闭集合以外的写入结果。
	ErrUnexpectedWithdrawalSave = errors.New("transport fulfillment: unexpected segment commission adoption withdrawal save outcome")
	// ErrWithdrawnCommissionVanished 说明当前采用指名的委托读不回来。连线落册时委托在册（迁移 0035 的外键），委托又不删，读不回就是
	// 库里的数据违约。
	ErrWithdrawnCommissionVanished = errors.New("transport fulfillment: the commission of the adoption being withdrawn is not registered")
	// ErrCommissionStartMovedUnderHold 说明握着开始锁写委托的开始，在册的开始却已不是解除读到的那一份。采用与解除都先取这把锁，取消
	// 又挡在已开始上，走到这里只剩没走这把锁的写入者，重试不会变。
	ErrCommissionStartMovedUnderHold = errors.New("transport fulfillment: the commission start moved while its hold was held")
)

// AdoptionWithdrawalOutcome 是解除的应用结果。各格续办两两不同：不在册（这一段此刻没有采用，核段）、代次不是当前（点名的那一代早已
// 不是这一段的当前采用，也没有它的解除——回显当前那一代）、委托不符（点名那一代采用的是另一份——回显它）、已形成发生项（先让它们失效
// ——回显那几项）、未受理（改请求）、未决（原样重交）。
type AdoptionWithdrawalOutcome uint8

const (
	AdoptionWithdrawalOutcomeInvalid AdoptionWithdrawalOutcome = iota
	AdoptionWithdrawn
	AdoptionWithdrawalExisting
	AdoptionWithdrawalNotFound
	AdoptionWithdrawalGenerationNotCurrent
	AdoptionWithdrawalCommissionMismatch
	AdoptionWithdrawalOccurrencesFormed
	AdoptionWithdrawalNotAccepted
	AdoptionWithdrawalUndecided
)

func (outcome AdoptionWithdrawalOutcome) String() string {
	switch outcome {
	case AdoptionWithdrawn:
		return "ADOPTION_WITHDRAWN"
	case AdoptionWithdrawalExisting:
		return "EXISTING_WITHDRAWAL"
	case AdoptionWithdrawalNotFound:
		return "ADOPTION_NOT_FOUND"
	case AdoptionWithdrawalGenerationNotCurrent:
		return "GENERATION_NOT_CURRENT"
	case AdoptionWithdrawalCommissionMismatch:
		return "ADOPTION_MISMATCH"
	case AdoptionWithdrawalOccurrencesFormed:
		return "OCCURRENCES_FORMED"
	case AdoptionWithdrawalNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case AdoptionWithdrawalUndecided:
		return "WITHDRAWAL_UNDECIDED"
	default:
		return ""
	}
}

// AdoptionWithdrawalUndecidedReason 指名解除停在哪一口。
type AdoptionWithdrawalUndecidedReason uint8

const (
	AdoptionWithdrawalUndecidedReasonNone AdoptionWithdrawalUndecidedReason = iota
	AdoptionWithdrawalStoreUnavailable
	AdoptionWithdrawalOccurrenceRegistryUnavailable
	AdoptionWithdrawalCommissionStoreUnavailable
)

func (reason AdoptionWithdrawalUndecidedReason) String() string {
	switch reason {
	case AdoptionWithdrawalStoreUnavailable:
		return "ADOPTION_STORE_UNAVAILABLE"
	case AdoptionWithdrawalOccurrenceRegistryUnavailable:
		return "OCCURRENCE_REGISTRY_UNAVAILABLE"
	case AdoptionWithdrawalCommissionStoreUnavailable:
		return "COMMISSION_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// WithdrawSegmentCommissionAdoptionCommand 携带一次解除：哪一段、解的是哪一代采用、那一代采用的是哪一份委托。代次与委托都由登记方
// 点名而不由段推出：段上此刻的采用若已不是他看到的那一代（他那一次早已解除，之后又再采用了），原样重交的旧请求不该落在新那一代上——
// 同一份委托解除又再采用，只凭（段、委托）分不出这是重放还是新的一次。租户与操作者取自认证结果。
type WithdrawSegmentCommissionAdoptionCommand struct {
	TenantID   domain.TenantID
	Operator   string
	Segment    string
	Commission string
	Generation int
}

type AdoptionWithdrawalResult struct {
	outcome       AdoptionWithdrawalOutcome
	reason        AdoptionWithdrawalUndecidedReason
	withdrawal    ports.SegmentCommissionAdoptionWithdrawalRecord
	hasWithdrawal bool
	adoption      ports.SegmentCommissionAdoptionRecord
	hasAdoption   bool
	commission    ports.TransportCommissionRecord
	hasCommission bool
	occurrences   []ports.ChargeOccurrenceRecord
	continuation  string
}

func (result AdoptionWithdrawalResult) Outcome() AdoptionWithdrawalOutcome { return result.outcome }

func (result AdoptionWithdrawalResult) UndecidedReason() AdoptionWithdrawalUndecidedReason {
	return result.reason
}

// Withdrawal 在解除成立、重放与委托不符（点名那一代已解除、解的是另一份委托）时交回那一次解除。
func (result AdoptionWithdrawalResult) Withdrawal() (ports.SegmentCommissionAdoptionWithdrawalRecord, bool) {
	return result.withdrawal, result.hasWithdrawal
}

// Adoption 在解除成立、代次不是当前、委托不符与已形成发生项时交回段上那一份当前采用。
func (result AdoptionWithdrawalResult) Adoption() (ports.SegmentCommissionAdoptionRecord, bool) {
	return result.adoption, result.hasAdoption
}

// Commission 在解除成立时交回委托此后的样子：开始已撤回，或改按其余各段重新派生。
func (result AdoptionWithdrawalResult) Commission() (ports.TransportCommissionRecord, bool) {
	return result.commission, result.hasCommission
}

// FormedOccurrences 只在`已形成发生项`时有值：这一段上按这份委托形成、此刻仍有效的各项（链头）。登记方逐项经失效口失效之后再解除。
func (result AdoptionWithdrawalResult) FormedOccurrences() []ports.ChargeOccurrenceRecord {
	return append([]ports.ChargeOccurrenceRecord(nil), result.occurrences...)
}

func (result AdoptionWithdrawalResult) ContinuationReference() string { return result.continuation }

// WithdrawSegmentCommissionAdoptionDeps 全部必备。
type WithdrawSegmentCommissionAdoptionDeps struct {
	Adoptions       ports.SegmentCommissionAdoptionStore
	Withdrawals     ports.SegmentCommissionAdoptionWithdrawalStore
	CommissionLinks ports.SegmentCommissionAdoptionsByCommission
	Occurrences     ports.ChargeOccurrenceScopeView
	OccurrenceScope ports.ChargeOccurrenceScopeHold
	Commissions     ports.TransportCommissionView
	CommissionHold  ports.TransportCommissionStartHold
	StartCorrection ports.TransportCommissionStartCorrection
	Clock           ports.Clock
}

type WithdrawSegmentCommissionAdoptionHandler struct {
	deps WithdrawSegmentCommissionAdoptionDeps
}

func NewWithdrawSegmentCommissionAdoptionHandler(deps WithdrawSegmentCommissionAdoptionDeps) *WithdrawSegmentCommissionAdoptionHandler {
	return &WithdrawSegmentCommissionAdoptionHandler{deps: deps}
}

// Withdraw 受理 → 取这一段实际履约范围的发生项锁 → 点名那一代已有解除就按它答重放或委托不符 → 读当前采用（没有答不在册，不是点名
// 那一代答代次不是当前）→ 委托须同 → 取委托的开始锁 → 读委托 → 判这一段上按它形成、仍有效的发生项 → 读其余各段的当前采用 → 领域
// 判开始怎么变 → 落解除 → 写委托的开始。
//
// 两把锁各管一种并发（ADR-0232 决定六）。发生项锁与登记查覆盖是同一把：登记在锁下重读采用，解除在锁下读这一段的发生项，两者排成
// 先后——登记先提交，解除读得到那一项、答已形成发生项；解除先提交，登记重读到采用已不在、答未决让登记方重交。开始锁与采用共用：
// 采用读到委托已开始、这一段不更早就不写委托，解除读其余各段再定开始，两笔若并发会各凭对方还没提交的样子判。先发生项锁、后开始锁，
// 采用只取后一把、登记只取前一把，不成环。
//
// **本编排答`解除成立`以外的任一格时，装配必须整笔回滚**：其余各格都不该留下写入。
func (handler *WithdrawSegmentCommissionAdoptionHandler) Withdraw(
	ctx context.Context,
	command WithdrawSegmentCommissionAdoptionCommand,
) (AdoptionWithdrawalResult, error) {
	segmentRef, err := domain.NewFulfillmentSegmentReference(command.Segment)
	if err != nil || strings.TrimSpace(command.TenantID.String()) == "" {
		return AdoptionWithdrawalResult{outcome: AdoptionWithdrawalNotAccepted}, nil
	}
	commissionRef, err := domain.NewTransportCommissionReference(command.Commission)
	if err != nil {
		return AdoptionWithdrawalResult{outcome: AdoptionWithdrawalNotAccepted}, nil
	}
	operator, err := domain.NewRegistrantReference(command.Operator)
	if err != nil || command.Generation < 1 {
		return AdoptionWithdrawalResult{outcome: AdoptionWithdrawalNotAccepted}, nil
	}
	scope, err := domain.SegmentOccurrenceScope(segmentRef)
	if err != nil {
		return AdoptionWithdrawalResult{outcome: AdoptionWithdrawalNotAccepted}, nil
	}
	continuation := commissionContinuation("SEGMENT_COMMISSION_ADOPTION_WITHDRAWAL", command.TenantID.String(), segmentRef.String())

	if err := handler.deps.OccurrenceScope.HoldOccurrenceScope(ctx, command.TenantID, domain.ActualFulfillmentOccurrence, scope); err != nil {
		return withdrawalUndecided(AdoptionWithdrawalOccurrenceRegistryUnavailable, continuation), nil
	}
	key := ports.SegmentCommissionAdoptionKey{TenantID: command.TenantID, Segment: segmentRef}
	if answer, answered, err := handler.answerFromWithdrawal(ctx, key, command.Generation, commissionRef, continuation); err != nil || answered {
		return answer, err
	}
	current, adopted, err := handler.deps.Adoptions.FindBySegment(ctx, key)
	if err != nil {
		return withdrawalUndecided(AdoptionWithdrawalStoreUnavailable, continuation), nil
	}
	if !adopted {
		return AdoptionWithdrawalResult{outcome: AdoptionWithdrawalNotFound}, nil
	}
	if current.Generation != command.Generation {
		return AdoptionWithdrawalResult{outcome: AdoptionWithdrawalGenerationNotCurrent, adoption: current, hasAdoption: true}, nil
	}
	if current.Adoption.Commission() != commissionRef {
		return AdoptionWithdrawalResult{outcome: AdoptionWithdrawalCommissionMismatch, adoption: current, hasAdoption: true}, nil
	}

	commissionKey := ports.TransportCommissionKey{TenantID: command.TenantID, Commission: commissionRef}
	if err := handler.deps.CommissionHold.HoldCommissionStart(ctx, commissionKey); err != nil {
		return withdrawalUndecided(AdoptionWithdrawalCommissionStoreUnavailable, continuation), nil
	}
	commission, found, err := handler.deps.Commissions.FindByKey(ctx, commissionKey)
	if err != nil {
		return withdrawalUndecided(AdoptionWithdrawalCommissionStoreUnavailable, continuation), nil
	}
	if !found {
		return AdoptionWithdrawalResult{}, fmt.Errorf("%w: segment %s, commission %s", ErrWithdrawnCommissionVanished, segmentRef, commissionRef)
	}

	heads, err := handler.deps.Occurrences.ListCurrentOnScope(ctx, command.TenantID, domain.ActualFulfillmentOccurrence, scope)
	if err != nil {
		return withdrawalUndecided(AdoptionWithdrawalOccurrenceRegistryUnavailable, continuation), nil
	}
	if formed := formedUnder(heads, commission.Commission); len(formed) > 0 {
		return AdoptionWithdrawalResult{
			outcome:     AdoptionWithdrawalOccurrencesFormed,
			adoption:    current,
			hasAdoption: true,
			occurrences: formed,
		}, nil
	}

	links, err := handler.deps.CommissionLinks.ListByCommission(ctx, commissionKey)
	if err != nil {
		return withdrawalUndecided(AdoptionWithdrawalStoreUnavailable, continuation), nil
	}
	remaining := make([]domain.SegmentCommissionAdoption, 0, len(links))
	for _, link := range links {
		if link.Key.Segment != segmentRef {
			remaining = append(remaining, link.Adoption)
		}
	}
	withdrawal, corrected, err := domain.WithdrawSegmentCommissionAdoption(current.Adoption, current.Generation, commission.Commission, remaining, operator)
	if err != nil {
		return AdoptionWithdrawalResult{}, fmt.Errorf("withdraw adoption of segment %s from commission %s: %w", segmentRef, commissionRef, err)
	}

	now := handler.deps.Clock.Now().UTC()
	record := ports.SegmentCommissionAdoptionWithdrawalRecord{Key: key, Withdrawal: withdrawal, RecordedAt: now}
	saved, err := handler.deps.Withdrawals.SaveWithdrawal(ctx, record)
	if err != nil {
		return withdrawalUndecided(AdoptionWithdrawalStoreUnavailable, continuation), nil
	}
	switch saved {
	case ports.AdoptionSaved:
	case ports.AdoptionAlreadyRegistered:
		// 同一段并发的另一次解除先落了同一代：它握过同一把锁、已写好委托的开始，读回它作答。
		if answer, answered, err := handler.answerFromWithdrawal(ctx, key, command.Generation, commissionRef, continuation); err != nil || answered {
			return answer, err
		}
		return withdrawalUndecided(AdoptionWithdrawalStoreUnavailable, continuation), nil
	default:
		return AdoptionWithdrawalResult{}, fmt.Errorf("%w: %d", ErrUnexpectedWithdrawalSave, saved)
	}

	next := commission
	next.Commission = corrected
	next.RecordedAt = now
	if withdrawal.StartChange() != domain.CommissionStartUnchanged {
		beforeBasis, beforeAt := withdrawal.StartBefore()
		written, err := handler.deps.StartCorrection.CorrectStart(ctx, next, beforeBasis, beforeAt)
		if err != nil {
			return withdrawalUndecided(AdoptionWithdrawalCommissionStoreUnavailable, continuation), nil
		}
		if !written {
			return AdoptionWithdrawalResult{}, fmt.Errorf("%w: commission %s", ErrCommissionStartMovedUnderHold, commissionRef)
		}
	} else {
		next = commission
	}
	return AdoptionWithdrawalResult{
		outcome:       AdoptionWithdrawn,
		withdrawal:    record,
		hasWithdrawal: true,
		adoption:      current,
		hasAdoption:   true,
		commission:    next,
		hasCommission: true,
	}, nil
}

// answerFromWithdrawal 按点名那一代的解除作答：那一代已解除、解的正是点名的委托，是重放；解的是另一份委托，是委托不符。那一代没有
// 解除答 answered=false，由调用方往下判。幂等因此按（段、委托、代次）认：解除之后同一份委托再采用，旧那一次原样重交点名的仍是旧那一代，
// 答重放，不落到新那一代上。
func (handler *WithdrawSegmentCommissionAdoptionHandler) answerFromWithdrawal(
	ctx context.Context,
	key ports.SegmentCommissionAdoptionKey,
	generation int,
	commission domain.TransportCommissionReference,
	continuation string,
) (AdoptionWithdrawalResult, bool, error) {
	withdrawn, found, err := handler.deps.Withdrawals.FindWithdrawal(ctx, key, generation)
	if err != nil {
		return withdrawalUndecided(AdoptionWithdrawalStoreUnavailable, continuation), true, nil
	}
	if !found {
		return AdoptionWithdrawalResult{}, false, nil
	}
	if withdrawn.Withdrawal.Commission() != commission {
		return AdoptionWithdrawalResult{outcome: AdoptionWithdrawalCommissionMismatch, withdrawal: withdrawn, hasWithdrawal: true}, true, nil
	}
	return AdoptionWithdrawalResult{outcome: AdoptionWithdrawalExisting, withdrawal: withdrawn, hasWithdrawal: true}, true, nil
}

// formedUnder 挑出段上仍有效的实际履约发生项里采购上下文出自这份委托的那几项：服务方同，协议快照是委托提交那一版或任一次换版的后继
// （TransportCommission.BoundAgreementSnapshot）。段已采用委托时登记从委托取这两样（ADR-0207 文末补记），换过版之后取业务时间所凭的那一版
// （ADR-0231 决定四），所以凡落在这一集里的都算按它形成；只比提交那一版，换版之后按后继形成的项就认不出、解除被放行。登记方在采用之前
// 自报了另一家、另一版协议的那几项不是这份委托的成本来源，解除不挡它们。
func formedUnder(heads []ports.ChargeOccurrenceRecord, commission domain.TransportCommission) []ports.ChargeOccurrenceRecord {
	var formed []ports.ChargeOccurrenceRecord
	for _, head := range heads {
		if head.Occurrence.Provider() == commission.Provider() && commission.BoundAgreementSnapshot(head.Occurrence.Agreement()) {
			formed = append(formed, head)
		}
	}
	return formed
}

func withdrawalUndecided(reason AdoptionWithdrawalUndecidedReason, continuation string) AdoptionWithdrawalResult {
	return AdoptionWithdrawalResult{outcome: AdoptionWithdrawalUndecided, reason: reason, continuation: continuation}
}
