package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// PublicationBases 记下价卡发布的不透明依据。已记下的不同依据不改。
type PublicationBases struct {
	db *bentopg.DB
}

func NewPublicationBases(db *bentopg.DB) (*PublicationBases, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &PublicationBases{db: db}, nil
}

var _ ports.PublicationBasisRecord = (*PublicationBases)(nil)

func (records *PublicationBases) SavePublicationBasis(ctx context.Context, tenant domain.TenantID, planID, planVersion, basis string) error {
	executor, err := records.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save publication basis: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.price_card_publication_basis (tenant_id, plan_id, plan_version, basis)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), planID, planVersion, basis,
	)
	if err != nil {
		return fmt.Errorf("save publication basis: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var existing string
	err = executor.QueryRow(ctx,
		`SELECT basis FROM parcel_pricing.price_card_publication_basis
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		tenant.String(), planID, planVersion,
	).Scan(&existing)
	if err != nil {
		return fmt.Errorf("save publication basis: %w", err)
	}
	if existing == basis {
		return nil
	}
	return fmt.Errorf("save publication basis: recorded basis %s differs from %s", existing, basis)
}

// ReadPublicationBasis 读这一版发布时记下的不透明依据。没有行就是没记下，不是错误。
func (records *PublicationBases) ReadPublicationBasis(ctx context.Context, tenant domain.TenantID, planID, planVersion string) (string, bool, error) {
	querier, err := records.db.ReadExecutor(ctx)
	if err != nil {
		return "", false, fmt.Errorf("read publication basis: %w", err)
	}
	basis, found, err := ReadPublicationBasis(ctx, querier, tenant.String(), planID, planVersion)
	if err != nil {
		return "", false, fmt.Errorf("read publication basis: %w", err)
	}
	return basis, found, nil
}

func ReadPublicationBasis(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tenant, planID, planVersion string) (string, bool, error) {
	var basis string
	err := querier.QueryRow(ctx,
		`SELECT basis FROM parcel_pricing.price_card_publication_basis
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		tenant, planID, planVersion,
	).Scan(&basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return basis, true, nil
}
