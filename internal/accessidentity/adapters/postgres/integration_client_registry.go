package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// IntegrationClientRegistry 实现集成客户端册的登记口与装载口（access_identity 0003、ADR-0149 决定三）。撞键从不覆盖：
// 同内容重放答原结果，异内容答冲突，册上已有的那一行原样不动。
type IntegrationClientRegistry struct {
	db *bentopg.DB
}

func NewIntegrationClientRegistry(db *bentopg.DB) (*IntegrationClientRegistry, error) {
	if db == nil {
		return nil, fmt.Errorf("access identity postgres: db is nil")
	}
	return &IntegrationClientRegistry{db: db}, nil
}

var (
	_ accessidentity.IntegrationClientRegistry  = (*IntegrationClientRegistry)(nil)
	_ accessidentity.IntegrationClientRegistrar = (*IntegrationClientRegistry)(nil)
)

func (registry *IntegrationClientRegistry) RegisterIntegrationClient(
	ctx context.Context,
	binding accessidentity.IntegrationClientBinding,
) (accessidentity.IntegrationClientRegistrationOutcome, error) {
	const operation = "register integration client"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	subject := binding.Subject()
	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.integration_client
			(issuer, subject, tenant_id, source_ref, credential_ref, certificate_bound_token_required, basis_ref)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		subject.Issuer(), subject.Subject(), binding.TenantID(), binding.SourceIdentity(),
		binding.CredentialReference().String(), binding.CertificateBoundTokenRequired(), binding.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.IntegrationClientRegistrationRecorded, nil
	}

	var (
		tenant, source, credential, basis string
		certificateBound                  bool
	)
	err = executor.QueryRow(ctx,
		`SELECT tenant_id, source_ref, credential_ref, certificate_bound_token_required, basis_ref
		   FROM access_identity.integration_client
		  WHERE issuer = $1 AND subject = $2`,
		subject.Issuer(), subject.Subject(),
	).Scan(&tenant, &source, &credential, &certificateBound, &basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	switch {
	case tenant != binding.TenantID():
		return accessidentity.IntegrationClientBoundToAnotherTenant, nil
	case source == binding.SourceIdentity() &&
		credential == binding.CredentialReference().String() &&
		certificateBound == binding.CertificateBoundTokenRequired() &&
		basis == binding.Basis():
		return accessidentity.IntegrationClientRegistrationAlreadyRegistered, nil
	default:
		return accessidentity.IntegrationClientRegistrationContentConflict, nil
	}
}

// RegisterIntegrationClientGrant 先查本租户册上有没有这个客户端再写，理由同 OperatorRegistry.RegisterGrant：让外键报错
// 会把事务打成失败态，调用方就拿不到「未登记」这一格答复。
func (registry *IntegrationClientRegistry) RegisterIntegrationClientGrant(
	ctx context.Context,
	grant accessidentity.IntegrationClientGrant,
) (accessidentity.IntegrationClientRegistrationOutcome, error) {
	const operation = "register integration client grant"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	subject := grant.Subject()
	var bound bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM access_identity.integration_client
			 WHERE issuer = $1 AND subject = $2 AND tenant_id = $3)`,
		subject.Issuer(), subject.Subject(), grant.TenantID(),
	).Scan(&bound); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !bound {
		return accessidentity.IntegrationClientNotRegistered, nil
	}

	startsAt := grant.Interval().StartsAt()
	var endsAt *time.Time
	if end, bounded := grant.Interval().EndsAt(); bounded {
		endsAt = &end
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.integration_client_grant
			(tenant_id, grant_id, issuer, subject, fact_type, effective_starts_at, effective_ends_at, basis_ref)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		grant.TenantID(), grant.GrantID(), subject.Issuer(), subject.Subject(), grant.FactType().String(),
		startsAt, endsAt, grant.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.IntegrationClientRegistrationRecorded, nil
	}

	var (
		storedIssuer, storedSubject, storedFact, storedBasis string
		storedStartsAt                                       time.Time
		storedEndsAt                                         *time.Time
	)
	err = executor.QueryRow(ctx,
		`SELECT issuer, subject, fact_type, effective_starts_at, effective_ends_at, basis_ref
		   FROM access_identity.integration_client_grant
		  WHERE tenant_id = $1 AND grant_id = $2`,
		grant.TenantID(), grant.GrantID(),
	).Scan(&storedIssuer, &storedSubject, &storedFact, &storedStartsAt, &storedEndsAt, &storedBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	same := storedIssuer == subject.Issuer() &&
		storedSubject == subject.Subject() &&
		storedFact == grant.FactType().String() &&
		sameInstant(storedStartsAt, startsAt) &&
		sameOptionalInstant(storedEndsAt, endsAt) &&
		storedBasis == grant.Basis()
	if same {
		return accessidentity.IntegrationClientRegistrationAlreadyRegistered, nil
	}
	return accessidentity.IntegrationClientRegistrationContentConflict, nil
}

// RegisterIntegrationClientRevocation 先查本租户册上有没有这笔授予，理由同 RegisterIntegrationClientGrant。按（租户、授予
// 标识）查，别的租户拿同一个授予标识撤不动这一笔。
func (registry *IntegrationClientRegistry) RegisterIntegrationClientRevocation(
	ctx context.Context,
	revocation accessidentity.GrantRevocation,
) (accessidentity.IntegrationClientRegistrationOutcome, error) {
	const operation = "register integration client grant revocation"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	var granted bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM access_identity.integration_client_grant
			 WHERE tenant_id = $1 AND grant_id = $2)`,
		revocation.TenantID(), revocation.GrantID(),
	).Scan(&granted); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !granted {
		return accessidentity.IntegrationClientGrantNotRegistered, nil
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.integration_client_grant_revocation (tenant_id, grant_id, revoked_at, basis_ref)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		revocation.TenantID(), revocation.GrantID(), revocation.RevokedAt(), revocation.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.IntegrationClientRegistrationRecorded, nil
	}

	var (
		storedRevokedAt time.Time
		storedBasis     string
	)
	err = executor.QueryRow(ctx,
		`SELECT revoked_at, basis_ref
		   FROM access_identity.integration_client_grant_revocation
		  WHERE tenant_id = $1 AND grant_id = $2`,
		revocation.TenantID(), revocation.GrantID(),
	).Scan(&storedRevokedAt, &storedBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if sameInstant(storedRevokedAt, revocation.RevokedAt()) && storedBasis == revocation.Basis() {
		return accessidentity.IntegrationClientRegistrationAlreadyRegistered, nil
	}
	return accessidentity.IntegrationClientRegistrationContentConflict, nil
}

// FindIntegrationClient 用一条查询取绑定与名下全部授予（连同撤销），理由同 FindOperator：分两次查会让绑定与授予取自两个
// 时点。此刻生不生效不在这里判。
func (registry *IntegrationClientRegistry) FindIntegrationClient(
	ctx context.Context,
	subject accessidentity.IntegrationClientSubject,
) (accessidentity.IntegrationClientStanding, bool, error) {
	const operation = "find integration client"
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return accessidentity.IntegrationClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT c.tenant_id, c.source_ref, c.credential_ref, c.certificate_bound_token_required, c.basis_ref,
		        g.grant_id, g.fact_type, g.effective_starts_at, g.effective_ends_at, g.basis_ref,
		        r.revoked_at, r.basis_ref
		   FROM access_identity.integration_client c
		   LEFT JOIN access_identity.integration_client_grant g
		     ON g.issuer = c.issuer AND g.subject = c.subject AND g.tenant_id = c.tenant_id
		   LEFT JOIN access_identity.integration_client_grant_revocation r
		     ON r.tenant_id = g.tenant_id AND r.grant_id = g.grant_id
		  WHERE c.issuer = $1 AND c.subject = $2
		  ORDER BY g.effective_starts_at, g.grant_id`,
		subject.Issuer(), subject.Subject(),
	)
	if err != nil {
		return accessidentity.IntegrationClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	var (
		found   bool
		binding accessidentity.IntegrationClientBinding
		grants  []accessidentity.RecordedClientGrant
	)
	for rows.Next() {
		var (
			tenant, source, credential, basis string
			certificateBound                  bool
			grantID, fact, grantBasis         *string
			startsAt, endsAt, revokedAt       *time.Time
			revocationBasis                   *string
		)
		if err := rows.Scan(&tenant, &source, &credential, &certificateBound, &basis,
			&grantID, &fact, &startsAt, &endsAt, &grantBasis,
			&revokedAt, &revocationBasis,
		); err != nil {
			return accessidentity.IntegrationClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
		}
		if !found {
			binding, err = clientBindingFromRow(subject, tenant, source, credential, certificateBound, basis)
			if err != nil {
				return accessidentity.IntegrationClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
			}
			found = true
		}
		if grantID == nil {
			continue
		}
		recorded, err := recordedClientGrantFromRow(tenant, subject, *grantID, *fact, *startsAt, endsAt, *grantBasis, revokedAt, revocationBasis)
		if err != nil {
			return accessidentity.IntegrationClientStanding{}, false, fmt.Errorf("%s: 授予 %s：%w", operation, *grantID, err)
		}
		grants = append(grants, recorded)
	}
	if err := rows.Err(); err != nil {
		return accessidentity.IntegrationClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return accessidentity.IntegrationClientStanding{}, false, nil
	}
	standing, err := accessidentity.NewIntegrationClientStanding(binding, grants)
	if err != nil {
		return accessidentity.IntegrationClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return standing, true, nil
}

// clientBindingFromRow 与 recordedClientGrantFromRow 经领域构造门重建，理由同 recordedGrantFromRow：门变严之后读得出
// 哪一行已经不合今天的规则。
func clientBindingFromRow(
	subject accessidentity.IntegrationClientSubject,
	tenant, source, credential string,
	certificateBound bool,
	basis string,
) (accessidentity.IntegrationClientBinding, error) {
	reference, err := accessidentity.NewCredentialReference(credential)
	if err != nil {
		return accessidentity.IntegrationClientBinding{}, err
	}
	return accessidentity.NewIntegrationClientBinding(subject, tenant, source, reference, certificateBound, basis)
}

func recordedClientGrantFromRow(
	tenant string,
	subject accessidentity.IntegrationClientSubject,
	grantID, rawFact string,
	startsAt time.Time,
	endsAt *time.Time,
	basis string,
	revokedAt *time.Time,
	revocationBasis *string,
) (accessidentity.RecordedClientGrant, error) {
	fact, err := accessidentity.ParseExternalFactType(rawFact)
	if err != nil {
		return accessidentity.RecordedClientGrant{}, err
	}
	end := time.Time{}
	if endsAt != nil {
		end = *endsAt
	}
	interval, err := accessidentity.NewEffectiveInterval(startsAt, end)
	if err != nil {
		return accessidentity.RecordedClientGrant{}, err
	}
	grant, err := accessidentity.NewIntegrationClientGrant(tenant, grantID, subject, fact, interval, basis)
	if err != nil {
		return accessidentity.RecordedClientGrant{}, err
	}
	if revokedAt == nil {
		return accessidentity.NewRecordedClientGrant(grant, nil)
	}
	reference := ""
	if revocationBasis != nil {
		reference = *revocationBasis
	}
	revocation, err := accessidentity.NewGrantRevocation(tenant, grantID, *revokedAt, reference)
	if err != nil {
		return accessidentity.RecordedClientGrant{}, err
	}
	return accessidentity.NewRecordedClientGrant(grant, &revocation)
}
