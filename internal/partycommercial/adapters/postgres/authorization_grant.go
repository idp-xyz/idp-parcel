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

// AuthorityGrants 实现 ports.AuthorityGrantStore：授权治理册的只增登记与按范围+时点装载。
type AuthorityGrants struct {
	db *bentopg.DB
}

func NewAuthorityGrants(db *bentopg.DB) (*AuthorityGrants, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &AuthorityGrants{db: db}, nil
}

var _ ports.AuthorityGrantStore = (*AuthorityGrants)(nil)

// LoadEffectiveGrants 取回该租户该范围在业务时点仍管得着的授权。空切片不是错误：
// 交给 Authorize 译`未配置`。
func (repository *AuthorityGrants) LoadEffectiveGrants(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.CommercialScopeReference,
	at time.Time,
) ([]domain.AuthorityGrant, error) {
	if tenant.String() == "" || scope.String() == "" || at.IsZero() {
		return nil, fmt.Errorf("load effective grants: tenant, scope and as-of time are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load effective grants: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT snapshot
		   FROM party_commercial.authorization_grant
		  WHERE tenant_id = $1
		    AND scope_ref = $2
		    AND effective_starts_at <= $3
		    AND (effective_ends_at IS NULL OR effective_ends_at > $3)
		  ORDER BY object_id, version_label`,
		tenant.String(),
		scope.String(),
		at.UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("load effective grants: %w", err)
	}
	defer rows.Close()

	var grants []domain.AuthorityGrant
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("load effective grants: %w", err)
		}
		grant, err := grantFromSnapshot(raw)
		if err != nil {
			return nil, fmt.Errorf("load effective grants: %w", err)
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load effective grants: %w", err)
	}
	return grants, nil
}

// SaveGrant 登记一条已生效授权。撞键不覆盖：同内容重放，异内容冲突。
func (repository *AuthorityGrants) SaveGrant(
	ctx context.Context,
	grant domain.AuthorityGrant,
) (ports.GrantSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.GrantSaveOutcomeInvalid, fmt.Errorf("save authority grant: %w", err)
	}

	document := documentOfGrant(grant)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.GrantSaveOutcomeInvalid, fmt.Errorf("save authority grant: %w", err)
	}
	digest := sha256.Sum256(raw)
	contentDigest := hex.EncodeToString(digest[:])

	var endsAt *time.Time
	if end, bounded := grant.Effective().EndsAt(); bounded {
		utc := end.UTC()
		endsAt = &utc
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.authorization_grant
			(tenant_id, object_id, version_label,
			 action, legal_entity_ref, authority_level, scope_ref,
			 effective_starts_at, effective_ends_at, content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		grant.Version().Tenant().String(),
		grant.Version().ObjectID().String(),
		grant.Version().Version().String(),
		grant.Action().String(),
		grant.LegalEntity().String(),
		grant.Level().String(),
		grant.Scope().String(),
		grant.Effective().StartsAt().UTC(),
		endsAt,
		contentDigest,
		raw,
	)
	if err != nil {
		return ports.GrantSaveOutcomeInvalid, fmt.Errorf("save authority grant: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.GrantSaved, nil
	}

	var existingDigest string
	err = executor.QueryRow(ctx,
		`SELECT content_digest
		   FROM party_commercial.authorization_grant
		  WHERE tenant_id = $1 AND object_id = $2 AND version_label = $3`,
		grant.Version().Tenant().String(),
		grant.Version().ObjectID().String(),
		grant.Version().Version().String(),
	).Scan(&existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.GrantSaveOutcomeInvalid, fmt.Errorf("save authority grant: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.GrantSaveOutcomeInvalid, fmt.Errorf("save authority grant: %w", err)
	}
	if existingDigest == contentDigest {
		return ports.GrantAlreadyRegistered, nil
	}
	return ports.GrantContentConflict, nil
}

type grantDocument struct {
	Version     versionDocument `json:"version"`
	Action      string          `json:"action"`
	LegalEntity string          `json:"legalEntity"`
	Level       string          `json:"level"`
	Scope       string          `json:"scope"`
	StartsAt    time.Time       `json:"startsAt"`
	EndsAt      *time.Time      `json:"endsAt,omitempty"`
}

func documentOfGrant(grant domain.AuthorityGrant) grantDocument {
	document := grantDocument{
		Version:     documentOfVersion(grant.Version()),
		Action:      grant.Action().String(),
		LegalEntity: grant.LegalEntity().String(),
		Level:       grant.Level().String(),
		Scope:       grant.Scope().String(),
		StartsAt:    grant.Effective().StartsAt().UTC(),
	}
	if end, bounded := grant.Effective().EndsAt(); bounded {
		utc := end.UTC()
		document.EndsAt = &utc
	}
	return document
}

func grantFromSnapshot(raw []byte) (domain.AuthorityGrant, error) {
	var document grantDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.AuthorityGrant{}, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	version, err := document.Version.version()
	if err != nil {
		return domain.AuthorityGrant{}, err
	}
	action, err := authorizedActionFrom(document.Action)
	if err != nil {
		return domain.AuthorityGrant{}, err
	}
	legalEntity, err := domain.NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return domain.AuthorityGrant{}, err
	}
	level, err := domain.NewAuthorityLevel(document.Level)
	if err != nil {
		return domain.AuthorityGrant{}, err
	}
	scope, err := domain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return domain.AuthorityGrant{}, err
	}
	endsAt := time.Time{}
	if document.EndsAt != nil {
		endsAt = *document.EndsAt
	}
	effective, err := domain.NewEffectiveInterval(document.StartsAt, endsAt)
	if err != nil {
		return domain.AuthorityGrant{}, err
	}
	return domain.NewAuthorityGrant(version, action, legalEntity, level, scope, effective)
}

// authorizedActionFrom 是 domain.AuthorizedAction 的名字镜像，default 报错不吸收——库上 CHECK 已经
// 钉死封闭集，读回集外取值即库与领域分叉。领域加格时这份名单要跟（pc-gaps/08、/13 各加过一次）。
func authorizedActionFrom(raw string) (domain.AuthorizedAction, error) {
	for _, action := range []domain.AuthorizedAction{
		domain.ManualReviewAction,
		domain.ActiveRejectionAction,
		domain.SourceDataAmendmentAction,
		domain.ControlledClosureAction,
		domain.ReopeningAction,
	} {
		if action.String() == raw {
			return action, nil
		}
	}
	return domain.AuthorizedActionInvalid, fmt.Errorf("unknown authorized action %q", raw)
}
