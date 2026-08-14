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

type componentRow struct {
	Source      string `json:"source"`
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
		components = append(components, domain.ResultComponent{
			Source:      source,
			Effect:      effect,
			AmountMinor: row.AmountMinor,
		})
	}
	return components, nil
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
