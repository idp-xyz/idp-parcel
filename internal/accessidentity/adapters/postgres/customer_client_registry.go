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

// CustomerClientRegistry 实现客户渠道册的登记口与装载口（access_identity 0010、ADR-0139 决定二）。撞键从不覆盖：同内容
// 重放答原结果，异内容答冲突，册上已有的那一行原样不动。写法照 IntegrationClientRegistry，两本册各认各的表。
type CustomerClientRegistry struct {
	db *bentopg.DB
}

func NewCustomerClientRegistry(db *bentopg.DB) (*CustomerClientRegistry, error) {
	if db == nil {
		return nil, fmt.Errorf("access identity postgres: db is nil")
	}
	return &CustomerClientRegistry{db: db}, nil
}

var (
	_ accessidentity.CustomerClientRegistry  = (*CustomerClientRegistry)(nil)
	_ accessidentity.CustomerClientRegistrar = (*CustomerClientRegistry)(nil)
)

// RegisterCustomerClient 撞键时：别的租户答已绑别的租户；同租户而客户账户不同答冲突——绑定不可改指，改指与改别的格同一
// 格答复，册上原行不动。
func (registry *CustomerClientRegistry) RegisterCustomerClient(
	ctx context.Context,
	binding accessidentity.CustomerClientBinding,
) (accessidentity.CustomerClientRegistrationOutcome, error) {
	const operation = "register customer client"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	subject := binding.Subject()
	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.customer_client
			(issuer, subject, tenant_id, customer_account_id, credential_ref, certificate_bound_token_required, basis_ref)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		subject.Issuer(), subject.Subject(), binding.TenantID(), binding.CustomerAccountID(),
		binding.CredentialReference().String(), binding.CertificateBoundTokenRequired(), binding.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.CustomerClientRegistrationRecorded, nil
	}

	var (
		tenant, account, credential, basis string
		certificateBound                   bool
	)
	err = executor.QueryRow(ctx,
		`SELECT tenant_id, customer_account_id, credential_ref, certificate_bound_token_required, basis_ref
		   FROM access_identity.customer_client
		  WHERE issuer = $1 AND subject = $2`,
		subject.Issuer(), subject.Subject(),
	).Scan(&tenant, &account, &credential, &certificateBound, &basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	switch {
	case tenant != binding.TenantID():
		return accessidentity.CustomerClientBoundToAnotherTenant, nil
	case account == binding.CustomerAccountID() &&
		credential == binding.CredentialReference().String() &&
		certificateBound == binding.CertificateBoundTokenRequired() &&
		basis == binding.Basis():
		return accessidentity.CustomerClientRegistrationAlreadyRegistered, nil
	default:
		return accessidentity.CustomerClientRegistrationContentConflict, nil
	}
}

// RegisterCustomerClientGrant 先查本租户册上有没有这个客户端再写，理由同 OperatorRegistry.RegisterGrant：让外键报错会把
// 事务打成失败态，调用方就拿不到「未登记」这一格答复。
func (registry *CustomerClientRegistry) RegisterCustomerClientGrant(
	ctx context.Context,
	grant accessidentity.CustomerClientGrant,
) (accessidentity.CustomerClientRegistrationOutcome, error) {
	const operation = "register customer client grant"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	subject := grant.Subject()
	var bound bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM access_identity.customer_client
			 WHERE issuer = $1 AND subject = $2 AND tenant_id = $3)`,
		subject.Issuer(), subject.Subject(), grant.TenantID(),
	).Scan(&bound); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !bound {
		return accessidentity.CustomerClientNotRegistered, nil
	}

	startsAt := grant.Interval().StartsAt()
	var endsAt *time.Time
	if end, bounded := grant.Interval().EndsAt(); bounded {
		endsAt = &end
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.customer_client_grant
			(tenant_id, grant_id, issuer, subject, capability, effective_starts_at, effective_ends_at, basis_ref)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		grant.TenantID(), grant.GrantID(), subject.Issuer(), subject.Subject(), grant.Capability().String(),
		startsAt, endsAt, grant.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.CustomerClientRegistrationRecorded, nil
	}

	var (
		storedIssuer, storedSubject, storedCapability, storedBasis string
		storedStartsAt                                             time.Time
		storedEndsAt                                               *time.Time
	)
	err = executor.QueryRow(ctx,
		`SELECT issuer, subject, capability, effective_starts_at, effective_ends_at, basis_ref
		   FROM access_identity.customer_client_grant
		  WHERE tenant_id = $1 AND grant_id = $2`,
		grant.TenantID(), grant.GrantID(),
	).Scan(&storedIssuer, &storedSubject, &storedCapability, &storedStartsAt, &storedEndsAt, &storedBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	same := storedIssuer == subject.Issuer() &&
		storedSubject == subject.Subject() &&
		storedCapability == grant.Capability().String() &&
		sameInstant(storedStartsAt, startsAt) &&
		sameOptionalInstant(storedEndsAt, endsAt) &&
		storedBasis == grant.Basis()
	if same {
		return accessidentity.CustomerClientRegistrationAlreadyRegistered, nil
	}
	return accessidentity.CustomerClientRegistrationContentConflict, nil
}

// RegisterCustomerClientRevocation 先查本租户册上有没有这笔授予，理由同 RegisterCustomerClientGrant。按（租户、授予标识）
// 查，别的租户拿同一个授予标识撤不动这一笔。
func (registry *CustomerClientRegistry) RegisterCustomerClientRevocation(
	ctx context.Context,
	revocation accessidentity.GrantRevocation,
) (accessidentity.CustomerClientRegistrationOutcome, error) {
	const operation = "register customer client grant revocation"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	var granted bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM access_identity.customer_client_grant
			 WHERE tenant_id = $1 AND grant_id = $2)`,
		revocation.TenantID(), revocation.GrantID(),
	).Scan(&granted); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !granted {
		return accessidentity.CustomerClientGrantNotRegistered, nil
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.customer_client_grant_revocation (tenant_id, grant_id, revoked_at, basis_ref)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		revocation.TenantID(), revocation.GrantID(), revocation.RevokedAt(), revocation.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.CustomerClientRegistrationRecorded, nil
	}

	var (
		storedRevokedAt time.Time
		storedBasis     string
	)
	err = executor.QueryRow(ctx,
		`SELECT revoked_at, basis_ref
		   FROM access_identity.customer_client_grant_revocation
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
		return accessidentity.CustomerClientRegistrationAlreadyRegistered, nil
	}
	return accessidentity.CustomerClientRegistrationContentConflict, nil
}

// FindCustomerClient 用一条查询取绑定与名下全部授予（连同撤销），理由同 FindOperator：分两次查会让绑定与授予取自两个
// 时点。此刻生不生效不在这里判。
func (registry *CustomerClientRegistry) FindCustomerClient(
	ctx context.Context,
	subject accessidentity.CustomerClientSubject,
) (accessidentity.CustomerClientStanding, bool, error) {
	const operation = "find customer client"
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT c.tenant_id, c.customer_account_id, c.credential_ref, c.certificate_bound_token_required, c.basis_ref,
		        g.grant_id, g.capability, g.effective_starts_at, g.effective_ends_at, g.basis_ref,
		        r.revoked_at, r.basis_ref
		   FROM access_identity.customer_client c
		   LEFT JOIN access_identity.customer_client_grant g
		     ON g.issuer = c.issuer AND g.subject = c.subject AND g.tenant_id = c.tenant_id
		   LEFT JOIN access_identity.customer_client_grant_revocation r
		     ON r.tenant_id = g.tenant_id AND r.grant_id = g.grant_id
		  WHERE c.issuer = $1 AND c.subject = $2
		  ORDER BY g.effective_starts_at, g.grant_id`,
		subject.Issuer(), subject.Subject(),
	)
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	var (
		found   bool
		binding accessidentity.CustomerClientBinding
		grants  []accessidentity.RecordedCustomerClientGrant
	)
	for rows.Next() {
		var (
			tenant, account, credential, basis string
			certificateBound                   bool
			grantID, capability, grantBasis    *string
			startsAt, endsAt, revokedAt        *time.Time
			revocationBasis                    *string
		)
		if err := rows.Scan(&tenant, &account, &credential, &certificateBound, &basis,
			&grantID, &capability, &startsAt, &endsAt, &grantBasis,
			&revokedAt, &revocationBasis,
		); err != nil {
			return accessidentity.CustomerClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
		}
		if !found {
			binding, err = customerBindingFromRow(subject, tenant, account, credential, certificateBound, basis)
			if err != nil {
				return accessidentity.CustomerClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
			}
			found = true
		}
		if grantID == nil {
			continue
		}
		recorded, err := recordedCustomerGrantFromRow(tenant, subject, *grantID, *capability, *startsAt, endsAt, *grantBasis, revokedAt, revocationBasis)
		if err != nil {
			return accessidentity.CustomerClientStanding{}, false, fmt.Errorf("%s: 授予 %s：%w", operation, *grantID, err)
		}
		grants = append(grants, recorded)
	}
	if err := rows.Err(); err != nil {
		return accessidentity.CustomerClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return accessidentity.CustomerClientStanding{}, false, nil
	}
	standing, err := accessidentity.NewCustomerClientStanding(binding, grants)
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return standing, true, nil
}

// customerBindingFromRow 与 recordedCustomerGrantFromRow 经领域构造门重建，理由同 recordedGrantFromRow：门变严之后读得出
// 哪一行已经不合今天的规则。
func customerBindingFromRow(
	subject accessidentity.CustomerClientSubject,
	tenant, account, credential string,
	certificateBound bool,
	basis string,
) (accessidentity.CustomerClientBinding, error) {
	reference, err := accessidentity.NewCredentialReference(credential)
	if err != nil {
		return accessidentity.CustomerClientBinding{}, err
	}
	return accessidentity.NewCustomerClientBinding(subject, tenant, account, reference, certificateBound, basis)
}

func recordedCustomerGrantFromRow(
	tenant string,
	subject accessidentity.CustomerClientSubject,
	grantID, rawCapability string,
	startsAt time.Time,
	endsAt *time.Time,
	basis string,
	revokedAt *time.Time,
	revocationBasis *string,
) (accessidentity.RecordedCustomerClientGrant, error) {
	capability, err := accessidentity.ParseCustomerCapability(rawCapability)
	if err != nil {
		return accessidentity.RecordedCustomerClientGrant{}, err
	}
	end := time.Time{}
	if endsAt != nil {
		end = *endsAt
	}
	interval, err := accessidentity.NewEffectiveInterval(startsAt, end)
	if err != nil {
		return accessidentity.RecordedCustomerClientGrant{}, err
	}
	grant, err := accessidentity.NewCustomerClientGrant(tenant, grantID, subject, capability, interval, basis)
	if err != nil {
		return accessidentity.RecordedCustomerClientGrant{}, err
	}
	if revokedAt == nil {
		return accessidentity.NewRecordedCustomerClientGrant(grant, nil)
	}
	reference := ""
	if revocationBasis != nil {
		reference = *revocationBasis
	}
	revocation, err := accessidentity.NewGrantRevocation(tenant, grantID, *revokedAt, reference)
	if err != nil {
		return accessidentity.RecordedCustomerClientGrant{}, err
	}
	return accessidentity.NewRecordedCustomerClientGrant(grant, &revocation)
}
