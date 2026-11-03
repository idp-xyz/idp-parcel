package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// SupplierAgreementScopes 实现 ports.SupplierAgreementScopeView：按两段式指称读供应商协议正文册上的适用范围
// （agreement_scope_ref，0021 迁移）。只读。
type SupplierAgreementScopes struct {
	db *bentopg.DB
}

func NewSupplierAgreementScopes(db *bentopg.DB) (*SupplierAgreementScopes, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &SupplierAgreementScopes{db: db}, nil
}

var _ ports.SupplierAgreementScopeView = (*SupplierAgreementScopes)(nil)

// LoadSupplierAgreementScope 取回那一版协议的适用范围。指称怎么比、歧义怎么报，见 loadQualifiedSupplierAgreement。
func (repository *SupplierAgreementScopes) LoadSupplierAgreementScope(
	ctx context.Context,
	tenant domain.TenantID,
	agreement domain.CommercialVersionLabel,
) (domain.CommercialScopeReference, bool, error) {
	none := domain.CommercialScopeReference{}
	row, found, err := loadQualifiedSupplierAgreement(ctx, repository.db, tenant, agreement)
	if err != nil {
		return none, false, fmt.Errorf("load supplier agreement scope: %w", err)
	}
	if !found {
		return none, false, nil
	}
	scope, err := domain.NewCommercialScopeReference(row.scope)
	if err != nil {
		return none, false, fmt.Errorf("load supplier agreement scope: %w", err)
	}
	return scope, true, nil
}

// SupplierAgreementSuppliers 实现 ports.SupplierAgreementSupplierView：按两段式指称读供应商协议正文册上的供应商
// （supplier_party_id，0021 迁移）。只读。
type SupplierAgreementSuppliers struct {
	db *bentopg.DB
}

func NewSupplierAgreementSuppliers(db *bentopg.DB) (*SupplierAgreementSuppliers, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &SupplierAgreementSuppliers{db: db}, nil
}

var _ ports.SupplierAgreementSupplierView = (*SupplierAgreementSuppliers)(nil)

// LoadSupplierAgreementSupplier 取回那一版协议的供应商。指称怎么比、歧义怎么报，与范围那一问同一处
// （loadQualifiedSupplierAgreement）。
func (repository *SupplierAgreementSuppliers) LoadSupplierAgreementSupplier(
	ctx context.Context,
	tenant domain.TenantID,
	agreement domain.CommercialVersionLabel,
) (domain.PartyID, bool, error) {
	none := domain.PartyID{}
	row, found, err := loadQualifiedSupplierAgreement(ctx, repository.db, tenant, agreement)
	if err != nil {
		return none, false, fmt.Errorf("load supplier agreement supplier: %w", err)
	}
	if !found {
		return none, false, nil
	}
	supplier, err := domain.NewPartyID(row.supplier)
	if err != nil {
		return none, false, fmt.Errorf("load supplier agreement supplier: %w", err)
	}
	return supplier, true, nil
}

// qualifiedSupplierAgreementRow 是按指称比中的那一版协议正文上，两只窄读口各取一格的那两列。
type qualifiedSupplierAgreementRow struct {
	scope    string
	supplier string
}

// loadQualifiedSupplierAgreement 按两段式指称找那一版协议的正文行，范围与供应商两问共用这一处比法。
//
// 不拆指称串：拆就得在这里再写一遍分隔符，而「对象/版本」只许 NewQualifiedVersionLabel 一处拼（QualifiedLabel 头注）。
// SQL 只按「对象标识是指称的前缀」收窄，每一行再用那一处拼出自己的指称去比，比中才算。对象标识或版本里自带斜杠时
// 一个指称可能比中两行，那是两段式指称本身的歧义，响亮报错（包 ports.ErrSupplierAgreementLabelAmbiguous），不挑一行。
func loadQualifiedSupplierAgreement(
	ctx context.Context,
	db *bentopg.DB,
	tenant domain.TenantID,
	agreement domain.CommercialVersionLabel,
) (qualifiedSupplierAgreementRow, bool, error) {
	none := qualifiedSupplierAgreementRow{}
	if tenant.String() == "" || agreement.String() == "" {
		return none, false, fmt.Errorf("tenant and agreement label are required")
	}
	querier, err := db.ReadExecutor(ctx)
	if err != nil {
		return none, false, err
	}
	rows, err := querier.Query(ctx,
		`SELECT object_id, version_label, agreement_scope_ref, supplier_party_id
		   FROM party_commercial.supplier_agreement
		  WHERE tenant_id = $1 AND object_kind = $2 AND starts_with($3, object_id)`,
		tenant.String(), uint8(domain.SupplierAgreementObject), agreement.String(),
	)
	if err != nil {
		return none, false, err
	}
	defer rows.Close()

	var matched []qualifiedSupplierAgreementRow
	for rows.Next() {
		var objectText, versionText string
		var row qualifiedSupplierAgreementRow
		if err := rows.Scan(&objectText, &versionText, &row.scope, &row.supplier); err != nil {
			return none, false, err
		}
		object, err := domain.NewCommercialObjectID(objectText)
		if err != nil {
			return none, false, err
		}
		version, err := domain.NewCommercialVersionLabel(versionText)
		if err != nil {
			return none, false, err
		}
		label, err := domain.NewQualifiedVersionLabel(object, version)
		if err != nil {
			return none, false, err
		}
		if label == agreement {
			matched = append(matched, row)
		}
	}
	if err := rows.Err(); err != nil {
		return none, false, err
	}

	switch len(matched) {
	case 0:
		return none, false, nil
	case 1:
		return matched[0], true, nil
	default:
		return none, false, fmt.Errorf("%w: label %q names %d agreement versions",
			ports.ErrSupplierAgreementLabelAmbiguous, agreement, len(matched))
	}
}
