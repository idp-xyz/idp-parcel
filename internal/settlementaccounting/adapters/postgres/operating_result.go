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

// OperatingResults 实现 ports.OperatingResultStore。Save 登记；Replace 换组成/版本
// 回指，不改口径三件。
type OperatingResults struct {
	db *bentopg.DB
}

func NewOperatingResults(db *bentopg.DB) (*OperatingResults, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &OperatingResults{db: db}, nil
}

// componentRow 是 operating_result.components 的元素形状。role 随 ADR-0087 决定三加入：
// 列是 jsonb，改元素形状无需迁移，但写侧与读侧必须同笔改——读回旧形状译不出角色，而表
// 此刻 0 行，这一点无历史负担。
type componentRow struct {
	Source      string `json:"source"`
	Role        string `json:"role"`
	Effect      string `json:"effect"`
	AmountMinor int64  `json:"amountMinor"`
}

func (repository *OperatingResults) FindByKey(
	ctx context.Context,
	key ports.OperatingResultKey,
) (ports.OperatingResultRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.OperatingResultRecord{}, false, fmt.Errorf("find operating result: %w", err)
	}

	var currency, version, digest string
	var componentsJSON []byte
	var margin int64
	var asOf, recordedAt time.Time
	var corrects *string
	err = querier.QueryRow(ctx,
		`SELECT currency, components, margin_minor, version, as_of, corrects,
		        content_digest, recorded_at
		   FROM settlement_accounting.operating_result
		  WHERE tenant_id = $1
		    AND scope_ref = $2
		    AND period_ref = $3
		    AND basis = $4`,
		key.TenantID.String(),
		key.Scope.String(),
		key.Period.String(),
		key.Basis.String(),
	).Scan(&currency, &componentsJSON, &margin, &version, &asOf, &corrects, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.OperatingResultRecord{}, false, nil
	}
	if err != nil {
		return ports.OperatingResultRecord{}, false, fmt.Errorf("find operating result: %w", err)
	}

	result, err := rebuildOperatingResult(key, currency, componentsJSON, version, asOf, corrects)
	if err != nil {
		return ports.OperatingResultRecord{}, false, fmt.Errorf("find operating result: %w", err)
	}
	if _, derived := result.Margin(); derived != margin {
		return ports.OperatingResultRecord{}, false, fmt.Errorf("find operating result: margin mismatch")
	}
	return ports.OperatingResultRecord{
		Key:           key,
		ContentDigest: digest,
		Result:        result,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

func (repository *OperatingResults) Save(
	ctx context.Context,
	record ports.OperatingResultRecord,
) (ports.OperatingResultSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.OperatingResultSaveOutcomeInvalid, fmt.Errorf("save operating result: %w", err)
	}
	componentsJSON, err := marshalComponents(record.Result.Components())
	if err != nil {
		return ports.OperatingResultSaveOutcomeInvalid, fmt.Errorf("save operating result: %w", err)
	}
	currency, margin := record.Result.Margin()
	corrects, hasCorrects := record.Result.Corrects()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.operating_result
			(tenant_id, scope_ref, period_ref, basis, currency, components, margin_minor,
			 version, as_of, corrects, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Scope.String(),
		record.Key.Period.String(),
		record.Key.Basis.String(),
		currency.String(),
		componentsJSON,
		margin,
		record.Result.Version().String(),
		record.Result.AsOf().UTC(),
		optionalRef(corrects, hasCorrects),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.OperatingResultSaveOutcomeInvalid, fmt.Errorf("save operating result: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.OperatingResultAlreadyDerived, nil
	}
	return ports.OperatingResultSaved, nil
}

func (repository *OperatingResults) Replace(
	ctx context.Context,
	record ports.OperatingResultRecord,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("replace operating result: %w", err)
	}
	corrects, hasCorrects := record.Result.Corrects()
	if !hasCorrects {
		return false, fmt.Errorf("replace operating result: replace requires a rederivation")
	}
	componentsJSON, err := marshalComponents(record.Result.Components())
	if err != nil {
		return false, fmt.Errorf("replace operating result: %w", err)
	}
	currency, margin := record.Result.Margin()
	tag, err := executor.Exec(ctx,
		`UPDATE settlement_accounting.operating_result
		    SET components = $5, margin_minor = $6, version = $7, as_of = $8,
		        corrects = $9, content_digest = $10, recorded_at = $11
		  WHERE tenant_id = $1
		    AND scope_ref = $2
		    AND period_ref = $3
		    AND basis = $4
		    AND currency = $12`,
		record.Key.TenantID.String(),
		record.Key.Scope.String(),
		record.Key.Period.String(),
		record.Key.Basis.String(),
		componentsJSON,
		margin,
		record.Result.Version().String(),
		record.Result.AsOf().UTC(),
		corrects.String(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
		currency.String(),
	)
	if err != nil {
		return false, fmt.Errorf("replace operating result: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func rebuildOperatingResult(
	key ports.OperatingResultKey,
	currency string,
	componentsJSON []byte,
	version string,
	asOf time.Time,
	corrects *string,
) (domain.OperatingResult, error) {
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.OperatingResult{}, err
	}
	versionRef, err := domain.NewOperatingResultVersion(version)
	if err != nil {
		return domain.OperatingResult{}, err
	}
	components, err := unmarshalComponents(componentsJSON)
	if err != nil {
		return domain.OperatingResult{}, err
	}
	spec := domain.RehydrateOperatingResultSpec{
		Scope:      key.Scope,
		Period:     key.Period,
		Basis:      key.Basis,
		Currency:   currencyCode,
		Components: components,
		Version:    versionRef,
		AsOf:       asOf,
	}
	if corrects != nil {
		spec.Corrects, err = domain.NewOperatingResultVersion(*corrects)
		if err != nil {
			return domain.OperatingResult{}, err
		}
	}
	return domain.RehydrateOperatingResult(spec)
}

func marshalComponents(components []domain.ResultComponent) ([]byte, error) {
	rows := make([]componentRow, 0, len(components))
	for _, component := range components {
		rows = append(rows, componentRow{
			Source:      component.Source.String(),
			Role:        component.Role.String(),
			Effect:      component.Effect.String(),
			AmountMinor: component.AmountMinor,
		})
	}
	return json.Marshal(rows)
}

func unmarshalComponents(raw []byte) ([]domain.ResultComponent, error) {
	var rows []componentRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	components := make([]domain.ResultComponent, 0, len(rows))
	for _, row := range rows {
		source, err := domain.NewComponentSourceReference(row.Source)
		if err != nil {
			return nil, err
		}
		effect, err := componentEffectFrom(row.Effect)
		if err != nil {
			return nil, err
		}
		role, err := componentRoleFrom(row.Role)
		if err != nil {
			return nil, err
		}
		components = append(components, domain.ResultComponent{
			Source:      source,
			Role:        role,
			Effect:      effect,
			AmountMinor: row.AmountMinor,
		})
	}
	return components, nil
}

// componentRoleFrom 逐格译回角色。空串明确报错而不是落成零值：一个零值角色在任何口径
// 下都不成立，交给重建门会答「角色在本口径不成立」，把「行是旧形状」说成「选料选错了」。
func componentRoleFrom(raw string) (domain.ComponentRole, error) {
	for _, role := range []domain.ComponentRole{
		domain.CustomerEstimateRole, domain.SupplierExpectedCostRole,
		domain.CustomerOperatingReceivableRole, domain.AuditedPayableRole, domain.SupplierCreditNoteRole,
		domain.SettledCustomerReceivableRole, domain.SettledAuditedPayableRole,
		domain.SettledSupplierCreditNoteRole,
	} {
		if role.String() == raw {
			return role, nil
		}
	}
	return 0, fmt.Errorf("unknown component role %q", raw)
}

func componentEffectFrom(raw string) (domain.ComponentEffect, error) {
	switch raw {
	case domain.IncreasesResult.String():
		return domain.IncreasesResult, nil
	case domain.DecreasesResult.String():
		return domain.DecreasesResult, nil
	default:
		return 0, fmt.Errorf("unknown component effect %q", raw)
	}
}
