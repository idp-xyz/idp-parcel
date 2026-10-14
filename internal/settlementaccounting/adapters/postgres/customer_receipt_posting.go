package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// CustomerReceiptPostings 读客户方向的几本账，并在同一事务里推进运营余额。
// 代收本金不进：这里只读本上下文的映射、核销、已确认应收，不读 collection-remittance。
type CustomerReceiptPostings struct {
	db *bentopg.DB
}

func NewCustomerReceiptPostings(db *bentopg.DB) (*CustomerReceiptPostings, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &CustomerReceiptPostings{db: db}, nil
}

var (
	_ ports.CustomerReceiptBooks      = (*CustomerReceiptPostings)(nil)
	_ ports.OperationalBalanceAdvance = (*CustomerReceiptPostings)(nil)
)

func (repository *CustomerReceiptPostings) CustomerStatementScope(
	ctx context.Context,
	tenant domain.TenantID,
	statement string,
) (domain.SettlementScope, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SettlementScope{}, false, fmt.Errorf("customer statement scope: %w", err)
	}

	var legalEntity, account, currency string
	err = querier.QueryRow(ctx,
		`SELECT a.legal_entity_id, a.account_id, a.currency
		   FROM settlement_accounting.customer_statement AS s
		   JOIN settlement_accounting.settlement_account AS a
		     ON a.tenant_id = s.tenant_id
		    AND a.account_id = s.account_id
		  WHERE s.tenant_id = $1
		    AND s.statement_number = $2
		    AND a.direction = 'RECEIVABLE'
		    AND s.currency = a.currency`,
		tenant.String(),
		statement,
	).Scan(&legalEntity, &account, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SettlementScope{}, false, nil
	}
	if err != nil {
		return domain.SettlementScope{}, false, fmt.Errorf("customer statement scope: %w", err)
	}

	scope, err := scopeFrom(legalEntity, account, currency)
	if err != nil {
		return domain.SettlementScope{}, false, fmt.Errorf("customer statement scope: %w", err)
	}
	return scope, true, nil
}

func (repository *CustomerReceiptPostings) CustomerDirectionNets(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
) (postedNet, writeoffNet, confirmed int64, err error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("customer direction nets: %w", err)
	}

	err = querier.QueryRow(ctx,
		`SELECT
		    COALESCE((
		        SELECT SUM(
		            CASE v.kind
		                WHEN 'RECEIPT_CONFIRMED' THEN v.amount_minor
		                WHEN 'FUNDS_RETURNED' THEN -v.amount_minor
		                ELSE 0
		            END)
		          FROM settlement_accounting.funds_mapping AS m
		          JOIN settlement_accounting.customer_statement AS s
		            ON s.tenant_id = m.tenant_id
		           AND s.statement_number = m.target_ref
		          JOIN settlement_accounting.settlement_account AS a
		            ON a.tenant_id = s.tenant_id
		           AND a.account_id = s.account_id
		          JOIN settlement_accounting.external_funds_fact_version AS v
		            ON v.tenant_id = m.tenant_id
		           AND v.fact_id = m.fact_id
		         WHERE m.tenant_id = $1
		           AND m.target_kind = 'STATEMENT'
		           AND a.direction = 'RECEIVABLE'
		           AND a.legal_entity_id = $2
		           AND a.account_id = $3
		           AND a.currency = $4
		           AND s.currency = a.currency
		           AND v.currency = a.currency
		           AND NOT EXISTS (
		               SELECT 1
		                 FROM settlement_accounting.external_funds_fact_version AS newer
		                WHERE newer.tenant_id = v.tenant_id
		                  AND newer.fact_id = v.fact_id
		                  AND newer.corrects = v.version)
		    ), 0),
		    COALESCE((
		        SELECT SUM(
		            CASE elem->>'direction'
		                WHEN 'DEBIT' THEN (elem->>'amountMinor')::bigint
		                WHEN 'CREDIT' THEN -(elem->>'amountMinor')::bigint
		                ELSE 0
		            END)
		          FROM settlement_accounting.settlement_application AS app
		          CROSS JOIN LATERAL jsonb_array_elements(app.allocations) AS elem
		          JOIN settlement_accounting.customer_statement AS s
		            ON s.tenant_id = app.tenant_id
		           AND s.statement_number = elem->>'target'
		          JOIN settlement_accounting.settlement_account AS a
		            ON a.tenant_id = s.tenant_id
		           AND a.account_id = s.account_id
		         WHERE app.tenant_id = $1
		           AND app.reversal_basis IS NULL
		           AND elem->>'targetKind' = 'STATEMENT'
		           AND a.direction = 'RECEIVABLE'
		           AND a.legal_entity_id = $2
		           AND a.account_id = $3
		           AND a.currency = $4
		           AND s.currency = a.currency
		    ), 0),
		    COALESCE((
		        SELECT SUM(c.settlement_minor)
		          FROM settlement_accounting.customer_charge AS c
		         WHERE c.tenant_id = $1
		           AND c.stage = 'CONFIRMED'
		           AND c.charge_direction = 'RECEIVABLE'
		           AND c.responsible_entity = $2
		           AND c.settlement_account_id = $3
		           AND c.settlement_currency = $4
		    ), 0)
		    + COALESCE((
		        SELECT SUM(
		            CASE adj.direction
		                WHEN 'DEBIT' THEN adj.settlement_minor
		                WHEN 'CREDIT' THEN -adj.settlement_minor
		                ELSE 0
		            END)
		          FROM settlement_accounting.charge_adjustment AS adj
		          JOIN settlement_accounting.customer_charge AS c
		            ON c.tenant_id = adj.tenant_id
		           AND c.charge_id = adj.charge_id
		         WHERE c.tenant_id = $1
		           AND c.stage = 'CONFIRMED'
		           AND c.charge_direction = 'RECEIVABLE'
		           AND c.responsible_entity = $2
		           AND c.settlement_account_id = $3
		           AND c.settlement_currency = $4
		           AND adj.settlement_currency = c.settlement_currency
		    ), 0)`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
	).Scan(&postedNet, &writeoffNet, &confirmed)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("customer direction nets: %w", err)
	}
	return postedNet, writeoffNet, confirmed, nil
}

// AdvanceOperationalBalance 插入时授信额度列写 0，冲突更新不碰该列。
// 同键只准往前：截至时点不更新则交回未推进。
func (repository *CustomerReceiptPostings) AdvanceOperationalBalance(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	postedMinor, unsettledMinor int64,
	asOf time.Time,
) (ports.OperationalBalanceAdvanceOutcome, error) {
	if asOf.IsZero() {
		return ports.OperationalBalanceAdvanceInvalid, errors.New(
			"advance operational balance: 余额登记必须带截至时点，否则先后无从判断")
	}

	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.OperationalBalanceAdvanceInvalid, fmt.Errorf("advance operational balance: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.operational_balance
			(tenant_id, legal_entity, account_id, currency,
			 posted_minor, credit_minor, unsettled_minor, as_of)
		 VALUES ($1, $2, $3, $4, $5, 0, $6, $7)
		 ON CONFLICT ON CONSTRAINT operational_balance_pkey DO UPDATE
		    SET posted_minor = EXCLUDED.posted_minor,
		        unsettled_minor = EXCLUDED.unsettled_minor,
		        as_of = EXCLUDED.as_of
		  WHERE EXCLUDED.as_of > settlement_accounting.operational_balance.as_of`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
		postedMinor,
		unsettledMinor,
		asOf.UTC(),
	)
	if err != nil {
		return ports.OperationalBalanceAdvanceInvalid, fmt.Errorf("advance operational balance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.OperationalBalanceNotAdvanced, nil
	}
	return ports.OperationalBalanceAdvanced, nil
}

func scopeFrom(legalEntity, account, currency string) (domain.SettlementScope, error) {
	entity, err := domain.NewLegalEntityReference(legalEntity)
	if err != nil {
		return domain.SettlementScope{}, err
	}
	accountID, err := domain.NewSettlementAccountID(account)
	if err != nil {
		return domain.SettlementScope{}, err
	}
	code, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.SettlementScope{}, err
	}
	return domain.NewSettlementScope(entity, accountID, code)
}
