package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// LoadLatestVersion 实现 ports.ReferenceSeriesLatestVersionLoader：取（租户、序列）最近登记的一版，
// 按登记时刻、同刻按版本号字典序，不看复核——来源连接器要接的是册上最完整的一版（每一版都整版
// 重述），不是在用的那一版。读回走 rehydrateRegisteredSeries 同一道门，与 LoadVersion 一处口径。
func (register *ReferenceSeriesVersions) LoadLatestVersion(
	ctx context.Context,
	tenant domain.TenantID,
	seriesID string,
) (domain.ReferenceSeriesRegistration, bool, error) {
	if tenant.String() == "" || seriesID == "" {
		return domain.ReferenceSeriesRegistration{}, false, fmt.Errorf(
			"load latest reference series version: tenant and series are required")
	}
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ReferenceSeriesRegistration{}, false, fmt.Errorf("load latest reference series version: %w", err)
	}

	var version, kind, grade, canonicalization, digest string
	var snapshot []byte
	err = querier.QueryRow(ctx,
		`SELECT series_version, kind, evidence_grade, canonicalization, content_digest, snapshot
		   FROM parcel_pricing.reference_series_version
		  WHERE tenant_id = $1 AND series_id = $2
		  ORDER BY registered_at DESC, series_version DESC
		  LIMIT 1`,
		tenant.String(), seriesID,
	).Scan(&version, &kind, &grade, &canonicalization, &digest, &snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReferenceSeriesRegistration{}, false, nil
	}
	if err != nil {
		return domain.ReferenceSeriesRegistration{}, false, fmt.Errorf("load latest reference series version: %w", err)
	}

	registration, err := rehydrateRegisteredSeries(snapshot, registeredSeriesColumns{
		tenant: tenant, seriesID: seriesID, seriesVersion: version,
		kind: kind, grade: grade, canonicalization: canonicalization, digest: digest,
	})
	if err != nil {
		return domain.ReferenceSeriesRegistration{}, false, fmt.Errorf("load latest reference series version: %w", err)
	}
	return registration, true, nil
}

var _ ports.ReferenceSeriesLatestVersionLoader = (*ReferenceSeriesVersions)(nil)
