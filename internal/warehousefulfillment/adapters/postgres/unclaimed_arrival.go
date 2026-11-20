package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// UnclaimedArrivals 是待认领到货的 PostgreSQL 适配器。
// 收货转来与作业事实直接登记共用一张表，认领、撤销与处置各落只追加子表；所有加锁与写入在没有环境事务时拒绝。
type UnclaimedArrivals struct {
	db *bentopg.DB
}

func NewUnclaimedArrivals(db *bentopg.DB) (*UnclaimedArrivals, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &UnclaimedArrivals{db: db}, nil
}

var (
	_ ports.UnclaimedArrivalStore         = (*UnclaimedArrivals)(nil)
	_ ports.UnclaimedArrivalCatalogueRead = (*UnclaimedArrivals)(nil)
)

const unclaimedArrivalColumns = `
	tenant_id, arrival_id, fact_id, origin, kind, status, warehouse_code,
	device_id, operator_id, occurred_at, device_sequence, received_at,
	observation, owner_account_id, pre_advice_id, storage_location, diverted_receipt_fact_id`

const unclaimedArrivalListed = `WITH listed AS (
	SELECT ` + unclaimedArrivalColumns + `
	  FROM warehouse_fulfillment.unclaimed_arrival
	 WHERE tenant_id = $1
	   AND ($2::timestamptz IS NULL OR received_at >= $2)
	   AND ($3::timestamptz IS NULL OR received_at <= $3)
)`

var unclaimedArrivalListBinding = ports.UnclaimedArrivalPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"receivedAt": "received_at",
		"arrivalId":  "arrival_id",
	},
	Identity: []string{"arrival_id"},
	Filters: map[string]string{
		"warehouseCode": "warehouse_code",
		"kind":          "kind",
		"status":        "status",
	},
	Keyword: []string{"arrival_id", "fact_id", "warehouse_code"},
})

func (store *UnclaimedArrivals) Save(ctx context.Context, arrival domain.UnclaimedArrival) (bool, error) {
	const operation = "save unclaimed arrival"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	receipt := arrival.Receipt()
	var id string
	err = executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_arrival (
		    tenant_id, arrival_id, fact_id, pre_advice_id, owner_account_id,
		    origin, kind, warehouse_code, device_id, operator_id,
		    occurred_at, received_at, diverted_receipt_fact_id, status
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 ON CONFLICT ON CONSTRAINT unclaimed_arrival_fact_key DO NOTHING
		 RETURNING arrival_id`,
		arrival.Tenant().String(), arrival.ID().String(), receipt.FactID().String(),
		arrival.PreAdvice().String(), arrival.Owner().String(),
		domain.UnclaimedArrivalOriginDivertedReceipt.String(), domain.UnclaimedArrivalKindInbound.String(),
		receipt.Warehouse().String(), receipt.Device().String(), receipt.Operator().String(),
		receipt.OccurredAt(), receipt.ReceivedAt(), receipt.FactID().String(),
		domain.UnclaimedArrivalStatusUnclaimed.String(),
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return true, nil
}

func (store *UnclaimedArrivals) HoldFact(
	ctx context.Context,
	tenant domain.TenantID,
	factID domain.UnclaimedArrivalFactID,
) error {
	const operation = "hold unclaimed arrival fact"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.unclaimed_arrival.fact'),
		    hashtext($1 || chr(31) || $2))`,
		tenant.String(), factID.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *UnclaimedArrivals) LoadByFact(
	ctx context.Context,
	tenant domain.TenantID,
	factID domain.UnclaimedArrivalFactID,
) (domain.RecordedUnclaimedArrival, bool, error) {
	const operation = "load unclaimed arrival by fact"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	row := querier.QueryRow(ctx,
		`SELECT `+unclaimedArrivalColumns+`
		   FROM warehouse_fulfillment.unclaimed_arrival
		  WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), factID.String())
	stored, err := scanStoredUnclaimedArrival(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RecordedUnclaimedArrival{}, false, nil
	}
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	built, err := store.hydrateArrival(ctx, querier, stored)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return built, true, nil
}

func (store *UnclaimedArrivals) SaveDirect(
	ctx context.Context,
	arrival domain.RecordedUnclaimedArrival,
) (bool, error) {
	const operation = "save direct unclaimed arrival"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	obs, _ := arrival.Observation()
	ownerClue, _ := arrival.OwnerClue()
	preAdviceClue, _ := arrival.PreAdviceClue()
	storage, _ := arrival.StorageLocation()
	var id string
	err = executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_arrival (
		    tenant_id, arrival_id, fact_id, origin, kind, status, warehouse_code,
		    device_id, operator_id, occurred_at, device_sequence, received_at,
		    observation, owner_account_id, pre_advice_id, storage_location
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 ON CONFLICT ON CONSTRAINT unclaimed_arrival_fact_key DO NOTHING
		 RETURNING arrival_id`,
		arrival.Tenant().String(), arrival.ID().String(), arrival.FactID().String(),
		arrival.Origin().String(), arrival.Kind().String(), arrival.Status().String(),
		arrival.Warehouse().String(), arrival.Device().String(), arrival.Operator().String(),
		arrival.OccurredAt(), arrival.DeviceSequence(), arrival.ReceivedAt(),
		obs.String(), nullableText(ownerClue.String()), nullableText(preAdviceClue.String()),
		nullableText(storage.String()),
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return true, nil
}

func (store *UnclaimedArrivals) HoldArrival(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.UnclaimedArrivalID,
) error {
	const operation = "hold unclaimed arrival"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.unclaimed_arrival.arrival'),
		    hashtext($1 || chr(31) || $2))`,
		tenant.String(), id.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *UnclaimedArrivals) HoldClaimDecision(
	ctx context.Context,
	tenant domain.TenantID,
	decisionID domain.RequestKey,
) error {
	const operation = "hold unclaimed arrival claim decision"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.unclaimed_arrival.claim_decision'),
		    hashtext($1 || chr(31) || $2))`,
		tenant.String(), decisionID.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *UnclaimedArrivals) HoldRevokeDecision(
	ctx context.Context,
	tenant domain.TenantID,
	decisionID domain.RequestKey,
) error {
	const operation = "hold unclaimed arrival revoke decision"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.unclaimed_arrival.revoke_decision'),
		    hashtext($1 || chr(31) || $2))`,
		tenant.String(), decisionID.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *UnclaimedArrivals) LoadClaimByDecision(
	ctx context.Context,
	tenant domain.TenantID,
	decisionID domain.RequestKey,
) (domain.UnclaimedArrivalClaim, bool, error) {
	const operation = "load unclaimed arrival claim by decision"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.UnclaimedArrivalClaim{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var (
		arrivalText, ownerText, operatorText string
		preAdviceText                        *string
		claimedAt                            time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT arrival_id, owner_account_id, pre_advice_id, operator_id, claimed_at
		   FROM warehouse_fulfillment.unclaimed_arrival_claim
		  WHERE tenant_id = $1 AND decision_id = $2`,
		tenant.String(), decisionID.String(),
	).Scan(&arrivalText, &ownerText, &preAdviceText, &operatorText, &claimedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UnclaimedArrivalClaim{}, false, nil
	}
	if err != nil {
		return domain.UnclaimedArrivalClaim{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	claim, err := claimFromColumns(tenant, decisionID.String(), arrivalText, ownerText, preAdviceText, operatorText, claimedAt)
	if err != nil {
		return domain.UnclaimedArrivalClaim{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return claim, true, nil
}

func (store *UnclaimedArrivals) LoadRevocationByDecision(
	ctx context.Context,
	tenant domain.TenantID,
	decisionID domain.RequestKey,
) (domain.UnclaimedArrivalClaimRevocation, bool, error) {
	const operation = "load unclaimed arrival revocation by decision"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.UnclaimedArrivalClaimRevocation{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var (
		arrivalText, claimDecisionText, operatorText string
		reasonText                                   *string
		revokedAt                                    time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT arrival_id, claim_decision_id, reason, operator_id, revoked_at
		   FROM warehouse_fulfillment.unclaimed_arrival_claim_revocation
		  WHERE tenant_id = $1 AND decision_id = $2`,
		tenant.String(), decisionID.String(),
	).Scan(&arrivalText, &claimDecisionText, &reasonText, &operatorText, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UnclaimedArrivalClaimRevocation{}, false, nil
	}
	if err != nil {
		return domain.UnclaimedArrivalClaimRevocation{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	rev, err := revocationFromColumns(tenant, decisionID.String(), arrivalText, claimDecisionText, reasonText, operatorText, revokedAt)
	if err != nil {
		return domain.UnclaimedArrivalClaimRevocation{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return rev, true, nil
}

func (store *UnclaimedArrivals) HasConfirmedReceiptReference(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.UnclaimedArrivalID,
) (bool, error) {
	const operation = "check confirmed receipt reference for unclaimed arrival"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	var exists bool
	err = querier.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1
		      FROM warehouse_fulfillment.receipt_confirmation
		     WHERE tenant_id = $1 AND unclaimed_arrival_id = $2 AND outcome = 'CONFIRMED'
		 )`,
		tenant.String(), id.String(),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return exists, nil
}

func (store *UnclaimedArrivals) RecordClaim(ctx context.Context, claim domain.UnclaimedArrivalClaim) error {
	const operation = "record unclaimed arrival claim"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	preAdvice, _ := claim.PreAdvice()
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_arrival_claim (
		    tenant_id, decision_id, arrival_id, owner_account_id, pre_advice_id, operator_id, claimed_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		claim.Tenant().String(), claim.DecisionID().String(), claim.ArrivalID().String(),
		claim.Owner().String(), nullableText(preAdvice.String()), claim.Operator().String(), claim.ClaimedAt(),
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.unclaimed_arrival
		    SET status = $3
		  WHERE tenant_id = $1 AND arrival_id = $2 AND status = $4`,
		claim.Tenant().String(), claim.ArrivalID().String(),
		domain.UnclaimedArrivalStatusClaimed.String(), domain.UnclaimedArrivalStatusUnclaimed.String(),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: 到货状态在锁内不是待认领", operation)
	}
	return nil
}

func (store *UnclaimedArrivals) RecordRevocation(ctx context.Context, revocation domain.UnclaimedArrivalClaimRevocation) error {
	const operation = "record unclaimed arrival claim revocation"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	reason, _ := revocation.Reason()
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_arrival_claim_revocation (
		    tenant_id, decision_id, arrival_id, claim_decision_id, reason, operator_id, revoked_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		revocation.Tenant().String(), revocation.DecisionID().String(), revocation.ArrivalID().String(),
		revocation.ClaimDecisionID().String(), nullableText(reason.String()),
		revocation.Operator().String(), revocation.RevokedAt(),
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.unclaimed_arrival
		    SET status = $3
		  WHERE tenant_id = $1 AND arrival_id = $2 AND status = $4`,
		revocation.Tenant().String(), revocation.ArrivalID().String(),
		domain.UnclaimedArrivalStatusUnclaimed.String(), domain.UnclaimedArrivalStatusClaimed.String(),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: 到货状态在锁内不是已认领", operation)
	}
	return nil
}

func (store *UnclaimedArrivals) RecordDisposition(ctx context.Context, disposition domain.UnclaimedArrivalDisposition) error {
	const operation = "record unclaimed arrival disposition"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	note, _ := disposition.Note()
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.unclaimed_arrival_disposition (
		    tenant_id, arrival_id, method, rule_warehouse_code, rule_revision, note, operator_id, disposed_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		disposition.Tenant().String(), disposition.ArrivalID().String(), disposition.Method().String(),
		disposition.RuleWarehouse().String(), disposition.RuleRevision(), nullableText(note.String()),
		disposition.Operator().String(), disposition.DisposedAt(),
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.unclaimed_arrival
		    SET status = $3
		  WHERE tenant_id = $1 AND arrival_id = $2 AND status = $4`,
		disposition.Tenant().String(), disposition.ArrivalID().String(),
		domain.UnclaimedArrivalStatusDisposed.String(), domain.UnclaimedArrivalStatusUnclaimed.String(),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: 到货状态在锁内不是待认领", operation)
	}
	return nil
}

// PreAdviceIDsWithUnclaimedArrival 只看由收货事实转来的待认领到货（origin = 'DIVERTED_RECEIPT'）：
// 作业事实直接登记时附带的预报线索不是认定，不得漏给货主的预报读口（AT-WF-055、AT-WF-260）。
func (store *UnclaimedArrivals) PreAdviceIDsWithUnclaimedArrival(
	ctx context.Context,
	tenant domain.TenantID,
	ids []domain.PreAdviceID,
) ([]domain.PreAdviceID, error) {
	const operation = "list pre-advices with unclaimed arrival"
	if len(ids) == 0 {
		return nil, nil
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	keys := make([]string, len(ids))
	for index, id := range ids {
		keys[index] = id.String()
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT pre_advice_id
		   FROM warehouse_fulfillment.unclaimed_arrival
		  WHERE tenant_id = $1 AND origin = 'DIVERTED_RECEIPT' AND pre_advice_id = ANY($2)`,
		tenant.String(), keys)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()
	var found []domain.PreAdviceID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		id, err := domain.NewPreAdviceID(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		found = append(found, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return found, nil
}

func (store *UnclaimedArrivals) LoadDetail(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.UnclaimedArrivalID,
) (domain.UnclaimedArrivalDetail, bool, error) {
	const operation = "load unclaimed arrival detail"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.UnclaimedArrivalDetail{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	row := querier.QueryRow(ctx,
		`SELECT `+unclaimedArrivalColumns+`
		   FROM warehouse_fulfillment.unclaimed_arrival
		  WHERE tenant_id = $1 AND arrival_id = $2`,
		tenant.String(), id.String())
	stored, err := scanStoredUnclaimedArrival(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UnclaimedArrivalDetail{}, false, nil
	}
	if err != nil {
		return domain.UnclaimedArrivalDetail{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	detail, err := store.hydrateDetail(ctx, querier, stored)
	if err != nil {
		return domain.UnclaimedArrivalDetail{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return detail, true, nil
}

func (store *UnclaimedArrivals) ListUnclaimedArrivals(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
	receivedFrom, receivedTo time.Time,
) (cataloguepage.Result[domain.UnclaimedArrivalDetail], error) {
	const operation = "list unclaimed arrivals"
	fail := func(err error) (cataloguepage.Result[domain.UnclaimedArrivalDetail], error) {
		return cataloguepage.Result[domain.UnclaimedArrivalDetail]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" {
		return fail(errors.New("tenant is required"))
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	var fromArg, toArg any
	if !receivedFrom.IsZero() {
		fromArg = receivedFrom.UTC()
	}
	if !receivedTo.IsZero() {
		toArg = receivedTo.UTC()
	}
	plan, err := unclaimedArrivalListBinding.Plan(query, "", []any{tenant.String(), fromArg, toArg}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, unclaimedArrivalListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, unclaimedArrivalListed+" "+plan.PageSQL(unclaimedArrivalColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	var storedList []storedUnclaimedArrival
	for rows.Next() {
		item, scanErr := scanStoredUnclaimedArrival(rows)
		if scanErr != nil {
			rows.Close()
			return fail(scanErr)
		}
		storedList = append(storedList, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fail(err)
	}
	rows.Close()

	details := make([]domain.UnclaimedArrivalDetail, 0, len(storedList))
	for _, stored := range storedList {
		detail, hydrateErr := store.hydrateDetail(ctx, querier, stored)
		if hydrateErr != nil {
			return fail(hydrateErr)
		}
		details = append(details, detail)
	}
	page, err := cataloguepage.Finish(query, details, limit, total,
		func(item domain.UnclaimedArrivalDetail, sortField string) cataloguepage.Position {
			switch sortField {
			case "arrivalId":
				return cataloguepage.Position{Value: item.Arrival.ID().String(), Identity: []string{item.Arrival.ID().String()}}
			default:
				return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.Arrival.ReceivedAt()), Identity: []string{item.Arrival.ID().String()}}
			}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

type storedUnclaimedArrival struct {
	tenant, arrivalID, factID, origin, kind, status, warehouse string
	device, operator                                           string
	occurredAt, receivedAt                                     time.Time
	deviceSequence                                             *int64
	observation, ownerClue, preAdviceClue, storageLocation     *string
	divertedReceiptFactID                                      *string
}

func scanStoredUnclaimedArrival(row measurementScanner) (storedUnclaimedArrival, error) {
	var s storedUnclaimedArrival
	err := row.Scan(
		&s.tenant, &s.arrivalID, &s.factID, &s.origin, &s.kind, &s.status, &s.warehouse,
		&s.device, &s.operator, &s.occurredAt, &s.deviceSequence, &s.receivedAt,
		&s.observation, &s.ownerClue, &s.preAdviceClue, &s.storageLocation, &s.divertedReceiptFactID,
	)
	return s, err
}

func (store *UnclaimedArrivals) hydrateArrival(
	ctx context.Context,
	querier bentopg.Querier,
	s storedUnclaimedArrival,
) (domain.RecordedUnclaimedArrival, error) {
	tenant, err := domain.NewTenantID(s.tenant)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, err
	}
	id, err := domain.NewUnclaimedArrivalID(s.arrivalID)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, err
	}
	factID, err := domain.NewUnclaimedArrivalFactID(s.factID)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, err
	}
	origin, err := domain.NewUnclaimedArrivalOrigin(s.origin)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, err
	}
	kind, err := domain.NewUnclaimedArrivalKind(s.kind)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, err
	}
	status, err := domain.NewUnclaimedArrivalStatus(s.status)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, err
	}
	warehouse, err := domain.NewWarehouseCode(s.warehouse)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, err
	}
	device, err := domain.NewReceiptDeviceID(s.device)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, err
	}
	operator, err := domain.NewReceiptOperatorID(s.operator)
	if err != nil {
		return domain.RecordedUnclaimedArrival{}, err
	}
	var seq int64
	if s.deviceSequence != nil {
		seq = *s.deviceSequence
	}
	var obs domain.UnclaimedArrivalObservation
	if s.observation != nil {
		obs, err = domain.NewUnclaimedArrivalObservation(*s.observation)
		if err != nil {
			return domain.RecordedUnclaimedArrival{}, err
		}
	}
	var ownerClue domain.OwnerAccountID
	if s.ownerClue != nil {
		ownerClue, err = domain.NewOwnerAccountID(*s.ownerClue)
		if err != nil {
			return domain.RecordedUnclaimedArrival{}, err
		}
	}
	var preAdviceClue domain.PreAdviceID
	if s.preAdviceClue != nil {
		preAdviceClue, err = domain.NewPreAdviceID(*s.preAdviceClue)
		if err != nil {
			return domain.RecordedUnclaimedArrival{}, err
		}
	}
	var storage domain.UnclaimedArrivalStorageLocation
	if s.storageLocation != nil {
		storage, err = domain.NewUnclaimedArrivalStorageLocation(*s.storageLocation)
		if err != nil {
			return domain.RecordedUnclaimedArrival{}, err
		}
	}
	var diverted *domain.RecordedReceipt
	if s.divertedReceiptFactID != nil {
		receipts, err := loadReceipts(ctx, querier, "tenant_id = $1 AND fact_id = $2", s.tenant, *s.divertedReceiptFactID)
		if err != nil {
			return domain.RecordedUnclaimedArrival{}, err
		}
		if len(receipts) == 1 {
			copied := receipts[0]
			diverted = &copied
		}
	}
	return domain.NewRecordedUnclaimedArrival(domain.RecordedUnclaimedArrivalSpec{
		Tenant: tenant, ID: id, FactID: factID, Origin: origin, Kind: kind, Status: status,
		Warehouse: warehouse, Device: device, Operator: operator,
		OccurredAt: s.occurredAt, DeviceSequence: seq, ReceivedAt: s.receivedAt,
		Observation: obs, OwnerClue: ownerClue, PreAdviceClue: preAdviceClue,
		StorageLocation: storage, DivertedReceipt: diverted,
	})
}

func (store *UnclaimedArrivals) hydrateDetail(
	ctx context.Context,
	querier bentopg.Querier,
	s storedUnclaimedArrival,
) (domain.UnclaimedArrivalDetail, error) {
	arrival, err := store.hydrateArrival(ctx, querier, s)
	if err != nil {
		return domain.UnclaimedArrivalDetail{}, err
	}
	claims, err := loadClaimHistory(ctx, querier, arrival.Tenant(), arrival.ID())
	if err != nil {
		return domain.UnclaimedArrivalDetail{}, err
	}
	refs, err := loadPostingReferences(ctx, querier, arrival.Tenant(), arrival.ID())
	if err != nil {
		return domain.UnclaimedArrivalDetail{}, err
	}
	disposition, err := loadDisposition(ctx, querier, arrival.Tenant(), arrival.ID())
	if err != nil {
		return domain.UnclaimedArrivalDetail{}, err
	}
	return domain.UnclaimedArrivalDetail{
		Arrival:           arrival,
		Claims:            claims,
		PostingReferences: refs,
		Disposition:       disposition,
	}, nil
}

func loadClaimHistory(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	arrivalID domain.UnclaimedArrivalID,
) ([]domain.ClaimHistoryEntry, error) {
	rows, err := querier.Query(ctx,
		`SELECT c.decision_id, c.owner_account_id, c.pre_advice_id, c.operator_id, c.claimed_at,
		        r.decision_id, r.reason, r.operator_id, r.revoked_at
		   FROM warehouse_fulfillment.unclaimed_arrival_claim c
		   LEFT JOIN warehouse_fulfillment.unclaimed_arrival_claim_revocation r
		     ON r.tenant_id = c.tenant_id AND r.claim_decision_id = c.decision_id
		  WHERE c.tenant_id = $1 AND c.arrival_id = $2
		  ORDER BY c.claim_seq ASC`,
		tenant.String(), arrivalID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []domain.ClaimHistoryEntry
	for rows.Next() {
		var (
			cDec, ownerText, cOp string
			preAdviceText        *string
			claimedAt            time.Time
			rDec, rReason, rOp   *string
			revokedAt            *time.Time
		)
		if err := rows.Scan(&cDec, &ownerText, &preAdviceText, &cOp, &claimedAt, &rDec, &rReason, &rOp, &revokedAt); err != nil {
			return nil, err
		}
		claim, err := claimFromColumns(tenant, cDec, arrivalID.String(), ownerText, preAdviceText, cOp, claimedAt)
		if err != nil {
			return nil, err
		}
		entry := domain.ClaimHistoryEntry{Claim: claim}
		if rDec != nil && rOp != nil && revokedAt != nil {
			rev, err := revocationFromColumns(tenant, *rDec, arrivalID.String(), cDec, rReason, *rOp, *revokedAt)
			if err != nil {
				return nil, err
			}
			entry.Revocation = &rev
		}
		history = append(history, entry)
	}
	return history, rows.Err()
}

func loadPostingReferences(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	arrivalID domain.UnclaimedArrivalID,
) ([]domain.ReceiptFactID, error) {
	rows, err := querier.Query(ctx,
		`SELECT fact_id
		   FROM warehouse_fulfillment.receipt_confirmation
		  WHERE tenant_id = $1 AND unclaimed_arrival_id = $2 AND outcome = 'CONFIRMED'
		  ORDER BY received_seq ASC`,
		tenant.String(), arrivalID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []domain.ReceiptFactID
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		ref, err := domain.NewReceiptFactID(text)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

func loadDisposition(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	arrivalID domain.UnclaimedArrivalID,
) (*domain.UnclaimedArrivalDisposition, error) {
	var (
		methodText, ruleWhText, opText string
		ruleRev                        int
		noteText                       *string
		disposedAt                     time.Time
	)
	err := querier.QueryRow(ctx,
		`SELECT method, rule_warehouse_code, rule_revision, note, operator_id, disposed_at
		   FROM warehouse_fulfillment.unclaimed_arrival_disposition
		  WHERE tenant_id = $1 AND arrival_id = $2`,
		tenant.String(), arrivalID.String(),
	).Scan(&methodText, &ruleWhText, &ruleRev, &noteText, &opText, &disposedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	method, err := domain.NewUnclaimedDisposalMethod(methodText)
	if err != nil {
		return nil, err
	}
	ruleWh, err := domain.NewWarehouseCode(ruleWhText)
	if err != nil {
		return nil, err
	}
	var note domain.UnclaimedArrivalNote
	if noteText != nil {
		note, err = domain.NewUnclaimedArrivalNote(*noteText)
		if err != nil {
			return nil, err
		}
	}
	op, err := domain.NewRegistrant(opText)
	if err != nil {
		return nil, err
	}
	disp, err := domain.NewUnclaimedArrivalDisposition(tenant, arrivalID, method, ruleWh, ruleRev, note, op, disposedAt)
	if err != nil {
		return nil, err
	}
	return &disp, nil
}

func claimFromColumns(
	tenant domain.TenantID,
	decisionText, arrivalText, ownerText string,
	preAdviceText *string,
	operatorText string,
	claimedAt time.Time,
) (domain.UnclaimedArrivalClaim, error) {
	dec, err := domain.NewRequestKey(decisionText)
	if err != nil {
		return domain.UnclaimedArrivalClaim{}, err
	}
	arr, err := domain.NewUnclaimedArrivalID(arrivalText)
	if err != nil {
		return domain.UnclaimedArrivalClaim{}, err
	}
	owner, err := domain.NewOwnerAccountID(ownerText)
	if err != nil {
		return domain.UnclaimedArrivalClaim{}, err
	}
	var advice domain.PreAdviceID
	if preAdviceText != nil {
		advice, err = domain.NewPreAdviceID(*preAdviceText)
		if err != nil {
			return domain.UnclaimedArrivalClaim{}, err
		}
	}
	op, err := domain.NewRegistrant(operatorText)
	if err != nil {
		return domain.UnclaimedArrivalClaim{}, err
	}
	return domain.NewUnclaimedArrivalClaim(tenant, dec, arr, owner, advice, op, claimedAt)
}

func revocationFromColumns(
	tenant domain.TenantID,
	decisionText, arrivalText, claimDecisionText string,
	reasonText *string,
	operatorText string,
	revokedAt time.Time,
) (domain.UnclaimedArrivalClaimRevocation, error) {
	dec, err := domain.NewRequestKey(decisionText)
	if err != nil {
		return domain.UnclaimedArrivalClaimRevocation{}, err
	}
	arr, err := domain.NewUnclaimedArrivalID(arrivalText)
	if err != nil {
		return domain.UnclaimedArrivalClaimRevocation{}, err
	}
	claimDec, err := domain.NewRequestKey(claimDecisionText)
	if err != nil {
		return domain.UnclaimedArrivalClaimRevocation{}, err
	}
	var reason domain.UnclaimedArrivalNote
	if reasonText != nil {
		reason, err = domain.NewUnclaimedArrivalNote(*reasonText)
		if err != nil {
			return domain.UnclaimedArrivalClaimRevocation{}, err
		}
	}
	op, err := domain.NewRegistrant(operatorText)
	if err != nil {
		return domain.UnclaimedArrivalClaimRevocation{}, err
	}
	return domain.NewUnclaimedArrivalClaimRevocation(tenant, dec, arr, claimDec, reason, op, revokedAt)
}
