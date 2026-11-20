package application

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ClosePreAdviceOutcome 是完结一份入库预报的结果。
type ClosePreAdviceOutcome uint8

const (
	ClosePreAdviceOutcomeInvalid ClosePreAdviceOutcome = iota
	ClosePreAdviceClosed
	ClosePreAdviceExistingResult
	ClosePreAdviceStatusMismatch
	ClosePreAdviceNotFound
	ClosePreAdviceNotConfigured
)

func (outcome ClosePreAdviceOutcome) String() string {
	switch outcome {
	case ClosePreAdviceClosed:
		return "CLOSED"
	case ClosePreAdviceExistingResult:
		return "EXISTING_RESULT"
	case ClosePreAdviceStatusMismatch:
		return "STATUS_MISMATCH"
	case ClosePreAdviceNotFound:
		return "PRE_ADVICE_NOT_FOUND"
	case ClosePreAdviceNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

// ClosePreAdviceResult 回显完结，或回显此刻不许完结的状态。查无与未配置不带预报。
type ClosePreAdviceResult struct {
	outcome    ClosePreAdviceOutcome
	advice     domain.PreAdvice
	hasAdvice  bool
	closure    domain.PreAdviceClosure
	hasClosure bool
	status     domain.PreAdviceStatus
	hasStatus  bool
}

func (result ClosePreAdviceResult) Outcome() ClosePreAdviceOutcome { return result.outcome }

func (result ClosePreAdviceResult) PreAdvice() (domain.PreAdvice, bool) {
	return result.advice, result.hasAdvice
}

func (result ClosePreAdviceResult) Closure() (domain.PreAdviceClosure, bool) {
	return result.closure, result.hasClosure
}

func (result ClosePreAdviceResult) Status() (domain.PreAdviceStatus, bool) {
	return result.status, result.hasStatus
}

func closePreAdviceBare(outcome ClosePreAdviceOutcome) ClosePreAdviceResult {
	return ClosePreAdviceResult{outcome: outcome}
}

func closePreAdviceRecorded(outcome ClosePreAdviceOutcome, advice domain.PreAdvice, closure domain.PreAdviceClosure) ClosePreAdviceResult {
	return ClosePreAdviceResult{
		outcome: outcome, advice: advice, hasAdvice: true, closure: closure, hasClosure: true,
	}
}

func closePreAdviceMismatch(advice domain.PreAdvice) ClosePreAdviceResult {
	return ClosePreAdviceResult{
		outcome: ClosePreAdviceStatusMismatch, advice: advice, hasAdvice: true,
		status: advice.Status(), hasStatus: true,
	}
}

// ClosePreAdviceCommand 是一次完结。租户与操作者只从信封来。说明可缺。重放身份是被指名的预报，不另带决定标识。
type ClosePreAdviceCommand struct {
	Tenant   domain.TenantID
	ID       domain.PreAdviceID
	Operator domain.Registrant
	Note     domain.ClosureNote
}

func (command ClosePreAdviceCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.ID.String() == "":
		return errors.New("缺预报")
	case command.Operator.String() == "":
		return errors.New("缺操作者")
	default:
		return nil
	}
}

// ClosePreAdviceHandler 把一份收货中的入库预报收成已完结，并把没到的数量记成少收。
// 事务边界在调用方。少收与推进状态必须在同一笔里：少收落不下，调用方回滚，预报不得留在已完结。
//
// notices 与 preAdvices 必须是同一只预报册。NoteClosed 拿的预报锁，和收货、撤回是同一把。
type ClosePreAdviceHandler struct {
	closures   ports.PreAdviceClosureStore
	preAdvices ports.PreAdviceStore
	notices    *AcceptPreAdviceHandler
	receipts   ports.ReceiptRead
	goods      ports.StockGoodsLookup
	warehouses ports.TenantWarehouseLookup
	identities ports.ReceiptDiscrepancyIdentities
	clock      ports.Clock
}

func NewClosePreAdviceHandler(
	closures ports.PreAdviceClosureStore,
	preAdvices ports.PreAdviceStore,
	notices *AcceptPreAdviceHandler,
	receipts ports.ReceiptRead,
	goods ports.StockGoodsLookup,
	warehouses ports.TenantWarehouseLookup,
	identities ports.ReceiptDiscrepancyIdentities,
	clock ports.Clock,
) *ClosePreAdviceHandler {
	return &ClosePreAdviceHandler{
		closures: closures, preAdvices: preAdvices, notices: notices, receipts: receipts,
		goods: goods, warehouses: warehouses, identities: identities, clock: clock,
	}
}

// ClosePreAdvice 的判定次序：
//
//  1. 缺租户、预报或操作者：装配错，交 error。
//  2. 本租户一个仓都没登记：NOT_CONFIGURED，到此为止，不查这份预报在不在。
//  3. 锁这份预报，再读。查不到：PRE_ADVICE_NOT_FOUND。
//  4. 已完结：EXISTING_RESULT，回显当初的完结与少收，不再记第二份。说明不同也不是另一份。
//  5. 已预报或已取消：STATUS_MISMATCH，回显状态。不调用 NoteClosed。那条缝允许已预报进入已完结，完结用例不走它。
//  6. 收货中、商品入库：同一笔里按商品折成基本单位，预报数量减已入账数量，只留下大于 0 的，写成少收行，再 NoteClosed。
//     已经到齐或超出的商品不记少收。
//  7. 收货中、整箱入库：预报所列、到这一刻还没入账的箱各记一行，数量是一箱，不折成件，也不看箱单。
//     入账认收货事实里结果是已入账的货主箱号。仓另编箱号入账的，算那个货主箱号已到。没入账的行不算到。
//     都到齐就不签差异标识。少收不入账，商品库存与整箱库存都不动。
func (handler *ClosePreAdviceHandler) ClosePreAdvice(
	ctx context.Context,
	command ClosePreAdviceCommand,
) (ClosePreAdviceResult, error) {
	if err := command.assemblyError(); err != nil {
		return ClosePreAdviceResult{}, fmt.Errorf("close pre-advice: %w", err)
	}
	registered, err := handler.warehouses.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return ClosePreAdviceResult{}, fmt.Errorf("close pre-advice: %w", err)
	}
	if !registered {
		return closePreAdviceBare(ClosePreAdviceNotConfigured), nil
	}
	if err := handler.preAdvices.HoldPreAdvice(ctx, command.Tenant, command.ID); err != nil {
		return ClosePreAdviceResult{}, fmt.Errorf("close pre-advice: %w", err)
	}
	advice, found, err := handler.preAdvices.Find(ctx, command.Tenant, command.ID)
	if err != nil {
		return ClosePreAdviceResult{}, fmt.Errorf("close pre-advice: %w", err)
	}
	if !found {
		return closePreAdviceBare(ClosePreAdviceNotFound), nil
	}
	switch advice.Status() {
	case domain.PreAdviceClosed:
		recorded, ok, err := handler.recorded(ctx, advice)
		if err != nil {
			return ClosePreAdviceResult{}, err
		}
		if !ok {
			return ClosePreAdviceResult{}, errors.New("close pre-advice: 预报已完结，完结行却读不回")
		}
		return closePreAdviceRecorded(ClosePreAdviceExistingResult, advice, recorded), nil
	case domain.PreAdviceAdvised, domain.PreAdviceCancelled:
		return closePreAdviceMismatch(advice), nil
	case domain.PreAdviceReceiving:
		closure, err := handler.shortageOf(ctx, advice, command)
		if err != nil {
			return ClosePreAdviceResult{}, fmt.Errorf("close pre-advice: %w", err)
		}
		inserted, err := handler.closures.Save(ctx, closure)
		if err != nil {
			return ClosePreAdviceResult{}, fmt.Errorf("close pre-advice: %w", err)
		}
		if !inserted {
			return ClosePreAdviceResult{}, errors.New("close pre-advice: 锁内完结行已被占")
		}
		noted, err := handler.notices.NoteClosed(ctx, NotePreAdviceCommand{Tenant: command.Tenant, ID: advice.ID()})
		if err != nil {
			return ClosePreAdviceResult{}, fmt.Errorf("close pre-advice: %w", err)
		}
		if noted.Outcome() != PreAdviceCloseClosed {
			return ClosePreAdviceResult{}, fmt.Errorf("close pre-advice: 收货中的预报没有进入已完结（%s）", noted.Outcome())
		}
		closed, ok := noted.PreAdvice()
		if !ok {
			return ClosePreAdviceResult{}, errors.New("close pre-advice: 完结没有回显预报")
		}
		return closePreAdviceRecorded(ClosePreAdviceClosed, closed, closure), nil
	default:
		return ClosePreAdviceResult{}, fmt.Errorf("close pre-advice: unexpected status %s", advice.Status())
	}
}

func (handler *ClosePreAdviceHandler) recorded(
	ctx context.Context,
	advice domain.PreAdvice,
) (domain.PreAdviceClosure, bool, error) {
	rows, err := handler.closures.ListByPreAdvices(ctx, advice.Tenant(), []domain.PreAdviceID{advice.ID()})
	if err != nil {
		return domain.PreAdviceClosure{}, false, fmt.Errorf("close pre-advice: %w", err)
	}
	for _, row := range rows {
		if row.PreAdvice() == advice.ID() {
			return row, true, nil
		}
	}
	return domain.PreAdviceClosure{}, false, nil
}

func (handler *ClosePreAdviceHandler) shortageOf(
	ctx context.Context,
	advice domain.PreAdvice,
	command ClosePreAdviceCommand,
) (domain.PreAdviceClosure, error) {
	switch advice.Inbound() {
	case domain.InboundByGoods:
		return handler.shortage(ctx, advice, command)
	case domain.InboundByCarton:
		return handler.cartonShortage(ctx, advice, command)
	default:
		return domain.PreAdviceClosure{}, fmt.Errorf("入库方式 %s", advice.Inbound())
	}
}

func (handler *ClosePreAdviceHandler) shortage(
	ctx context.Context,
	advice domain.PreAdvice,
	command ClosePreAdviceCommand,
) (domain.PreAdviceClosure, error) {
	cache := map[string]domain.GoodsRevision{}
	factor := func(goods domain.GoodsCode, unit domain.GoodsUnitName) (int64, error) {
		return closureFactor(ctx, handler.goods, advice, goods, unit, cache)
	}
	advised, err := baseByGoods(advice.GoodsLines(), func(goods domain.GoodsCode, unit domain.GoodsUnitName, quantity int64) (int64, error) {
		value, err := factor(goods, unit)
		if err != nil {
			return 0, err
		}
		base, ok := domain.BaseQuantity(quantity, value)
		if !ok {
			return 0, errors.New("预报数量折成基本单位放不下")
		}
		return base, nil
	})
	if err != nil {
		return domain.PreAdviceClosure{}, err
	}
	receipts, err := handler.receipts.ListByPreAdvices(ctx, advice.Tenant(), []domain.PreAdviceID{advice.ID()})
	if err != nil {
		return domain.PreAdviceClosure{}, err
	}
	posted, err := postedBase(receipts, factor)
	if err != nil {
		return domain.PreAdviceClosure{}, err
	}
	codes := make([]string, 0, len(advised))
	for code := range advised {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	var lines []domain.PreAdviceShortage
	for _, code := range codes {
		gap, ok := domain.ShortageQuantity(advised[code], posted[code])
		if !ok {
			return domain.PreAdviceClosure{}, errors.New("少收算不出来")
		}
		if gap == 0 {
			continue
		}
		goods, err := domain.NewGoodsCode(code)
		if err != nil {
			return domain.PreAdviceClosure{}, err
		}
		line, err := domain.NewPreAdviceShortage(goods, gap)
		if err != nil {
			return domain.PreAdviceClosure{}, err
		}
		lines = append(lines, line)
	}
	return handler.finishShortage(advice, command, lines)
}

func (handler *ClosePreAdviceHandler) cartonShortage(
	ctx context.Context,
	advice domain.PreAdvice,
	command ClosePreAdviceCommand,
) (domain.PreAdviceClosure, error) {
	receipts, err := handler.receipts.ListByPreAdvices(ctx, advice.Tenant(), []domain.PreAdviceID{advice.ID()})
	if err != nil {
		return domain.PreAdviceClosure{}, err
	}
	posted := postedCartons(receipts)
	var lines []domain.PreAdviceShortage
	for _, carton := range advice.Cartons() {
		if posted[carton.Number().String()] {
			continue
		}
		line, err := domain.NewCartonPreAdviceShortage(carton.Number())
		if err != nil {
			return domain.PreAdviceClosure{}, err
		}
		lines = append(lines, line)
	}
	return handler.finishShortage(advice, command, lines)
}

func postedCartons(receipts []domain.RecordedReceipt) map[string]bool {
	posted := map[string]bool{}
	for _, receipt := range receipts {
		if receipt.Outcome() != domain.RecordedReceiptConfirmed {
			continue
		}
		for _, line := range receipt.CartonLines() {
			if line.Result() != domain.ReceiptLinePosted {
				continue
			}
			posted[line.Number().String()] = true
		}
	}
	return posted
}

func (handler *ClosePreAdviceHandler) finishShortage(
	advice domain.PreAdvice,
	command ClosePreAdviceCommand,
	lines []domain.PreAdviceShortage,
) (domain.PreAdviceClosure, error) {
	var discrepancy domain.ReceiptDiscrepancyID
	if len(lines) > 0 {
		issued, err := handler.identities.NextReceiptDiscrepancyID()
		if err != nil {
			return domain.PreAdviceClosure{}, err
		}
		discrepancy = issued
	}
	return domain.NewPreAdviceClosure(
		advice.Tenant(), advice.ID(), command.Operator, command.Note, handler.clock.Now(), lines, discrepancy,
	)
}

func closureFactor(
	ctx context.Context,
	goods ports.StockGoodsLookup,
	advice domain.PreAdvice,
	code domain.GoodsCode,
	unit domain.GoodsUnitName,
	cache map[string]domain.GoodsRevision,
) (int64, error) {
	key := advice.Owner().String() + "\x1f" + code.String()
	revision, ok := cache[key]
	if !ok {
		loaded, found, err := goods.LoadLatestGoods(ctx, advice.Tenant(), advice.Owner(), code)
		if err != nil {
			return 0, err
		}
		if !found {
			return 0, fmt.Errorf("商品 %s 没有当前修订", code)
		}
		revision = loaded
		cache[key] = revision
	}
	value, ok := unitFactor(revision, unit)
	if !ok {
		return 0, fmt.Errorf("商品 %s 的计量单位 %s 没有折合数", code, unit)
	}
	return value, nil
}
