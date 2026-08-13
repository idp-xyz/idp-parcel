package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrUnexpectedCommissionSave 说明委托库交回了封闭集合以外的写入结果。
	ErrUnexpectedCommissionSave = errors.New("transport fulfillment: unexpected commission save outcome")
	// ErrUnexpectedBookingSave 说明订舱库交回了封闭集合以外的写入结果。
	ErrUnexpectedBookingSave = errors.New("transport fulfillment: unexpected booking save outcome")
)

// CommissionOutcome 是委托/订舱编排的应用处理结果。`运输已开始`是业务负向格——已
// 开始的委托取消不了，只能按事实形成中断等实际结果（领域硬句编排分格）。
type CommissionOutcome uint8

const (
	CommissionOutcomeInvalid CommissionOutcome = iota
	CommissionSubmitted
	CommissionExistingResult
	CommissionConflict
	BookingSubmitted
	BookingExistingResult
	BookingConflict
	BookingAnswered
	BookingAnswerExists
	BookingAnswerConflict
	CommissionCancelled
	CommissionTransportStarted
	CommissionNotAccepted
	CommissionUndecided
)

func (outcome CommissionOutcome) String() string {
	switch outcome {
	case CommissionSubmitted:
		return "COMMISSION_SUBMITTED"
	case CommissionExistingResult:
		return "EXISTING_COMMISSION"
	case CommissionConflict:
		return "COMMISSION_CONFLICT"
	case BookingSubmitted:
		return "BOOKING_SUBMITTED"
	case BookingExistingResult:
		return "EXISTING_BOOKING"
	case BookingConflict:
		return "BOOKING_CONFLICT"
	case BookingAnswered:
		return "BOOKING_ANSWERED"
	case BookingAnswerExists:
		return "EXISTING_ANSWER"
	case BookingAnswerConflict:
		return "ANSWER_CONFLICT"
	case CommissionCancelled:
		return "COMMISSION_CANCELLED"
	case CommissionTransportStarted:
		return "TRANSPORT_STARTED"
	case CommissionNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case CommissionUndecided:
		return "COMMISSION_UNDECIDED"
	default:
		return ""
	}
}

// CommissionUndecidedReason 指名提交停在哪一步等谁。
type CommissionUndecidedReason uint8

const (
	CommissionUndecidedReasonNone CommissionUndecidedReason = iota
	CommissionStoreUnavailable
	BookingStoreUnavailable
	AnswerStoreUnavailable
)

func (reason CommissionUndecidedReason) String() string {
	switch reason {
	case CommissionStoreUnavailable:
		return "COMMISSION_STORE_UNAVAILABLE"
	case BookingStoreUnavailable:
		return "BOOKING_STORE_UNAVAILABLE"
	case AnswerStoreUnavailable:
		return "ANSWER_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// SubmitCommissionCommand 携带一份运输委托提交：协议/条件/角色/责任四快照在成形时
// 冻结（领域已钉，编排不补不改）。
type SubmitCommissionCommand struct {
	TenantID       domain.TenantID
	Commission     string
	Provider       string
	Agreement      string
	Conditions     string
	Role           string
	Responsibility string
	Members        []string
	SubmittedAt    time.Time
}

// SubmitBookingCommand 携带一次订舱申请。
type SubmitBookingCommand struct {
	TenantID    domain.TenantID
	Booking     string
	Commission  string
	Quantity    int64
	Unit        string
	RequestedAt time.Time
}

// AnswerBookingCommand 携带承运方对订舱的应答（封闭四值：接受/拒绝/失效/撤回）。
type AnswerBookingCommand struct {
	TenantID   domain.TenantID
	Booking    string
	Acceptance string
	Outcome    domain.CarrierAcceptanceOutcome
	Quantity   int64
	Basis      string
	DecidedAt  time.Time
}

// CancelCommissionCommand 携带取消请求：只及未开始的运输意图。
type CancelCommissionCommand struct {
	TenantID    domain.TenantID
	Commission  string
	CancelledAt time.Time
}

type CommissionResult struct {
	outcome      CommissionOutcome
	reason       CommissionUndecidedReason
	commission   ports.TransportCommissionRecord
	booking      ports.BookingRecord
	answer       ports.BookingAnswerRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result CommissionResult) Outcome() CommissionOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result CommissionResult) UndecidedReason() CommissionUndecidedReason {
	return result.reason
}

func (result CommissionResult) Commission() (ports.TransportCommissionRecord, bool) {
	return result.commission, result.hasRecord && result.commission.Key.Commission.String() != ""
}

func (result CommissionResult) Booking() (ports.BookingRecord, bool) {
	return result.booking, result.hasRecord && result.booking.Key.Booking.String() != ""
}

func (result CommissionResult) Answer() (ports.BookingAnswerRecord, bool) {
	return result.answer, result.hasRecord && result.answer.Key.Booking.String() != ""
}

func (result CommissionResult) ContinuationReference() string {
	return result.continuation
}

// CommissionHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result CommissionResult) CommissionHandoffReference() string {
	return result.handoff
}

type CommissionTransportDeps struct {
	Commissions ports.TransportCommissionStore
	Bookings    ports.BookingStore
	Answers     ports.BookingAnswerStore
	Downstream  ports.TransportCommissionHandoff
	Clock       ports.Clock
}

type CommissionTransportHandler struct {
	deps CommissionTransportDeps
}

func NewCommissionTransportHandler(deps CommissionTransportDeps) *CommissionTransportHandler {
	return &CommissionTransportHandler{deps: deps}
}

// SubmitCommission 提交运输委托：四快照由 SubmitTransportCommission 冻结 → 幂等按
// （租户+委托）分重放/冲突 → 意图交 SA 成本预期源。
func (handler *CommissionTransportHandler) SubmitCommission(
	ctx context.Context,
	command SubmitCommissionCommand,
) (CommissionResult, error) {
	commission, err := commissionFrom(command)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}

	key := ports.TransportCommissionKey{TenantID: command.TenantID, Commission: commission.Commission()}
	digest := commissionDigest(command)
	existing, found, err := handler.deps.Commissions.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(CommissionStoreUnavailable, command.Commission), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一委托标识携带不同快照或范围：冲突保留原委托，不按最后到达顶替。
			return CommissionResult{outcome: CommissionConflict}, nil
		}
		return handler.existingCommission(ctx, existing), nil
	}

	record := ports.TransportCommissionRecord{
		Key:           key,
		ContentDigest: digest,
		Commission:    commission,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Commissions.Save(ctx, record)
	if err != nil {
		return commissionUndecided(CommissionStoreUnavailable, command.Commission), nil
	}
	switch saved {
	case ports.CommissionSaved:
		result := CommissionResult{outcome: CommissionSubmitted, commission: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.CommissionAlreadyRegistered:
		winner, found, err := handler.deps.Commissions.FindByKey(ctx, key)
		if err != nil || !found {
			return commissionUndecided(CommissionStoreUnavailable, command.Commission), nil
		}
		return handler.existingCommission(ctx, winner), nil
	default:
		return CommissionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedCommissionSave, saved)
	}
}

// SubmitBooking 提交订舱申请：订舱不占容量、不等于接受（领域词条），幂等分重放/冲突。
func (handler *CommissionTransportHandler) SubmitBooking(
	ctx context.Context,
	command SubmitBookingCommand,
) (CommissionResult, error) {
	booking, err := bookingFrom(command)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}

	key := ports.BookingKey{TenantID: command.TenantID, Booking: booking.Booking()}
	digest := bookingDigest(command)
	existing, found, err := handler.deps.Bookings.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
	}
	if found {
		if existing.ContentDigest != digest {
			return CommissionResult{outcome: BookingConflict}, nil
		}
		return CommissionResult{outcome: BookingExistingResult, booking: existing, hasRecord: true}, nil
	}

	record := ports.BookingRecord{
		Key:           key,
		ContentDigest: digest,
		Booking:       booking,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Bookings.Save(ctx, record)
	if err != nil {
		return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
	}
	switch saved {
	case ports.BookingSaved:
		return CommissionResult{outcome: BookingSubmitted, booking: record, hasRecord: true}, nil
	case ports.BookingAlreadyRegistered:
		winner, found, err := handler.deps.Bookings.FindByKey(ctx, key)
		if err != nil || !found {
			return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
		}
		return CommissionResult{outcome: BookingExistingResult, booking: winner, hasRecord: true}, nil
	default:
		return CommissionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedBookingSave, saved)
	}
}

// AnswerBooking 登记承运方应答：一次订舱一个应答——已接受的订舱不能再被拒绝，同键
// 异应答是冲突不是覆盖；应答四格完备性由 FormCarrierAcceptance 把门（接受限申请量、
// 拒/失效/撤回带因）。
func (handler *CommissionTransportHandler) AnswerBooking(
	ctx context.Context,
	command AnswerBookingCommand,
) (CommissionResult, error) {
	bookingRef, err := domain.NewBookingReference(command.Booking)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}
	key := ports.BookingKey{TenantID: command.TenantID, Booking: bookingRef}

	bookingRecord, found, err := handler.deps.Bookings.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
	}
	if !found {
		// 应答一个不存在的订舱：提交矛盾，改单重来。
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}

	digest := answerDigest(command)
	existingAnswer, answered, err := handler.deps.Answers.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(AnswerStoreUnavailable, command.Booking), nil
	}
	if answered {
		if existingAnswer.ContentDigest != digest {
			// 已接受的订舱不能再被拒绝（反之亦然）：应答不可覆盖，改约走撤回与新订舱。
			return CommissionResult{outcome: BookingAnswerConflict}, nil
		}
		return CommissionResult{outcome: BookingAnswerExists, answer: existingAnswer, hasRecord: true}, nil
	}

	acceptance, err := acceptanceFrom(command, bookingRecord.Booking)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}
	record := ports.BookingAnswerRecord{
		Key:           key,
		ContentDigest: digest,
		Acceptance:    acceptance,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Answers.Save(ctx, record)
	if err != nil {
		return commissionUndecided(AnswerStoreUnavailable, command.Booking), nil
	}
	switch saved {
	case ports.BookingAnswerSaved:
		return CommissionResult{outcome: BookingAnswered, answer: record, hasRecord: true}, nil
	case ports.BookingAlreadyAnswered:
		winner, found, err := handler.deps.Answers.FindByKey(ctx, key)
		if err != nil || !found {
			return commissionUndecided(AnswerStoreUnavailable, command.Booking), nil
		}
		return CommissionResult{outcome: BookingAnswerExists, answer: winner, hasRecord: true}, nil
	default:
		return CommissionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedBookingSave, saved)
	}
}

// CancelCommission 取消未开始的运输意图：已开始 → TRANSPORT_STARTED 业务负向（只能
// 按事实形成中断等实际结果）；已取消重放 → 返回原取消。
func (handler *CommissionTransportHandler) CancelCommission(
	ctx context.Context,
	command CancelCommissionCommand,
) (CommissionResult, error) {
	commissionRef, err := domain.NewTransportCommissionReference(command.Commission)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}
	key := ports.TransportCommissionKey{TenantID: command.TenantID, Commission: commissionRef}

	existing, found, err := handler.deps.Commissions.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(CommissionStoreUnavailable, command.Commission), nil
	}
	if !found {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}
	if _, cancelled := existing.Commission.Cancelled(); cancelled {
		// 已取消重放：返回原取消，不二取。
		return CommissionResult{outcome: CommissionExistingResult, commission: existing, hasRecord: true}, nil
	}

	cancelled, err := existing.Commission.Cancel(command.CancelledAt)
	if errors.Is(err, domain.ErrTransportAlreadyStarted) {
		// 实际运输已开始：取消不了——业务负向，后续按事实形成中断/改降/折返。
		return CommissionResult{outcome: CommissionTransportStarted}, nil
	}
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}

	replaced := existing
	replaced.Commission = cancelled
	replaced.RecordedAt = handler.deps.Clock.Now()
	ok, err := handler.deps.Commissions.Replace(ctx, replaced)
	if err != nil {
		return commissionUndecided(CommissionStoreUnavailable, command.Commission), nil
	}
	if !ok {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}
	return CommissionResult{outcome: CommissionCancelled, commission: replaced, hasRecord: true}, nil
}

func commissionFrom(command SubmitCommissionCommand) (domain.TransportCommission, error) {
	spec := domain.TransportCommissionSpec{
		TenantID:    command.TenantID,
		SubmittedAt: command.SubmittedAt,
	}
	var err error
	if spec.Commission, err = domain.NewTransportCommissionReference(command.Commission); err != nil {
		return domain.TransportCommission{}, err
	}
	if spec.Provider, err = domain.NewServiceProviderReference(command.Provider); err != nil {
		return domain.TransportCommission{}, err
	}
	if spec.Agreement, err = domain.NewAgreementSnapshotReference(command.Agreement); err != nil {
		return domain.TransportCommission{}, err
	}
	if spec.Conditions, err = domain.NewConditionsSnapshotReference(command.Conditions); err != nil {
		return domain.TransportCommission{}, err
	}
	if spec.Role, err = domain.NewRoleSnapshotReference(command.Role); err != nil {
		return domain.TransportCommission{}, err
	}
	if spec.Responsibility, err = domain.NewResponsibilitySnapshotReference(command.Responsibility); err != nil {
		return domain.TransportCommission{}, err
	}
	for _, raw := range command.Members {
		member, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return domain.TransportCommission{}, err
		}
		spec.Members = append(spec.Members, member)
	}
	return domain.SubmitTransportCommission(spec)
}

func bookingFrom(command SubmitBookingCommand) (domain.BookingRequest, error) {
	spec := domain.BookingRequestSpec{
		TenantID:    command.TenantID,
		Quantity:    command.Quantity,
		RequestedAt: command.RequestedAt,
	}
	var err error
	if spec.Booking, err = domain.NewBookingReference(command.Booking); err != nil {
		return domain.BookingRequest{}, err
	}
	if spec.Commission, err = domain.NewTransportCommissionReference(command.Commission); err != nil {
		return domain.BookingRequest{}, err
	}
	if spec.Unit, err = domain.NewQuantityUnitReference(command.Unit); err != nil {
		return domain.BookingRequest{}, err
	}
	return domain.SubmitBookingRequest(spec)
}

func acceptanceFrom(command AnswerBookingCommand, booking domain.BookingRequest) (domain.CarrierAcceptance, error) {
	acceptanceRef, err := domain.NewCarrierAcceptanceReference(command.Acceptance)
	if err != nil {
		return domain.CarrierAcceptance{}, err
	}
	basis := domain.AcceptanceBasisReference{}
	if strings.TrimSpace(command.Basis) != "" {
		if basis, err = domain.NewAcceptanceBasisReference(command.Basis); err != nil {
			return domain.CarrierAcceptance{}, err
		}
	}
	return domain.FormCarrierAcceptance(booking, acceptanceRef, command.Outcome, command.Quantity, basis, command.DecidedAt)
}

func commissionUndecided(reason CommissionUndecidedReason, subject string) CommissionResult {
	return CommissionResult{
		outcome:      CommissionUndecided,
		reason:       reason,
		continuation: commissionContinuation(reason.String(), subject),
	}
}

// existingCommission 按已有委托作答并重发同一份意图。
func (handler *CommissionTransportHandler) existingCommission(
	ctx context.Context,
	record ports.TransportCommissionRecord,
) CommissionResult {
	return CommissionResult{
		outcome:    CommissionExistingResult,
		commission: record,
		hasRecord:  true,
		handoff:    handler.handOff(ctx, record),
	}
}

// handOff 把委托快照交给 settlement-accounting 作成本预期源。投递失败不翻结果，留
// 续办引用重放时重发同一份。
func (handler *CommissionTransportHandler) handOff(
	ctx context.Context,
	record ports.TransportCommissionRecord,
) string {
	if err := handler.deps.Downstream.HandOffTransportCommission(ctx, ports.TransportCommissionIntent{Record: record}); err == nil {
		return ""
	}
	return commissionContinuation("TRANSPORT_COMMISSION_HANDOFF", record.Key.TenantID.String(), record.Key.Commission.String())
}

func commissionContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

func commissionDigest(command SubmitCommissionCommand) string {
	members := append([]string(nil), command.Members...)
	sort.Strings(members)
	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		command.Provider,
		command.Agreement,
		command.Conditions,
		command.Role,
		command.Responsibility,
		command.SubmittedAt.UTC().Format(time.RFC3339Nano),
	}, members...), "\x00")))
	return hex.EncodeToString(digest[:])
}

func bookingDigest(command SubmitBookingCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Commission,
		fmt.Sprintf("%d", command.Quantity),
		command.Unit,
		command.RequestedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func answerDigest(command AnswerBookingCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.Outcome),
		command.Acceptance,
		fmt.Sprintf("%d", command.Quantity),
		command.Basis,
		command.DecidedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
