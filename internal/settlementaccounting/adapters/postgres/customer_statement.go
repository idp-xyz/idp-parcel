package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// CustomerStatements 实现 ports.PublishedStatementStore。主键=（租户+单号），单号是
// 寻址键；发布意图（账户、账期、截止版本）另有一道唯一，同一意图恰一张单（ADR-0355
// 决定九）。撞哪一道都答`已有记录`（ON CONFLICT 代数，ADR-0031）。
//
// 发布密封落在写路径的形状上：内容列（行、总额、账户、账期）只在 Save 的 INSERT
// 出现，Replace 的 UPDATE 语句根本不含它们——作废只写留痕两列，替代单用新单号
// （AT-SA-069）。
type CustomerStatements struct {
	db *bentopg.DB
}

func NewCustomerStatements(db *bentopg.DB) (*CustomerStatements, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &CustomerStatements{db: db}, nil
}

var (
	_ ports.PublishedStatementStore = (*CustomerStatements)(nil)
	_ ports.ChargeBillingLock       = (*CustomerStatements)(nil)
)

// LockChargeBilling 先以 FOR UPDATE 锁住费用行，再查它在不在一张未作废的已发布对账单上。锁住的是费用行而不是
// 对账单：要挡的是两笔截单并发各自读到「未出账」，它们共同碰到的只有费用行。读已提交隔离下，等锁之后的
// 那条查询拿新快照，读得到先到者已提交的单。必须在事务里：锁到提交才放，离开事务就只是一次读。
func (repository *CustomerStatements) LockChargeBilling(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
) (domain.StatementNumber, bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return domain.StatementNumber{}, false, fmt.Errorf("lock charge billing: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT 1
		   FROM settlement_accounting.customer_charge
		  WHERE tenant_id = $1
		    AND charge_id = $2
		    FOR UPDATE`,
		tenant.String(), charge.String()); err != nil {
		return domain.StatementNumber{}, false, fmt.Errorf("lock charge billing: %w", err)
	}
	var number string
	err = executor.QueryRow(ctx,
		`SELECT statement_number
		   FROM settlement_accounting.customer_statement
		  WHERE tenant_id = $1
		    AND voided_at IS NULL
		    AND lines @> jsonb_build_array(jsonb_build_object('charge', $2::text))
		  ORDER BY published_at, statement_number
		  LIMIT 1`,
		tenant.String(), charge.String()).Scan(&number)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StatementNumber{}, false, nil
	}
	if err != nil {
		return domain.StatementNumber{}, false, fmt.Errorf("lock charge billing: %w", err)
	}
	billed, err := domain.NewStatementNumber(number)
	if err != nil {
		return domain.StatementNumber{}, false, fmt.Errorf("lock charge billing: %w", err)
	}
	return billed, true, nil
}

// statementLineRow 与 statementAdjustmentRow 是两列 jsonb 的行模型。方向存封闭字串，
// 读回经 adjustmentDirectionFrom 译码。
type statementLineRow struct {
	Charge      string `json:"charge"`
	AmountMinor int64  `json:"amount_minor"`
}

type statementAdjustmentRow struct {
	Adjustment  string `json:"adjustment"`
	Charge      string `json:"charge"`
	Direction   string `json:"direction"`
	AmountMinor int64  `json:"amount_minor"`
}

const (
	directionDebitRow  = "DEBIT"
	directionCreditRow = "CREDIT"
)

// statementRowColumns 是读回一张对账单的列，FindByKey、FindByIntent 与 FindUnversionedInPeriod 共用，读法只有一份。
const statementRowColumns = `statement_number, account_id, period_ref, currency, total_minor,
		        content_digest, lines, adjustment_lines,
		        published_at, void_basis, voided_at, recorded_at, draft_version`

// FindByKey 按（租户+单号）取回已发布对账单。否定结果只回 false，不区分「不存在」
// 与「属于另一个租户」。读回经 RehydratePublishedStatement 复验勾稽与作废留痕
// ——存进去时勾稽过的数字读出来不平，在这里响亮暴露。
func (repository *CustomerStatements) FindByKey(
	ctx context.Context,
	key ports.StatementKey,
) (ports.StatementRecord, bool, error) {
	records, err := repository.findStatements(ctx, key.TenantID,
		`WHERE tenant_id = $1 AND statement_number = $2`, key.TenantID.String(), key.Number.String())
	if err != nil || len(records) == 0 {
		return ports.StatementRecord{}, false, err
	}
	return records[0], true, nil
}

// LockPublicationIntent 以事务级咨询锁把同一发布意图的发布排成一队（见 ports.PublishedStatementStore）。意图还没有行
// 可锁——要挡的正是两笔都还没写下行的时候，所以锁在意图本身上，不锁任何表行。
func (repository *CustomerStatements) LockPublicationIntent(
	ctx context.Context,
	intent ports.StatementPublicationIntent,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock statement publication intent: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.customer_statement.publication_intent'),
		                              hashtext($1 || chr(31) || $2 || chr(31) || $3 || chr(31) || $4))`,
		intent.TenantID.String(), intent.Account.String(), intent.Period.String(), intent.Version.String()); err != nil {
		return fmt.Errorf("lock statement publication intent: %w", err)
	}
	return nil
}

// FindByIntent 按发布意图取回那一张单。
func (repository *CustomerStatements) FindByIntent(
	ctx context.Context,
	intent ports.StatementPublicationIntent,
) (ports.StatementRecord, bool, error) {
	records, err := repository.findStatements(ctx, intent.TenantID,
		`WHERE tenant_id = $1 AND account_id = $2 AND period_ref = $3 AND draft_version = $4`,
		intent.TenantID.String(), intent.Account.String(), intent.Period.String(), intent.Version.String())
	if err != nil || len(records) == 0 {
		return ports.StatementRecord{}, false, err
	}
	return records[0], true, nil
}

// FindUnversionedInPeriod 取回本决定之前手工发布、没记下截止版本的同账户同账期各单，按单号排。
func (repository *CustomerStatements) FindUnversionedInPeriod(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
	period domain.BillingPeriodReference,
) ([]ports.StatementRecord, error) {
	return repository.findStatements(ctx, tenant,
		`WHERE tenant_id = $1 AND account_id = $2 AND period_ref = $3 AND draft_version IS NULL
		 ORDER BY statement_number`,
		tenant.String(), account.String(), period.String())
}

// StatementNumberUsed 答一个单号是否已在本租户的对账单册上（作废单的号也算已用）。读执行器的理由同 documentNumberUsed。
func (repository *CustomerStatements) StatementNumberUsed(
	ctx context.Context,
	tenant domain.TenantID,
	number string,
) (bool, error) {
	executor, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("statement number used: %w", err)
	}
	var used bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM settlement_accounting.customer_statement
		                 WHERE tenant_id = $1 AND statement_number = $2)`,
		tenant.String(), number).Scan(&used); err != nil {
		return false, fmt.Errorf("statement number used: %w", err)
	}
	return used, nil
}

func (repository *CustomerStatements) findStatements(
	ctx context.Context,
	tenant domain.TenantID,
	where string,
	args ...any,
) ([]ports.StatementRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find statement: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+statementRowColumns+`
		   FROM settlement_accounting.customer_statement `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("find statement: %w", err)
	}
	defer rows.Close()

	var records []ports.StatementRecord
	for rows.Next() {
		var (
			number, account, period, currency string
			totalMinor                        int64
			digest                            string
			linesJSON, adjustsJSON            []byte
			publishedAt, recordedAt           time.Time
			voidBasis, draftVersion           *string
			voidedAt                          *time.Time
		)
		if err := rows.Scan(&number, &account, &period, &currency, &totalMinor,
			&digest, &linesJSON, &adjustsJSON,
			&publishedAt, &voidBasis, &voidedAt, &recordedAt, &draftVersion); err != nil {
			return nil, fmt.Errorf("find statement: %w", err)
		}
		statementNumber, err := domain.NewStatementNumber(number)
		if err != nil {
			return nil, fmt.Errorf("find statement: %w", err)
		}
		key := ports.StatementKey{TenantID: tenant, Number: statementNumber}
		statement, err := rebuildStatement(key, statementColumns{
			account:     account,
			period:      period,
			currency:    currency,
			totalMinor:  totalMinor,
			publishedAt: publishedAt,
			voidBasis:   voidBasis,
			voidedAt:    voidedAt,
		}, linesJSON, adjustsJSON)
		if err != nil {
			return nil, fmt.Errorf("find statement: %w", err)
		}
		record := ports.StatementRecord{
			Key:           key,
			ContentDigest: digest,
			Statement:     statement,
			RecordedAt:    recordedAt.UTC(),
		}
		if draftVersion != nil {
			if record.DraftVersion, err = domain.NewStatementDraftVersion(*draftVersion); err != nil {
				return nil, fmt.Errorf("find statement: %w", err)
			}
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find statement: %w", err)
	}
	return records, nil
}

// Save 写下一次发布。同（租户+单号）或同一发布意图已有记录时答`已发布`——业务答案
// 不是错误（ADR-0031）。新写的单必须带截止版本：意图唯一只管带了版本的行，漏带的一张
// 会躲过同一意图的查重。
func (repository *CustomerStatements) Save(
	ctx context.Context,
	record ports.StatementRecord,
) (ports.StatementSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.StatementSaveOutcomeInvalid, fmt.Errorf("save statement: %w", err)
	}

	statement := record.Statement
	if statement.Number() != record.Key.Number {
		return ports.StatementSaveOutcomeInvalid, fmt.Errorf(
			"save statement: record key disagrees with the statement's number")
	}
	if record.DraftVersion.String() == "" {
		return ports.StatementSaveOutcomeInvalid, fmt.Errorf(
			"save statement: a new statement must carry the draft version of its publication intent")
	}
	linesJSON, adjustsJSON, err := marshalStatementLines(statement)
	if err != nil {
		return ports.StatementSaveOutcomeInvalid, fmt.Errorf("save statement: %w", err)
	}

	var (
		voidBasis *string
		voidedAt  *time.Time
	)
	if basis, at, voided := statement.Voided(); voided {
		value := basis.String()
		utc := at.UTC()
		voidBasis, voidedAt = &value, &utc
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_statement
			(tenant_id, statement_number, account_id, period_ref, currency,
			 total_minor, content_digest, lines, adjustment_lines,
			 published_at, void_basis, voided_at, recorded_at, draft_version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Number.String(),
		statement.Account().String(),
		statement.Period().String(),
		statement.Currency().String(),
		statement.TotalMinor(),
		record.ContentDigest,
		linesJSON,
		adjustsJSON,
		statement.PublishedAt(),
		voidBasis,
		voidedAt,
		record.RecordedAt.UTC(),
		record.DraftVersion.String(),
	)
	if err != nil {
		return ports.StatementSaveOutcomeInvalid, fmt.Errorf("save statement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.StatementAlreadyPublished, nil
	}
	return ports.StatementSaved, nil
}

// Replace 只承担作废留痕：UPDATE 语句只写 void_basis/voided_at/recorded_at 三列，
// 内容列不在语句里——发布后的行、总额与账期在物理上改不动。WHERE 带 voided_at IS
// NULL：并发第二废与单号不存在同样答 false（不二废），由编排折成未受理。
func (repository *CustomerStatements) Replace(
	ctx context.Context,
	record ports.StatementRecord,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("replace statement: %w", err)
	}

	basis, voidedAt, voided := record.Statement.Voided()
	if !voided {
		// Replace 的唯一业务语义是作废（PublishedStatement 上没有别的可变痕）；
		// 拿一份未作废的快照来换值是编排缺陷，响亮报错不落库。
		return false, fmt.Errorf("replace statement: only a voided statement may replace the published row")
	}

	tag, err := executor.Exec(ctx,
		`UPDATE settlement_accounting.customer_statement
		    SET void_basis = $3, voided_at = $4, recorded_at = $5
		  WHERE tenant_id = $1
		    AND statement_number = $2
		    AND voided_at IS NULL`,
		record.Key.TenantID.String(),
		record.Key.Number.String(),
		basis.String(),
		voidedAt.UTC(),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("replace statement: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func marshalStatementLines(statement domain.PublishedStatement) ([]byte, []byte, error) {
	lines := statement.Lines()
	lineRows := make([]statementLineRow, 0, len(lines))
	for _, line := range lines {
		lineRows = append(lineRows, statementLineRow{
			Charge:      line.Charge.String(),
			AmountMinor: line.AmountMinor,
		})
	}
	adjustments := statement.AdjustmentLines()
	adjustmentRows := make([]statementAdjustmentRow, 0, len(adjustments))
	for _, adjustment := range adjustments {
		direction := directionDebitRow
		if adjustment.Direction == domain.AdjustmentCredit {
			direction = directionCreditRow
		}
		adjustmentRows = append(adjustmentRows, statementAdjustmentRow{
			Adjustment:  adjustment.Adjustment.String(),
			Charge:      adjustment.Charge.String(),
			Direction:   direction,
			AmountMinor: adjustment.AmountMinor,
		})
	}
	linesJSON, err := json.Marshal(lineRows)
	if err != nil {
		return nil, nil, err
	}
	adjustsJSON, err := json.Marshal(adjustmentRows)
	if err != nil {
		return nil, nil, err
	}
	return linesJSON, adjustsJSON, nil
}

type statementColumns struct {
	account     string
	period      string
	currency    string
	totalMinor  int64
	publishedAt time.Time
	voidBasis   *string
	voidedAt    *time.Time
}

func rebuildStatement(
	key ports.StatementKey,
	columns statementColumns,
	linesJSON, adjustsJSON []byte,
) (domain.PublishedStatement, error) {
	account, err := domain.NewSettlementAccountID(columns.account)
	if err != nil {
		return domain.PublishedStatement{}, err
	}
	period, err := domain.NewBillingPeriodReference(columns.period)
	if err != nil {
		return domain.PublishedStatement{}, err
	}
	currency, err := domain.NewCurrencyCode(columns.currency)
	if err != nil {
		return domain.PublishedStatement{}, err
	}

	var lineRows []statementLineRow
	if err := json.Unmarshal(linesJSON, &lineRows); err != nil {
		return domain.PublishedStatement{}, err
	}
	lines := make([]domain.StatementLine, 0, len(lineRows))
	for _, row := range lineRows {
		charge, err := domain.NewCustomerChargeID(row.Charge)
		if err != nil {
			return domain.PublishedStatement{}, err
		}
		lines = append(lines, domain.StatementLine{Charge: charge, AmountMinor: row.AmountMinor})
	}

	var adjustmentRows []statementAdjustmentRow
	if err := json.Unmarshal(adjustsJSON, &adjustmentRows); err != nil {
		return domain.PublishedStatement{}, err
	}
	adjustments := make([]domain.StatementAdjustmentLine, 0, len(adjustmentRows))
	for _, row := range adjustmentRows {
		adjustment, err := domain.NewChargeAdjustmentID(row.Adjustment)
		if err != nil {
			return domain.PublishedStatement{}, err
		}
		charge, err := domain.NewCustomerChargeID(row.Charge)
		if err != nil {
			return domain.PublishedStatement{}, err
		}
		direction, err := adjustmentDirectionFrom(row.Direction)
		if err != nil {
			return domain.PublishedStatement{}, err
		}
		adjustments = append(adjustments, domain.StatementAdjustmentLine{
			Adjustment:  adjustment,
			Charge:      charge,
			Direction:   direction,
			AmountMinor: row.AmountMinor,
		})
	}

	spec := domain.RehydratePublishedStatementSpec{
		Number:      key.Number,
		Account:     account,
		Period:      period,
		Currency:    currency,
		Lines:       lines,
		Adjustments: adjustments,
		TotalMinor:  columns.totalMinor,
		PublishedAt: columns.publishedAt,
	}
	if columns.voidBasis != nil {
		if spec.VoidBasis, err = domain.NewStatementVoidBasisReference(*columns.voidBasis); err != nil {
			return domain.PublishedStatement{}, err
		}
	}
	if columns.voidedAt != nil {
		spec.VoidedAt = *columns.voidedAt
	}
	return domain.RehydratePublishedStatement(spec)
}

func adjustmentDirectionFrom(raw string) (domain.AdjustmentDirection, error) {
	switch raw {
	case directionDebitRow:
		return domain.AdjustmentDebit, nil
	case directionCreditRow:
		return domain.AdjustmentCredit, nil
	default:
		return domain.AdjustmentDirectionInvalid, fmt.Errorf("unknown adjustment direction %q", raw)
	}
}
