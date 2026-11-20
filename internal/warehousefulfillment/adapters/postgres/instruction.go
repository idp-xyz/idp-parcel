package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Instructions 是加工与换标指令的 PostgreSQL 适配器。
type Instructions struct {
	db *bentopg.DB
}

func NewInstructions(db *bentopg.DB) (*Instructions, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Instructions{db: db}, nil
}

var _ ports.InstructionStore = (*Instructions)(nil)

func (store *Instructions) HoldSubmission(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) error {
	return store.hold(ctx, "hold instruction submission", "warehouse_fulfillment.instruction.request", tenant.String(), key.String())
}

func (store *Instructions) HoldDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) error {
	return store.hold(ctx, "hold instruction decision", "warehouse_fulfillment.instruction.decision", tenant.String(), key.String())
}

func (store *Instructions) HoldInstruction(ctx context.Context, tenant domain.TenantID, id domain.InstructionID) error {
	return store.hold(ctx, "hold instruction", "warehouse_fulfillment.instruction", tenant.String(), id.String())
}

func (store *Instructions) HoldExecution(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) error {
	return store.hold(ctx, "hold instruction execution", "warehouse_fulfillment.instruction_execution", tenant.String(), factID.String())
}

func (store *Instructions) hold(ctx context.Context, operation, namespace, tenant, id string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2 || chr(31) || $3))`,
		namespace, tenant, id,
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *Instructions) LoadBySubmission(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.Instruction, bool, error) {
	return store.loadWhere(ctx, "load instruction by submission", `tenant_id = $1 AND request_key = $2`, tenant.String(), key.String())
}

func (store *Instructions) LoadByDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.Instruction, bool, error) {
	return store.loadWhere(ctx, "load instruction by decision", `tenant_id = $1 AND decision_id = $2`, tenant.String(), key.String())
}

func (store *Instructions) LoadByWithdrawal(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.Instruction, bool, error) {
	return store.loadWhere(ctx, "load instruction by withdrawal", `tenant_id = $1 AND withdrawal_key = $2`, tenant.String(), key.String())
}

func (store *Instructions) Load(ctx context.Context, tenant domain.TenantID, id domain.InstructionID) (domain.Instruction, bool, error) {
	return store.loadWhere(ctx, "load instruction", `tenant_id = $1 AND instruction_id = $2`, tenant.String(), id.String())
}

func (store *Instructions) loadWhere(ctx context.Context, operation, where string, args ...any) (domain.Instruction, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.Instruction{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	instruction, found, err := scanInstruction(querier.QueryRow(ctx, `SELECT `+instructionColumns+` FROM warehouse_fulfillment.instruction WHERE `+where, args...))
	if err != nil {
		return domain.Instruction{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return domain.Instruction{}, false, nil
	}
	components, err := loadInstructionComponents(ctx, querier, instruction.Tenant(), instruction.ID())
	if err != nil {
		return domain.Instruction{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	rebuilt, err := instructionWithComponents(instruction, components)
	if err != nil {
		return domain.Instruction{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return rebuilt, true, nil
}

func (store *Instructions) Save(ctx context.Context, instruction domain.Instruction) error {
	const operation = "save instruction"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err := insertInstruction(ctx, executor, instruction); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	for position, component := range instruction.Components() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.instruction_component (
			    tenant_id, instruction_id, position, goods_code, quantity
			 ) VALUES ($1,$2,$3,$4,$5)`,
			instruction.Tenant().String(), instruction.ID().String(), position, component.Goods().String(), component.Quantity(),
		); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
	}
	return nil
}

func (store *Instructions) MarkVoid(ctx context.Context, instruction domain.Instruction) error {
	const operation = "void instruction"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.instruction
		    SET status = 'VOID', void_origin = $3, void_reason = $4, void_operator_id = $5, withdrawal_key = $6, voided_at = $7
		  WHERE tenant_id = $1 AND instruction_id = $2 AND status = 'PENDING'`,
		instruction.Tenant().String(), instruction.ID().String(), instruction.VoidOrigin().String(),
		nullableText(instruction.VoidReason()), nullableText(instruction.VoidOperator().String()),
		nullableText(instruction.WithdrawalKey().String()), instruction.VoidedAt(),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: instruction %s was not pending", operation, instruction.ID())
	}
	return nil
}

func (store *Instructions) LoadExecution(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) (ports.InstructionExecution, bool, error) {
	const operation = "load instruction execution"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.InstructionExecution{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	execution, found, err := scanExecution(querier.QueryRow(ctx,
		`SELECT tenant_id, fact_id, instruction_id, device_id, operator_id, occurred_at, device_sequence, received_at, done_quantity, movement_id
		   FROM warehouse_fulfillment.instruction_execution
		  WHERE tenant_id = $1 AND fact_id = $2`, tenant.String(), factID.String()))
	if err != nil {
		return ports.InstructionExecution{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return ports.InstructionExecution{}, false, nil
	}
	lines, err := loadExecutionLines(ctx, querier, tenant, factID)
	if err != nil {
		return ports.InstructionExecution{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	execution.Lines = lines
	return execution, true, nil
}

func (store *Instructions) SaveExecution(ctx context.Context, execution ports.InstructionExecution) error {
	const operation = "save instruction execution"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.instruction_execution (
		    tenant_id, fact_id, instruction_id, device_id, operator_id, occurred_at, device_sequence, received_at, done_quantity, movement_id
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		execution.Tenant.String(), execution.FactID.String(), execution.InstructionID.String(), execution.Device.String(),
		execution.Operator.String(), execution.OccurredAt, execution.DeviceSequence, execution.ReceivedAt, execution.DoneQuantity, execution.MovementID.String(),
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	for position, line := range execution.Lines {
		direction := "OUT"
		if line.Direction == domain.StockIn {
			direction = "IN"
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.instruction_execution_line (
			    tenant_id, fact_id, position, direction, location_code, goods_code, unit_name, stock_state,
			    label_alias, batch_number, expiry_date, inbound_batch_id, quantity
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			execution.Tenant.String(), execution.FactID.String(), position, direction, line.Location.String(), line.Goods.String(),
			line.Unit.String(), line.State.String(), nullableText(line.Alias.String()), nullableText(line.Batch.String()),
			nullableDate(line.Expiry), nullableText(line.NamedBatch.String()), line.Quantity,
		); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.instruction
		    SET status = 'DONE', done_quantity = $3, execution_fact_id = $4
		  WHERE tenant_id = $1 AND instruction_id = $2 AND status = 'PENDING'`,
		execution.Tenant.String(), execution.InstructionID.String(), execution.DoneQuantity, execution.FactID.String(),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: instruction %s was not pending", operation, execution.InstructionID)
	}
	return nil
}

const instructionColumns = `
	tenant_id, instruction_id, owner_account_id, warehouse_code, kind, status, origin, goods_code, unit_name, quantity,
	from_alias, to_alias, batch_number, expiry_date::text, reference_text, request_key, decision_id, operator_id, reason,
	basis_reference, named_revision, fixed_revision, done_quantity, execution_fact_id, void_origin, void_reason, void_operator_id, withdrawal_key, voided_at, received_at`

func insertInstruction(ctx context.Context, executor bentopg.Executor, instruction domain.Instruction) error {
	_, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.instruction (
		    tenant_id, instruction_id, owner_account_id, warehouse_code, kind, status, origin, goods_code, unit_name, quantity,
		    from_alias, to_alias, batch_number, expiry_date, reference_text, request_key, decision_id, operator_id, reason,
		    basis_reference, named_revision, fixed_revision, done_quantity, execution_fact_id, received_at
		 ) VALUES ($1,$2,$3,$4,$5,'PENDING',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,0,NULL,$22)`,
		instruction.Tenant().String(), instruction.ID().String(), instruction.Owner().String(), instruction.Warehouse().String(),
		instruction.Kind().String(), instruction.Origin().String(), instruction.Goods().String(), nullableText(instruction.Unit().String()),
		instruction.Quantity(), nullableText(instruction.FromAlias().String()), nullableText(instruction.ToAlias().String()),
		nullableText(instruction.Batch().String()), nullableDate(instruction.Expiry()), nullableText(instruction.Reference()),
		nullableText(instruction.RequestKey().String()), nullableText(instruction.DecisionID().String()), nullableText(instruction.Operator().String()),
		nullableText(instruction.Reason()), nullableText(instruction.Basis()), instruction.NamedRevision(), instruction.FixedRevision(),
		instruction.ReceivedAt(),
	)
	return err
}

func scanInstruction(row pgx.Row) (domain.Instruction, bool, error) {
	var (
		rawTenant, rawID, rawOwner, rawWarehouse, rawKind, rawStatus, rawOrigin, rawGoods        string
		unit, fromAlias, toAlias, batch, expiry, reference, requestKey, decisionID               *string
		operator, reason, basis, executionFact, voidOrigin, voidReason, voidOperator, withdrawal *string
		quantity                                                                                 int64
		named, fixed                                                                             int
		done                                                                                     int64
		voided                                                                                   *time.Time
		received                                                                                 time.Time
	)
	err := row.Scan(
		&rawTenant, &rawID, &rawOwner, &rawWarehouse, &rawKind, &rawStatus, &rawOrigin, &rawGoods, &unit, &quantity,
		&fromAlias, &toAlias, &batch, &expiry, &reference, &requestKey, &decisionID, &operator, &reason, &basis,
		&named, &fixed, &done, &executionFact, &voidOrigin, &voidReason, &voidOperator, &withdrawal, &voided, &received,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Instruction{}, false, nil
	}
	if err != nil {
		return domain.Instruction{}, false, err
	}
	kind, err := parseInstructionKind(rawKind)
	if err != nil {
		return domain.Instruction{}, false, err
	}
	origin, err := parseInstructionOrigin(rawOrigin)
	if err != nil {
		return domain.Instruction{}, false, err
	}
	parsedExpiry, err := mustExpiryText(expiry)
	if err != nil {
		return domain.Instruction{}, false, err
	}
	spec := domain.InstructionSpec{
		Tenant: mustTenant(rawTenant), ID: mustInstructionID(rawID), Owner: mustOwnerID(rawOwner), Warehouse: mustWarehouse(rawWarehouse),
		Kind: kind, Origin: origin, Goods: mustGoodsID(rawGoods), Unit: mustUnit(unit), Quantity: quantity,
		FromAlias: mustAlias(fromAlias), ToAlias: mustAlias(toAlias), Batch: mustBatch(batch), Expiry: parsedExpiry,
		Reference: valueOrEmpty(reference), RequestKey: mustRequest(requestKey), DecisionID: mustRequest(decisionID),
		Operator: mustRegistrant(operator), Reason: valueOrEmpty(reason), Basis: valueOrEmpty(basis),
		NamedRevision: named, FixedRevision: fixed, ReceivedAt: received,
	}
	instruction, err := domain.NewPendingInstruction(spec)
	if err != nil {
		return domain.Instruction{}, false, err
	}
	switch rawStatus {
	case domain.InstructionDone.String():
		fact, err := domain.NewInternalJobFactID(valueOrEmpty(executionFact))
		if err != nil {
			return domain.Instruction{}, false, err
		}
		instruction, err = instruction.WithDone(done, fact)
		if err != nil {
			return domain.Instruction{}, false, err
		}
	case domain.InstructionVoid.String():
		voidedOrigin, err := parseInstructionOrigin(valueOrEmpty(voidOrigin))
		if err != nil {
			return domain.Instruction{}, false, err
		}
		at := received
		if voided != nil {
			at = *voided
		}
		instruction, err = instruction.WithVoid(voidedOrigin, valueOrEmpty(voidReason), at, mustRegistrant(voidOperator), mustRequest(withdrawal))
		if err != nil {
			return domain.Instruction{}, false, err
		}
	}
	return instruction, true, nil
}

func instructionWithComponents(instruction domain.Instruction, components []domain.InstructionComponent) (domain.Instruction, error) {
	if instruction.Kind() == domain.InstructionRelabel {
		return instruction, nil
	}
	spec := domain.InstructionSpec{
		Tenant: instruction.Tenant(), ID: instruction.ID(), Owner: instruction.Owner(), Warehouse: instruction.Warehouse(),
		Kind: instruction.Kind(), Origin: instruction.Origin(), Goods: instruction.Goods(), Unit: instruction.Unit(),
		Quantity: instruction.Quantity(), Reference: instruction.Reference(), RequestKey: instruction.RequestKey(),
		DecisionID: instruction.DecisionID(), Operator: instruction.Operator(), Reason: instruction.Reason(), Basis: instruction.Basis(),
		NamedRevision: instruction.NamedRevision(), FixedRevision: instruction.FixedRevision(), Components: components,
		ReceivedAt: instruction.ReceivedAt(),
	}
	rebuilt, err := domain.NewPendingInstruction(spec)
	if err != nil {
		return domain.Instruction{}, err
	}
	switch instruction.Status() {
	case domain.InstructionDone:
		return rebuilt.WithDone(instruction.DoneQuantity(), instruction.ExecutionFact())
	case domain.InstructionVoid:
		return rebuilt.WithVoid(instruction.VoidOrigin(), instruction.VoidReason(), instruction.VoidedAt(), instruction.VoidOperator(), instruction.WithdrawalKey())
	default:
		return rebuilt, nil
	}
}

func loadInstructionComponents(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, id domain.InstructionID) ([]domain.InstructionComponent, error) {
	rows, err := querier.Query(ctx,
		`SELECT goods_code, quantity FROM warehouse_fulfillment.instruction_component
		  WHERE tenant_id = $1 AND instruction_id = $2 ORDER BY position`,
		tenant.String(), id.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var components []domain.InstructionComponent
	for rows.Next() {
		var goods string
		var quantity int64
		if err := rows.Scan(&goods, &quantity); err != nil {
			return nil, err
		}
		code, err := domain.NewGoodsCode(goods)
		if err != nil {
			return nil, err
		}
		component, err := domain.NewInstructionComponent(code, quantity)
		if err != nil {
			return nil, err
		}
		components = append(components, component)
	}
	return components, rows.Err()
}

func scanExecution(row pgx.Row) (ports.InstructionExecution, bool, error) {
	var (
		rawTenant, rawFact, rawInstruction, rawDevice, rawOperator, rawMovement string
		occurred, received                                                      time.Time
		sequence, done                                                          int64
	)
	err := row.Scan(&rawTenant, &rawFact, &rawInstruction, &rawDevice, &rawOperator, &occurred, &sequence, &received, &done, &rawMovement)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.InstructionExecution{}, false, nil
	}
	if err != nil {
		return ports.InstructionExecution{}, false, err
	}
	tenant, err := domain.NewTenantID(rawTenant)
	if err != nil {
		return ports.InstructionExecution{}, false, err
	}
	fact, err := domain.NewInternalJobFactID(rawFact)
	if err != nil {
		return ports.InstructionExecution{}, false, err
	}
	instruction, err := domain.NewInstructionID(rawInstruction)
	if err != nil {
		return ports.InstructionExecution{}, false, err
	}
	device, err := domain.NewReceiptDeviceID(rawDevice)
	if err != nil {
		return ports.InstructionExecution{}, false, err
	}
	operator, err := domain.NewReceiptOperatorID(rawOperator)
	if err != nil {
		return ports.InstructionExecution{}, false, err
	}
	movement, err := domain.NewStockMovementID(rawMovement)
	if err != nil {
		return ports.InstructionExecution{}, false, err
	}
	return ports.InstructionExecution{
		Tenant: tenant, FactID: fact, InstructionID: instruction, Device: device, Operator: operator,
		OccurredAt: occurred, DeviceSequence: sequence, ReceivedAt: received, DoneQuantity: done, MovementID: movement,
	}, true, nil
}

func loadExecutionLines(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, factID domain.InternalJobFactID) ([]ports.InstructionExecutionLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT direction, location_code, goods_code, unit_name, stock_state, label_alias, batch_number, expiry_date::text, inbound_batch_id, quantity
		   FROM warehouse_fulfillment.instruction_execution_line
		  WHERE tenant_id = $1 AND fact_id = $2
		  ORDER BY position`, tenant.String(), factID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []ports.InstructionExecutionLine
	for rows.Next() {
		var direction, location, goods, unit, state string
		var alias, batch, expiry, named *string
		var quantity int64
		if err := rows.Scan(&direction, &location, &goods, &unit, &state, &alias, &batch, &expiry, &named, &quantity); err != nil {
			return nil, err
		}
		parsedExpiry, err := mustExpiryText(expiry)
		if err != nil {
			return nil, err
		}
		stockDirection := domain.StockOut
		if direction == "IN" {
			stockDirection = domain.StockIn
		}
		parsedState, err := domain.NewStockState(state)
		if err != nil {
			return nil, err
		}
		lines = append(lines, ports.InstructionExecutionLine{
			Direction: stockDirection, Location: mustLocation(&location), Goods: mustGoods(&goods), Unit: mustUnit(&unit),
			State: parsedState, Alias: mustAlias(alias), Batch: mustBatch(batch), Expiry: parsedExpiry, NamedBatch: mustInbound(named), Quantity: quantity,
		})
	}
	return lines, rows.Err()
}

func parseInstructionKind(word string) (domain.InstructionKind, error) {
	for _, kind := range []domain.InstructionKind{domain.InstructionProcess, domain.InstructionDisassemble, domain.InstructionRelabel} {
		if kind.String() == word {
			return kind, nil
		}
	}
	return domain.InstructionKindInvalid, fmt.Errorf("unknown instruction kind %q", word)
}

func parseInstructionOrigin(word string) (domain.InstructionOrigin, error) {
	for _, origin := range []domain.InstructionOrigin{domain.InstructionOriginCustomer, domain.InstructionOriginOperator} {
		if origin.String() == word {
			return origin, nil
		}
	}
	return domain.InstructionOriginInvalid, fmt.Errorf("unknown instruction origin %q", word)
}

func mustTenant(value string) domain.TenantID {
	parsed, _ := domain.NewTenantID(value)
	return parsed
}

func mustInstructionID(value string) domain.InstructionID {
	parsed, _ := domain.NewInstructionID(value)
	return parsed
}

func mustOwnerID(value string) domain.OwnerAccountID {
	parsed, _ := domain.NewOwnerAccountID(value)
	return parsed
}

func mustWarehouse(value string) domain.WarehouseCode {
	parsed, _ := domain.NewWarehouseCode(value)
	return parsed
}

func mustGoodsID(value string) domain.GoodsCode {
	parsed, _ := domain.NewGoodsCode(value)
	return parsed
}

func mustRequest(value *string) domain.RequestKey {
	if value == nil {
		return domain.RequestKey{}
	}
	parsed, _ := domain.NewRequestKey(*value)
	return parsed
}

func mustRegistrant(value *string) domain.Registrant {
	if value == nil {
		return domain.Registrant{}
	}
	parsed, _ := domain.NewRegistrant(*value)
	return parsed
}
