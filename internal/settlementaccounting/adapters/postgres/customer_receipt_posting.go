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
	_ ports.CreditStandingAdvance     = (*CustomerReceiptPostings)(nil)
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

func (repository *CustomerReceiptPostings) ReceivableAccountScope(
	ctx context.Context,
	tenant domain.TenantID,
	account string,
) (domain.SettlementScope, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SettlementScope{}, false, fmt.Errorf("receivable account scope: %w", err)
	}

	var legalEntity, accountID, currency string
	err = querier.QueryRow(ctx,
		`SELECT legal_entity_id, account_id, currency
		   FROM settlement_accounting.settlement_account
		  WHERE tenant_id = $1
		    AND account_id = $2
		    AND direction = 'RECEIVABLE'`,
		tenant.String(),
		account,
	).Scan(&legalEntity, &accountID, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SettlementScope{}, false, nil
	}
	if err != nil {
		return domain.SettlementScope{}, false, fmt.Errorf("receivable account scope: %w", err)
	}

	scope, err := scopeFrom(legalEntity, accountID, currency)
	if err != nil {
		return domain.SettlementScope{}, false, fmt.Errorf("receivable account scope: %w", err)
	}
	return scope, true, nil
}

// mappingReceivableScopesSQL 把租户 $1 的每条资金映射收成它落到的客户应收作用域（法人、账户、币种）：对账单映射经那张
// 对账单落到它的应收账户上（单与账户同币种），账户映射落到那个应收账户本身；收不成的（应付、贷项、金额本体两格，或
// 账户不是应收）不出行。CustomerStatementScope 与 ReceivableAccountScope 收单个目标用同一组条件。一条事实落在几个作用域
// 上、一笔分配指向哪个作用域（分配沿它的映射走，目标与映射一致）、读面数作用域、编排取要重算的作用域都从这一处取，
// 口径不分叉。账户在租户内唯一，账户一定作用域就定。
const mappingReceivableScopesSQL = `
	SELECT m.mapping_id, m.fact_id, a.legal_entity_id, a.account_id, a.currency
	  FROM settlement_accounting.funds_mapping AS m
	  JOIN settlement_accounting.customer_statement AS s
	    ON s.tenant_id = m.tenant_id
	   AND s.statement_number = m.target_ref
	  JOIN settlement_accounting.settlement_account AS a
	    ON a.tenant_id = s.tenant_id
	   AND a.account_id = s.account_id
	 WHERE m.tenant_id = $1
	   AND m.target_kind = 'STATEMENT'
	   AND a.direction = 'RECEIVABLE'
	   AND s.currency = a.currency
	UNION ALL
	SELECT m.mapping_id, m.fact_id, a.legal_entity_id, a.account_id, a.currency
	  FROM settlement_accounting.funds_mapping AS m
	  JOIN settlement_accounting.settlement_account AS a
	    ON a.tenant_id = m.tenant_id
	   AND a.account_id = m.target_ref
	 WHERE m.tenant_id = $1
	   AND m.target_kind = 'ACCOUNT'
	   AND a.direction = 'RECEIVABLE'`

// FactCustomerScopes 交回一条事实经对账单或账户映射落到的客户应收作用域。币种与事实不同的作用域也算一个：它从这条
// 事实上什么都记不到，可映射到它本身就说明这笔钱该归谁还没分清（ADR-0283），不能因此让另一个作用域按全额放行。
// 次序不作数，编排自己按键排。
func (repository *CustomerReceiptPostings) FactCustomerScopes(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.FundsFactReference,
) ([]domain.SettlementScope, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("fact customer scopes: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT mapped.legal_entity_id, mapped.account_id, mapped.currency
		   FROM (`+mappingReceivableScopesSQL+`) AS mapped
		  WHERE mapped.fact_id = $2`,
		tenant.String(),
		fact.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("fact customer scopes: %w", err)
	}
	defer rows.Close()
	var scopes []domain.SettlementScope
	for rows.Next() {
		var legalEntity, account, currency string
		if err := rows.Scan(&legalEntity, &account, &currency); err != nil {
			return nil, fmt.Errorf("fact customer scopes: %w", err)
		}
		scope, err := scopeFrom(legalEntity, account, currency)
		if err != nil {
			return nil, fmt.Errorf("fact customer scopes: %w", err)
		}
		scopes = append(scopes, scope)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fact customer scopes: %w", err)
	}
	return scopes, nil
}

// CustomerDirectionNets 的 P 逐条事实记一次：每个作用域取这条事实在它身上可能的最小记入（ADR-0283），唯一的推定是
// 一条收款只落到一个客户应收作用域时，没核销的部分归它。记作 n 为这条事实落到的客户应收作用域个数、d 为指向本作用域
// 的未撤销核销分配净额（分配沿的映射落在本作用域上，对账单分配与账户分配都算）、r 为事实金额减它全部未撤销核销的
// 合计（含核到应付、贷项、金额本体的，不低于 0）：
//
//   - 收款记 d，n 为 1 时再加 r。只有客户映射、只落到本作用域时这就是全额；核到别处的那部分离开，没分清归谁的那部分
//     不计入任何作用域——每个作用域都按全额记，钱只到了一次却在几处各当一次用。
//   - 退回扣 d + r，不论 n：钱离开了，没分清从谁那里离开之前每个作用域都按可能是自己扣。
//   - 一个作用域从一条事实上记到或扣掉的都不超过事实金额，超核（外部更正把金额改到已核销合计以下）时也是。
//
// 一条事实映到本作用域几个目标只算一次；币种与作用域不同的事实不进。W 只加总指向本作用域对账单、尚未撤销的核销分配。
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
		`WITH mapped AS (`+mappingReceivableScopesSQL+`),
		      here AS (
		          SELECT DISTINCT mapped.fact_id
		            FROM mapped
		           WHERE mapped.legal_entity_id = $2
		             AND mapped.account_id = $3
		             AND mapped.currency = $4),
		      spread AS (
		          SELECT mapped.fact_id, COUNT(DISTINCT mapped.account_id) AS scopes
		            FROM mapped
		            JOIN here ON here.fact_id = mapped.fact_id
		           GROUP BY mapped.fact_id),
		      head AS (
		          SELECT v.fact_id,
		                 CASE v.kind
		                     WHEN 'RECEIPT_CONFIRMED' THEN 1
		                     WHEN 'FUNDS_RETURNED' THEN -1
		                     ELSE 0
		                 END AS sign,
		                 v.amount_minor
		            FROM settlement_accounting.external_funds_fact_version AS v
		            JOIN here ON here.fact_id = v.fact_id
		           WHERE v.tenant_id = $1
		             AND v.currency = $4
		             AND NOT EXISTS (
		                 SELECT 1
		                   FROM settlement_accounting.external_funds_fact_version AS newer
		                  WHERE newer.tenant_id = v.tenant_id
		                    AND newer.fact_id = v.fact_id
		                    AND newer.corrects = v.version)),
		      applied AS (
		          SELECT app.fact_id, SUM(app.applied_minor) AS applied_minor
		            FROM settlement_accounting.settlement_application AS app
		            JOIN here ON here.fact_id = app.fact_id
		           WHERE app.tenant_id = $1
		             AND app.reversal_basis IS NULL
		           GROUP BY app.fact_id),
		      directed AS (
		          SELECT app.fact_id,
		                 SUM(
		                     CASE elem->>'direction'
		                         WHEN 'DEBIT' THEN (elem->>'amountMinor')::bigint
		                         WHEN 'CREDIT' THEN -(elem->>'amountMinor')::bigint
		                         ELSE 0
		                     END) AS net
		            FROM settlement_accounting.settlement_application AS app
		            JOIN here ON here.fact_id = app.fact_id
		           CROSS JOIN LATERAL jsonb_array_elements(app.allocations) AS elem
		            JOIN mapped
		              ON mapped.mapping_id = elem->>'mapping'
		             AND mapped.fact_id = app.fact_id
		           WHERE app.tenant_id = $1
		             AND app.reversal_basis IS NULL
		             AND mapped.legal_entity_id = $2
		             AND mapped.account_id = $3
		             AND mapped.currency = $4
		           GROUP BY app.fact_id)
		SELECT
		    COALESCE((
		        SELECT SUM(
		            head.sign * LEAST(head.amount_minor,
		                COALESCE(directed.net, 0)
		                + CASE
		                      WHEN head.sign < 0 OR spread.scopes = 1
		                          THEN GREATEST(head.amount_minor - COALESCE(applied.applied_minor, 0), 0)
		                      ELSE 0
		                  END))
		          FROM head
		          JOIN spread ON spread.fact_id = head.fact_id
		          LEFT JOIN applied ON applied.fact_id = head.fact_id
		          LEFT JOIN directed ON directed.fact_id = head.fact_id
		    ), 0)::bigint,
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

// LockOperationalBalanceScope 取（租户，结算作用域）的事务级咨询锁。不锁 operational_balance 行：作用域第一次重算时还没有
// 行可锁。锁空间与接受前控制的作用域锁分开——那一路只读余额、不重算，与重算排在同一把锁上只会多等，不多护什么。
// 键照本仓其余事务级锁的双 int 形式；撞上时两个不相干的作用域共用一把锁，一笔事务要取几个作用域时这会打乱键序，
// 后果见 ports.FundsFactApplicationLock 头注。
func (repository *CustomerReceiptPostings) LockOperationalBalanceScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock operational balance scope: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.operational_balance.scope'),
		        hashtext($1 || chr(31) || $2 || chr(31) || $3 || chr(31) || $4))`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
	); err != nil {
		return fmt.Errorf("lock operational balance scope: %w", err)
	}
	return nil
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

func (repository *CustomerReceiptPostings) ConfirmedChargeAging(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	asOf time.Time,
) (ports.ChargeDuePicture, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ChargeDuePicture{}, fmt.Errorf("confirmed charge aging: %w", err)
	}

	var picture ports.ChargeDuePicture
	var missing int64
	err = querier.QueryRow(ctx,
		`SELECT COUNT(*)::bigint,
		        COUNT(*) FILTER (WHERE due_date IS NULL)::bigint,
		        COALESCE(BOOL_OR(
		            due_date IS NOT NULL
		            AND (due_date::timestamp AT TIME ZONE 'UTC') < $5
		        ), false)
		   FROM settlement_accounting.customer_charge
		  WHERE tenant_id = $1
		    AND stage = 'CONFIRMED'
		    AND charge_direction = 'RECEIVABLE'
		    AND responsible_entity = $2
		    AND settlement_account_id = $3
		    AND settlement_currency = $4`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
		asOf.UTC(),
	).Scan(&picture.ConfirmedCount, &missing, &picture.Overdue)
	if err != nil {
		return ports.ChargeDuePicture{}, fmt.Errorf("confirmed charge aging: %w", err)
	}
	picture.MissingDueDate = missing > 0
	return picture, nil
}

// AdvanceCreditStanding 插入时登记额度列写 0，冲突更新不碰该列。
// 同键只准往前：截至时点不更新则交回未推进。
func (repository *CustomerReceiptPostings) AdvanceCreditStanding(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	overdue bool,
	asOf time.Time,
) (ports.CreditStandingAdvanceOutcome, error) {
	if asOf.IsZero() {
		return ports.CreditStandingAdvanceInvalid, errors.New(
			"advance credit standing: 信用状况登记必须带截至时点，否则先后无从判断")
	}

	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CreditStandingAdvanceInvalid, fmt.Errorf("advance credit standing: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.credit_standing
			(tenant_id, legal_entity, account_id, currency, limit_minor, overdue, as_of)
		 VALUES ($1, $2, $3, $4, 0, $5, $6)
		 ON CONFLICT ON CONSTRAINT credit_standing_pkey DO UPDATE
		    SET overdue = EXCLUDED.overdue,
		        as_of = EXCLUDED.as_of
		  WHERE EXCLUDED.as_of > settlement_accounting.credit_standing.as_of`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
		overdue,
		asOf.UTC(),
	)
	if err != nil {
		return ports.CreditStandingAdvanceInvalid, fmt.Errorf("advance credit standing: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CreditStandingNotAdvanced, nil
	}
	return ports.CreditStandingAdvanced, nil
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
