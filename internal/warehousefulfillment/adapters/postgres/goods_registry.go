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

// Goods 实现 ports.GoodsRegistry。修订只追加。同一个键的重放与冲突、接不上的修订号、识别码交叠，都在同一货主的事务级咨询锁里判定后再插入：
// 识别码的区间交叠不是主键能守的唯一，两笔并发抢同一个码时后到的一笔要等先到的提交，再按刚提交的当前修订作答。事务是 READ COMMITTED。
type Goods struct {
	db *bentopg.DB
}

func NewGoods(db *bentopg.DB) (*Goods, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Goods{db: db}, nil
}

var _ ports.GoodsRegistry = (*Goods)(nil)

func (repository *Goods) SaveGoodsRevision(ctx context.Context, revision domain.GoodsRevision) (ports.GoodsSaveResult, error) {
	const operation = "save goods revision"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.GoodsSaveResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockGoodsOwnerExclusive(ctx, executor, revision.Tenant().String(), revision.Owner().String()); err != nil {
		return ports.GoodsSaveResult{}, fmt.Errorf("%s: %w", operation, err)
	}

	existing, found, err := repository.loadBySubmission(ctx, executor, revision.Tenant(), revision.Submission())
	if err != nil {
		return ports.GoodsSaveResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if found {
		if existing.Request().SameAs(revision.Request()) {
			return ports.GoodsSaveResult{Outcome: ports.GoodsRevisionAlreadyRegistered}, nil
		}
		return ports.GoodsSaveResult{Outcome: ports.GoodsSourceConflict}, nil
	}

	latest, hasLatest, err := loadLatestGoodsRevision(ctx, executor, revision.Tenant(), revision.Owner(), revision.Code())
	if errors.Is(err, pgx.ErrNoRows) {
		hasLatest, err = false, nil
	}
	if err != nil {
		return ports.GoodsSaveResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	want := 1
	if hasLatest {
		want = latest.Revision() + 1
	}
	if revision.Revision() != want {
		return ports.GoodsSaveResult{Outcome: ports.GoodsBasisStale}, nil
	}

	taken, collides, err := conflictingGoodsCode(ctx, executor, revision.Tenant(), revision.Owner(), revision.Code(), revision.IdentifierClaims())
	if err != nil {
		return ports.GoodsSaveResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	if collides {
		return ports.GoodsSaveResult{Outcome: ports.GoodsIdentifierConflict, ConflictingCode: taken}, nil
	}
	if err := insertGoodsRevision(ctx, executor, revision); err != nil {
		return ports.GoodsSaveResult{}, fmt.Errorf("%s: %w", operation, err)
	}
	return ports.GoodsSaveResult{Outcome: ports.GoodsRevisionSaved}, nil
}

func (repository *Goods) LoadLatestGoods(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) (domain.GoodsRevision, bool, error) {
	const operation = "load latest goods"
	if tenant.String() == "" || owner.String() == "" || code.String() == "" {
		return domain.GoodsRevision{}, false, fmt.Errorf("%s: tenant, owner and goods code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.GoodsRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	loaded, found, err := loadLatestGoodsRevision(ctx, querier, tenant, owner, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.GoodsRevision{}, false, nil
	}
	if err != nil {
		return domain.GoodsRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return loaded, found, nil
}

func (repository *Goods) LoadGoodsAt(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
	at time.Time,
) (domain.GoodsRevision, bool, error) {
	const operation = "load goods at instant"
	if tenant.String() == "" || owner.String() == "" || code.String() == "" {
		return domain.GoodsRevision{}, false, fmt.Errorf("%s: tenant, owner and goods code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.GoodsRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	loaded, found, err := loadGoodsRevisionAt(ctx, querier, tenant, owner, code, at)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.GoodsRevision{}, false, nil
	}
	if err != nil {
		return domain.GoodsRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return loaded, found, nil
}

func (repository *Goods) LoadGoodsRevisionBySubmission(
	ctx context.Context,
	tenant domain.TenantID,
	submission domain.GoodsSubmission,
) (domain.GoodsRevision, bool, error) {
	const operation = "load goods revision by submission"
	if tenant.String() == "" || submission.Path() == domain.SubmissionPathInvalid || submission.Key().String() == "" {
		return domain.GoodsRevision{}, false, fmt.Errorf("%s: tenant and submission are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.GoodsRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return repository.loadBySubmission(ctx, querier, tenant, submission)
}

func (repository *Goods) FindGoodsIdentifierConflict(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
	claims []domain.IdentifierClaim,
) (domain.IdentifierCode, bool, error) {
	const operation = "find goods identifier conflict"
	if tenant.String() == "" || owner.String() == "" || code.String() == "" {
		return domain.IdentifierCode{}, false, fmt.Errorf("%s: tenant, owner and goods code are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.IdentifierCode{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	taken, collides, err := conflictingGoodsCode(ctx, querier, tenant, owner, code, claims)
	if err != nil {
		return domain.IdentifierCode{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return taken, collides, nil
}

func (repository *Goods) loadBySubmission(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	submission domain.GoodsSubmission,
) (domain.GoodsRevision, bool, error) {
	source := ""
	if named, has := submission.Source(); has {
		source = named.String()
	}
	stored, err := scanStoredGoodsHeader(querier.QueryRow(ctx,
		`SELECT `+goodsHeaderColumns+`
		   FROM warehouse_fulfillment.goods_revision
		  WHERE tenant_id = $1 AND submission_path = $2 AND submission_source = $3 AND request_key = $4`,
		tenant.String(), submission.Path().String(), source, submission.Key().String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.GoodsRevision{}, false, nil
	}
	if err != nil {
		return domain.GoodsRevision{}, false, err
	}
	loaded, err := goodsRevisionOfStored(ctx, querier, stored)
	if err != nil {
		return domain.GoodsRevision{}, false, err
	}
	return loaded, true, nil
}

func conflictingGoodsCode(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
	claims []domain.IdentifierClaim,
) (domain.IdentifierCode, bool, error) {
	codes := make([]string, 0, len(claims))
	for _, claim := range claims {
		if claim.Validity().Inverted() {
			continue
		}
		codes = append(codes, claim.Code().String())
	}
	if len(codes) == 0 {
		return domain.IdentifierCode{}, false, nil
	}
	rows, err := querier.Query(ctx,
		`WITH latest AS (
		    SELECT DISTINCT ON (goods_code) goods_code, revision
		      FROM warehouse_fulfillment.goods_revision
		     WHERE tenant_id = $1 AND owner_account_id = $2 AND goods_code <> $3
		     ORDER BY goods_code, revision DESC
		 )
		 SELECT latest.goods_code
		   FROM latest
		  WHERE latest.goods_code = ANY($4)
		     OR EXISTS (
		        SELECT 1 FROM warehouse_fulfillment.goods_alias alias
		         WHERE alias.tenant_id = $1 AND alias.owner_account_id = $2
		           AND alias.goods_code = latest.goods_code AND alias.revision = latest.revision
		           AND alias.alias_code = ANY($4)
		     )
		  ORDER BY latest.goods_code`,
		tenant.String(), owner.String(), code.String(), codes)
	if err != nil {
		return domain.IdentifierCode{}, false, err
	}
	defer rows.Close()
	var others []string
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return domain.IdentifierCode{}, false, err
		}
		others = append(others, other)
	}
	if err := rows.Err(); err != nil {
		return domain.IdentifierCode{}, false, err
	}
	for _, claim := range claims {
		if claim.Validity().Inverted() {
			continue
		}
		for _, other := range others {
			otherCode, err := domain.NewGoodsCode(other)
			if err != nil {
				return domain.IdentifierCode{}, false, err
			}
			held, found, err := loadLatestGoodsRevision(ctx, querier, tenant, owner, otherCode)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && !found) {
				return domain.IdentifierCode{}, false, fmt.Errorf("读商品 %q 的当前修订：没有这一笔", other)
			}
			if err != nil {
				return domain.IdentifierCode{}, false, err
			}
			for _, existing := range held.IdentifierClaims() {
				if claim.Collides(existing) {
					return claim.Code(), true, nil
				}
			}
		}
	}
	return domain.IdentifierCode{}, false, nil
}

func insertGoodsRevision(ctx context.Context, executor bentopg.Executor, revision domain.GoodsRevision) error {
	content := revision.Content()
	submission := revision.Submission()
	source := ""
	if named, has := submission.Source(); has {
		source = named.String()
	}
	var evidence *string
	if cited, has := submission.EvidenceReference(); has {
		evidence = textPtr(cited.String())
	}
	var outbound *string
	if mode, has := content.OutboundMode(); has {
		outbound = textPtr(mode.String())
	}
	declaration := content.Declaration()
	var declaredName, declaredHS, declaredOrigin, declaredCurrency *string
	var declaredPrice *string
	if name, has := declaration.Name(); has {
		declaredName = textPtr(name.String())
	}
	if hs, has := declaration.HSCode(); has {
		declaredHS = textPtr(hs.String())
	}
	if origin, has := declaration.OriginCountry(); has {
		declaredOrigin = textPtr(origin.String())
	}
	if price, currency, has := declaration.UnitPrice(); has {
		declaredPrice = textPtr(price.String())
		declaredCurrency = textPtr(currency.String())
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.goods_revision (
		    tenant_id, owner_account_id, goods_code, revision, goods_name, base_unit,
		    track_batch, track_expiry, composite, outbound_mode,
		    declared_name, declared_hs_code, declared_origin_country, declared_unit_price, declared_currency,
		    submission_path, submission_source, request_key, evidence_reference, registrant, registered_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::numeric,$15,$16,$17,$18,$19,$20,$21)`,
		revision.Tenant().String(), revision.Owner().String(), revision.Code().String(), revision.Revision(),
		content.Name().String(), content.BaseUnit().String(), content.TrackBatch(), content.TrackExpiry(), content.Composite(), outbound,
		declaredName, declaredHS, declaredOrigin, declaredPrice, declaredCurrency,
		submission.Path().String(), source, submission.Key().String(), evidence, revision.Registrant().String(), revision.RegisteredAt(),
	); err != nil {
		return err
	}
	for index, level := range content.PackagingLevels() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.goods_packaging_level
			    (tenant_id, owner_account_id, goods_code, revision, position, unit_name, factor)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			revision.Tenant().String(), revision.Owner().String(), revision.Code().String(), revision.Revision(),
			index, level.Name().String(), level.Factor(),
		); err != nil {
			return err
		}
	}
	for index, alias := range content.Aliases() {
		var platform *string
		if named, has := alias.Platform(); has {
			platform = textPtr(named.String())
		}
		var validFrom, validTo *time.Time
		if at, has := alias.Validity().From(); has {
			validFrom = timePtr(at)
		}
		if at, has := alias.Validity().To(); has {
			validTo = timePtr(at)
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.goods_alias
			    (tenant_id, owner_account_id, goods_code, revision, position, alias_code, alias_type, platform_name, valid_from, valid_to)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			revision.Tenant().String(), revision.Owner().String(), revision.Code().String(), revision.Revision(),
			index, alias.Code().String(), alias.Kind().String(), platform, validFrom, validTo,
		); err != nil {
			return err
		}
	}
	for index, component := range content.Components() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.goods_component
			    (tenant_id, owner_account_id, goods_code, revision, position, component_code, quantity)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			revision.Tenant().String(), revision.Owner().String(), revision.Code().String(), revision.Revision(),
			index, component.Code().String(), component.Quantity(),
		); err != nil {
			return err
		}
	}
	for index, measurement := range content.DeclaredMeasurements() {
		var length, width, height, dimUnit, weight, weightUnit *string
		if dimensions, has := measurement.Dimensions(); has {
			length = textPtr(dimensions.Length().String())
			width = textPtr(dimensions.Width().String())
			height = textPtr(dimensions.Height().String())
			dimUnit = textPtr(dimensions.Unit().String())
		}
		if weighed, has := measurement.Weight(); has {
			weight = textPtr(weighed.Value().String())
			weightUnit = textPtr(weighed.Unit().String())
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.goods_declared_measurement (
			    tenant_id, owner_account_id, goods_code, revision, position, level_name,
			    length_value, width_value, height_value, dimensions_unit, weight_value, weight_unit)
			 VALUES ($1,$2,$3,$4,$5,$6,$7::numeric,$8::numeric,$9::numeric,$10,$11::numeric,$12)`,
			revision.Tenant().String(), revision.Owner().String(), revision.Code().String(), revision.Revision(),
			index, measurement.Level().String(), length, width, height, dimUnit, weight, weightUnit,
		); err != nil {
			return err
		}
	}
	return nil
}
