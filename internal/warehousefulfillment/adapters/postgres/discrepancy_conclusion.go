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

// DiscrepancyConclusions 是收货差异结论的 PostgreSQL 适配器。事务是 READ COMMITTED。
// 差异标识上的咨询锁把同一份差异的货主口与代录口串在一起。来源请求键的锁只在差异锁之后拿，避免交叉等待。
type DiscrepancyConclusions struct {
	db *bentopg.DB
}

func NewDiscrepancyConclusions(db *bentopg.DB) (*DiscrepancyConclusions, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &DiscrepancyConclusions{db: db}, nil
}

var _ ports.DiscrepancyConclusionStore = (*DiscrepancyConclusions)(nil)

func (store *DiscrepancyConclusions) Hold(
	ctx context.Context,
	tenant domain.TenantID,
	discrepancy domain.ReceiptDiscrepancyID,
) error {
	const operation = "hold receipt discrepancy"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.receipt_discrepancy.conclusion'),
		    hashtext($1 || chr(31) || $2))`,
		tenant.String(), discrepancy.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *DiscrepancyConclusions) HoldRequestKey(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.RequestKey,
) error {
	const operation = "hold receipt discrepancy request key"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.receipt_discrepancy.conclusion_key'),
		    hashtext($1 || chr(31) || $2))`,
		tenant.String(), key.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *DiscrepancyConclusions) FindSubject(
	ctx context.Context,
	tenant domain.TenantID,
	discrepancy domain.ReceiptDiscrepancyID,
) (ports.DiscrepancySubject, bool, error) {
	const operation = "find receipt discrepancy subject"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DiscrepancySubject{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	receipt, receiptFound, err := findReceiptSubject(ctx, querier, tenant.String(), discrepancy.String())
	if err != nil {
		return ports.DiscrepancySubject{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	shortage, shortageFound, err := findShortageSubject(ctx, querier, tenant.String(), discrepancy.String())
	if err != nil {
		return ports.DiscrepancySubject{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	switch {
	case receiptFound && shortageFound:
		return ports.DiscrepancySubject{}, false, fmt.Errorf("%s: discrepancy %s is both a receipt discrepancy and a shortage", operation, discrepancy)
	case receiptFound:
		return receipt, true, nil
	case shortageFound:
		return shortage, true, nil
	default:
		return ports.DiscrepancySubject{}, false, nil
	}
}

func (store *DiscrepancyConclusions) FindRequestKey(
	ctx context.Context,
	tenant domain.TenantID,
	key domain.RequestKey,
) (domain.ReceiptDiscrepancyID, bool, error) {
	const operation = "find receipt discrepancy request key"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ReceiptDiscrepancyID{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var discrepancy string
	err = querier.QueryRow(ctx,
		`SELECT discrepancy_id
		   FROM warehouse_fulfillment.receipt_discrepancy_conclusion
		  WHERE tenant_id = $1 AND source_request_key = $2`,
		tenant.String(), key.String()).Scan(&discrepancy)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReceiptDiscrepancyID{}, false, nil
	}
	if err != nil {
		return domain.ReceiptDiscrepancyID{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	id, err := domain.NewReceiptDiscrepancyID(discrepancy)
	if err != nil {
		return domain.ReceiptDiscrepancyID{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return id, true, nil
}

func (store *DiscrepancyConclusions) History(
	ctx context.Context,
	tenant domain.TenantID,
	discrepancy domain.ReceiptDiscrepancyID,
) ([]domain.DiscrepancyConclusion, error) {
	rows, err := store.ListByDiscrepancies(ctx, tenant, []domain.ReceiptDiscrepancyID{discrepancy})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (store *DiscrepancyConclusions) ListByDiscrepancies(
	ctx context.Context,
	tenant domain.TenantID,
	ids []domain.ReceiptDiscrepancyID,
) ([]domain.DiscrepancyConclusion, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	keys := make([]string, len(ids))
	for index, id := range ids {
		keys[index] = id.String()
	}
	const operation = "list receipt discrepancy conclusions"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT discrepancy_id, verdict, note, submission_path, source_request_key,
		        operator_id, evidence_reference, recorded_at, current
		   FROM warehouse_fulfillment.receipt_discrepancy_conclusion
		  WHERE tenant_id = $1 AND discrepancy_id = ANY($2)
		  ORDER BY discrepancy_id, recorded_seq`,
		tenant.String(), keys)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()
	var conclusions []domain.DiscrepancyConclusion
	for rows.Next() {
		conclusion, err := scanConclusion(tenant, rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		conclusions = append(conclusions, conclusion)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return conclusions, nil
}

func (store *DiscrepancyConclusions) Record(
	ctx context.Context,
	conclusion domain.DiscrepancyConclusion,
	supersede bool,
) error {
	const operation = "record receipt discrepancy conclusion"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if supersede {
		tag, err := executor.Exec(ctx,
			`UPDATE warehouse_fulfillment.receipt_discrepancy_conclusion
			    SET current = false
			  WHERE tenant_id = $1 AND discrepancy_id = $2 AND current`,
			conclusion.Tenant().String(), conclusion.Discrepancy().String())
		if err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("%s: the current conclusion was not there to supersede", operation)
		}
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.receipt_discrepancy_conclusion (
		    tenant_id, discrepancy_id, verdict, note, submission_path, source_request_key,
		    operator_id, evidence_reference, recorded_at, current
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		conclusion.Tenant().String(), conclusion.Discrepancy().String(), conclusion.Verdict().String(),
		nullableText(conclusion.Note().String()), conclusion.Path().String(),
		nullableText(conclusion.RequestKey().String()), nullableText(conclusion.Operator().String()),
		nullableText(conclusion.Evidence().String()), conclusion.RecordedAt(), conclusion.Current(),
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

type conclusionScanner interface {
	Scan(dest ...any) error
}

func scanConclusion(tenant domain.TenantID, row conclusionScanner) (domain.DiscrepancyConclusion, error) {
	var (
		discrepancyID, verdict, path string
		note, requestKey, operator   *string
		evidence                     *string
		recordedAt                   time.Time
		current                      bool
	)
	if err := row.Scan(&discrepancyID, &verdict, &note, &path, &requestKey, &operator, &evidence, &recordedAt, &current); err != nil {
		return domain.DiscrepancyConclusion{}, err
	}
	discrepancy, err := domain.NewReceiptDiscrepancyID(discrepancyID)
	if err != nil {
		return domain.DiscrepancyConclusion{}, err
	}
	verdictValue, err := domain.NewConclusionVerdict(verdict)
	if err != nil {
		return domain.DiscrepancyConclusion{}, err
	}
	var written domain.ConclusionNote
	if note != nil {
		written, err = domain.NewConclusionNote(*note)
		if err != nil {
			return domain.DiscrepancyConclusion{}, err
		}
	}
	submission, err := domain.NewSubmissionPath(path)
	if err != nil {
		return domain.DiscrepancyConclusion{}, err
	}
	var key domain.RequestKey
	if requestKey != nil {
		key, err = domain.NewRequestKey(*requestKey)
		if err != nil {
			return domain.DiscrepancyConclusion{}, err
		}
	}
	var actor domain.Registrant
	if operator != nil {
		actor, err = domain.NewRegistrant(*operator)
		if err != nil {
			return domain.DiscrepancyConclusion{}, err
		}
	}
	var reference domain.EvidenceReference
	if evidence != nil {
		reference, err = domain.NewEvidenceReference(*evidence)
		if err != nil {
			return domain.DiscrepancyConclusion{}, err
		}
	}
	return domain.NewDiscrepancyConclusion(
		tenant, discrepancy, verdictValue, written, submission, key, actor, reference, recordedAt, current,
	)
}

func findReceiptSubject(
	ctx context.Context,
	querier bentopg.Querier,
	tenant, discrepancy string,
) (ports.DiscrepancySubject, bool, error) {
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT pre_advice.pre_advice_id, pre_advice.owner_account_id
		   FROM warehouse_fulfillment.receipt_discrepancy AS discrepancy
		   JOIN warehouse_fulfillment.receipt_confirmation AS receipt
		     ON receipt.tenant_id = discrepancy.tenant_id
		    AND receipt.fact_id = discrepancy.fact_id
		   JOIN warehouse_fulfillment.inbound_pre_advice AS pre_advice
		     ON pre_advice.tenant_id = receipt.tenant_id
		    AND pre_advice.pre_advice_id = receipt.pre_advice_id
		  WHERE discrepancy.tenant_id = $1 AND discrepancy.discrepancy_id = $2`,
		tenant, discrepancy)
	if err != nil {
		return ports.DiscrepancySubject{}, false, err
	}
	defer rows.Close()
	return oneSubject(discrepancy, rows)
}

func findShortageSubject(
	ctx context.Context,
	querier bentopg.Querier,
	tenant, discrepancy string,
) (ports.DiscrepancySubject, bool, error) {
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT pre_advice.pre_advice_id, pre_advice.owner_account_id
		   FROM warehouse_fulfillment.inbound_pre_advice_shortage AS shortage
		   JOIN warehouse_fulfillment.inbound_pre_advice AS pre_advice
		     ON pre_advice.tenant_id = shortage.tenant_id
		    AND pre_advice.pre_advice_id = shortage.pre_advice_id
		  WHERE shortage.tenant_id = $1 AND shortage.discrepancy_id = $2`,
		tenant, discrepancy)
	if err != nil {
		return ports.DiscrepancySubject{}, false, err
	}
	defer rows.Close()
	return oneSubject(discrepancy, rows)
}

func oneSubject(discrepancy string, rows pgx.Rows) (ports.DiscrepancySubject, bool, error) {
	var subject ports.DiscrepancySubject
	found := false
	for rows.Next() {
		var preAdvice, owner string
		if err := rows.Scan(&preAdvice, &owner); err != nil {
			return ports.DiscrepancySubject{}, false, err
		}
		if found {
			return ports.DiscrepancySubject{}, false, fmt.Errorf("discrepancy %s belongs to more than one pre-advice", discrepancy)
		}
		id, err := domain.NewReceiptDiscrepancyID(discrepancy)
		if err != nil {
			return ports.DiscrepancySubject{}, false, err
		}
		advice, err := domain.NewPreAdviceID(preAdvice)
		if err != nil {
			return ports.DiscrepancySubject{}, false, err
		}
		account, err := domain.NewOwnerAccountID(owner)
		if err != nil {
			return ports.DiscrepancySubject{}, false, err
		}
		subject = ports.DiscrepancySubject{Discrepancy: id, PreAdvice: advice, Owner: account}
		found = true
	}
	if err := rows.Err(); err != nil {
		return ports.DiscrepancySubject{}, false, err
	}
	return subject, found, nil
}
