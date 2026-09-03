package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 供应商商业协议册的持久化面（票 party-commercial-context-gaps/03，0021 迁移）。写口挂在
// CommercialPublications 上与其余正文册同笔登记；读口是独立的 SupplierAgreementContents——
// 正文不进整册装载，理由见 ports.SupplierAgreementContentView。

// SaveSupplierAgreement 登记一份供应商商业协议版本的正文。撞键不覆盖：同内容是重放，异内容是
// 需要商业责任方修正的冲突。方向不入列——领域把它钉死为 BUY。
func (repository *CommercialPublications) SaveSupplierAgreement(
	ctx context.Context,
	agreement domain.SupplierAgreement,
) (ports.SupplierAgreementSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SupplierAgreementSaveOutcomeInvalid, fmt.Errorf("save supplier agreement: %w", err)
	}

	version := agreement.Version()
	startsAt, endsAt := intervalColumns(agreement.Effective())

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.supplier_agreement
			(tenant_id, object_kind, object_id, version_label,
			 supplier_party_id, legal_entity_ref, agreement_scope_ref, purchase_plan_ref,
			 effective_starts_at, effective_ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
		agreement.Supplier().String(),
		agreement.LegalEntity().String(),
		agreement.Scope().String(),
		agreement.PurchasePricingPlan().String(),
		startsAt,
		endsAt,
	)
	if err != nil {
		return ports.SupplierAgreementSaveOutcomeInvalid, fmt.Errorf("save supplier agreement: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.SupplierAgreementSaved, nil
	}

	var existing scannedSupplierAgreement
	err = executor.QueryRow(ctx,
		`SELECT supplier_party_id, legal_entity_ref, agreement_scope_ref, purchase_plan_ref,
		        effective_starts_at, effective_ends_at
		   FROM party_commercial.supplier_agreement
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
	).Scan(&existing.supplier, &existing.legalEntity, &existing.scope, &existing.purchasePlan,
		&existing.startsAt, &existing.endsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SupplierAgreementSaveOutcomeInvalid, fmt.Errorf("save supplier agreement: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.SupplierAgreementSaveOutcomeInvalid, fmt.Errorf("save supplier agreement: %w", err)
	}
	if existing.supplier == agreement.Supplier().String() &&
		existing.legalEntity == agreement.LegalEntity().String() &&
		existing.scope == agreement.Scope().String() &&
		existing.purchasePlan == agreement.PurchasePricingPlan().String() &&
		existing.startsAt.Equal(startsAt) &&
		sameOptionalTime(existing.endsAt, endsAt) {
		return ports.SupplierAgreementAlreadyRegistered, nil
	}
	return ports.SupplierAgreementContentConflict, nil
}

type scannedSupplierAgreement struct {
	supplier     string
	legalEntity  string
	scope        string
	purchasePlan string
	startsAt     time.Time
	endsAt       *time.Time
}

// SupplierAgreementContents 实现 ports.SupplierAgreementContentView：按已唯一选出的协议版本取回
// 正文。只读，判据同 CreditPolicyContents。
type SupplierAgreementContents struct {
	db *bentopg.DB
}

func NewSupplierAgreementContents(db *bentopg.DB) (*SupplierAgreementContents, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &SupplierAgreementContents{db: db}, nil
}

var _ ports.SupplierAgreementContentView = (*SupplierAgreementContents)(nil)

// LoadSupplierAgreement 取回协议正文。
//
// found=false = 正文未登记（无行）。显式租户与版本必须同一身份，否则 error 且不交内容。读回
// 的每一行都过 NewSupplierAgreement 重建，不按列直接拼结构体。交回的协议不带终止（本表不登
// 终止，见 0021 头注）。
func (repository *SupplierAgreementContents) LoadSupplierAgreement(
	ctx context.Context,
	tenant domain.TenantID,
	agreement domain.CommercialVersion,
) (domain.SupplierAgreement, bool, error) {
	none := domain.SupplierAgreement{}
	if tenant.String() == "" ||
		agreement.ObjectID().String() == "" || agreement.Version().String() == "" {
		return none, false, fmt.Errorf("load supplier agreement: tenant and agreement identity are required")
	}
	if tenant != agreement.Tenant() {
		return none, false, fmt.Errorf("load supplier agreement: tenant does not own this agreement")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load supplier agreement: %w", err)
	}

	var row scannedSupplierAgreement
	err = querier.QueryRow(ctx,
		`SELECT supplier_party_id, legal_entity_ref, agreement_scope_ref, purchase_plan_ref,
		        effective_starts_at, effective_ends_at
		   FROM party_commercial.supplier_agreement
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant.String(),
		uint8(domain.SupplierAgreementObject),
		agreement.ObjectID().String(),
		agreement.Version().String(),
	).Scan(&row.supplier, &row.legalEntity, &row.scope, &row.purchasePlan, &row.startsAt, &row.endsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load supplier agreement: %w", err)
	}

	content, err := supplierAgreementFrom(agreement, row)
	if err != nil {
		return none, false, fmt.Errorf("load supplier agreement: %w", err)
	}
	return content, true, nil
}

func supplierAgreementFrom(version domain.CommercialVersion, row scannedSupplierAgreement) (domain.SupplierAgreement, error) {
	supplier, err := domain.NewPartyID(row.supplier)
	if err != nil {
		return domain.SupplierAgreement{}, err
	}
	legalEntity, err := domain.NewLegalEntityReference(row.legalEntity)
	if err != nil {
		return domain.SupplierAgreement{}, err
	}
	scope, err := domain.NewCommercialScopeReference(row.scope)
	if err != nil {
		return domain.SupplierAgreement{}, err
	}
	purchasePlan, err := domain.NewPricingPlanReference(row.purchasePlan)
	if err != nil {
		return domain.SupplierAgreement{}, err
	}
	interval, err := intervalFrom(row.startsAt, row.endsAt)
	if err != nil {
		return domain.SupplierAgreement{}, err
	}
	return domain.NewSupplierAgreement(version, supplier, legalEntity, scope, purchasePlan, interval)
}
