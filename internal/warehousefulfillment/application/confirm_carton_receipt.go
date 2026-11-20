package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// receiptShapeMixed 是整条事实里两种行的形态掺在一起。商品入库不收整箱行，整箱入库不收商品行。
func receiptShapeMixed(command ConfirmReceiptCommand) bool {
	switch command.Shape {
	case domain.InboundByGoods:
		return len(command.CartonLines) != 0
	case domain.InboundByCarton:
		return len(command.Lines) != 0
	default:
		return true
	}
}

// confirmCarton 把一条整箱入库的收货事实落册。成立的箱写入整箱账，一箱一个入库批次。
// 箱单留在预报上，不拆成商品库存。锁序是先预报、再整箱账，不拿商品库存的货主锁。
func (handler *ConfirmReceiptHandler) confirmCarton(
	ctx context.Context,
	warehouse domain.WarehouseRevision,
	advice domain.PreAdvice,
	command ConfirmReceiptCommand,
) (ConfirmReceiptResult, error) {
	if handler.cartons == nil || handler.cartonIDs == nil {
		return ConfirmReceiptResult{}, errors.New("confirm receipt: 整箱账没有接上")
	}
	// 预报锁已在 Confirm 里拿过；这里只核还能收货的箱行。已不能收货的走 divertUnscreened，不在这里核库位。
	screened, issues, err := handler.screenCarton(ctx, command)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	if len(issues) > 0 {
		return ContentInvalidReceipt(issues), nil
	}
	if warehouse.Content().Kind() != domain.WarehouseKindSelfOperated {
		return ConfirmReceiptResult{}, errors.New("confirm receipt: 仓不是自营仓，收货入账无由落")
	}
	noted, err := handler.notices.NoteReceiptStarted(ctx, NotePreAdviceCommand{Tenant: command.Tenant, ID: advice.ID()})
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	receivedAt := handler.clock.Now()
	switch noted.Outcome() {
	case PreAdviceReceiptNotReceivable:
		return handler.divertUnscreened(ctx, warehouse, advice, command)
	case PreAdviceReceiptStarted, PreAdviceReceiptAlreadyReceiving:
	default:
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: unexpected pre-advice receipt %s", noted.Outcome())
	}
	// 预报锁已经在 NoteReceiptStarted 里。整箱锁排在它后面，并且不碰商品库存的货主锁。
	if err := handler.cartons.Serialize(ctx, command.Tenant, advice.Owner(), command.Warehouse); err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	prior, err := handler.receipts.ListByPreAdvices(ctx, advice.Tenant(), []domain.PreAdviceID{advice.ID()})
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	onHand, err := handler.cartons.OnHandCartons(ctx, command.Tenant, advice.Owner(), command.Warehouse)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	lines, postable, differences, err := judgeCartonLines(advice, prior, onHand, screened)
	if err != nil {
		return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
	}
	if len(postable) > 0 {
		if err := handler.postCartons(ctx, advice, command, receivedAt, postable, onHand); err != nil {
			return ConfirmReceiptResult{}, err
		}
	}
	var discrepancyID domain.ReceiptDiscrepancyID
	if len(differences) > 0 {
		discrepancyID, err = handler.identities.NextReceiptDiscrepancyID()
		if err != nil {
			return ConfirmReceiptResult{}, fmt.Errorf("confirm receipt: %w", err)
		}
	}
	receipt, err := handler.record(ctx, command, receivedAt, domain.RecordedReceiptConfirmed, nil, lines, discrepancyID, differences)
	if err != nil {
		return ConfirmReceiptResult{}, err
	}
	return confirmReceiptEcho(ConfirmReceiptConfirmed, receipt), nil
}

func (handler *ConfirmReceiptHandler) screenCarton(
	ctx context.Context,
	command ConfirmReceiptCommand,
) ([]domain.CartonReceiptLine, []domain.ReceiptContentIssue, error) {
	if len(command.CartonLines) == 0 {
		return nil, []domain.ReceiptContentIssue{domain.WholeReceiptIssue(domain.ReceiptIssueNoLines)}, nil
	}
	seenLocation := map[string]bool{}
	var issues []domain.ReceiptContentIssue
	for index, line := range command.CartonLines {
		location := line.Location().String()
		rejected, seen := seenLocation[location]
		if !seen {
			var err error
			rejected, err = handler.cartonLocationRejected(ctx, command.Tenant, command.Warehouse, line.Location())
			if err != nil {
				return nil, nil, err
			}
			seenLocation[location] = rejected
		}
		if rejected {
			issues = append(issues, domain.ReceiptLineIssue(index, domain.ReceiptIssueLocationNotReceiving))
		}
	}
	if len(issues) > 0 {
		return nil, issues, nil
	}
	return command.CartonLines, nil, nil
}

// cartonLocationRejected 核收货库位。整箱只许停在收货暂存、存储与待出库，这一段的收货再收窄到收货暂存。
func (handler *ConfirmReceiptHandler) cartonLocationRejected(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
) (bool, error) {
	location, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, code)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, location.Content().ZoneCode())
	if err != nil {
		return false, err
	}
	if !found {
		return false, fmt.Errorf("库位 %s 已登记，它的库区 %s 读不到", code, location.Content().ZoneCode())
	}
	usage := zone.Content().Usage()
	if usage != domain.ZoneUsageReceivingStaging || !domain.CartonStockZoneAllowed(usage) {
		return true, nil
	}
	return false, nil
}

func markCartonsNotReceivable(lines []domain.CartonReceiptLine) ([]domain.CartonReceiptLine, error) {
	marked := make([]domain.CartonReceiptLine, len(lines))
	for index, line := range lines {
		next, err := line.AsNotReceivable()
		if err != nil {
			return nil, err
		}
		marked[index] = next
	}
	return marked, nil
}

// judgeCartonLines 逐行决定入不入账。
// 预报没列，或这一箱在这份预报上已经收过：不入账，记错货。
// 要入账的箱号已经在这个货主这个仓的在库箱号里：不入账，不形成差异。
// 货主箱号在库、又带了另一个仓另编的箱号：改用仓另编的箱号，这一步由 BookedNumber 决定。
func judgeCartonLines(
	advice domain.PreAdvice,
	prior []domain.RecordedReceipt,
	onHand []domain.CartonNumber,
	lines []domain.CartonReceiptLine,
) ([]domain.CartonReceiptLine, []domain.CartonReceiptLine, []domain.ReceiptDiscrepancyLine, error) {
	listed := map[string]struct{}{}
	for _, carton := range advice.Cartons() {
		listed[carton.Number().String()] = struct{}{}
	}
	received := map[string]struct{}{}
	for _, receipt := range prior {
		if receipt.Outcome() != domain.RecordedReceiptConfirmed {
			continue
		}
		for _, line := range receipt.CartonLines() {
			if line.Result() == domain.ReceiptLinePosted {
				received[line.Number().String()] = struct{}{}
			}
		}
	}
	held := map[string]struct{}{}
	for _, number := range onHand {
		held[number.String()] = struct{}{}
	}
	marked := make([]domain.CartonReceiptLine, len(lines))
	var postable []domain.CartonReceiptLine
	var differences []domain.ReceiptDiscrepancyLine
	for index, line := range lines {
		owner := line.Number().String()
		_, onAdvice := listed[owner]
		_, already := received[owner]
		if !onAdvice || already {
			next, err := line.AsWrongCarton()
			if err != nil {
				return nil, nil, nil, err
			}
			marked[index] = next
			item, err := domain.NewCartonDiscrepancyLine(domain.ReceiptDiscrepancyWrongGoods, line.Number())
			if err != nil {
				return nil, nil, nil, err
			}
			differences = append(differences, item)
			continue
		}
		_, ownerOnHand := held[owner]
		booked := line.BookedNumber(ownerOnHand)
		if _, inUse := held[booked.String()]; inUse {
			next, err := line.AsCartonNumberInUse()
			if err != nil {
				return nil, nil, nil, err
			}
			marked[index] = next
			continue
		}
		next, err := line.AsPosted()
		if err != nil {
			return nil, nil, nil, err
		}
		marked[index] = next
		postable = append(postable, next)
		received[owner] = struct{}{}
		held[booked.String()] = struct{}{}
		if !line.Damaged() {
			continue
		}
		item, err := domain.NewCartonDiscrepancyLine(domain.ReceiptDiscrepancyDamage, booked)
		if err != nil {
			return nil, nil, nil, err
		}
		differences = append(differences, item)
	}
	return marked, postable, differences, nil
}

// postCartons 把这一次入账的箱写成一笔收货流水。每一箱各一个入库批次，按入账的箱号区分。
// 入账箱号的取法与 judgeCartonLines 相同：从调用前的在库集合出发，按行的先后把这一笔新占的箱号算进去。
func (handler *ConfirmReceiptHandler) postCartons(
	ctx context.Context,
	advice domain.PreAdvice,
	command ConfirmReceiptCommand,
	postedAt time.Time,
	lines []domain.CartonReceiptLine,
	onHand []domain.CartonNumber,
) error {
	source, err := domain.NewSourceFactReference(command.FactID.String())
	if err != nil {
		return fmt.Errorf("confirm receipt: %w", err)
	}
	movementID, err := handler.cartonIDs.NextCartonMovementID()
	if err != nil {
		return fmt.Errorf("confirm receipt: %w", err)
	}
	held := map[string]struct{}{}
	for _, number := range onHand {
		held[number.String()] = struct{}{}
	}
	movementLines := make([]domain.CartonMovementLine, len(lines))
	formed := make([]domain.CartonInboundBatch, len(lines))
	for index, line := range lines {
		_, ownerOnHand := held[line.Number().String()]
		booked := line.BookedNumber(ownerOnHand)
		batchID, err := handler.cartonIDs.NextCartonInboundBatchID()
		if err != nil {
			return fmt.Errorf("confirm receipt: %w", err)
		}
		batch, err := domain.NewCartonInboundBatch(
			command.Tenant, batchID, advice.Owner(), command.Warehouse, booked, postedAt, movementID,
		)
		if err != nil {
			return fmt.Errorf("confirm receipt: %w", err)
		}
		movementLine, err := domain.NewCartonMovementLine(line.Location(), booked, line.OwnerCartonOnBook(ownerOnHand), batchID)
		if err != nil {
			return fmt.Errorf("confirm receipt: %w", err)
		}
		movementLines[index] = movementLine
		formed[index] = batch
		held[booked.String()] = struct{}{}
	}
	movement, err := domain.NewCartonMovement(
		command.Tenant, movementID, advice.Owner(), command.Warehouse, domain.CartonMovementReceipt,
		source, command.OccurredAt, postedAt, movementLines,
	)
	if err != nil {
		return fmt.Errorf("confirm receipt: %w", err)
	}
	if err := handler.cartons.Append(ctx, movement, formed); err != nil {
		return fmt.Errorf("confirm receipt: %w", err)
	}
	return nil
}
