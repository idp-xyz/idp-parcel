package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ParcelAssociations 按正式包裹身份列出此刻关联着它的作业实物。
// 谓词与交接对象按包裹认的那一支相同：只读判断链的当前有效版本。
type ParcelAssociations struct {
	db *bentopg.DB
}

func NewParcelAssociations(db *bentopg.DB) (*ParcelAssociations, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &ParcelAssociations{db: db}, nil
}

func (associations *ParcelAssociations) ListHandlingUnits(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
) ([]domain.HandlingUnitID, error) {
	querier, err := associations.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list handling units for parcel: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT r.intake ->> 'unit'
		   FROM node_operations.reception AS r
		  WHERE r.tenant_id = $1
		    AND r.intake IS NOT NULL
		    AND (r.intake ->> 'association') = $2
		    AND NOT EXISTS (
		        SELECT 1
		          FROM node_operations.reception AS successor
		         WHERE successor.tenant_id = r.tenant_id
		           AND successor.predecessor_version = r.judgment_version)
		  ORDER BY 1`,
		tenant.String(), parcel.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list handling units for parcel: %w", err)
	}
	defer rows.Close()
	units := []domain.HandlingUnitID{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("list handling units for parcel: %w", err)
		}
		unit, err := domain.NewHandlingUnitID(raw)
		if err != nil {
			return nil, fmt.Errorf("list handling units for parcel: %w", err)
		}
		units = append(units, unit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list handling units for parcel: %w", err)
	}
	return units, nil
}

// ListParcels 按作业实物列出此刻关联着它的正式包裹。谓词与 ListHandlingUnits 同一支判断链。
func (associations *ParcelAssociations) ListParcels(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.HandlingUnitID,
) ([]domain.ParcelAssociationReference, error) {
	querier, err := associations.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list parcels for handling unit: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT r.intake ->> 'association'
		   FROM node_operations.reception AS r
		  WHERE r.tenant_id = $1
		    AND r.intake IS NOT NULL
		    AND (r.intake ->> 'unit') = $2
		    AND (r.intake ->> 'association') IS NOT NULL
		    AND NOT EXISTS (
		        SELECT 1
		          FROM node_operations.reception AS successor
		         WHERE successor.tenant_id = r.tenant_id
		           AND successor.predecessor_version = r.judgment_version)
		  ORDER BY 1`,
		tenant.String(), unit.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list parcels for handling unit: %w", err)
	}
	defer rows.Close()
	parcels := []domain.ParcelAssociationReference{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("list parcels for handling unit: %w", err)
		}
		parcel, err := domain.NewParcelAssociationReference(raw)
		if err != nil {
			return nil, fmt.Errorf("list parcels for handling unit: %w", err)
		}
		parcels = append(parcels, parcel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list parcels for handling unit: %w", err)
	}
	return parcels, nil
}

var _ ports.CurrentParcelAssociation = (*ParcelAssociations)(nil)
