package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// AcceptPreAdviceOutcome 是入库预报提交、告知更新与撤回的结果，平铺，不套登记册的 NOT_ACCEPTED。
type AcceptPreAdviceOutcome uint8

const (
	AcceptPreAdviceOutcomeInvalid AcceptPreAdviceOutcome = iota
	PreAdviceOutcomeAdvised
	PreAdviceOutcomeNoticeUpdated
	PreAdviceOutcomeCancelled
	PreAdviceOutcomeExistingResult
	PreAdviceOutcomeSourceConflict
	PreAdviceOutcomeWarehouseNotFound
	PreAdviceOutcomeNotConfigured
	PreAdviceOutcomeWarehouseNotAccepting
	PreAdviceOutcomeContentInvalid
	PreAdviceOutcomeStatusMismatch
	PreAdviceOutcomeReceiptStarted
	PreAdviceOutcomeNotFound
	PreAdviceOutcomeNumberingRuleCollision
)

func (outcome AcceptPreAdviceOutcome) String() string {
	switch outcome {
	case PreAdviceOutcomeAdvised:
		return "ADVISED"
	case PreAdviceOutcomeNoticeUpdated:
		return "NOTICE_UPDATED"
	case PreAdviceOutcomeCancelled:
		return "CANCELLED"
	case PreAdviceOutcomeExistingResult:
		return "EXISTING_RESULT"
	case PreAdviceOutcomeSourceConflict:
		return "SOURCE_CONFLICT"
	case PreAdviceOutcomeWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case PreAdviceOutcomeNotConfigured:
		return "NOT_CONFIGURED"
	case PreAdviceOutcomeWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	case PreAdviceOutcomeContentInvalid:
		return "CONTENT_INVALID"
	case PreAdviceOutcomeStatusMismatch:
		return "STATUS_MISMATCH"
	case PreAdviceOutcomeReceiptStarted:
		return "RECEIPT_STARTED"
	case PreAdviceOutcomeNotFound:
		return "PRE_ADVICE_NOT_FOUND"
	case PreAdviceOutcomeNumberingRuleCollision:
		return "NUMBERING_RULE_COLLISION"
	default:
		return ""
	}
}

// WarehouseNotAcceptingReason 指出仓不受理是因为停用还是代理仓。
type WarehouseNotAcceptingReason uint8

const (
	WarehouseNotAcceptingReasonInvalid WarehouseNotAcceptingReason = iota
	WarehouseNotAcceptingDeactivated
	WarehouseNotAcceptingAgent
)

func (reason WarehouseNotAcceptingReason) String() string {
	switch reason {
	case WarehouseNotAcceptingDeactivated:
		return "DEACTIVATED"
	case WarehouseNotAcceptingAgent:
		return "AGENT"
	default:
		return ""
	}
}

// AcceptPreAdviceResult 携带落点。预报只在受理、已有结果、告知已更新与已取消上有；
// 告知那一版在告知已更新与告知的已有结果上有；不合之处只在内容不合上有。
type AcceptPreAdviceResult struct {
	outcome   AcceptPreAdviceOutcome
	advice    domain.PreAdvice
	hasAdvice bool
	notice    domain.PreAdviceNotice
	hasNotice bool
	reason    WarehouseNotAcceptingReason
	status    domain.PreAdviceStatus
	hasStatus bool
	issues    []domain.PreAdviceContentIssue
}

func (result AcceptPreAdviceResult) Outcome() AcceptPreAdviceOutcome { return result.outcome }

func (result AcceptPreAdviceResult) PreAdvice() (domain.PreAdvice, bool) {
	return result.advice, result.hasAdvice
}

func (result AcceptPreAdviceResult) Notice() (domain.PreAdviceNotice, bool) {
	return result.notice, result.hasNotice
}

func (result AcceptPreAdviceResult) NotAcceptingReason() (WarehouseNotAcceptingReason, bool) {
	return result.reason, result.outcome == PreAdviceOutcomeWarehouseNotAccepting
}

func (result AcceptPreAdviceResult) Status() (domain.PreAdviceStatus, bool) {
	return result.status, result.hasStatus
}

func (result AcceptPreAdviceResult) Issues() []domain.PreAdviceContentIssue {
	return slices.Clone(result.issues)
}

// PreAdviceReceiptOutcome 是给收货侧的缝：第一次收货确认把预报推进收货中。
type PreAdviceReceiptOutcome uint8

const (
	PreAdviceReceiptOutcomeInvalid PreAdviceReceiptOutcome = iota
	PreAdviceReceiptStarted
	PreAdviceReceiptAlreadyReceiving
	PreAdviceReceiptNotReceivable
)

func (outcome PreAdviceReceiptOutcome) String() string {
	switch outcome {
	case PreAdviceReceiptStarted:
		return "STARTED"
	case PreAdviceReceiptAlreadyReceiving:
		return "ALREADY_RECEIVING"
	case PreAdviceReceiptNotReceivable:
		return "NOT_RECEIVABLE"
	default:
		return ""
	}
}

// NotePreAdviceReceiptResult 回显缝跑完之后的那份预报。
type NotePreAdviceReceiptResult struct {
	outcome   PreAdviceReceiptOutcome
	advice    domain.PreAdvice
	hasAdvice bool
}

func (result NotePreAdviceReceiptResult) Outcome() PreAdviceReceiptOutcome { return result.outcome }

func (result NotePreAdviceReceiptResult) PreAdvice() (domain.PreAdvice, bool) {
	return result.advice, result.hasAdvice
}

// PreAdviceCloseOutcome 是给完结侧的缝。
type PreAdviceCloseOutcome uint8

const (
	PreAdviceCloseOutcomeInvalid PreAdviceCloseOutcome = iota
	PreAdviceCloseClosed
	PreAdviceCloseAlreadyClosed
	PreAdviceCloseNotClosable
)

func (outcome PreAdviceCloseOutcome) String() string {
	switch outcome {
	case PreAdviceCloseClosed:
		return "CLOSED"
	case PreAdviceCloseAlreadyClosed:
		return "ALREADY_CLOSED"
	case PreAdviceCloseNotClosable:
		return "NOT_CLOSABLE"
	default:
		return ""
	}
}

// NotePreAdviceCloseResult 回显缝跑完之后的那份预报。
type NotePreAdviceCloseResult struct {
	outcome   PreAdviceCloseOutcome
	advice    domain.PreAdvice
	hasAdvice bool
}

func (result NotePreAdviceCloseResult) Outcome() PreAdviceCloseOutcome { return result.outcome }

func (result NotePreAdviceCloseResult) PreAdvice() (domain.PreAdvice, bool) {
	return result.advice, result.hasAdvice
}

// AcceptPreAdviceCommand 是提交一份入库预报。租户与登记责任方取自信封；货主在货主口取自信封，代录时是载荷里的业务内容。
type AcceptPreAdviceCommand struct {
	Tenant     domain.TenantID
	Owner      domain.OwnerAccountID
	Warehouse  domain.WarehouseCode
	Arrival    domain.ArrivalMode
	Inbound    domain.InboundMode
	GoodsLines []domain.GoodsInboundLine
	Cartons    []domain.Carton
	ExpectedAt time.Time
	Clue       domain.ArrivalClue
	Reference  domain.OwnerReference
	Submission domain.PreAdviceSubmission
	Registrant domain.Registrant
}

func (command AcceptPreAdviceCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.Owner.String() == "":
		return errors.New("缺货主")
	case command.Warehouse.String() == "":
		return errors.New("缺仓")
	case command.Arrival.String() == "":
		return errors.New("缺到仓方式")
	case command.Inbound.String() == "":
		return errors.New("缺入库方式")
	case command.ExpectedAt.IsZero():
		return errors.New("缺预计到仓时间")
	case command.Registrant.String() == "":
		return errors.New("缺登记责任方")
	case command.Submission.Path() == domain.SubmissionPathInvalid || command.Submission.Key().String() == "":
		return errors.New("缺提交")
	case command.Inbound == domain.InboundByGoods && len(command.Cartons) != 0:
		return errors.New("入库方式与明细对不上")
	case command.Inbound == domain.InboundByCarton && len(command.GoodsLines) != 0:
		return errors.New("入库方式与明细对不上")
	default:
		return nil
	}
}

// UpdatePreAdviceNoticeCommand 更新告知。代录不带货主，货主取自预报。
type UpdatePreAdviceNoticeCommand struct {
	Tenant     domain.TenantID
	Owner      domain.OwnerAccountID
	ID         domain.PreAdviceID
	ExpectedAt time.Time
	Clue       domain.ArrivalClue
	Reference  domain.OwnerReference
	Submission domain.PreAdviceSubmission
	Registrant domain.Registrant
}

func (command UpdatePreAdviceNoticeCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.ID.String() == "":
		return errors.New("缺预报")
	case command.ExpectedAt.IsZero():
		return errors.New("缺预计到仓时间")
	case command.Registrant.String() == "":
		return errors.New("缺登记责任方")
	case command.Submission.Path() == domain.SubmissionPathInvalid || command.Submission.Key().String() == "":
		return errors.New("缺提交")
	case command.Submission.Path() == domain.SubmissionByOwner && command.Owner.String() == "":
		return errors.New("缺货主")
	default:
		return nil
	}
}

// WithdrawPreAdviceCommand 撤回一份预报。代录不带货主。
type WithdrawPreAdviceCommand struct {
	Tenant     domain.TenantID
	Owner      domain.OwnerAccountID
	ID         domain.PreAdviceID
	Submission domain.PreAdviceSubmission
	Registrant domain.Registrant
}

func (command WithdrawPreAdviceCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.ID.String() == "":
		return errors.New("缺预报")
	case command.Registrant.String() == "":
		return errors.New("缺登记责任方")
	case command.Submission.Path() == domain.SubmissionPathInvalid || command.Submission.Key().String() == "":
		return errors.New("缺提交")
	case command.Submission.Path() == domain.SubmissionByOwner && command.Owner.String() == "":
		return errors.New("缺货主")
	default:
		return nil
	}
}

// NotePreAdviceCommand 是收货缝与完结缝的指名：租户与预报标识。
type NotePreAdviceCommand struct {
	Tenant domain.TenantID
	ID     domain.PreAdviceID
}

func (command NotePreAdviceCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.ID.String() == "":
		return errors.New("缺预报")
	default:
		return nil
	}
}

// AcceptPreAdviceHandler 受理入库预报，并接告知更新、撤回和给收货侧留的两条缝。
// 事务边界在调用方。库存账不读不写：预报不是库存。
type AcceptPreAdviceHandler struct {
	preAdvices   ports.PreAdviceStore
	warehouses   ports.WarehouseLookup
	anyWarehouse ports.TenantWarehouseLookup
	goods        ports.StockGoodsLookup
	numbers      ports.PreAdviceNumberIssuer
	identities   ports.PreAdviceIdentities
	owners       ports.OwnerAccountLookup
	clock        ports.Clock
}

func NewAcceptPreAdviceHandler(
	preAdvices ports.PreAdviceStore,
	warehouses ports.WarehouseLookup,
	anyWarehouse ports.TenantWarehouseLookup,
	goods ports.StockGoodsLookup,
	numbers ports.PreAdviceNumberIssuer,
	identities ports.PreAdviceIdentities,
	owners ports.OwnerAccountLookup,
	clock ports.Clock,
) *AcceptPreAdviceHandler {
	return &AcceptPreAdviceHandler{
		preAdvices: preAdvices, warehouses: warehouses, anyWarehouse: anyWarehouse, goods: goods,
		numbers: numbers, identities: identities, owners: owners, clock: clock,
	}
}

// Accept 受理一份入库预报。判定次序：
//
//  1. 缺租户、货主、仓、到仓方式、入库方式、预计到仓时间、提交或登记责任方：装配错，交 error；
//  2. 代录先核货主客户账户，查不到交 ErrOwnerAccountNotInTenant，不读仓、不落册；货主口不问；
//  3. 按（租户，途径，来源，键）认重放与冲突。内容相同答 EXISTING_RESULT 并回显在册的那一份与原号；重放不取号、不签发标识；
//  4. 仓没登记答 WAREHOUSE_NOT_FOUND；已停用答 WAREHOUSE_NOT_ACCEPTING/DEACTIVATED；代理仓答 AGENT；
//  5. 明细逐行收齐，任一处不合答 CONTENT_INVALID，不落、不取号；
//  6. 同一事务里取号再落成已预报，告知第 1 版同笔写入。撞号答 NUMBERING_RULE_COLLISION，不留预报。
//
// 重放排在核仓之前：仓后来停用，原样重投仍是已经形成的答案。
func (handler *AcceptPreAdviceHandler) Accept(
	ctx context.Context,
	command AcceptPreAdviceCommand,
) (AcceptPreAdviceResult, error) {
	if err := command.assemblyError(); err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
	}
	if command.Submission.Path() == domain.SubmissionOnBehalf {
		if err := handler.requireOwner(ctx, command.Tenant, command.Owner); err != nil {
			return AcceptPreAdviceResult{}, err
		}
	}
	if err := handler.preAdvices.HoldSubmission(ctx, command.Tenant, command.Submission); err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
	}
	existing, found, err := handler.preAdvices.FindBySubmission(ctx, command.Tenant, command.Submission)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
	}
	if found {
		return submissionReplay(existing, command), nil
	}

	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
	}
	if !found {
		return AcceptPreAdviceResult{outcome: PreAdviceOutcomeWarehouseNotFound}, nil
	}
	if warehouse.Content().Deactivated() {
		return notAccepting(WarehouseNotAcceptingDeactivated), nil
	}
	if warehouse.Content().Kind() != domain.WarehouseKindSelfOperated {
		return notAccepting(WarehouseNotAcceptingAgent), nil
	}

	issues, err := handler.contentIssues(ctx, command)
	if err != nil {
		return AcceptPreAdviceResult{}, err
	}
	if len(issues) > 0 {
		return AcceptPreAdviceResult{outcome: PreAdviceOutcomeContentInvalid, issues: issues}, nil
	}

	issued, err := handler.numbers.IssuePreAdviceNumber(ctx, command.Tenant, warehouse.Content().BusinessTimeZone())
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
	}
	number, absence, collided, err := numberFrom(issued)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
	}
	if collided {
		return AcceptPreAdviceResult{outcome: PreAdviceOutcomeNumberingRuleCollision}, nil
	}

	id, err := handler.identities.NextPreAdviceID()
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
	}
	acceptedAt := handler.clock.Now()
	notice, err := domain.NewPreAdviceNotice(
		1, command.ExpectedAt, command.Clue, command.Reference, command.Submission, command.Registrant, acceptedAt,
	)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
	}
	advice, err := domain.NewAdvisedPreAdvice(
		command.Tenant, id, command.Owner, command.Warehouse, command.Arrival, command.Inbound,
		command.GoodsLines, command.Cartons, number, absence, command.Submission, command.Registrant, acceptedAt, notice,
	)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
	}
	written, err := handler.preAdvices.Insert(ctx, advice)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
	}
	if written == ports.PreAdviceWriteKeyTaken {
		existing, found, err = handler.preAdvices.FindBySubmission(ctx, command.Tenant, command.Submission)
		if err != nil {
			return AcceptPreAdviceResult{}, fmt.Errorf("accept pre-advice: %w", err)
		}
		if !found {
			return AcceptPreAdviceResult{}, errors.New("accept pre-advice: 判重位置已被占，却读不回那一份预报")
		}
		return submissionReplay(existing, command), nil
	}
	return advised(advice, notice), nil
}

// UpdateNotice 更新告知。本租户一个仓都没登记时答 NOT_CONFIGURED，这一步在查找之前，不查这份预报在不在。
func (handler *AcceptPreAdviceHandler) UpdateNotice(
	ctx context.Context,
	command UpdatePreAdviceNoticeCommand,
) (AcceptPreAdviceResult, error) {
	if err := command.assemblyError(); err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("update pre-advice notice: %w", err)
	}
	if result, stopped, err := handler.configured(ctx, command.Tenant, "update pre-advice notice"); stopped || err != nil {
		return result, err
	}
	if err := handler.preAdvices.HoldPreAdvice(ctx, command.Tenant, command.ID); err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("update pre-advice notice: %w", err)
	}
	recordedID, recorded, found, err := handler.preAdvices.FindNoticeBySubmission(ctx, command.Tenant, command.Submission)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("update pre-advice notice: %w", err)
	}
	if found {
		return handler.noticeReplay(ctx, command, recordedID, recorded)
	}
	advice, found, err := handler.loadScoped(ctx, command.Tenant, command.Owner, command.ID, command.Submission.Path())
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("update pre-advice notice: %w", err)
	}
	if !found {
		return AcceptPreAdviceResult{outcome: PreAdviceOutcomeNotFound}, nil
	}
	if advice.Status() == domain.PreAdviceCancelled || advice.Status() == domain.PreAdviceClosed {
		return statusEcho(PreAdviceOutcomeStatusMismatch, advice.Status()), nil
	}
	notice, err := domain.NewPreAdviceNotice(
		len(advice.Notices())+1, command.ExpectedAt, command.Clue, command.Reference,
		command.Submission, command.Registrant, handler.clock.Now(),
	)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("update pre-advice notice: %w", err)
	}
	updated, err := advice.AppendNotice(notice)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("update pre-advice notice: %w", err)
	}
	written, err := handler.preAdvices.AppendNotice(ctx, updated.Tenant(), updated.ID(), notice)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("update pre-advice notice: %w", err)
	}
	if written == ports.PreAdviceWriteKeyTaken {
		recordedID, recorded, found, err = handler.preAdvices.FindNoticeBySubmission(ctx, command.Tenant, command.Submission)
		if err != nil {
			return AcceptPreAdviceResult{}, fmt.Errorf("update pre-advice notice: %w", err)
		}
		if !found {
			return AcceptPreAdviceResult{}, errors.New("update pre-advice notice: 判重位置已被占，却读不回那一版告知")
		}
		return handler.noticeReplay(ctx, command, recordedID, recorded)
	}
	return noticeUpdated(updated, notice), nil
}

// Withdraw 撤回一份已预报的预报。一个仓都没登记时答 NOT_CONFIGURED，不查预报。
// 已取消的再撤答 EXISTING_RESULT，不论这次的途径与键。已落册的撤回键再去撤另一份，答 SOURCE_CONFLICT。
func (handler *AcceptPreAdviceHandler) Withdraw(
	ctx context.Context,
	command WithdrawPreAdviceCommand,
) (AcceptPreAdviceResult, error) {
	if err := command.assemblyError(); err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("withdraw pre-advice: %w", err)
	}
	if result, stopped, err := handler.configured(ctx, command.Tenant, "withdraw pre-advice"); stopped || err != nil {
		return result, err
	}
	if err := handler.preAdvices.HoldPreAdvice(ctx, command.Tenant, command.ID); err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("withdraw pre-advice: %w", err)
	}
	advice, found, err := handler.loadScoped(ctx, command.Tenant, command.Owner, command.ID, command.Submission.Path())
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("withdraw pre-advice: %w", err)
	}
	if !found {
		return AcceptPreAdviceResult{outcome: PreAdviceOutcomeNotFound}, nil
	}
	if advice.Status() == domain.PreAdviceCancelled {
		return existingAdvice(advice), nil
	}
	other, taken, err := handler.preAdvices.FindWithdrawalBySubmission(ctx, command.Tenant, command.Submission)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("withdraw pre-advice: %w", err)
	}
	if taken && other != advice.ID() {
		return AcceptPreAdviceResult{outcome: PreAdviceOutcomeSourceConflict}, nil
	}
	if advice.Status() == domain.PreAdviceReceiving || advice.Status() == domain.PreAdviceClosed {
		return statusEcho(PreAdviceOutcomeReceiptStarted, advice.Status()), nil
	}
	if advice.Status() != domain.PreAdviceAdvised {
		return AcceptPreAdviceResult{}, fmt.Errorf("withdraw pre-advice: unexpected status %s", advice.Status())
	}
	withdrawal, err := domain.NewPreAdviceWithdrawal(command.Submission, command.Registrant, handler.clock.Now())
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("withdraw pre-advice: %w", err)
	}
	cancelled, err := advice.Cancel(withdrawal)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("withdraw pre-advice: %w", err)
	}
	written, err := handler.preAdvices.Cancel(ctx, cancelled)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("withdraw pre-advice: %w", err)
	}
	if written == ports.PreAdviceWriteKeyTaken {
		return handler.withdrawAfterKeyTaken(ctx, command)
	}
	return cancelledAdvice(cancelled), nil
}

// NoteReceiptStarted 把已预报推进收货中。已在收货中答 ALREADY_RECEIVING；已取消或已完结答 NOT_RECEIVABLE。
func (handler *AcceptPreAdviceHandler) NoteReceiptStarted(
	ctx context.Context,
	command NotePreAdviceCommand,
) (NotePreAdviceReceiptResult, error) {
	if err := command.assemblyError(); err != nil {
		return NotePreAdviceReceiptResult{}, fmt.Errorf("note pre-advice receipt started: %w", err)
	}
	advice, err := handler.holdAndLoad(ctx, command, "note pre-advice receipt started")
	if err != nil {
		return NotePreAdviceReceiptResult{}, err
	}
	switch advice.Status() {
	case domain.PreAdviceReceiving:
		return NotePreAdviceReceiptResult{outcome: PreAdviceReceiptAlreadyReceiving, advice: advice, hasAdvice: true}, nil
	case domain.PreAdviceCancelled, domain.PreAdviceClosed:
		return NotePreAdviceReceiptResult{outcome: PreAdviceReceiptNotReceivable, advice: advice, hasAdvice: true}, nil
	case domain.PreAdviceAdvised:
		started, err := advice.StartReceiving()
		if err != nil {
			return NotePreAdviceReceiptResult{}, fmt.Errorf("note pre-advice receipt started: %w", err)
		}
		if err := handler.preAdvices.MarkReceiving(ctx, started.Tenant(), started.ID()); err != nil {
			return NotePreAdviceReceiptResult{}, fmt.Errorf("note pre-advice receipt started: %w", err)
		}
		return NotePreAdviceReceiptResult{outcome: PreAdviceReceiptStarted, advice: started, hasAdvice: true}, nil
	default:
		return NotePreAdviceReceiptResult{}, fmt.Errorf("note pre-advice receipt started: unexpected status %s", advice.Status())
	}
}

// NoteClosed 把已预报或收货中推进已完结。已完结答 ALREADY_CLOSED；已取消答 NOT_CLOSABLE。
func (handler *AcceptPreAdviceHandler) NoteClosed(
	ctx context.Context,
	command NotePreAdviceCommand,
) (NotePreAdviceCloseResult, error) {
	if err := command.assemblyError(); err != nil {
		return NotePreAdviceCloseResult{}, fmt.Errorf("note pre-advice closed: %w", err)
	}
	advice, err := handler.holdAndLoad(ctx, command, "note pre-advice closed")
	if err != nil {
		return NotePreAdviceCloseResult{}, err
	}
	switch advice.Status() {
	case domain.PreAdviceClosed:
		return NotePreAdviceCloseResult{outcome: PreAdviceCloseAlreadyClosed, advice: advice, hasAdvice: true}, nil
	case domain.PreAdviceCancelled:
		return NotePreAdviceCloseResult{outcome: PreAdviceCloseNotClosable, advice: advice, hasAdvice: true}, nil
	case domain.PreAdviceAdvised, domain.PreAdviceReceiving:
		closed, err := advice.CloseOut()
		if err != nil {
			return NotePreAdviceCloseResult{}, fmt.Errorf("note pre-advice closed: %w", err)
		}
		if err := handler.preAdvices.MarkClosed(ctx, closed.Tenant(), closed.ID()); err != nil {
			return NotePreAdviceCloseResult{}, fmt.Errorf("note pre-advice closed: %w", err)
		}
		return NotePreAdviceCloseResult{outcome: PreAdviceCloseClosed, advice: closed, hasAdvice: true}, nil
	default:
		return NotePreAdviceCloseResult{}, fmt.Errorf("note pre-advice closed: unexpected status %s", advice.Status())
	}
}

func (handler *AcceptPreAdviceHandler) requireOwner(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID) error {
	if handler.owners == nil {
		return fmt.Errorf("accept pre-advice: %w", errOwnerLookupMissing)
	}
	registered, err := handler.owners.OwnerAccountRegistered(ctx, tenant, owner)
	if err != nil {
		return fmt.Errorf("accept pre-advice: %w", err)
	}
	if !registered {
		return ErrOwnerAccountNotInTenant
	}
	return nil
}

// configured 在告知更新与撤回查找预报之前回答本租户有没有登记过仓。没有就停，调用方不再碰预报册。
func (handler *AcceptPreAdviceHandler) configured(
	ctx context.Context,
	tenant domain.TenantID,
	operation string,
) (AcceptPreAdviceResult, bool, error) {
	registered, err := handler.anyWarehouse.HasAnyWarehouse(ctx, tenant)
	if err != nil {
		return AcceptPreAdviceResult{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	if !registered {
		return AcceptPreAdviceResult{outcome: PreAdviceOutcomeNotConfigured}, true, nil
	}
	return AcceptPreAdviceResult{}, false, nil
}

func (handler *AcceptPreAdviceHandler) holdAndLoad(
	ctx context.Context,
	command NotePreAdviceCommand,
	operation string,
) (domain.PreAdvice, error) {
	if err := handler.preAdvices.HoldPreAdvice(ctx, command.Tenant, command.ID); err != nil {
		return domain.PreAdvice{}, fmt.Errorf("%s: %w", operation, err)
	}
	advice, found, err := handler.preAdvices.Find(ctx, command.Tenant, command.ID)
	if err != nil {
		return domain.PreAdvice{}, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return domain.PreAdvice{}, fmt.Errorf("%s: pre-advice not found", operation)
	}
	return advice, nil
}

func (handler *AcceptPreAdviceHandler) loadScoped(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	id domain.PreAdviceID,
	path domain.SubmissionPath,
) (domain.PreAdvice, bool, error) {
	if path == domain.SubmissionOnBehalf {
		return handler.preAdvices.Find(ctx, tenant, id)
	}
	return handler.preAdvices.FindForOwner(ctx, tenant, owner, id)
}

func (handler *AcceptPreAdviceHandler) noticeReplay(
	ctx context.Context,
	command UpdatePreAdviceNoticeCommand,
	recordedID domain.PreAdviceID,
	recorded domain.PreAdviceNotice,
) (AcceptPreAdviceResult, error) {
	if recordedID != command.ID || !recorded.SameContentAs(command.ExpectedAt, command.Clue, command.Reference, command.Submission) {
		return AcceptPreAdviceResult{outcome: PreAdviceOutcomeSourceConflict}, nil
	}
	advice, found, err := handler.preAdvices.Find(ctx, command.Tenant, recordedID)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("update pre-advice notice: %w", err)
	}
	if !found {
		return AcceptPreAdviceResult{}, errors.New("update pre-advice notice: 告知在册，预报却读不回")
	}
	return AcceptPreAdviceResult{
		outcome: PreAdviceOutcomeExistingResult, advice: advice, hasAdvice: true, notice: recorded, hasNotice: true,
	}, nil
}

func (handler *AcceptPreAdviceHandler) withdrawAfterKeyTaken(
	ctx context.Context,
	command WithdrawPreAdviceCommand,
) (AcceptPreAdviceResult, error) {
	advice, found, err := handler.loadScoped(ctx, command.Tenant, command.Owner, command.ID, command.Submission.Path())
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("withdraw pre-advice: %w", err)
	}
	if found && advice.Status() == domain.PreAdviceCancelled {
		return existingAdvice(advice), nil
	}
	other, taken, err := handler.preAdvices.FindWithdrawalBySubmission(ctx, command.Tenant, command.Submission)
	if err != nil {
		return AcceptPreAdviceResult{}, fmt.Errorf("withdraw pre-advice: %w", err)
	}
	if taken && (!found || other != advice.ID()) {
		return AcceptPreAdviceResult{outcome: PreAdviceOutcomeSourceConflict}, nil
	}
	return AcceptPreAdviceResult{}, errors.New("withdraw pre-advice: 撤回键已被占，却对不上在册的那一份")
}

func (handler *AcceptPreAdviceHandler) contentIssues(
	ctx context.Context,
	command AcceptPreAdviceCommand,
) ([]domain.PreAdviceContentIssue, error) {
	switch command.Inbound {
	case domain.InboundByGoods:
		facts, err := handler.goodsFacts(ctx, command.Tenant, command.Owner, goodsCodesOf(command.GoodsLines))
		if err != nil {
			return nil, err
		}
		return domain.CollectGoodsInboundIssues(command.GoodsLines, facts), nil
	case domain.InboundByCarton:
		registered, err := handler.registeredGoods(ctx, command.Tenant, command.Owner, cartonGoodsCodes(command.Cartons))
		if err != nil {
			return nil, err
		}
		return domain.CollectCartonIssues(command.Cartons, func(code domain.GoodsCode) bool {
			_, ok := registered[code.String()]
			return ok
		}), nil
	default:
		return nil, errors.New("accept pre-advice: 缺入库方式")
	}
}

func (handler *AcceptPreAdviceHandler) goodsFacts(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	codes []domain.GoodsCode,
) (map[string]domain.PreAdviceGoodsFacts, error) {
	facts := make(map[string]domain.PreAdviceGoodsFacts, len(codes))
	for _, code := range codes {
		revision, found, err := handler.goods.LoadLatestGoods(ctx, tenant, owner, code)
		if err != nil {
			return nil, fmt.Errorf("accept pre-advice: %w", err)
		}
		if !found {
			continue
		}
		levels := revision.Content().PackagingLevels()
		units := make([]domain.GoodsUnitName, len(levels))
		for index, level := range levels {
			units[index] = level.Name()
		}
		facts[code.String()] = domain.RegisteredPreAdviceGoods(units, revision.Content().TrackBatch(), revision.Content().TrackExpiry())
	}
	return facts, nil
}

func (handler *AcceptPreAdviceHandler) registeredGoods(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	codes []domain.GoodsCode,
) (map[string]struct{}, error) {
	registered := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		_, found, err := handler.goods.LoadLatestGoods(ctx, tenant, owner, code)
		if err != nil {
			return nil, fmt.Errorf("accept pre-advice: %w", err)
		}
		if found {
			registered[code.String()] = struct{}{}
		}
	}
	return registered, nil
}

func goodsCodesOf(lines []domain.GoodsInboundLine) []domain.GoodsCode {
	seen := make(map[string]struct{}, len(lines))
	codes := make([]domain.GoodsCode, 0, len(lines))
	for _, line := range lines {
		if _, ok := seen[line.Goods().String()]; ok {
			continue
		}
		seen[line.Goods().String()] = struct{}{}
		codes = append(codes, line.Goods())
	}
	return codes
}

func cartonGoodsCodes(cartons []domain.Carton) []domain.GoodsCode {
	seen := map[string]struct{}{}
	var codes []domain.GoodsCode
	for _, carton := range cartons {
		for _, line := range carton.Lines() {
			if _, ok := seen[line.Goods().String()]; ok {
				continue
			}
			seen[line.Goods().String()] = struct{}{}
			codes = append(codes, line.Goods())
		}
	}
	return codes
}

func numberFrom(issued ports.PreAdviceNumberIssue) (domain.DocumentNumber, domain.NumberingAbsence, bool, error) {
	switch issued.Outcome {
	case ports.PreAdviceNumberAssigned:
		if !issued.Number.Assigned() {
			return domain.DocumentNumber{}, 0, false, errors.New("取号答取到了号，号却是空的")
		}
		return issued.Number, domain.NumberingAbsenceNone, false, nil
	case ports.PreAdviceNumberTimeZoneNotConfigured:
		return domain.DocumentNumber{}, domain.NumberingTimeZoneNotConfigured, false, nil
	case ports.PreAdviceNumberRuleCollision:
		return domain.DocumentNumber{}, 0, true, nil
	default:
		return domain.DocumentNumber{}, 0, false, fmt.Errorf("unexpected numbering outcome %s", issued.Outcome)
	}
}

func submissionReplay(existing domain.PreAdvice, command AcceptPreAdviceCommand) AcceptPreAdviceResult {
	if existing.SameSubmissionAs(
		command.Owner, command.Warehouse, command.Arrival, command.Inbound,
		command.GoodsLines, command.Cartons, command.ExpectedAt, command.Clue, command.Reference, command.Submission,
	) {
		return existingAdvice(existing)
	}
	return AcceptPreAdviceResult{outcome: PreAdviceOutcomeSourceConflict}
}

func advised(advice domain.PreAdvice, notice domain.PreAdviceNotice) AcceptPreAdviceResult {
	return AcceptPreAdviceResult{
		outcome: PreAdviceOutcomeAdvised, advice: advice, hasAdvice: true, notice: notice, hasNotice: true,
	}
}

func existingAdvice(advice domain.PreAdvice) AcceptPreAdviceResult {
	return AcceptPreAdviceResult{outcome: PreAdviceOutcomeExistingResult, advice: advice, hasAdvice: true}
}

func noticeUpdated(advice domain.PreAdvice, notice domain.PreAdviceNotice) AcceptPreAdviceResult {
	return AcceptPreAdviceResult{
		outcome: PreAdviceOutcomeNoticeUpdated, advice: advice, hasAdvice: true, notice: notice, hasNotice: true,
	}
}

func cancelledAdvice(advice domain.PreAdvice) AcceptPreAdviceResult {
	return AcceptPreAdviceResult{outcome: PreAdviceOutcomeCancelled, advice: advice, hasAdvice: true}
}

func notAccepting(reason WarehouseNotAcceptingReason) AcceptPreAdviceResult {
	return AcceptPreAdviceResult{outcome: PreAdviceOutcomeWarehouseNotAccepting, reason: reason}
}

func statusEcho(outcome AcceptPreAdviceOutcome, status domain.PreAdviceStatus) AcceptPreAdviceResult {
	return AcceptPreAdviceResult{outcome: outcome, status: status, hasStatus: true}
}
