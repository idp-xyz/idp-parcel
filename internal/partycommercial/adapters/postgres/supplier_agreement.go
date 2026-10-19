package postgres

import (
	"context"
	"encoding/json"
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

var _ ports.SupplierAgreementVersionView = (*SupplierAgreementContents)(nil)

// LoadSupplierAgreementVersion 按（租户、协议对象、版本标签）点读壳与正文（票 demo-chain-gaps/22）。
//
// 壳与正文由**一条语句**取回，判据同 LoadForScope：分两次读，中间若有一次发布落地，交回的壳与正文会对应一个
// 从未同时存在过的状态。壳只信快照列（列是查询投影，重建只信文档，同 versionDocument 的约定），读回后再核它
// 与所问的键是同一版——WHERE 已按键取，快照却说是另一版，那是写坏的行，不是答案。
//
// 正文只在壳已生效时交。已收尾的壳上若有正文行，那是生效期间登下的历史，照只交壳；已计划生效的壳上有正文行
// 是坏数据——发布用例对挂在已计划生效版本上的声明整项拒绝，这一行不可能经它写下。
func (repository *SupplierAgreementContents) LoadSupplierAgreementVersion(
	ctx context.Context,
	tenant domain.TenantID,
	agreement domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (ports.SupplierAgreementVersionRead, error) {
	none := ports.SupplierAgreementVersionRead{}
	if tenant.String() == "" || agreement.String() == "" || version.String() == "" {
		return none, fmt.Errorf("load supplier agreement version: tenant, agreement and version label are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("load supplier agreement version: %w", err)
	}

	var raw []byte
	var supplier, legalEntity, scope, purchasePlan *string
	var startsAt, endsAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT version.snapshot,
		        content.supplier_party_id, content.legal_entity_ref, content.agreement_scope_ref,
		        content.purchase_plan_ref, content.effective_starts_at, content.effective_ends_at
		   FROM party_commercial.commercial_version AS version
		   LEFT JOIN party_commercial.supplier_agreement AS content
		          ON content.tenant_id     = version.tenant_id
		         AND content.object_kind   = version.object_kind
		         AND content.object_id     = version.object_id
		         AND content.version_label = version.version_label
		  WHERE version.tenant_id     = $1
		    AND version.object_kind   = $2
		    AND version.object_id     = $3
		    AND version.version_label = $4`,
		tenant.String(),
		uint8(domain.SupplierAgreementObject),
		agreement.String(),
		version.String(),
	).Scan(&raw, &supplier, &legalEntity, &scope, &purchasePlan, &startsAt, &endsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, nil
	}
	if err != nil {
		return none, fmt.Errorf("load supplier agreement version: %w", err)
	}

	var document versionDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return none, fmt.Errorf("load supplier agreement version: 快照不是本适配器写下的形状：%w", err)
	}
	shell, err := document.version()
	if err != nil {
		return none, fmt.Errorf("load supplier agreement version: %w", err)
	}
	if shell.Tenant() != tenant || shell.Kind() != domain.SupplierAgreementObject ||
		shell.ObjectID() != agreement || shell.Version() != version {
		return none, fmt.Errorf("load supplier agreement version: %s/%s 的快照记的是另一版", agreement, version)
	}
	read := ports.SupplierAgreementVersionRead{Version: shell, HasVersion: true}

	// supplier_party_id 在正文表上 NOT NULL，它的在场即正文行的在场；其余正文列除区间终点外同为 NOT NULL，
	// 到这里缺一列即坏数据（判据同 ListSupplierAgreements）。
	if supplier == nil {
		return read, nil
	}
	if legalEntity == nil || scope == nil || purchasePlan == nil || startsAt == nil {
		return none, fmt.Errorf("load supplier agreement version: %s/%s 的正文行不完整", agreement, version)
	}
	switch shell.Status() {
	case domain.CommercialVersionEffective:
	case domain.CommercialVersionExpired, domain.CommercialVersionRetired, domain.CommercialVersionSuperseded:
		return read, nil
	default:
		return none, fmt.Errorf("load supplier agreement version: %s/%s 的正文挂在 %s 的壳上", agreement, version, shell.Status())
	}
	content, err := supplierAgreementFrom(shell, scannedSupplierAgreement{
		supplier:     *supplier,
		legalEntity:  *legalEntity,
		scope:        *scope,
		purchasePlan: *purchasePlan,
		startsAt:     *startsAt,
		endsAt:       endsAt,
	})
	if err != nil {
		return none, fmt.Errorf("load supplier agreement version: %w", err)
	}
	read.Agreement = content
	read.HasAgreement = true
	return read, nil
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
