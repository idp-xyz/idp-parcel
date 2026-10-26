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
//
// 授予先折所挂授权规则版本的收尾再交出（ADR-0261 决定一）：已退役那一版的授予不交出，壳上有区间更正的按最新一条的
// 终点收窄、截止点及之后不交出。折法在领域一处（AuthorityGrant.NarrowedBy / EndedBy / InForceAt），这里只取行与折进。
// 授予册不带对象类别列（它只收授权规则），连收尾两册时补上那一格；版本册上没有这一版的授予连不到任何收尾，照原样交出
// ——收尾口只收册上的版本，连不到就是没收尾。收尾行有却立不起来是坏数据，整次上抛：跳过它会让一版已退役的授予照旧放行。
//
// 顺序按（对象、版本标签）稳定，但裁定不取首个：命中多于一条时 Authorize 答适用冲突（ADR-0261 决定三）。
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
		`SELECT granted.snapshot, `+versionClosureColumns+`
		   FROM (SELECT g.tenant_id, $4::smallint AS object_kind, g.object_id, g.version_label, g.snapshot
		           FROM party_commercial.authorization_grant AS g
		          WHERE g.tenant_id = $1
		            AND g.scope_ref = $2
		            AND g.effective_starts_at <= $3
		            AND (g.effective_ends_at IS NULL OR g.effective_ends_at > $3)) AS granted
		   `+versionClosureJoins("granted")+`
		  ORDER BY granted.object_id, granted.version_label`,
		tenant.String(),
		scope.String(),
		at.UTC(),
		uint8(domain.AuthorizationRuleObject),
	)
	if err != nil {
		return nil, fmt.Errorf("load effective grants: %w", err)
	}
	defer rows.Close()

	var grants []domain.AuthorityGrant
	for rows.Next() {
		var raw []byte
		var closure scannedVersionClosure
		if err := rows.Scan(append([]any{&raw}, closure.targets()...)...); err != nil {
			return nil, fmt.Errorf("load effective grants: %w", err)
		}
		grant, err := grantFromSnapshot(raw)
		if err != nil {
			return nil, fmt.Errorf("load effective grants: %w", err)
		}
		if grant, err = foldGrantClosure(grant, closure); err != nil {
			return nil, fmt.Errorf("load effective grants: %w", err)
		}
		if grant.InForceAt(at) {
			grants = append(grants, grant)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load effective grants: %w", err)
	}
	return grants, nil
}

func foldGrantClosure(grant domain.AuthorityGrant, closure scannedVersionClosure) (domain.AuthorityGrant, error) {
	correction, corrected, err := closure.correctionFor(grant.Version())
	if err != nil {
		return domain.AuthorityGrant{}, err
	}
	if corrected {
		if grant, err = grant.NarrowedBy(correction); err != nil {
			return domain.AuthorityGrant{}, err
		}
	}
	retirement, retired, err := closure.retirementFor(grant.Version())
	if err != nil {
		return domain.AuthorityGrant{}, err
	}
	if retired {
		if grant, err = grant.EndedBy(retirement); err != nil {
			return domain.AuthorityGrant{}, err
		}
	}
	return grant, nil
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
	return insertAuthorityGrant(ctx, executor, grant)
}

// insertAuthorityGrant 是授予登记的唯一落库处：授权册自己的 SaveGrant 与发布编排里的授予通道
// （CommercialPublications.SaveAuthorityGrant）共用它，重放与冲突的判据只此一份。
func insertAuthorityGrant(
	ctx context.Context,
	executor bentopg.Executor,
	grant domain.AuthorityGrant,
) (ports.GrantSaveOutcome, error) {
	document := documentOfGrant(grant)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.GrantSaveOutcomeInvalid, fmt.Errorf("save authority grant: %w", err)
	}
	// 摘要只盖授予自己的内容，不盖快照里版本壳的发布时刻、取效时刻与状态：发布重放时这些随时钟变，
	// 盖进去同一份授予重放就会被读成内容冲突（演示种子整批重放首先撞上）。
	identity := document
	identity.Version = versionDocument{
		TenantID:      document.Version.TenantID,
		Kind:          document.Version.Kind,
		ObjectID:      document.Version.ObjectID,
		Version:       document.Version.Version,
		Scope:         document.Version.Scope,
		ContentDigest: document.Version.ContentDigest,
	}
	identityRaw, err := json.Marshal(identity)
	if err != nil {
		return ports.GrantSaveOutcomeInvalid, fmt.Errorf("save authority grant: %w", err)
	}
	digest := sha256.Sum256(identityRaw)
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

// SaveAuthorityGrant 是发布编排里的授予通道（ADR-0312）：授予挂在刚入册的授权规则版本上，与其余声明同一事务。
// 重放与内容冲突的判据与授权册的 SaveGrant 是同一处（insertAuthorityGrant）。
func (repository *CommercialPublications) SaveAuthorityGrant(
	ctx context.Context,
	grant domain.AuthorityGrant,
) (ports.DeclarationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save authority grant: %w", err)
	}
	outcome, err := insertAuthorityGrant(ctx, executor, grant)
	if err != nil {
		return ports.DeclarationSaveOutcomeInvalid, err
	}
	switch outcome {
	case ports.GrantSaved:
		return ports.DeclarationSaved, nil
	case ports.GrantAlreadyRegistered:
		return ports.DeclarationAlreadyRegistered, nil
	case ports.GrantContentConflict:
		return ports.DeclarationContentConflict, nil
	default:
		return ports.DeclarationSaveOutcomeInvalid, fmt.Errorf("save authority grant: unexpected outcome %v", outcome)
	}
}
