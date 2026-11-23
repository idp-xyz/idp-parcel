package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

var _ ports.DirectionAuthorizationView = (*CommercialPublications)(nil)

// ReadDirectionAuthorization 按（租户、授权规则、版本）读那一版，连同收尾（区间更正与退役），
// 再读它指名的价格政策在同一时刻仍在效力内的方向。不在册答未发布，不答错误。
func (repository *CommercialPublications) ReadDirectionAuthorization(
	ctx context.Context,
	tenant domain.TenantID,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
	at time.Time,
) (domain.DirectionAuthorizationFact, error) {
	if at.IsZero() || tenant.String() == "" || objectID.String() == "" || version.String() == "" {
		return domain.DirectionAuthorizationFact{}, fmt.Errorf("read direction authorization: tenant, version and at are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.DirectionAuthorizationFact{}, fmt.Errorf("read direction authorization: %w", err)
	}
	var raw []byte
	var closure scannedVersionClosure
	err = querier.QueryRow(ctx,
		`SELECT version.snapshot, `+versionClosureColumns+`
		   FROM party_commercial.commercial_version AS version
		   `+versionClosureJoins("version")+`
		  WHERE version.tenant_id = $1
		    AND version.object_kind = $2
		    AND version.object_id = $3
		    AND version.version_label = $4`,
		tenant.String(), uint8(domain.AuthorizationRuleObject), objectID.String(), version.String(),
	).Scan(append([]any{&raw}, closure.targets()...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.JudgeCitedDirectionAuthorization(false, domain.CommercialVersion{}, domain.EffectiveInterval{}, at, nil)
	}
	if err != nil {
		return domain.DirectionAuthorizationFact{}, fmt.Errorf("read direction authorization: %w", err)
	}
	loaded, selection, err := foldCitedVersion(raw, closure)
	if err != nil {
		return domain.DirectionAuthorizationFact{}, fmt.Errorf("read direction authorization: %w", err)
	}
	if loaded.Kind() != domain.AuthorizationRuleObject {
		return domain.DirectionAuthorizationFact{}, fmt.Errorf("read direction authorization: 读回的不是授权规则")
	}
	var directions []domain.PriceDirection
	if domain.CitedAuthorizationInForce(loaded, selection, at) {
		policyID, named := loaded.ReferenceTo(domain.PriceRuleObject)
		if named {
			directions, err = repository.pricePolicyDirectionsInForce(ctx, tenant, policyID, at)
			if err != nil {
				return domain.DirectionAuthorizationFact{}, err
			}
		}
	}
	return domain.JudgeCitedDirectionAuthorization(true, loaded, selection, at, directions)
}

func foldCitedVersion(raw []byte, closure scannedVersionClosure) (domain.CommercialVersion, domain.EffectiveInterval, error) {
	var document versionDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.CommercialVersion{}, domain.EffectiveInterval{}, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	version, err := document.version()
	if err != nil {
		return domain.CommercialVersion{}, domain.EffectiveInterval{}, err
	}
	selection := version.Effective()
	if correction, corrected, err := closure.correctionFor(version); err != nil {
		return domain.CommercialVersion{}, domain.EffectiveInterval{}, err
	} else if corrected {
		selection = correction.CorrectedInterval()
	}
	if retirement, retired, err := closure.retirementFor(version); err != nil {
		return domain.CommercialVersion{}, domain.EffectiveInterval{}, err
	} else if retired {
		closed, err := version.Retire(retirement.Reference(), retirement.RetiredAt())
		if err != nil {
			return domain.CommercialVersion{}, domain.EffectiveInterval{}, err
		}
		version = closed
	}
	return version, selection, nil
}

func (repository *CommercialPublications) pricePolicyDirectionsInForce(
	ctx context.Context,
	tenant domain.TenantID,
	policyID domain.CommercialObjectID,
	at time.Time,
) ([]domain.PriceDirection, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("read direction authorization: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT version.snapshot, price.direction, `+versionClosureColumns+`
		   FROM party_commercial.commercial_version AS version
		   JOIN party_commercial.commercial_price_policy AS price
		     ON price.tenant_id = version.tenant_id
		    AND price.object_kind = version.object_kind
		    AND price.object_id = version.object_id
		    AND price.version_label = version.version_label
		   `+versionClosureJoins("version")+`
		  WHERE version.tenant_id = $1
		    AND version.object_kind = $2
		    AND version.object_id = $3`,
		tenant.String(), uint8(domain.PriceRuleObject), policyID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("read direction authorization: %w", err)
	}
	defer rows.Close()
	var directions []domain.PriceDirection
	for rows.Next() {
		var raw []byte
		var directionWord string
		var closure scannedVersionClosure
		if err := rows.Scan(append([]any{&raw, &directionWord}, closure.targets()...)...); err != nil {
			return nil, fmt.Errorf("read direction authorization: %w", err)
		}
		version, selection, err := foldCitedVersion(raw, closure)
		if err != nil {
			return nil, fmt.Errorf("read direction authorization: %w", err)
		}
		if !domain.CitedAuthorizationInForce(version, selection, at) {
			continue
		}
		direction, known := domain.PriceDirectionNamed(directionWord)
		if !known {
			return nil, fmt.Errorf("read direction authorization: 价格政策方向 %q 不在封闭集内", directionWord)
		}
		directions = append(directions, direction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read direction authorization: %w", err)
	}
	return directions, nil
}
