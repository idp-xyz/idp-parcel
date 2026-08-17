package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// CommercialResolutions 实现 ports.CommercialResolutionStore：按标识固定并取回一次解析。
//
// Save 撞键不覆盖：ON CONFLICT DO NOTHING 保事务可用，零行命中后读回摘要——同内容是
// 重放，异内容是冲突。
type CommercialResolutions struct {
	db *bentopg.DB
}

func NewCommercialResolutions(db *bentopg.DB) (*CommercialResolutions, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &CommercialResolutions{db: db}, nil
}

var _ ports.CommercialResolutionStore = (*CommercialResolutions)(nil)

func (repository *CommercialResolutions) LoadResolution(
	ctx context.Context,
	tenant domain.TenantID,
	resolution domain.ResolutionID,
) (domain.CommercialClosure, bool, error) {
	if tenant.String() == "" || resolution.String() == "" {
		return domain.CommercialClosure{}, false, fmt.Errorf("load commercial resolution: tenant and resolution ID are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CommercialClosure{}, false, fmt.Errorf("load commercial resolution: %w", err)
	}

	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT snapshot
		   FROM party_commercial.commercial_resolution
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		tenant.String(),
		resolution.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CommercialClosure{}, false, nil
	}
	if err != nil {
		return domain.CommercialClosure{}, false, fmt.Errorf("load commercial resolution: %w", err)
	}

	var document closureDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.CommercialClosure{}, false, fmt.Errorf("load commercial resolution: 快照不是本适配器写下的形状：%w", err)
	}
	closure, err := document.closure()
	if err != nil {
		return domain.CommercialClosure{}, false, fmt.Errorf("load commercial resolution: %w", err)
	}
	return closure, true, nil
}

func (repository *CommercialResolutions) Save(
	ctx context.Context,
	closure domain.CommercialClosure,
) (ports.ResolutionSaveOutcome, error) {
	if closure.ResolutionID().String() == "" || closure.Outcome() != domain.UniquelyResolved {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: uniquely resolved closure with ID is required")
	}
	key := closure.ResolutionKey()
	if key.TenantID.String() == "" {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: tenant is required")
	}

	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: %w", err)
	}

	document := documentOfClosure(closure)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: %w", err)
	}
	digest := sha256.Sum256(raw)
	contentDigest := hex.EncodeToString(digest[:])

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.commercial_resolution
			(tenant_id, resolution_id, customer_account_id, outcome, content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(),
		closure.ResolutionID().String(),
		key.CustomerAccountID.String(),
		uint8(closure.Outcome()),
		contentDigest,
		raw,
	)
	if err != nil {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.ResolutionSaved, nil
	}

	var existingDigest string
	err = executor.QueryRow(ctx,
		`SELECT content_digest
		   FROM party_commercial.commercial_resolution
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		key.TenantID.String(),
		closure.ResolutionID().String(),
	).Scan(&existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.ResolutionSaveOutcomeInvalid, fmt.Errorf("save commercial resolution: %w", err)
	}
	if existingDigest == contentDigest {
		return ports.ResolutionAlreadyRecorded, nil
	}
	return ports.ResolutionContentConflict, nil
}

type closureDocument struct {
	Outcome      uint8             `json:"outcome"`
	ResolutionID string            `json:"resolutionId"`
	TenantID     string            `json:"tenantId"`
	Customer     string            `json:"customerAccountId"`
	LegalEntity  string            `json:"legalEntity"`
	Scope        string            `json:"scope"`
	Purpose      uint8             `json:"purpose"`
	Direction    uint8             `json:"priceDirection"`
	AnchorAt     time.Time         `json:"anchorAt"`
	AnchorPolicy string            `json:"anchorPolicy"`
	ViewRevision string            `json:"viewRevision"`
	Adopted      []adoptedDocument `json:"adopted"`
}

type adoptedDocument struct {
	Kind    uint8           `json:"kind"`
	Version versionDocument `json:"version"`
	// Form 只在采用了服务产品时出现（ADR-0050）。omitempty：既有不含形态的快照
	// 读回仍是缺席，不发明 NETWORK_SERVICE。
	Form string `json:"form,omitempty"`
}

func documentOfClosure(closure domain.CommercialClosure) closureDocument {
	key := closure.ResolutionKey()
	document := closureDocument{
		Outcome:      uint8(closure.Outcome()),
		ResolutionID: closure.ResolutionID().String(),
		TenantID:     key.TenantID.String(),
		Customer:     key.CustomerAccountID.String(),
		LegalEntity:  key.LegalEntityCandidate.String(),
		Scope:        key.Scope.String(),
		Purpose:      uint8(key.Purpose),
		Direction:    uint8(key.PriceDirection),
		AnchorAt:     key.Anchor.At().UTC(),
		AnchorPolicy: key.Anchor.PolicyVersion().String(),
	}
	if revision, ok := closure.ViewRevision(); ok {
		document.ViewRevision = revision.String()
	}
	for _, adopted := range closure.Adopted() {
		item := adoptedDocument{
			Kind:    uint8(adopted.Kind()),
			Version: documentOfVersion(adopted.Version()),
		}
		if product, ok := adopted.ServiceProduct(); ok {
			item.Form = product.Form().String()
		}
		document.Adopted = append(document.Adopted, item)
	}
	return document
}

func (document closureDocument) closure() (domain.CommercialClosure, error) {
	resolutionID, err := domain.NewResolutionID(document.ResolutionID)
	if err != nil {
		return domain.CommercialClosure{}, err
	}
	anchorPolicy, err := domain.NewAnchorPolicyVersion(document.AnchorPolicy)
	if err != nil {
		return domain.CommercialClosure{}, err
	}
	anchor, err := domain.NewSelectionAnchor(document.AnchorAt, anchorPolicy)
	if err != nil {
		return domain.CommercialClosure{}, err
	}
	viewRevision, err := domain.NewAuthorityViewRevision(document.ViewRevision)
	if err != nil {
		return domain.CommercialClosure{}, err
	}

	key := domain.ClosureResolutionKey{
		Purpose:        domain.ResolutionPurpose(document.Purpose),
		PriceDirection: domain.PriceDirection(document.Direction),
		Anchor:         anchor,
	}
	if key.TenantID, err = domain.NewTenantID(document.TenantID); err != nil {
		return domain.CommercialClosure{}, err
	}
	if key.CustomerAccountID, err = domain.NewCustomerAccountID(document.Customer); err != nil {
		return domain.CommercialClosure{}, err
	}
	if key.LegalEntityCandidate, err = domain.NewLegalEntityReference(document.LegalEntity); err != nil {
		return domain.CommercialClosure{}, err
	}
	if key.Scope, err = domain.NewCommercialScopeReference(document.Scope); err != nil {
		return domain.CommercialClosure{}, err
	}

	adopted := make([]domain.RehydrateAdoptedBasisSpec, 0, len(document.Adopted))
	bases := make([]domain.CommercialObjectKind, 0, len(document.Adopted))
	for _, item := range document.Adopted {
		version, err := item.Version.version()
		if err != nil {
			return domain.CommercialClosure{}, err
		}
		kind := domain.CommercialObjectKind(item.Kind)
		spec := domain.RehydrateAdoptedBasisSpec{Kind: kind, Version: version}
		if item.Form != "" {
			product, err := rehydrateServiceProduct(version, item.Form)
			if err != nil {
				return domain.CommercialClosure{}, err
			}
			spec.ServiceProduct = product
			spec.HasServiceProduct = true
		}
		adopted = append(adopted, spec)
		bases = append(bases, kind)
	}
	key.RequiredBases = bases

	return domain.RehydrateCommercialClosure(domain.RehydrateCommercialClosureSpec{
		Outcome:      domain.ResolutionOutcome(document.Outcome),
		ResolutionID: resolutionID,
		Key:          key,
		Anchor:       anchor,
		ViewRevision: viewRevision,
		Adopted:      adopted,
	})
}

// rehydrateServiceProduct 把快照里的形态字符串译回产品。未知取值响亮失败，不吸收成
// NETWORK_SERVICE——那正是 ADR-0050 要堵的默认值。
func rehydrateServiceProduct(version domain.CommercialVersion, form string) (domain.ServiceProduct, error) {
	var parsed domain.ServiceProductForm
	switch form {
	case domain.NetworkServiceForm.String():
		parsed = domain.NetworkServiceForm
	default:
		return domain.ServiceProduct{}, fmt.Errorf("load commercial resolution: 无法翻译的服务形态 %q", form)
	}
	product, err := domain.NewServiceProduct(version, parsed)
	if err != nil {
		return domain.ServiceProduct{}, fmt.Errorf("load commercial resolution: %w", err)
	}
	return product, nil
}
