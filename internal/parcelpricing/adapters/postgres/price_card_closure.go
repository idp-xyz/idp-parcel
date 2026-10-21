package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

var _ ports.PriceCardClosureRegistry = (*PriceCards)(nil)

// priceCardClosureColumns 是选卡各口在版本行之外一并读出的两格收尾事实：这一版册上最新一条截止的终点（没有即 NULL），与它
// 退役了没有。两格交 domain.SelectablePriceCardPeriod 判此刻还选不选得中——收尾怎样施加到选择上只在领域那一处，SQL 只按
// 发布时的适用期预筛（那是选得中的必要条件）。外层查询的版本表必须取别名 v。
const priceCardClosureColumns = `(SELECT c.selection_ends_at
		           FROM parcel_pricing.price_card_version_cut c
		          WHERE c.tenant_id = v.tenant_id AND c.plan_id = v.plan_id AND c.plan_version = v.plan_version
		          ORDER BY c.registration_id DESC
		          LIMIT 1),
		        EXISTS (SELECT 1
		                  FROM parcel_pricing.price_card_version_retirement r
		                 WHERE r.tenant_id = v.tenant_id AND r.plan_id = v.plan_id AND r.plan_version = v.plan_version)`

// selectablePriceCardRow 是一行版本连同它的两格收尾事实。
type selectablePriceCardRow struct {
	priceCardRow
	latestCutEnd *time.Time
	retired      bool
}

func scanSelectablePriceCardRow(scanner interface{ Scan(dest ...any) error }) (selectablePriceCardRow, error) {
	var row selectablePriceCardRow
	err := scanner.Scan(&row.planID, &row.planVersion, &row.direction, &row.purpose, &row.scope,
		&row.tableID, &row.tableVersion, &row.effectiveFrom, &row.effectiveTo,
		&row.canonicalization, &row.digest, &row.sourceSHA, &row.snapshot,
		&row.latestCutEnd, &row.retired)
	return row, err
}

// selection 按领域那一处判这一版此刻还能被选中的那一段。适用期列立不住说明行被改过，响亮报错。
func (row selectablePriceCardRow) selection() (domain.EffectivePeriod, bool, error) {
	var ends time.Time
	if row.effectiveTo != nil {
		ends = *row.effectiveTo
	}
	published, err := domain.NewEffectivePeriod(row.effectiveFrom, ends)
	if err != nil {
		return domain.EffectivePeriod{}, false, fmt.Errorf("%s/%s：适用期列立不住：%w", row.planID, row.planVersion, err)
	}
	var cutEnd time.Time
	if row.latestCutEnd != nil {
		cutEnd = *row.latestCutEnd
	}
	period, selects := domain.SelectablePriceCardPeriod(published, cutEnd, row.retired)
	return period, selects, nil
}

// selectsAt 答这一版在 at 这一刻还选不选得中。
func (row selectablePriceCardRow) selectsAt(at time.Time) (bool, error) {
	period, selects, err := row.selection()
	if err != nil {
		return false, err
	}
	return selects && period.Contains(at), nil
}

// LockPriceCardVersionForClosure 锁住旧版的版本行（ports.PriceCardClosureRegistry）。锁到调用方事务结束：同一旧版上的另一笔
// 收尾停在这里，等这一笔提交之后再读到已收尾的样子。
func (catalog *PriceCards) LockPriceCardVersionForClosure(
	ctx context.Context,
	tenant domain.TenantID,
	plan domain.VersionReference,
) (bool, error) {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("lock price card version for closure: %w", err)
	}
	var locked int
	err = executor.QueryRow(ctx,
		`SELECT 1
		   FROM parcel_pricing.price_card_version
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3
		    FOR UPDATE`,
		tenant.String(), plan.ID(), plan.Version(),
	).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock price card version for closure: %w", err)
	}
	return true, nil
}

// LoadPriceCardStanding 读回一版此刻在册上的样子（ports.PriceCardClosureRegistry）：版本行经 priceCardRow.plan 那道整图重验，
// 截止按登记顺序，退役至多一次。读走 ReadExecutor：调用方在收尾那一笔事务里、锁之后问，读到的是锁住之后的册。
func (catalog *PriceCards) LoadPriceCardStanding(
	ctx context.Context,
	tenant domain.TenantID,
	plan domain.VersionReference,
) (domain.PriceCardStanding, bool, error) {
	none := domain.PriceCardStanding{}
	querier, err := catalog.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load price card standing: %w", err)
	}
	row, err := scanPriceCardRow(querier.QueryRow(ctx,
		`SELECT `+priceCardRowColumns+`
		   FROM parcel_pricing.price_card_version
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		tenant.String(), plan.ID(), plan.Version(),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load price card standing: %w", err)
	}
	version, err := row.plan(tenant)
	if err != nil {
		return none, false, fmt.Errorf("load price card standing: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT successor_version, selection_ends_at, closure_reference, registered_at
		   FROM parcel_pricing.price_card_version_cut
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3
		  ORDER BY registration_id`,
		tenant.String(), plan.ID(), plan.Version(),
	)
	if err != nil {
		return none, false, fmt.Errorf("load price card standing: cuts: %w", err)
	}
	defer rows.Close()
	var cuts []domain.PriceCardCut
	for rows.Next() {
		var successor, reference string
		var endsAt, registeredAt time.Time
		if err := rows.Scan(&successor, &endsAt, &reference, &registeredAt); err != nil {
			return none, false, fmt.Errorf("load price card standing: cuts: %w", err)
		}
		cut, err := rehydrateCut(tenant, version.Reference(), successor, endsAt, reference, registeredAt)
		if err != nil {
			return none, false, fmt.Errorf("load price card standing: cut %s→%s: %w", plan.Version(), successor, err)
		}
		cuts = append(cuts, cut)
	}
	if err := rows.Err(); err != nil {
		return none, false, fmt.Errorf("load price card standing: cuts: %w", err)
	}

	var retirement *domain.PriceCardRetirement
	var successor, reference string
	var retiredAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT successor_version, closure_reference, retired_at
		   FROM parcel_pricing.price_card_version_retirement
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		tenant.String(), plan.ID(), plan.Version(),
	).Scan(&successor, &reference, &retiredAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return none, false, fmt.Errorf("load price card standing: retirement: %w", err)
	default:
		retired, err := rehydrateRetirement(tenant, version.Reference(), successor, reference, retiredAt)
		if err != nil {
			return none, false, fmt.Errorf("load price card standing: retirement of %s: %w", plan.Version(), err)
		}
		retirement = &retired
	}

	standing, err := domain.NewPriceCardStanding(tenant, version, cuts, retirement)
	if err != nil {
		return none, false, fmt.Errorf("load price card standing: %w", err)
	}
	return standing, true, nil
}

// SavePriceCardCut 落一条截止（ports.PriceCardClosureRegistry）。锁版本行与写入在同一条语句里：编排在锁之后读、判、再写，
// 这里再锁一次是让「成功接纳的顺序与串行化顺序一致、后接纳者的序轴更大」不靠调用方记得先锁（ADR-0056 决定三的同一落法）。
// 同一后继、同一依据撞唯一键是重放；版本不在册答 error，不把空锁读成重放。
func (catalog *PriceCards) SavePriceCardCut(ctx context.Context, cut domain.PriceCardCut) (ports.PriceCardClosureSaveOutcome, error) {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PriceCardClosureSaveOutcomeInvalid, fmt.Errorf("save price card cut: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`WITH locked AS (
		     SELECT tenant_id, plan_id, plan_version
		       FROM parcel_pricing.price_card_version
		      WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3
		        FOR UPDATE)
		 INSERT INTO parcel_pricing.price_card_version_cut
		     (tenant_id, plan_id, plan_version, successor_version, selection_ends_at, closure_reference, registered_at)
		 SELECT tenant_id, plan_id, plan_version, $4, $5, $6, $7 FROM locked
		 ON CONFLICT ON CONSTRAINT price_card_version_cut_once_per_decision DO NOTHING`,
		cut.Tenant().String(), cut.Plan().ID(), cut.Plan().Version(), cut.Successor().Version(),
		cut.SelectionEndsAt().UTC(), cut.Reference().String(), cut.RegisteredAt().UTC(),
	)
	if err != nil {
		return ports.PriceCardClosureSaveOutcomeInvalid, fmt.Errorf("save price card cut: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.PriceCardClosureSaved, nil
	}
	var exists bool
	err = executor.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM parcel_pricing.price_card_version_cut
		                 WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3
		                   AND successor_version = $4 AND closure_reference = $5)`,
		cut.Tenant().String(), cut.Plan().ID(), cut.Plan().Version(), cut.Successor().Version(), cut.Reference().String(),
	).Scan(&exists)
	if err != nil {
		return ports.PriceCardClosureSaveOutcomeInvalid, fmt.Errorf("save price card cut: 比对在册行：%w", err)
	}
	if !exists {
		return ports.PriceCardClosureSaveOutcomeInvalid, fmt.Errorf("save price card cut: %s/%s is not on the register",
			cut.Plan().ID(), cut.Plan().Version())
	}
	return ports.PriceCardClosureAlreadyRegistered, nil
}

// SavePriceCardRetirement 落一次退役（ports.PriceCardClosureRegistry）。一版只退一次：撞主键读回比对，同一后继、同一依据是
// 重放，否则答已凭别的退役。
func (catalog *PriceCards) SavePriceCardRetirement(
	ctx context.Context,
	retirement domain.PriceCardRetirement,
) (ports.PriceCardClosureSaveOutcome, error) {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PriceCardClosureSaveOutcomeInvalid, fmt.Errorf("save price card retirement: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.price_card_version_retirement
		     (tenant_id, plan_id, plan_version, successor_version, closure_reference, retired_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT ON CONSTRAINT price_card_version_retirement_pkey DO NOTHING`,
		retirement.Tenant().String(), retirement.Plan().ID(), retirement.Plan().Version(), retirement.Successor().Version(),
		retirement.Reference().String(), retirement.RetiredAt().UTC(),
	)
	if err != nil {
		return ports.PriceCardClosureSaveOutcomeInvalid, fmt.Errorf("save price card retirement: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.PriceCardClosureSaved, nil
	}
	var successor, reference string
	err = executor.QueryRow(ctx,
		`SELECT successor_version, closure_reference
		   FROM parcel_pricing.price_card_version_retirement
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		retirement.Tenant().String(), retirement.Plan().ID(), retirement.Plan().Version(),
	).Scan(&successor, &reference)
	if err != nil {
		return ports.PriceCardClosureSaveOutcomeInvalid, fmt.Errorf("save price card retirement: 比对在册行：%w", err)
	}
	if successor == retirement.Successor().Version() && reference == retirement.Reference().String() {
		return ports.PriceCardClosureAlreadyRegistered, nil
	}
	return ports.PriceCardRetiredOtherwise, nil
}

func rehydrateCut(
	tenant domain.TenantID,
	plan domain.VersionReference,
	successorVersion string,
	endsAt time.Time,
	rawReference string,
	registeredAt time.Time,
) (domain.PriceCardCut, error) {
	successor, reference, err := closureReferences(plan, successorVersion, rawReference)
	if err != nil {
		return domain.PriceCardCut{}, err
	}
	return domain.RehydratePriceCardCut(tenant, plan, successor, endsAt, reference, registeredAt)
}

func rehydrateRetirement(
	tenant domain.TenantID,
	plan domain.VersionReference,
	successorVersion string,
	rawReference string,
	retiredAt time.Time,
) (domain.PriceCardRetirement, error) {
	successor, reference, err := closureReferences(plan, successorVersion, rawReference)
	if err != nil {
		return domain.PriceCardRetirement{}, err
	}
	return domain.RehydratePriceCardRetirement(tenant, plan, successor, reference, retiredAt)
}

func closureReferences(
	plan domain.VersionReference,
	successorVersion string,
	rawReference string,
) (domain.VersionReference, domain.PriceCardClosureReference, error) {
	successor, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, plan.ID(), successorVersion)
	if err != nil {
		return domain.VersionReference{}, domain.PriceCardClosureReference{}, err
	}
	reference, err := domain.NewPriceCardClosureReference(rawReference)
	if err != nil {
		return domain.VersionReference{}, domain.PriceCardClosureReference{}, err
	}
	return successor, reference, nil
}
