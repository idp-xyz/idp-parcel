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

var (
	// ErrBalanceNotRegistered 是「这个结算作用域还没有登记过运营余额」。
	//
	// 它必须是错误而不是一份各项为零的余额：零余额看起来像「查过了、就是没钱」，
	// 而实际是没人回答过。编排把读取失败翻成`余额不可用`并保持未决，那正是这里
	// 该走到的格；交回零值则会让一次没答话的读取变成一次看起来执行过的控制。
	ErrBalanceNotRegistered = errors.New("settlement accounting postgres: 结算作用域未登记运营余额")

	// ErrCreditStandingNotRegistered 同理：额度未登记不等于额度为零，更不等于有额度。
	ErrCreditStandingNotRegistered = errors.New("settlement accounting postgres: 结算作用域未登记信用状况")
)

// PositionRecordOutcome 是余额与信用状况登记的落点封闭代数。它们是随外部事实推进的
// 状态而不是一次性配置，因此允许同键推进——但只准往前：迟到的快照不得静默盖掉更新的
// 事实（CONTEXT「账期快照不得被迟到费用静默覆盖」的同款纪律）。
type PositionRecordOutcome uint8

const (
	PositionRecordOutcomeInvalid PositionRecordOutcome = iota
	PositionRecorded
	PositionNotAdvanced
)

func (outcome PositionRecordOutcome) String() string {
	switch outcome {
	case PositionRecorded:
		return "RECORDED"
	case PositionNotAdvanced:
		return "NOT_ADVANCED"
	default:
		return ""
	}
}

// BalancePosting 是运营余额里由本上下文之外的事实决定的三项：入账余额来自 UC-SA-005
// 映射的真实收付，授信额度来自商业侧信用政策，已确认未结来自已出账未核销的应收。
//
// 冻结金额不在其中——那一项由冻结账本当场求和，不经登记面进来。
type BalancePosting struct {
	PostedMinor    int64
	CreditMinor    int64
	UnsettledMinor int64
	AsOf           time.Time
}

// CreditPosition 是信用状况里同样由外部事实决定的两项。已占用暴露不在其中，理由与
// 冻结金额一样。
type CreditPosition struct {
	LimitMinor int64
	Overdue    bool
	AsOf       time.Time
}

// OperationalBalances 实现 ports.OperationalBalanceView。
//
// 它读两处：登记表给出入账余额、授信额度与已确认未结，冻结账本给出冻结金额。冻结
// 那一项刻意不落在登记表里——`ledger.Freeze` 判可用余额时不自行扣减册内已有冻结，
// 全靠余额里的这一项，两处存两份只要差一次释放，同一笔钱就会被冻第二次。
type OperationalBalances struct {
	db      *bentopg.DB
	freezes *FreezeLedgers
}

func NewOperationalBalances(db *bentopg.DB, freezes *FreezeLedgers) (*OperationalBalances, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if freezes == nil {
		return nil, fmt.Errorf("settlement accounting postgres: freeze ledgers is nil")
	}
	return &OperationalBalances{db: db, freezes: freezes}, nil
}

var _ ports.OperationalBalanceView = (*OperationalBalances)(nil)

// LoadBalance 取一个结算作用域当前的运营结算余额。作用域四维全写在 SQL 条件里：
// 作用域不是过滤器而是身份的一部分，少一维就可能拿另一个作用域的钱来冻这一笔。
func (repository *OperationalBalances) LoadBalance(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
) (domain.OperationalBalance, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.OperationalBalance{}, fmt.Errorf("load operational balance: %w", err)
	}

	var posted, credit, unsettled int64
	err = querier.QueryRow(ctx,
		`SELECT posted_minor, credit_minor, unsettled_minor
		   FROM settlement_accounting.operational_balance
		  WHERE tenant_id = $1
		    AND legal_entity = $2
		    AND account_id = $3
		    AND currency = $4`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
	).Scan(&posted, &credit, &unsettled)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OperationalBalance{}, ErrBalanceNotRegistered
	}
	if err != nil {
		return domain.OperationalBalance{}, fmt.Errorf("load operational balance: %w", err)
	}

	ledger, err := repository.freezes.LoadForScope(ctx, tenant, scope)
	if err != nil {
		return domain.OperationalBalance{}, fmt.Errorf("load operational balance: %w", err)
	}

	balance, err := domain.NewOperationalBalance(scope, posted, credit, ledger.HeldMinor(), unsettled)
	if err != nil {
		return domain.OperationalBalance{}, fmt.Errorf("load operational balance: %w", err)
	}
	return balance, nil
}

// RecordBalance 推进一个结算作用域的余额登记。同键只准往前：`as_of` 不更新的写入
// 交回`未推进`而不是错误——迟到的快照到达是常态，不是故障。
func (repository *OperationalBalances) RecordBalance(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	posting BalancePosting,
) (PositionRecordOutcome, error) {
	if posting.AsOf.IsZero() {
		return PositionRecordOutcomeInvalid, errors.New(
			"record operational balance: 余额登记必须带截至时点，否则先后无从判断")
	}

	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return PositionRecordOutcomeInvalid, fmt.Errorf("record operational balance: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.operational_balance
			(tenant_id, legal_entity, account_id, currency,
			 posted_minor, credit_minor, unsettled_minor, as_of)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT ON CONSTRAINT operational_balance_pkey DO UPDATE
		    SET posted_minor = EXCLUDED.posted_minor,
		        credit_minor = EXCLUDED.credit_minor,
		        unsettled_minor = EXCLUDED.unsettled_minor,
		        as_of = EXCLUDED.as_of
		  WHERE EXCLUDED.as_of > settlement_accounting.operational_balance.as_of`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
		posting.PostedMinor,
		posting.CreditMinor,
		posting.UnsettledMinor,
		posting.AsOf.UTC(),
	)
	if err != nil {
		return PositionRecordOutcomeInvalid, fmt.Errorf("record operational balance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return PositionNotAdvanced, nil
	}
	return PositionRecorded, nil
}

// CreditStandings 实现 ports.CreditStandingView。
//
// 与运营余额分立到底：另一张表、另一个类型、另一本账（ADR-0047）。预付冻结不与同一
// 客户账期范围共用余额与额度，共用实现会让两本账在这里先合流，而那正是 SET-03 要拦的。
type CreditStandings struct {
	db        *bentopg.DB
	exposures *CreditExposureLedgers
}

func NewCreditStandings(db *bentopg.DB, exposures *CreditExposureLedgers) (*CreditStandings, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if exposures == nil {
		return nil, fmt.Errorf("settlement accounting postgres: credit exposure ledgers is nil")
	}
	return &CreditStandings{db: db, exposures: exposures}, nil
}

var _ ports.CreditStandingView = (*CreditStandings)(nil)

// LoadCreditStanding 取一个账期作用域当前的信用状况：额度与逾期来自登记，已占用
// 暴露由暴露账本当场求和。
func (repository *CreditStandings) LoadCreditStanding(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
) (domain.CreditStanding, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CreditStanding{}, fmt.Errorf("load credit standing: %w", err)
	}

	var limit int64
	var overdue bool
	err = querier.QueryRow(ctx,
		`SELECT limit_minor, overdue
		   FROM settlement_accounting.credit_standing
		  WHERE tenant_id = $1
		    AND legal_entity = $2
		    AND account_id = $3
		    AND currency = $4`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
	).Scan(&limit, &overdue)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CreditStanding{}, ErrCreditStandingNotRegistered
	}
	if err != nil {
		return domain.CreditStanding{}, fmt.Errorf("load credit standing: %w", err)
	}

	ledger, err := repository.exposures.LoadForScope(ctx, tenant, scope)
	if err != nil {
		return domain.CreditStanding{}, fmt.Errorf("load credit standing: %w", err)
	}

	standing, err := domain.NewCreditStanding(scope, limit, ledger.ExposedMinor(), overdue)
	if err != nil {
		return domain.CreditStanding{}, fmt.Errorf("load credit standing: %w", err)
	}
	return standing, nil
}

// RecordStanding 推进一个账期作用域的信用状况登记。只准往前，理由同 RecordBalance。
func (repository *CreditStandings) RecordStanding(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	position CreditPosition,
) (PositionRecordOutcome, error) {
	if position.AsOf.IsZero() {
		return PositionRecordOutcomeInvalid, errors.New(
			"record credit standing: 信用状况登记必须带截至时点，否则先后无从判断")
	}

	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return PositionRecordOutcomeInvalid, fmt.Errorf("record credit standing: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.credit_standing
			(tenant_id, legal_entity, account_id, currency, limit_minor, overdue, as_of)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT ON CONSTRAINT credit_standing_pkey DO UPDATE
		    SET limit_minor = EXCLUDED.limit_minor,
		        overdue = EXCLUDED.overdue,
		        as_of = EXCLUDED.as_of
		  WHERE EXCLUDED.as_of > settlement_accounting.credit_standing.as_of`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
		position.LimitMinor,
		position.Overdue,
		position.AsOf.UTC(),
	)
	if err != nil {
		return PositionRecordOutcomeInvalid, fmt.Errorf("record credit standing: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return PositionNotAdvanced, nil
	}
	return PositionRecorded, nil
}
