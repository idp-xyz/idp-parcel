// Package postgres 是 party-commercial 自有语义端口的 PostgreSQL 适配器。
//
// 显式 SQL、行模型与冲突翻译都留在这里，不进领域对象。所有语句显式携带租户条件：
// 作用域不是过滤器而是身份的一部分（ADR-0003/0040）。
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

// CommercialPublications 实现 ports.PublicationRegistry：已发布版本的只增登记。
//
// SaveVersion 撞键不覆盖：ON CONFLICT DO NOTHING 保事务可用，零行命中后读回既有行
// 比内容——同内容是重放（已登记），异内容是需要商业责任方修正的冲突（登记册在内存
// 侧的 Register 同一套代数，这里是它的持久化面）。
type CommercialPublications struct {
	db *bentopg.DB
}

func NewCommercialPublications(db *bentopg.DB) (*CommercialPublications, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &CommercialPublications{db: db}, nil
}

// LoadForScope 按（租户+范围）读回整册。读回的每一行先过领域重建门，再经 Register 进
// 登记册——两道门互补：前者拦一行坏数据，后者拦拼出来的重复键。
//
// 版本册与各正文件由**一条语句**取回，不是各查一次。登记册的 ViewRevision 由各通道内容
// 共同派生，分两次读之间若有写入落地，派生出的修订会对应一个从未存在过的中间状态；
// ReadExecutor 不保证两条语句同处一个快照，而一条语句保证。
//
// 形态行与价格/结算政策行都以版本四元组为主键、与版本一一对应，左连接后仍是每个版本
// 一行。区间更正按 D-5 只增多条，直接左连接会放大结果集、把其余正文件登记重复进册。
// 因此更正以 LATERAL 聚成 JSON 数组挂在版本行上，登记顺序随 registration_id 保留。
func (repository *CommercialPublications) LoadForScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.CommercialScopeReference,
) (*domain.CommercialRegistry, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load publication registry: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT version.snapshot, product.form, correction.items,
		        price.direction, price.plan_ref, price.plan_direction, price.binding_conversion,
		        price.policy_scope_ref, price.effective_starts_at, price.effective_ends_at,
		        settlement.method, settlement.legal_entity_ref, settlement.counterparty_ref,
		        settlement.contract_label, settlement.charge_scope_ref, settlement.currency_code,
		        settlement.effective_starts_at, settlement.effective_ends_at
		   FROM party_commercial.commercial_version AS version
		   LEFT JOIN party_commercial.service_product_form AS product
		          ON product.tenant_id     = version.tenant_id
		         AND product.object_kind   = version.object_kind
		         AND product.object_id     = version.object_id
		         AND product.version_label = version.version_label
		   LEFT JOIN party_commercial.commercial_price_policy AS price
		          ON price.tenant_id     = version.tenant_id
		         AND price.object_kind   = version.object_kind
		         AND price.object_id     = version.object_id
		         AND price.version_label = version.version_label
		   LEFT JOIN party_commercial.commercial_settlement_policy AS settlement
		          ON settlement.tenant_id     = version.tenant_id
		         AND settlement.object_kind   = version.object_kind
		         AND settlement.object_id     = version.object_id
		         AND settlement.version_label = version.version_label
		   LEFT JOIN LATERAL (
		        SELECT COALESCE(
		                   json_agg(
		                       json_build_object(
		                           'startsAt', c.corrected_starts_at,
		                           'endsAt', c.corrected_ends_at,
		                           'reference', c.correction_ref,
		                           'correctedAt', c.corrected_at
		                       )
		                       ORDER BY c.registration_id
		                   ),
		                   '[]'::json
		               ) AS items
		          FROM party_commercial.commercial_validity_correction AS c
		         WHERE c.tenant_id     = version.tenant_id
		           AND c.object_kind   = version.object_kind
		           AND c.object_id     = version.object_id
		           AND c.version_label = version.version_label
		   ) AS correction ON TRUE
		  WHERE version.tenant_id = $1
		    AND version.scope_ref = $2
		  ORDER BY version.object_kind, version.object_id, version.version_label`,
		tenant.String(),
		scope.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load publication registry: %w", err)
	}
	defer rows.Close()

	registry := domain.NewCommercialRegistry()
	for rows.Next() {
		var raw []byte
		var rawForm *string
		var rawCorrections []byte
		var price scannedPricePolicy
		var settlement scannedSettlementPolicy
		if err := rows.Scan(
			&raw, &rawForm, &rawCorrections,
			&price.direction, &price.planRef, &price.planDirection, &price.conversion,
			&price.scope, &price.startsAt, &price.endsAt,
			&settlement.method, &settlement.legalEntity, &settlement.counterparty,
			&settlement.contract, &settlement.chargeScope, &settlement.currency,
			&settlement.startsAt, &settlement.endsAt,
		); err != nil {
			return nil, fmt.Errorf("load publication registry: %w", err)
		}
		var document versionDocument
		if err := json.Unmarshal(raw, &document); err != nil {
			return nil, fmt.Errorf("load publication registry: 快照不是本适配器写下的形状：%w", err)
		}
		version, err := document.version()
		if err != nil {
			return nil, fmt.Errorf("load publication registry: %w", err)
		}
		if _, err := registry.Register(version); err != nil {
			return nil, fmt.Errorf("load publication registry: %w", err)
		}
		if rawForm != nil {
			if err := registerServiceProduct(registry, version, *rawForm); err != nil {
				return nil, fmt.Errorf("load publication registry: %w", err)
			}
		}
		if err := registerValidityCorrections(registry, version, rawCorrections); err != nil {
			return nil, fmt.Errorf("load publication registry: %w", err)
		}
		if err := registerPricePolicy(registry, version, price); err != nil {
			return nil, fmt.Errorf("load publication registry: %w", err)
		}
		if err := registerSettlementPolicy(registry, version, settlement); err != nil {
			return nil, fmt.Errorf("load publication registry: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load publication registry: %w", err)
	}
	return registry, nil
}

// SaveVersion 登记一份已发布版本。
func (repository *CommercialPublications) SaveVersion(
	ctx context.Context,
	version domain.CommercialVersion,
) (ports.PublicationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PublicationSaveOutcomeInvalid, fmt.Errorf("save commercial version: %w", err)
	}

	document := documentOfVersion(version)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.PublicationSaveOutcomeInvalid, fmt.Errorf("save commercial version: %w", err)
	}
	var endsAt *time.Time
	if end, bounded := version.Effective().EndsAt(); bounded {
		utc := end.UTC()
		endsAt = &utc
	}
	publishedAt, _ := version.PublishedAt()

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.commercial_version
			(tenant_id, object_kind, object_id, version_label,
			 scope_ref, content_digest, effective_starts_at, effective_ends_at,
			 status, snapshot, published_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
		version.Scope().String(),
		version.ContentDigest().String(),
		version.Effective().StartsAt().UTC(),
		endsAt,
		uint8(version.Status()),
		raw,
		publishedAt.UTC(),
	)
	if err != nil {
		return ports.PublicationSaveOutcomeInvalid, fmt.Errorf("save commercial version: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.PublicationSaved, nil
	}

	// 撞键：读回既有行判重放还是冲突。判据与登记册 Register 同源——内容摘要、范围
	// 与原区间任一不同即是另一份内容（生命周期位置刻意不算，重登一个后来生效的版本
	// 不是冲突）。
	var existingDigest, existingScope string
	var existingStarts time.Time
	var existingEnds *time.Time
	err = executor.QueryRow(ctx,
		`SELECT content_digest, scope_ref, effective_starts_at, effective_ends_at
		   FROM party_commercial.commercial_version
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
	).Scan(&existingDigest, &existingScope, &existingStarts, &existingEnds)
	if errors.Is(err, pgx.ErrNoRows) {
		// 撞键后行又不见了：并发删除在本表不存在，这是库或适配器的 bug。
		return ports.PublicationSaveOutcomeInvalid, fmt.Errorf("save commercial version: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.PublicationSaveOutcomeInvalid, fmt.Errorf("save commercial version: %w", err)
	}

	sameEnds := (existingEnds == nil && endsAt == nil) ||
		(existingEnds != nil && endsAt != nil && existingEnds.Equal(*endsAt))
	if existingDigest == version.ContentDigest().String() &&
		existingScope == version.Scope().String() &&
		existingStarts.Equal(version.Effective().StartsAt().UTC()) &&
		sameEnds {
		return ports.PublicationAlreadyRegistered, nil
	}
	return ports.PublicationContentConflict, nil
}

// versionDocument 是快照列里的文档形状——RehydrateCommercialVersionSpec 的 JSON
// 表达。键与链查列（摘要/区间/状态）在文档里仍整份保留：列是查询投影，文档是重建
// 来源，重建只信文档一处。
type versionDocument struct {
	TenantID      string              `json:"tenantId"`
	Kind          uint8               `json:"kind"`
	ObjectID      string              `json:"objectId"`
	Version       string              `json:"version"`
	Scope         string              `json:"scope"`
	ContentDigest string              `json:"contentDigest"`
	StartsAt      time.Time           `json:"startsAt"`
	EndsAt        *time.Time          `json:"endsAt,omitempty"`
	Status        uint8               `json:"status"`
	Approval      approvalDocument    `json:"approval"`
	PublishedAt   time.Time           `json:"publishedAt"`
	EffectiveAt   *time.Time          `json:"effectiveAt,omitempty"`
	ClosedAt      *time.Time          `json:"closedAt,omitempty"`
	RetirementRef string              `json:"retirementRef,omitempty"`
	Successor     string              `json:"successor,omitempty"`
	References    []referenceDocument `json:"references,omitempty"`
}

type approvalDocument struct {
	Reference  string    `json:"reference"`
	Source     string    `json:"source"`
	ApprovedAt time.Time `json:"approvedAt"`
}

type referenceDocument struct {
	Kind     uint8  `json:"kind"`
	ObjectID string `json:"objectId"`
}

func documentOfVersion(version domain.CommercialVersion) versionDocument {
	document := versionDocument{
		TenantID:      version.Tenant().String(),
		Kind:          uint8(version.Kind()),
		ObjectID:      version.ObjectID().String(),
		Version:       version.Version().String(),
		Scope:         version.Scope().String(),
		ContentDigest: version.ContentDigest().String(),
		StartsAt:      version.Effective().StartsAt().UTC(),
		Status:        uint8(version.Status()),
	}
	if end, bounded := version.Effective().EndsAt(); bounded {
		utc := end.UTC()
		document.EndsAt = &utc
	}
	if approval, approved := version.ApprovalBasis(); approved {
		document.Approval = approvalDocument{
			Reference:  approval.Reference().String(),
			Source:     approval.Source().String(),
			ApprovedAt: approval.ApprovedAt().UTC(),
		}
	}
	if publishedAt, published := version.PublishedAt(); published {
		document.PublishedAt = publishedAt.UTC()
	}
	if effectiveAt, effective := version.EffectiveAt(); effective {
		utc := effectiveAt.UTC()
		document.EffectiveAt = &utc
	}
	if closedAt, closed := version.ClosedAt(); closed {
		utc := closedAt.UTC()
		document.ClosedAt = &utc
	}
	if retirement, retired := version.RetirementReference(); retired {
		document.RetirementRef = retirement.String()
	}
	if successor, superseded := version.Successor(); superseded {
		document.Successor = successor.String()
	}
	for _, reference := range version.DeclaredReferences() {
		document.References = append(document.References, referenceDocument{
			Kind:     uint8(reference.Kind()),
			ObjectID: reference.ObjectID().String(),
		})
	}
	return document
}

func (document versionDocument) version() (domain.CommercialVersion, error) {
	spec := domain.RehydrateCommercialVersionSpec{
		Kind:        domain.CommercialObjectKind(document.Kind),
		Status:      domain.CommercialVersionStatus(document.Status),
		PublishedAt: document.PublishedAt,
	}
	var err error
	if spec.TenantID, err = domain.NewTenantID(document.TenantID); err != nil {
		return domain.CommercialVersion{}, err
	}
	if spec.ObjectID, err = domain.NewCommercialObjectID(document.ObjectID); err != nil {
		return domain.CommercialVersion{}, err
	}
	if spec.Version, err = domain.NewCommercialVersionLabel(document.Version); err != nil {
		return domain.CommercialVersion{}, err
	}
	if spec.Scope, err = domain.NewCommercialScopeReference(document.Scope); err != nil {
		return domain.CommercialVersion{}, err
	}
	if spec.ContentDigest, err = domain.NewCommercialContentDigest(document.ContentDigest); err != nil {
		return domain.CommercialVersion{}, err
	}
	endsAt := time.Time{}
	if document.EndsAt != nil {
		endsAt = *document.EndsAt
	}
	if spec.Effective, err = domain.NewEffectiveInterval(document.StartsAt, endsAt); err != nil {
		return domain.CommercialVersion{}, err
	}

	approvalReference, err := domain.NewApprovalReference(document.Approval.Reference)
	if err != nil {
		return domain.CommercialVersion{}, err
	}
	approvalSource, err := domain.NewCommercialSourceReference(document.Approval.Source)
	if err != nil {
		return domain.CommercialVersion{}, err
	}
	if spec.Approval, err = domain.NewApprovalBasis(approvalReference, approvalSource, document.Approval.ApprovedAt); err != nil {
		return domain.CommercialVersion{}, err
	}

	if document.EffectiveAt != nil {
		spec.EffectiveAt = *document.EffectiveAt
	}
	if document.ClosedAt != nil {
		spec.ClosedAt = *document.ClosedAt
	}
	if document.RetirementRef != "" {
		if spec.RetirementRef, err = domain.NewRetirementReference(document.RetirementRef); err != nil {
			return domain.CommercialVersion{}, err
		}
	}
	if document.Successor != "" {
		if spec.Successor, err = domain.NewCommercialVersionLabel(document.Successor); err != nil {
			return domain.CommercialVersion{}, err
		}
	}
	if len(document.References) > 0 {
		spec.References = make(map[domain.CommercialObjectKind]domain.CommercialObjectID, len(document.References))
		for _, reference := range document.References {
			objectID, err := domain.NewCommercialObjectID(reference.ObjectID)
			if err != nil {
				return domain.CommercialVersion{}, err
			}
			spec.References[domain.CommercialObjectKind(reference.Kind)] = objectID
		}
	}
	return domain.RehydrateCommercialVersion(spec)
}
