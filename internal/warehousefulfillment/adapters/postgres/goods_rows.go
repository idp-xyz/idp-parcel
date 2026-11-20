package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// goodsHeaderColumns 是商品修订表被读出的列。可空数值按文本读出，再经领域的数值构造门回到规范写法，不经浮点。
const goodsHeaderColumns = `tenant_id, owner_account_id, goods_code, revision, goods_name, base_unit,
	track_batch, track_expiry, composite, outbound_mode,
	declared_name, declared_hs_code, declared_origin_country, declared_unit_price::text, declared_currency,
	submission_path, submission_source, request_key, evidence_reference, registrant, registered_at`

type storedGoodsHeader struct {
	tenant           string
	owner            string
	code             string
	revision         int
	name             string
	baseUnit         string
	trackBatch       bool
	trackExpiry      bool
	composite        bool
	outboundMode     *string
	declaredName     *string
	declaredHS       *string
	declaredOrigin   *string
	declaredPrice    *string
	declaredCurrency *string
	path             string
	source           string
	requestKey       string
	evidence         *string
	registrant       string
	registeredAt     time.Time
}

func scanStoredGoodsHeader(scanner rowScanner) (storedGoodsHeader, error) {
	var stored storedGoodsHeader
	err := scanner.Scan(
		&stored.tenant, &stored.owner, &stored.code, &stored.revision, &stored.name, &stored.baseUnit,
		&stored.trackBatch, &stored.trackExpiry, &stored.composite, &stored.outboundMode,
		&stored.declaredName, &stored.declaredHS, &stored.declaredOrigin, &stored.declaredPrice, &stored.declaredCurrency,
		&stored.path, &stored.source, &stored.requestKey, &stored.evidence, &stored.registrant, &stored.registeredAt,
	)
	return stored, err
}

type storedGoodsLevel struct {
	name   string
	factor int64
}

type storedGoodsAlias struct {
	code     string
	kind     string
	platform *string
	from     *time.Time
	to       *time.Time
}

type storedGoodsComponent struct {
	code     string
	quantity int64
}

type storedGoodsMeasurement struct {
	level      string
	length     *string
	width      *string
	height     *string
	dimUnit    *string
	weight     *string
	weightUnit *string
}

func loadGoodsRevision(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
	revision int,
) (domain.GoodsRevision, bool, error) {
	stored, err := scanStoredGoodsHeader(querier.QueryRow(ctx,
		`SELECT `+goodsHeaderColumns+`
		   FROM warehouse_fulfillment.goods_revision
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND goods_code = $3 AND revision = $4`,
		tenant.String(), owner.String(), code.String(), revision))
	if err != nil {
		return domain.GoodsRevision{}, false, err
	}
	loaded, err := goodsRevisionOfStored(ctx, querier, stored)
	if err != nil {
		return domain.GoodsRevision{}, false, err
	}
	return loaded, true, nil
}

func loadLatestGoodsRevision(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) (domain.GoodsRevision, bool, error) {
	stored, err := scanStoredGoodsHeader(querier.QueryRow(ctx,
		`SELECT `+goodsHeaderColumns+`
		   FROM warehouse_fulfillment.goods_revision
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND goods_code = $3
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), owner.String(), code.String()))
	if err != nil {
		return domain.GoodsRevision{}, false, err
	}
	loaded, err := goodsRevisionOfStored(ctx, querier, stored)
	if err != nil {
		return domain.GoodsRevision{}, false, err
	}
	return loaded, true, nil
}

func goodsRevisionOfStored(ctx context.Context, querier bentopg.Querier, stored storedGoodsHeader) (domain.GoodsRevision, error) {
	fail := func(cell string, err error) (domain.GoodsRevision, error) {
		return domain.GoodsRevision{}, fmt.Errorf("在册的商品 %q 修订 %d 的%s读不回：%w", stored.code, stored.revision, cell, err)
	}
	content, err := goodsContentOfStored(ctx, querier, stored)
	if err != nil {
		return domain.GoodsRevision{}, err
	}
	submission, err := goodsSubmissionOfStored(stored)
	if err != nil {
		return fail("提交", err)
	}
	tenant, err := domain.NewTenantID(stored.tenant)
	if err != nil {
		return fail("租户", err)
	}
	owner, err := domain.NewOwnerAccountID(stored.owner)
	if err != nil {
		return fail("货主", err)
	}
	code, err := domain.NewGoodsCode(stored.code)
	if err != nil {
		return fail("商品编码", err)
	}
	registrant, err := domain.NewRegistrant(stored.registrant)
	if err != nil {
		return fail("登记责任方", err)
	}
	revision, err := domain.NewGoodsRevision(tenant, owner, code, stored.revision, content, submission, registrant, stored.registeredAt)
	if err != nil {
		return fail("修订", err)
	}
	return revision, nil
}

func goodsSubmissionOfStored(stored storedGoodsHeader) (domain.GoodsSubmission, error) {
	path, err := domain.NewSubmissionPath(stored.path)
	if err != nil {
		return domain.GoodsSubmission{}, err
	}
	key, err := domain.NewRequestKey(stored.requestKey)
	if err != nil {
		return domain.GoodsSubmission{}, err
	}
	switch path {
	case domain.SubmissionByOwner:
		source, err := domain.NewSubmissionSource(stored.source)
		if err != nil {
			return domain.GoodsSubmission{}, err
		}
		return domain.NewOwnerSubmission(source, key)
	case domain.SubmissionOnBehalf:
		if stored.evidence == nil {
			return domain.GoodsSubmission{}, fmt.Errorf("代录缺依据引用")
		}
		evidence, err := domain.NewEvidenceReference(*stored.evidence)
		if err != nil {
			return domain.GoodsSubmission{}, err
		}
		return domain.NewOnBehalfSubmission(key, evidence)
	default:
		return domain.GoodsSubmission{}, fmt.Errorf("提交途径 %q", stored.path)
	}
}

func goodsContentOfStored(ctx context.Context, querier bentopg.Querier, stored storedGoodsHeader) (domain.GoodsContent, error) {
	fail := func(cell string, err error) (domain.GoodsContent, error) {
		return domain.GoodsContent{}, fmt.Errorf("在册的商品 %q 修订 %d 的%s读不回：%w", stored.code, stored.revision, cell, err)
	}
	name, err := domain.NewGoodsName(stored.name)
	if err != nil {
		return fail("名称", err)
	}
	baseUnit, err := domain.NewGoodsUnitName(stored.baseUnit)
	if err != nil {
		return fail("基本单位", err)
	}
	levels, err := loadGoodsLevels(ctx, querier, stored)
	if err != nil {
		return fail("包装层级", err)
	}
	aliases, err := loadGoodsAliases(ctx, querier, stored)
	if err != nil {
		return fail("别名", err)
	}
	components, err := loadGoodsComponents(ctx, querier, stored)
	if err != nil {
		return fail("组成", err)
	}
	measurements, err := loadGoodsMeasurements(ctx, querier, stored)
	if err != nil {
		return fail("声明的尺寸重量", err)
	}
	declaration, err := goodsDeclarationOfStored(stored)
	if err != nil {
		return fail("申报资料", err)
	}
	var mode domain.GoodsOutboundMode
	if stored.outboundMode != nil {
		mode, err = domain.NewGoodsOutboundMode(*stored.outboundMode)
		if err != nil {
			return fail("出库方式", err)
		}
	}
	content, err := domain.NewGoodsContent(domain.GoodsContentSpec{
		Name: name, BaseUnit: baseUnit, PackagingLevels: levels,
		TrackBatch: stored.trackBatch, TrackExpiry: stored.trackExpiry,
		Aliases: aliases, Composite: stored.composite, OutboundMode: mode, Components: components,
		Declaration: declaration, DeclaredMeasurements: measurements,
	})
	if err != nil {
		return fail("资料", err)
	}
	return content, nil
}

func goodsDeclarationOfStored(stored storedGoodsHeader) (domain.GoodsDeclaration, error) {
	spec := domain.GoodsDeclarationSpec{}
	var err error
	if stored.declaredName != nil {
		spec.Name, err = domain.NewDeclaredGoodsName(*stored.declaredName)
		if err != nil {
			return domain.GoodsDeclaration{}, err
		}
	}
	if stored.declaredHS != nil {
		spec.HSCode, err = domain.NewHSCode(*stored.declaredHS)
		if err != nil {
			return domain.GoodsDeclaration{}, err
		}
	}
	if stored.declaredOrigin != nil {
		spec.OriginCountry, err = domain.NewCountryCode(*stored.declaredOrigin)
		if err != nil {
			return domain.GoodsDeclaration{}, err
		}
	}
	if stored.declaredPrice != nil || stored.declaredCurrency != nil {
		if stored.declaredPrice == nil || stored.declaredCurrency == nil {
			return domain.GoodsDeclaration{}, fmt.Errorf("申报单价与币种没有成对")
		}
		spec.UnitPrice, err = domain.NewMeasure(*stored.declaredPrice)
		if err != nil {
			return domain.GoodsDeclaration{}, err
		}
		spec.Currency, err = domain.NewCurrencyCode(*stored.declaredCurrency)
		if err != nil {
			return domain.GoodsDeclaration{}, err
		}
	}
	return domain.NewGoodsDeclaration(spec)
}

func loadGoodsLevels(ctx context.Context, querier bentopg.Querier, stored storedGoodsHeader) ([]domain.PackagingLevel, error) {
	rows, err := querier.Query(ctx,
		`SELECT unit_name, factor
		   FROM warehouse_fulfillment.goods_packaging_level
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND goods_code = $3 AND revision = $4
		  ORDER BY position`,
		stored.tenant, stored.owner, stored.code, stored.revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var levels []domain.PackagingLevel
	for rows.Next() {
		var level storedGoodsLevel
		if err := rows.Scan(&level.name, &level.factor); err != nil {
			return nil, err
		}
		name, err := domain.NewGoodsUnitName(level.name)
		if err != nil {
			return nil, err
		}
		levels = append(levels, domain.NewPackagingLevel(name, level.factor))
	}
	return levels, rows.Err()
}

func loadGoodsAliases(ctx context.Context, querier bentopg.Querier, stored storedGoodsHeader) ([]domain.GoodsAlias, error) {
	rows, err := querier.Query(ctx,
		`SELECT alias_code, alias_type, platform_name, valid_from, valid_to
		   FROM warehouse_fulfillment.goods_alias
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND goods_code = $3 AND revision = $4
		  ORDER BY position`,
		stored.tenant, stored.owner, stored.code, stored.revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var aliases []domain.GoodsAlias
	for rows.Next() {
		var alias storedGoodsAlias
		if err := rows.Scan(&alias.code, &alias.kind, &alias.platform, &alias.from, &alias.to); err != nil {
			return nil, err
		}
		code, err := domain.NewGoodsAliasCode(alias.code)
		if err != nil {
			return nil, err
		}
		kind, err := domain.NewGoodsAliasKind(alias.kind)
		if err != nil {
			return nil, err
		}
		var platform domain.PlatformName
		if alias.platform != nil {
			platform, err = domain.NewPlatformName(*alias.platform)
			if err != nil {
				return nil, err
			}
		}
		var from, to time.Time
		if alias.from != nil {
			from = *alias.from
		}
		if alias.to != nil {
			to = *alias.to
		}
		aliases = append(aliases, domain.NewGoodsAlias(code, kind, platform, domain.NewAliasValidity(from, to)))
	}
	return aliases, rows.Err()
}

func loadGoodsComponents(ctx context.Context, querier bentopg.Querier, stored storedGoodsHeader) ([]domain.GoodsComponent, error) {
	rows, err := querier.Query(ctx,
		`SELECT component_code, quantity
		   FROM warehouse_fulfillment.goods_component
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND goods_code = $3 AND revision = $4
		  ORDER BY position`,
		stored.tenant, stored.owner, stored.code, stored.revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var components []domain.GoodsComponent
	for rows.Next() {
		var component storedGoodsComponent
		if err := rows.Scan(&component.code, &component.quantity); err != nil {
			return nil, err
		}
		code, err := domain.NewGoodsCode(component.code)
		if err != nil {
			return nil, err
		}
		components = append(components, domain.NewGoodsComponent(code, component.quantity))
	}
	return components, rows.Err()
}

func loadGoodsMeasurements(ctx context.Context, querier bentopg.Querier, stored storedGoodsHeader) ([]domain.DeclaredMeasurement, error) {
	rows, err := querier.Query(ctx,
		`SELECT level_name, length_value::text, width_value::text, height_value::text, dimensions_unit,
		        weight_value::text, weight_unit
		   FROM warehouse_fulfillment.goods_declared_measurement
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND goods_code = $3 AND revision = $4
		  ORDER BY position`,
		stored.tenant, stored.owner, stored.code, stored.revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var measurements []domain.DeclaredMeasurement
	for rows.Next() {
		var measurement storedGoodsMeasurement
		if err := rows.Scan(&measurement.level, &measurement.length, &measurement.width, &measurement.height, &measurement.dimUnit, &measurement.weight, &measurement.weightUnit); err != nil {
			return nil, err
		}
		built, err := declaredMeasurementOfStored(measurement)
		if err != nil {
			return nil, err
		}
		measurements = append(measurements, built)
	}
	return measurements, rows.Err()
}

func declaredMeasurementOfStored(stored storedGoodsMeasurement) (domain.DeclaredMeasurement, error) {
	level, err := domain.NewGoodsUnitName(stored.level)
	if err != nil {
		return domain.DeclaredMeasurement{}, err
	}
	spec := domain.DeclaredMeasurementSpec{Level: level}
	if stored.length != nil || stored.width != nil || stored.height != nil || stored.dimUnit != nil {
		if stored.length == nil || stored.width == nil || stored.height == nil || stored.dimUnit == nil {
			return domain.DeclaredMeasurement{}, fmt.Errorf("尺寸四格没有成组")
		}
		length, err := domain.NewMeasure(*stored.length)
		if err != nil {
			return domain.DeclaredMeasurement{}, err
		}
		width, err := domain.NewMeasure(*stored.width)
		if err != nil {
			return domain.DeclaredMeasurement{}, err
		}
		height, err := domain.NewMeasure(*stored.height)
		if err != nil {
			return domain.DeclaredMeasurement{}, err
		}
		unit, err := domain.NewUnitCode(*stored.dimUnit)
		if err != nil {
			return domain.DeclaredMeasurement{}, err
		}
		dimensions, err := domain.NewDeclaredDimensions(length, width, height, unit)
		if err != nil {
			return domain.DeclaredMeasurement{}, err
		}
		spec.Dimensions = &dimensions
	}
	if stored.weight != nil || stored.weightUnit != nil {
		if stored.weight == nil || stored.weightUnit == nil {
			return domain.DeclaredMeasurement{}, fmt.Errorf("重量两格没有成组")
		}
		value, err := domain.NewMeasure(*stored.weight)
		if err != nil {
			return domain.DeclaredMeasurement{}, err
		}
		unit, err := domain.NewUnitCode(*stored.weightUnit)
		if err != nil {
			return domain.DeclaredMeasurement{}, err
		}
		weight, err := domain.NewDeclaredWeight(value, unit)
		if err != nil {
			return domain.DeclaredMeasurement{}, err
		}
		spec.Weight = &weight
	}
	return domain.NewDeclaredMeasurement(spec)
}

func goodsRowOf(revision domain.GoodsRevision) ports.GoodsRow {
	content := revision.Content()
	row := ports.GoodsRow{
		OwnerAccount:   revision.Owner().String(),
		GoodsCode:      revision.Code().String(),
		Revision:       revision.Revision(),
		Name:           content.Name().String(),
		BaseUnit:       content.BaseUnit().String(),
		TrackBatch:     content.TrackBatch(),
		TrackExpiry:    content.TrackExpiry(),
		Composite:      content.Composite(),
		SubmissionPath: revision.Submission().Path().String(),
		Registrant:     revision.Registrant().String(),
		RegisteredAt:   revision.RegisteredAt().UTC(),
	}
	if evidence, has := revision.Submission().EvidenceReference(); has {
		row.EvidenceReference = evidence.String()
	}
	if mode, has := content.OutboundMode(); has {
		row.OutboundMode = mode.String()
	}
	for _, level := range content.PackagingLevels() {
		row.PackagingLevels = append(row.PackagingLevels, ports.GoodsLevelRow{Name: level.Name().String(), Factor: level.Factor()})
	}
	for _, alias := range content.Aliases() {
		aliasRow := ports.GoodsAliasRow{Code: alias.Code().String(), Type: alias.Kind().String()}
		if platform, has := alias.Platform(); has {
			aliasRow.Platform = platform.String()
		}
		if from, has := alias.Validity().From(); has {
			aliasRow.ValidFrom = timePtr(from)
		}
		if to, has := alias.Validity().To(); has {
			aliasRow.ValidTo = timePtr(to)
		}
		row.Aliases = append(row.Aliases, aliasRow)
	}
	for _, component := range content.Components() {
		row.Components = append(row.Components, ports.GoodsComponentRow{Code: component.Code().String(), Quantity: component.Quantity()})
	}
	declaration := content.Declaration()
	if name, has := declaration.Name(); has {
		row.Declaration.Name = name.String()
	}
	if hs, has := declaration.HSCode(); has {
		row.Declaration.HSCode = hs.String()
	}
	if origin, has := declaration.OriginCountry(); has {
		row.Declaration.OriginCountry = origin.String()
	}
	if price, currency, has := declaration.UnitPrice(); has {
		row.Declaration.UnitPrice = price.String()
		row.Declaration.Currency = currency.String()
	}
	for _, measurement := range content.DeclaredMeasurements() {
		measured := ports.GoodsMeasurementRow{Level: measurement.Level().String()}
		if dimensions, has := measurement.Dimensions(); has {
			measured.Dimensions = &ports.GoodsDimensionsRow{
				Length: dimensions.Length().String(), Width: dimensions.Width().String(),
				Height: dimensions.Height().String(), Unit: dimensions.Unit().String(),
			}
		}
		if weight, has := measurement.Weight(); has {
			measured.Weight = &ports.GoodsWeightRow{Value: weight.Value().String(), Unit: weight.Unit().String()}
		}
		row.DeclaredMeasurements = append(row.DeclaredMeasurements, measured)
	}
	return row
}

func timePtr(at time.Time) *time.Time {
	utc := at.UTC()
	return &utc
}

func textPtr(value string) *string {
	return &value
}
