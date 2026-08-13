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

// CustomerStatements 实现 ports.PublishedStatementStore。主键=（租户+单号）：同一
// 发布意图恰一个单号，撞键即`已有记录`（ON CONFLICT 代数，ADR-0031）。
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

// FindByKey 按（租户+单号）取回已发布对账单。否定结果只回 false，不区分「不存在」
// 与「属于另一个租户」。读回经 RehydratePublishedStatement 复验勾稽与作废留痕
// ——存进去时勾稽过的数字读出来不平，在这里响亮暴露。
func (repository *CustomerStatements) FindByKey(
	ctx context.Context,
	key ports.StatementKey,
) (ports.StatementRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.StatementRecord{}, false, fmt.Errorf("find statement: %w", err)
	}

	var (
		account, period, currency string
		totalMinor                int64
		digest                    string
		linesJSON, adjustsJSON    []byte
		publishedAt, recordedAt   time.Time
		voidBasis                 *string
		voidedAt                  *time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT account_id, period_ref, currency, total_minor,
		        content_digest, lines, adjustment_lines,
		        published_at, void_basis, voided_at, recorded_at
		   FROM settlement_accounting.customer_statement
		  WHERE tenant_id = $1
		    AND statement_number = $2`,
		key.TenantID.String(),
		key.Number.String(),
	).Scan(&account, &period, &currency, &totalMinor,
		&digest, &linesJSON, &adjustsJSON,
		&publishedAt, &voidBasis, &voidedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.StatementRecord{}, false, nil
	}
	if err != nil {
		return ports.StatementRecord{}, false, fmt.Errorf("find statement: %w", err)
	}

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
		return ports.StatementRecord{}, false, fmt.Errorf("find statement: %w", err)
	}

	return ports.StatementRecord{
		Key:           key,
		ContentDigest: digest,
		Statement:     statement,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次发布。同（租户+单号）已有记录时答`已发布`——业务答案不是错误
// （ADR-0031），编排据此读回赢家、按 content_digest 分重放与同号异文冲突。
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
			 published_at, void_basis, voided_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
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
