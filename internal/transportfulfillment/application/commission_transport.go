package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	// ErrUnexpectedAgreementCheck 说明协议核对口交回了封闭集合以外的格，或在册格缺了快照与法人——核对口违约，
	// 不是登记方能改的输入，也不是稍后再交就会好的依赖故障。
	ErrUnexpectedAgreementCheck = errors.New("transport fulfillment: unexpected supplier agreement check")
)

// CommissionOutcome 是委托/订舱编排的应用处理结果。`运输已开始`是业务负向格——已
// 开始的委托取消不了，只能按事实形成中断等实际结果（领域硬句编排分格）。协议核对不过的各格同是业务答案：
// 续办各不相同（去 party-commercial 发布或补正文、换一版此刻生效的、取得新协议、改服务方），折成一格登记方就只能
// 去猜（ADR-0029），所以逐格分开，不并进`未受理`。
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
	CommissionAgreementNotRegistered
	CommissionAgreementNotEffective
	CommissionAgreementTerminated
	CommissionProviderMismatch
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
		return "INPUT_NOT_ACCEPTED"
	case CommissionUndecided:
		return "COMMISSION_UNDECIDED"
	case CommissionAgreementNotRegistered:
		return "AGREEMENT_NOT_REGISTERED"
	case CommissionAgreementNotEffective:
		return "AGREEMENT_NOT_EFFECTIVE"
	case CommissionAgreementTerminated:
		return "AGREEMENT_TERMINATED"
	case CommissionProviderMismatch:
		return "PROVIDER_MISMATCH"
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
	SupplierAgreementUnavailable
)

func (reason CommissionUndecidedReason) String() string {
	switch reason {
	case CommissionStoreUnavailable:
		return "COMMISSION_STORE_UNAVAILABLE"
	case BookingStoreUnavailable:
		return "BOOKING_STORE_UNAVAILABLE"
	case AnswerStoreUnavailable:
		return "ANSWER_STORE_UNAVAILABLE"
	case SupplierAgreementUnavailable:
		return "SUPPLIER_AGREEMENT_UNAVAILABLE"
	default:
		return ""
	}
}

// SubmitCommissionCommand 携带一份运输委托提交。协议只由登记方指名（协议对象与版本标签两格），快照引用与采购责任
// 法人在核过之后从 party-commercial 那一版取，命令里没有这两格（ADR-0202 决定三）。条件、角色、责任仍是登记方声明的
// 快照引用，不核（ADR-0202 越权风险点）。
type SubmitCommissionCommand struct {
	TenantID         domain.TenantID
	Commission       string
	Provider         string
	AgreementObject  string
	AgreementVersion string
	Conditions       string
	Role             string
	Responsibility   string
	Members          []string
	SubmittedAt      time.Time
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
	outcome           CommissionOutcome
	reason            CommissionUndecidedReason
	commission        ports.TransportCommissionRecord
	booking           ports.BookingRecord
	answer            ports.BookingAnswerRecord
	hasRecord         bool
	continuation      string
	handoff           string
	agreementSupplier domain.ServiceProviderReference
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

// AgreementSupplier 只在`服务方不符`时有值：指名的那一版协议约定的供应商。登记方据此决定改服务方还是换协议。
func (result CommissionResult) AgreementSupplier() (domain.ServiceProviderReference, bool) {
	return result.agreementSupplier, result.outcome == CommissionProviderMismatch
}

type CommissionTransportDeps struct {
	Commissions ports.TransportCommissionStore
	Bookings    ports.BookingStore
	Answers     ports.BookingAnswerStore
	Agreements  ports.SupplierAgreementSource
	Downstream  ports.TransportCommissionHandoff
	Clock       ports.Clock
}

type CommissionTransportHandler struct {
	deps CommissionTransportDeps
}

func NewCommissionTransportHandler(deps CommissionTransportDeps) *CommissionTransportHandler {
	return &CommissionTransportHandler{deps: deps}
}

// SubmitCommission 提交运输委托（UC-TF-004 步 2、3）：草稿过构造门 → 按（租户+委托）分重放与冲突 → 在提交时点核
// 登记方指名的那一版供应商协议 → 以核出的快照与采购责任法人提交 → 意图与委托同笔入队。
//
// 重放判在核协议之前：同一份委托原样再交答原委托，哪怕那一版协议此后已到期或终止；也不为一次重放去问
// party-commercial。协议核对不过的各格不落任何行，登记方按格续办后原样重交即可。
func (handler *CommissionTransportHandler) SubmitCommission(
	ctx context.Context,
	command SubmitCommissionCommand,
) (CommissionResult, error) {
	draft, err := commissionDraftFrom(command)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}

	key := ports.TransportCommissionKey{TenantID: command.TenantID, Commission: draft.Commission()}
	_, digest, err := domain.CanonicalizeCommissionPayload(command.Provider, command.AgreementObject, command.AgreementVersion,
		command.Conditions, command.Role, command.Responsibility, command.SubmittedAt, command.Members)
	if err != nil {
		return CommissionResult{}, err
	}
	existing, found, err := handler.deps.Commissions.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(CommissionStoreUnavailable, command.Commission), nil
	}
	if found {
		return handler.answerFromStored(ctx, existing, digest, command.Commission), nil
	}

	check, err := handler.deps.Agreements.CheckSupplierAgreement(ctx, command.TenantID, draft.Agreement(), draft.Provider(), draft.SubmittedAt())
	if err != nil {
		// 读不动与 PC 侧坏数据在值上分不开（ports.SupplierAgreementSource 的头注），对命令面续办相同：稍后原样重交。
		return commissionUndecided(SupplierAgreementUnavailable, command.Commission), nil
	}
	switch check.Standing {
	case ports.SupplierAgreementRegistered:
	case ports.SupplierAgreementNotRegistered:
		return CommissionResult{outcome: CommissionAgreementNotRegistered}, nil
	case ports.SupplierAgreementNotEffective:
		return CommissionResult{outcome: CommissionAgreementNotEffective}, nil
	case ports.SupplierAgreementTerminated:
		return CommissionResult{outcome: CommissionAgreementTerminated}, nil
	case ports.SupplierAgreementProviderMismatch:
		return CommissionResult{outcome: CommissionProviderMismatch, agreementSupplier: check.Supplier}, nil
	default:
		return CommissionResult{}, fmt.Errorf("%w: standing %d", ErrUnexpectedAgreementCheck, check.Standing)
	}
	commission, err := draft.Submit(check.Snapshot, check.LegalEntity)
	if err != nil {
		return CommissionResult{}, fmt.Errorf("%w: registered without snapshot or legal entity: %v", ErrUnexpectedAgreementCheck, err)
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
		// 并发首交的输家照样按摘要分重放与冲突：先到者的内容可能与本次不同，读回赢家就答「已有」会把冲突吞掉。
		winner, found, err := handler.deps.Commissions.FindByKey(ctx, key)
		if err != nil || !found {
			return commissionUndecided(CommissionStoreUnavailable, command.Commission), nil
		}
		return handler.answerFromStored(ctx, winner, digest, command.Commission), nil
	default:
		return CommissionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedCommissionSave, saved)
	}
}

// answerFromStored 按已存委托作答：同内容是重放，交回原委托并重发同一份意图；异内容是冲突，保留原委托，不按最后到达
// 顶替；认不出形状答未决。
func (handler *CommissionTransportHandler) answerFromStored(
	ctx context.Context,
	stored ports.TransportCommissionRecord,
	digest string,
	subject string,
) CommissionResult {
	switch compareCommissionDigest(stored.ContentDigest, digest) {
	case domain.SamePayload:
		return handler.existingCommission(ctx, stored)
	case domain.DifferentPayload:
		return CommissionResult{outcome: CommissionConflict}
	default:
		return commissionUndecided(CommissionStoreUnavailable, subject)
	}
}

// compareCommissionDigest 比委托上的已存摘要。委托表没有 TFC-1 之前的行（TF 迁移 0025 在表里有行时拒绝施加），没有
// 无版本那一形可比，所以 CompareStoredDigest 的无版本一格交空串：不带前缀的已存摘要只会是坏行，比不上就不是重放。
func compareCommissionDigest(stored, current string) domain.StoredDigestComparison {
	return domain.CompareStoredDigest(stored, current, "")
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
	_, digest, err := domain.CanonicalizeBookingPayload(command.Commission, command.Quantity, command.Unit, command.RequestedAt)
	if err != nil {
		return CommissionResult{}, err
	}
	existing, found, err := handler.deps.Bookings.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
	}
	if found {
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversionedBookingDigest(command)) {
		case domain.SamePayload:
			return CommissionResult{outcome: BookingExistingResult, booking: existing, hasRecord: true}, nil
		case domain.DifferentPayload:
			return CommissionResult{outcome: BookingConflict}, nil
		default:
			return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
		}
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

	_, digest, err := domain.CanonicalizeBookingAnswerPayload(command.Outcome, command.Acceptance, command.Quantity, command.Basis, command.DecidedAt)
	if err != nil {
		return CommissionResult{}, err
	}
	existingAnswer, answered, err := handler.deps.Answers.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(AnswerStoreUnavailable, command.Booking), nil
	}
	if answered {
		switch domain.CompareStoredDigest(existingAnswer.ContentDigest, digest, unversionedAnswerDigest(command)) {
		case domain.SamePayload:
			return CommissionResult{outcome: BookingAnswerExists, answer: existingAnswer, hasRecord: true}, nil
		case domain.DifferentPayload:
			// 已接受的订舱不能再被拒绝（反之亦然）：应答不可覆盖，改约走撤回与新订舱。
			return CommissionResult{outcome: BookingAnswerConflict}, nil
		default:
			return commissionUndecided(AnswerStoreUnavailable, command.Booking), nil
		}
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

func commissionDraftFrom(command SubmitCommissionCommand) (domain.TransportCommissionDraft, error) {
	spec := domain.TransportCommissionDraftSpec{
		TenantID:    command.TenantID,
		SubmittedAt: command.SubmittedAt,
	}
	var err error
	if spec.Commission, err = domain.NewTransportCommissionReference(command.Commission); err != nil {
		return domain.TransportCommissionDraft{}, err
	}
	if spec.Provider, err = domain.NewServiceProviderReference(command.Provider); err != nil {
		return domain.TransportCommissionDraft{}, err
	}
	if spec.Agreement, err = domain.NewSupplierAgreementVersion(command.AgreementObject, command.AgreementVersion); err != nil {
		return domain.TransportCommissionDraft{}, err
	}
	if spec.Conditions, err = domain.NewConditionsSnapshotReference(command.Conditions); err != nil {
		return domain.TransportCommissionDraft{}, err
	}
	if spec.Role, err = domain.NewRoleSnapshotReference(command.Role); err != nil {
		return domain.TransportCommissionDraft{}, err
	}
	if spec.Responsibility, err = domain.NewResponsibilitySnapshotReference(command.Responsibility); err != nil {
		return domain.TransportCommissionDraft{}, err
	}
	for _, raw := range command.Members {
		member, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return domain.TransportCommissionDraft{}, err
		}
		spec.Members = append(spec.Members, member)
	}
	return domain.DraftTransportCommission(spec)
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

// handOff 把「委托已提交」意图入队，与委托同笔落库（装配点把整次提交包进一笔事务）。这封信封今天没有消费者，
// 照 ADR-0066 不登路由，派发如实答 `dispatch.no_subscriber`；供应商成本的来源是运输收费发生项加 BUY 评价
// （ADR-0164），不是委托。入队仍不能省：后继消费者要的就是这份按委托认领的意图（ADR-0043）。投递失败不翻结果，
// 留续办引用，重放时重发同一份。
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
