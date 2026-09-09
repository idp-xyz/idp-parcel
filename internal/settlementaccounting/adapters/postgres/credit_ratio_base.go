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

// CreditRatioBases 实现 ports.CreditRatioBaseView：比例额度声明的基数在本上下文账本里的当前取值
// （ADR-0129 决定二的取数路径，逐格一段 SQL）。
//
// 它读的是本上下文自己写下的两张表，不新建任何表也不建第二份数：入账余额就是 operational_balance
// 登记面上的 posted_minor，上一结算周期已确认费用合计就是 customer_statement 最近一张已发布对账单的
// 费用行之和——在这里另存一份「基数」列，只要有一次登记没同步过来，同一个客户就会被按两个分母授信。
//
// 三格照 ports.CreditRatioBaseView：有事实交数（可为负，折不折由领域说）；尚无事实答 found=false
// 而不是 0——一个为 0 的基数看起来像「查过了、就是没钱」，而实际是没人回答过；读不回 error。
type CreditRatioBases struct {
	db *bentopg.DB
}

func NewCreditRatioBases(db *bentopg.DB) (*CreditRatioBases, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &CreditRatioBases{db: db}, nil
}

var _ ports.CreditRatioBaseView = (*CreditRatioBases)(nil)

// LoadCreditRatioBase 按声明的基数选取数路径。集外基数上抛：封闭集在领域，适配器不替新成员发明一条
// 取数路径——加一格成员就得在这里加一支 case（ADR-0025 全函数）。
func (repository *CreditRatioBases) LoadCreditRatioBase(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	base domain.CreditRatioBase,
) (int64, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("load credit ratio base: %w", err)
	}
	switch base {
	case domain.PostedBalanceBase:
		return postedBalanceBase(ctx, querier, tenant, scope)
	case domain.PriorPeriodConfirmedChargesBase:
		return priorPeriodConfirmedChargesBase(ctx, querier, tenant, scope)
	default:
		return 0, false, fmt.Errorf("load credit ratio base: %w: base %d has no data path", domain.ErrInvalidCreditBasis, uint8(base))
	}
}

// postedBalanceBase 读运营余额登记面的入账余额。作用域四维全写在条件里（同 OperationalBalances.LoadBalance）；
// 无行 = 该作用域未登记运营余额 = 尚无事实。不经 LoadBalance 取整份余额：那一口把「未登记」与「读不回」
// 折在同一个 error 里，而本口的三格要把两者分开。
func postedBalanceBase(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	scope domain.SettlementScope,
) (int64, bool, error) {
	var posted int64
	err := querier.QueryRow(ctx,
		`SELECT posted_minor
		   FROM settlement_accounting.operational_balance
		  WHERE tenant_id = $1
		    AND legal_entity = $2
		    AND account_id = $3
		    AND currency = $4`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
	).Scan(&posted)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("load credit ratio base (posted balance): %w", err)
	}
	return posted, true, nil
}

// priorPeriodConfirmedChargesBase 取本账户最近一张已发布对账单的费用行之和。
//
// 「上一结算周期」取自对账单快照而不另算周期边界：本上下文不拥有周期定义（结算归属日与周期由费用类型和
// 合同规则决定），截单留下的快照是它自己手里唯一的周期记录（ADR-0129 决定二）。费用行是那个周期截下来的
// 已确认费用（对账单只纳已确认费用，UC-SA-003）；调整行是调整不是费用，不计。对账单按（账户、币种）键入
// （CONTEXT「针对一个结算账户和结算周期发布」），责任法人由账户蕴含。
//
// 最近一张按 published_at 取——含已作废的：最近一张已作废且尚无替代时，上一周期没有有效快照，答尚无事实；
// 若只在未作废里取最近，会静默退回更早一个周期的数当分母。替代单发布在作废之后，published_at 更晚，自然
// 成为最近一张。
func priorPeriodConfirmedChargesBase(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	scope domain.SettlementScope,
) (int64, bool, error) {
	var linesJSON []byte
	var voidedAt *time.Time
	err := querier.QueryRow(ctx,
		`SELECT lines, voided_at
		   FROM settlement_accounting.customer_statement
		  WHERE tenant_id = $1
		    AND account_id = $2
		    AND currency = $3
		  ORDER BY published_at DESC, statement_number DESC
		  LIMIT 1`,
		tenant.String(),
		scope.Account().String(),
		scope.Currency().String(),
	).Scan(&linesJSON, &voidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("load credit ratio base (prior period confirmed charges): %w", err)
	}
	if voidedAt != nil {
		return 0, false, nil
	}
	var lines []statementLineRow
	if err := json.Unmarshal(linesJSON, &lines); err != nil {
		return 0, false, fmt.Errorf("load credit ratio base (prior period confirmed charges): 费用行不是本适配器写下的形状：%w", err)
	}
	total := int64(0)
	for _, line := range lines {
		total += line.AmountMinor
	}
	return total, true, nil
}
