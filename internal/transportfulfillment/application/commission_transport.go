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
	// 不是登记方能改的输入，也不是稍后再交就会好的依赖故障。只认本层看得见的这几种；核对口以 error 交回的违约落未决。
	ErrUnexpectedAgreementCheck = errors.New("transport fulfillment: unexpected supplier agreement check")
	// ErrAnswerSourceMissing 说明承运答复命令没带答复方的来源身份。来源身份只取自集成客户端绑定（ADR-0202 决定二），绑定
	// 建不成就没有来源，所以缺它是装配违约，不是答复方能改的输入。
	ErrAnswerSourceMissing = errors.New("transport fulfillment: booking answer carries no answering source")
	// ErrBookingWithoutCommission 说明在册的订舱指名的委托读不回来。订舱只在委托在册时成立，委托又不删，读不回就是库里的
	// 数据违约；答「订舱不存在」会让承运方去核一个确实存在的订舱。
	ErrBookingWithoutCommission = errors.New("transport fulfillment: booking names a commission that is not registered")
)

// CommissionOutcome 是委托/订舱编排的应用处理结果。`运输已开始`是业务负向格——已
// 开始的委托取消不了，只能按事实形成中断等实际结果（领域硬句编排分格）。协议核对不过的各格同是业务答案：
// 续办各不相同（去 party-commercial 发布或补正文、换一版此刻生效的、取得新协议、改服务方），折成一格登记方就只能
// 去猜（ADR-0029），所以逐格分开，不并进`未受理`。
//
// 订舱与承运答复的引用格同一条纪律：委托不在册（去提交委托或改引用）、委托已取消（另起委托）、班次不在册（去登记班次或
// 改引在册班次）、订舱不在册（核订舱标识，或等运营侧把订舱录进来再答）、答复方不是该订舱所属委托的服务提供方（这份
// 答复不归它答）各自续办不同，不并进`未受理`。他租户名下的同名委托、班次与订舱按本租户的键读不回，同落「不在册」那一格。
//
// 取消两口沿用同一套格（ADR-0204 决定三）：委托不在册与订舱不在册各是一格，不并进`未受理`；运输已开始答
// `TRANSPORT_ALREADY_STARTED`，与领域哨兵 ErrTransportAlreadyStarted 同名（AT-TF-092）。取消的重放另起一格
// `EXISTING_CANCELLATION`，不借提交口的 `EXISTING_COMMISSION` / `EXISTING_BOOKING`：取消口的调用方拿到「已有委托」读不出
// 这是自己那一次取消的重放，还得去翻响应体里有没有取消时刻。
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
	CommissionNotFound
	BookingCommissionNotBookable
	BookingScheduleNotFound
	BookingNotFound
	BookingSourceNotProvider
	BookingCancelled
	CancellationExistingResult
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
		return "TRANSPORT_ALREADY_STARTED"
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
	case CommissionNotFound:
		return "COMMISSION_NOT_FOUND"
	case BookingCommissionNotBookable:
		return "COMMISSION_NOT_BOOKABLE"
	case BookingScheduleNotFound:
		return "SCHEDULE_NOT_FOUND"
	case BookingNotFound:
		return "BOOKING_NOT_FOUND"
	case BookingSourceNotProvider:
		return "SOURCE_NOT_SERVICE_PROVIDER"
	case BookingCancelled:
		return "BOOKING_CANCELLED"
	case CancellationExistingResult:
		return "EXISTING_CANCELLATION"
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
	ScheduleStoreUnavailable
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
	case ScheduleStoreUnavailable:
		return "SCHEDULE_STORE_UNAVAILABLE"
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

// SubmitBookingCommand 携带一次订舱申请：在哪一份已提交的委托下、就哪一个具体班次申请多少舱位（ADR-0202 决定五）。
type SubmitBookingCommand struct {
	TenantID    domain.TenantID
	Booking     string
	Commission  string
	Schedule    string
	Quantity    int64
	Unit        string
	RequestedAt time.Time
}

// AnswerBookingCommand 携带承运方对订舱的答复（封闭五值：接受/拒绝/失效/撤回/候补）。Source 是答复方的来源身份，
// 只取自集成客户端绑定（ADR-0202 决定二），不由载荷给——载荷里能写的来源等于自报。
type AnswerBookingCommand struct {
	TenantID   domain.TenantID
	Source     string
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

// CancelBookingCommand 携带订舱取消请求：只及尚未随运输开始而消耗的订舱范围。取消时刻照 ADR-0023 由提交方给。
type CancelBookingCommand struct {
	TenantID    domain.TenantID
	Booking     string
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
	Commissions          ports.TransportCommissionStore
	Bookings             ports.BookingStore
	BookingCancellations ports.BookingCancellationStore
	Answers              ports.BookingAnswerLedger
	Schedules            ports.TransportScheduleReader
	Agreements           ports.SupplierAgreementSource
	Downstream           ports.TransportCommissionHandoff
	Clock                ports.Clock
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
		// 读不动与 PC 侧坏数据在值上分不开（ports.SupplierAgreementSource 的头注），对命令面续办相同：稍后原样重交。适配器
		// 侧的不可译答复也落这一格：它的哨兵在适配器包里，本层认不出（ADR-0202 决定三与风险点 10）。
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

// SubmitBooking 提交订舱申请（UC-TF-004 步 1、3；ADR-0202 决定五）：订舱过构造门 → 按（租户+订舱）分重放与冲突 → 核
// 委托在册且未取消 → 核班次在册 → 落库。订舱不占容量、不等于接受（领域词条）；确认订舱与容量的关系另归一张票，这里不碰
// 任何池。
//
// 重放判在两项核对之前：那一格答的是「这次订舱是谁」，交回的是在册那一份，与委托、班次此后变成什么样无关——委托事后
// 取消了，原样重放仍是原订舱；先核对会把重放改答成去处理委托或班次，而处理完再投还是重放。委托核在班次之前：订舱属于
// 委托，委托不在册时班次在不在册已无从谈起。
//
// 订舱不入队：今天没有它的消费者（tf-chain-plan「跨票通则」X3），出站订舱连接器也不在本口（同计划 D02-8）。
func (handler *CommissionTransportHandler) SubmitBooking(
	ctx context.Context,
	command SubmitBookingCommand,
) (CommissionResult, error) {
	booking, err := bookingFrom(command)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}

	key := ports.BookingKey{TenantID: command.TenantID, Booking: booking.Booking()}
	_, digest, err := domain.CanonicalizeBookingPayload(command.Commission, command.Schedule, command.Quantity, command.Unit, command.RequestedAt)
	if err != nil {
		return CommissionResult{}, err
	}
	existing, found, err := handler.deps.Bookings.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
	}
	if found {
		return answerFromStoredBooking(existing, digest, command.Booking), nil
	}

	commission, found, err := handler.deps.Commissions.FindByKey(ctx, ports.TransportCommissionKey{
		TenantID:   command.TenantID,
		Commission: booking.Commission(),
	})
	if err != nil {
		return commissionUndecided(CommissionStoreUnavailable, command.Booking), nil
	}
	if !found {
		return CommissionResult{outcome: CommissionNotFound}, nil
	}
	if _, cancelled := commission.Commission.Cancelled(); cancelled {
		// 委托取消停止的正是尚未消耗的运输意图（CONTEXT 生命周期「尚未消耗的运输委托或订舱范围可以取消」），在它下面新订舱
		// 等于把停掉的意图又续上。已开始的委托不挡：开始之后仍可能为剩余范围订舱。
		return CommissionResult{outcome: BookingCommissionNotBookable}, nil
	}
	_, registered, err := handler.deps.Schedules.FindByKey(ctx, ports.ScheduleKey{TenantID: command.TenantID, Schedule: booking.Schedule()})
	if err != nil {
		return commissionUndecided(ScheduleStoreUnavailable, command.Booking), nil
	}
	if !registered {
		return CommissionResult{outcome: BookingScheduleNotFound}, nil
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
		// 并发首交的输家照样按摘要分重放与冲突，理由同委托：读回赢家就答「已有」会把冲突吞掉。
		winner, found, err := handler.deps.Bookings.FindByKey(ctx, key)
		if err != nil || !found {
			return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
		}
		return answerFromStoredBooking(winner, digest, command.Booking), nil
	default:
		return CommissionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedBookingSave, saved)
	}
}

// answerFromStoredBooking 按已存订舱作答：同内容是重放，异内容是冲突，认不出形状答未决。订舱表没有 TFC-1 之前的行
// （TF 迁移 0029 在表里有行时拒绝施加），无版本那一形交空串，理由同 compareCommissionDigest。
func answerFromStoredBooking(stored ports.BookingRecord, digest, subject string) CommissionResult {
	switch domain.CompareStoredDigest(stored.ContentDigest, digest, "") {
	case domain.SamePayload:
		return CommissionResult{outcome: BookingExistingResult, booking: stored, hasRecord: true}
	case domain.DifferentPayload:
		return CommissionResult{outcome: BookingConflict}
	default:
		return commissionUndecided(BookingStoreUnavailable, subject)
	}
}

// AnswerBooking 登记承运方对订舱的答复（UC-TF-004 步 4；ADR-0202 决定二、五）：订舱在册 → 答复方是该订舱所属委托的服务
// 提供方 → 同一答复标识分重放与冲突 → 答复过构造门 → 已有答复时只有候补接得上后继 → 落库。
//
// 来源核对排在重放之前：同一租户下的另一个承运商拿别人的答复标识原样重放，不该读回别人的答复。答复方与服务方按同一串
// 字面比，不比名称、不归一（ADR-0202 决定四同一条纪律）。
//
// 一次订舱至多一份定局：已接受的订舱不能再被拒绝，同订舱另一份答复是冲突不是覆盖；候补之后可以接一份定局，两份都留
// （domain.CarrierAcceptance.AdmitsSuccessor）。答复不入队，理由同订舱。
func (handler *CommissionTransportHandler) AnswerBooking(
	ctx context.Context,
	command AnswerBookingCommand,
) (CommissionResult, error) {
	if strings.TrimSpace(command.Source) == "" {
		return CommissionResult{}, ErrAnswerSourceMissing
	}
	bookingRef, err := domain.NewBookingReference(command.Booking)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}
	acceptanceRef, err := domain.NewCarrierAcceptanceReference(command.Acceptance)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}
	key := ports.BookingKey{TenantID: command.TenantID, Booking: bookingRef}

	bookingRecord, found, err := handler.deps.Bookings.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
	}
	if !found {
		return CommissionResult{outcome: BookingNotFound}, nil
	}
	commission, found, err := handler.deps.Commissions.FindByKey(ctx, ports.TransportCommissionKey{
		TenantID:   command.TenantID,
		Commission: bookingRecord.Booking.Commission(),
	})
	if err != nil {
		return commissionUndecided(CommissionStoreUnavailable, command.Booking), nil
	}
	if !found {
		return CommissionResult{}, fmt.Errorf("%w: booking %s, commission %s", ErrBookingWithoutCommission,
			bookingRef, bookingRecord.Booking.Commission())
	}
	if commission.Commission.Provider().String() != command.Source {
		return CommissionResult{outcome: BookingSourceNotProvider}, nil
	}

	_, digest, err := domain.CanonicalizeBookingAnswerPayload(command.Outcome, command.Acceptance, command.Quantity, command.Basis, command.DecidedAt)
	if err != nil {
		return CommissionResult{}, err
	}
	// 先锁后读：判后继要看见此刻的全部答复，锁到本笔事务结束，并发的另一份答复等这一笔落定再判。
	answers, err := handler.deps.Answers.FindAnswersForUpdate(ctx, key)
	if err != nil {
		return commissionUndecided(AnswerStoreUnavailable, command.Booking), nil
	}
	if stored, present := answerWithAcceptance(answers, acceptanceRef); present {
		return answerFromStoredAnswer(stored, digest, command), nil
	}

	acceptance, err := acceptanceFrom(command, bookingRecord.Booking)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}
	if head, answered := currentAnswer(answers); answered && !head.Acceptance.AdmitsSuccessor(acceptance) {
		// 已有定局，或这份不是候补的合格后继：应答不可覆盖，改约走撤回与新订舱。
		return CommissionResult{outcome: BookingAnswerConflict}, nil
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
		// 持锁之后仍撞约束，只能是不走本编排的写入先落：同一答复标识按摘要分重放与冲突；撞的是别的标识（另一份定局或
		// 候补先占了位）就是冲突。
		winners, err := handler.deps.Answers.FindAnswersForUpdate(ctx, key)
		if err != nil {
			return commissionUndecided(AnswerStoreUnavailable, command.Booking), nil
		}
		if stored, present := answerWithAcceptance(winners, acceptanceRef); present {
			return answerFromStoredAnswer(stored, digest, command), nil
		}
		return CommissionResult{outcome: BookingAnswerConflict}, nil
	default:
		return CommissionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedBookingSave, saved)
	}
}

// answerWithAcceptance 在同一订舱名下的答复里找同一答复标识的那一份。
func answerWithAcceptance(answers []ports.BookingAnswerRecord, acceptance domain.CarrierAcceptanceReference) (ports.BookingAnswerRecord, bool) {
	for _, stored := range answers {
		if stored.Acceptance.Acceptance() == acceptance {
			return stored, true
		}
	}
	return ports.BookingAnswerRecord{}, false
}

// currentAnswer 交这次订舱当前的那一份答复：有定局交定局，没有定局交最后一份候补。
func currentAnswer(answers []ports.BookingAnswerRecord) (ports.BookingAnswerRecord, bool) {
	for _, stored := range answers {
		if stored.Acceptance.Settles() {
			return stored, true
		}
	}
	if len(answers) == 0 {
		return ports.BookingAnswerRecord{}, false
	}
	return answers[len(answers)-1], true
}

// answerFromStoredAnswer 按同一答复标识的已存答复作答：同内容是重放，异内容是冲突（不覆盖），认不出形状答未决。
func answerFromStoredAnswer(stored ports.BookingAnswerRecord, digest string, command AnswerBookingCommand) CommissionResult {
	switch domain.CompareStoredDigest(stored.ContentDigest, digest, unversionedAnswerDigest(command)) {
	case domain.SamePayload:
		return CommissionResult{outcome: BookingAnswerExists, answer: stored, hasRecord: true}
	case domain.DifferentPayload:
		return CommissionResult{outcome: BookingAnswerConflict}
	default:
		return commissionUndecided(AnswerStoreUnavailable, command.Booking)
	}
}

// CancelCommission 取消未开始的运输意图（ADR-0204 决定三）：委托在册 → 已取消即重放 → 领域判开始与时刻 → 落取消。已开始
// 答`运输已开始`（AT-TF-092）：委托下的运输一经段采用就开始了，取消不了，只能按事实形成中断、改降、折返等实际结果；已经
// 形成的订舱、承运接受与容量预占各按自己的规则保留，取消不碰它们（CONTEXT 生命周期「尚未消耗的运输委托或订舱范围可以
// 取消」）。
//
// 取消是整份的：委托一旦开始就整份拒，没有「只停尚未开始的成员」这一种部分取消；取消成立时也不向尚未消耗的订舱或预占发
// 取消、释放请求——CONTEXT「上游运输意图取消只对尚未消耗的下游范围发出取消或释放请求」的下游那一步本编排没做，要运营人员
// 经取消订舱口、释放口逐个登记（ADR-0204 越权风险点「取消只有整份，也不往下游传」）。
//
// 已取消的重放答原取消、不二取，哪怕这次给的取消时刻不同：取消只有一次，第二个时刻没有可落的地方。
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
		return CommissionResult{outcome: CommissionNotFound}, nil
	}
	if answered, settled := answerFromSettledCommission(existing); settled {
		return answered, nil
	}

	cancelled, err := existing.Commission.Cancel(command.CancelledAt)
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
	if ok {
		return CommissionResult{outcome: CommissionCancelled, commission: replaced, hasRecord: true}, nil
	}
	// 读到时既未开始也未取消，落的那一刻却没落上：另一笔在中间先开始（段采用了它）或先取消了它。读回按此刻答，不答未受理
	// ——并发里先到的那一笔才是事实。
	current, found, err := handler.deps.Commissions.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(CommissionStoreUnavailable, command.Commission), nil
	}
	if !found {
		return CommissionResult{}, fmt.Errorf("%w: commission %s vanished during cancellation", ErrUnexpectedCommissionSave, commissionRef)
	}
	if answered, settled := answerFromSettledCommission(current); settled {
		return answered, nil
	}
	return CommissionResult{}, fmt.Errorf("%w: commission %s neither started nor cancelled after a refused cancellation", ErrUnexpectedCommissionSave, commissionRef)
}

// answerFromSettledCommission 对已开始或已取消的委托作取消的答复：已开始答运输已开始，已取消答原取消（回显在册那一份）。
func answerFromSettledCommission(record ports.TransportCommissionRecord) (CommissionResult, bool) {
	if _, _, started := record.Commission.TransportStarted(); started {
		return CommissionResult{outcome: CommissionTransportStarted, commission: record, hasRecord: true}, true
	}
	if _, cancelled := record.Commission.Cancelled(); cancelled {
		return CommissionResult{outcome: CancellationExistingResult, commission: record, hasRecord: true}, true
	}
	return CommissionResult{}, false
}

// CancelBooking 取消尚未消耗的订舱范围（ADR-0204 决定三）：订舱在册 → 已取消即重放 → 订舱所属委托下的运输已开始、且这次
// 订舱申请不晚于开始，答`运输已开始` → 领域判取消时刻 → 落取消。
//
// 「已开始」按订舱申请时刻对委托的开始时刻判：开始之前申请的舱位，可能正是已经开始的那一程在用的，取消不了；开始之后
// 仍可为剩余范围订舱（票 24 的核对次序不挡已开始的委托），那样的订舱尚未消耗，可以取消。逐订舱的消耗关系本上下文没有
// 记，这是能判出「尚可停止」的最近一层事实（ADR-0204 越权风险点）。
//
// 委托已取消不挡取消订舱：取消委托不向下游订舱传取消（CancelCommission 的头注），已取消委托下的订舱只能经这一口逐个停掉，
// 挡了它那些订舱就永远停不掉。已形成的承运答复不因取消而改写，接受是独立对象，本编排碰不到它。
func (handler *CommissionTransportHandler) CancelBooking(
	ctx context.Context,
	command CancelBookingCommand,
) (CommissionResult, error) {
	bookingRef, err := domain.NewBookingReference(command.Booking)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}
	key := ports.BookingKey{TenantID: command.TenantID, Booking: bookingRef}

	existing, found, err := handler.deps.Bookings.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
	}
	if !found {
		return CommissionResult{outcome: BookingNotFound}, nil
	}
	if _, cancelled := existing.Booking.Cancelled(); cancelled {
		return CommissionResult{outcome: CancellationExistingResult, booking: existing, hasRecord: true}, nil
	}
	commission, found, err := handler.deps.Commissions.FindByKey(ctx, ports.TransportCommissionKey{
		TenantID:   command.TenantID,
		Commission: existing.Booking.Commission(),
	})
	if err != nil {
		return commissionUndecided(CommissionStoreUnavailable, command.Booking), nil
	}
	if !found {
		return CommissionResult{}, fmt.Errorf("%w: booking %s, commission %s", ErrBookingWithoutCommission,
			bookingRef, existing.Booking.Commission())
	}
	if bookingConsumedByStart(existing.Booking, commission.Commission) {
		return CommissionResult{outcome: CommissionTransportStarted, booking: existing, hasRecord: true}, nil
	}

	cancelled, err := existing.Booking.Cancel(command.CancelledAt)
	if err != nil {
		return CommissionResult{outcome: CommissionNotAccepted}, nil
	}
	replaced := existing
	replaced.Booking = cancelled
	ok, err := handler.deps.BookingCancellations.Replace(ctx, replaced)
	if err != nil {
		return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
	}
	if ok {
		return CommissionResult{outcome: BookingCancelled, booking: replaced, hasRecord: true}, nil
	}
	// 读到时尚未取消，落的那一刻却没落上：另一笔先取消了它。读回答原取消。
	current, found, err := handler.deps.Bookings.FindByKey(ctx, key)
	if err != nil {
		return commissionUndecided(BookingStoreUnavailable, command.Booking), nil
	}
	if found {
		if _, cancelled := current.Booking.Cancelled(); cancelled {
			return CommissionResult{outcome: CancellationExistingResult, booking: current, hasRecord: true}, nil
		}
	}
	return CommissionResult{}, fmt.Errorf("%w: booking %s not cancelled after a refused cancellation", ErrUnexpectedBookingSave, bookingRef)
}

// bookingConsumedByStart 答订舱所属委托下的运输开始时，这次订舱是否已经提出：已提出的就可能正被那一程消耗。
func bookingConsumedByStart(booking domain.BookingRequest, commission domain.TransportCommission) bool {
	_, startedAt, started := commission.TransportStarted()
	return started && !booking.RequestedAt().After(startedAt)
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
	if spec.Schedule, err = domain.NewScheduleReference(command.Schedule); err != nil {
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
