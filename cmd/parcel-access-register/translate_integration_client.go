package main

// 本文件把集成客户端册的三种登记批译成领域类型（票 operator-channel/11），纪律同 translate.go：未知字段、缺件与
// 形状错在触库之前拒收，一格不代填。

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

type integrationClientBatchDocument struct {
	TenantID string                          `json:"tenantId"`
	Clients  []integrationClientItemDocument `json:"clients"`
}

// integrationClientItemDocument 里没有凭据本体的位置：客户端密钥与私钥不进批文，未知字段拒收会把误填的一格当场拦下。
type integrationClientItemDocument struct {
	Issuer         string `json:"issuer"`
	Subject        string `json:"subject"`
	SourceIdentity string `json:"sourceIdentity"`
	CredentialRef  string `json:"credentialRef"`
	// CertificateBoundTokenRequired 用指针，是为了分得开「写了 false」与「没写」：后者不代填成 false。
	CertificateBoundTokenRequired *bool  `json:"certificateBoundTokenRequired"`
	Basis                         string `json:"basis"`
}

type integrationClientGrantBatchDocument struct {
	TenantID string                               `json:"tenantId"`
	Grants   []integrationClientGrantItemDocument `json:"grants"`
}

type integrationClientGrantItemDocument struct {
	GrantID           string     `json:"grantId"`
	Issuer            string     `json:"issuer"`
	Subject           string     `json:"subject"`
	FactType          string     `json:"factType"`
	EffectiveStartsAt time.Time  `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time `json:"effectiveEndsAt,omitempty"`
	Basis             string     `json:"basis"`
}

func integrationClientBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document integrationClientBatchDocument
	if err := decodeStrict(raw, &document, "集成客户端登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Clients), "集成客户端登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Clients))
	for _, entry := range document.Clients {
		binding, err := integrationClientBindingFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("clients/%s：%w", entry.Subject, err)
		}
		subject := binding.Subject()
		items = append(items, batchItem{
			label: fmt.Sprintf("集成客户端 %s %s", subject.Issuer(), subject.Subject()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.clients.RegisterIntegrationClient(ctx, binding)
				return clientOutcome(answer), err
			},
		})
	}
	return items, nil
}

func integrationClientBindingFrom(tenantID string, entry integrationClientItemDocument) (accessidentity.IntegrationClientBinding, error) {
	subject, err := accessidentity.NewIntegrationClientSubject(entry.Issuer, entry.Subject)
	if err != nil {
		return accessidentity.IntegrationClientBinding{}, err
	}
	reference, err := accessidentity.NewCredentialReference(entry.CredentialRef)
	if err != nil {
		return accessidentity.IntegrationClientBinding{}, fmt.Errorf("credentialRef：%w", err)
	}
	// 要不要求证书绑定令牌是租户对这个客户端的取值（ADR-0149 决定三「高保证场景可要求」），产品不替它选。
	if entry.CertificateBoundTokenRequired == nil {
		return accessidentity.IntegrationClientBinding{}, fmt.Errorf("certificateBoundTokenRequired 缺席：要不要求证书绑定令牌不代填")
	}
	return accessidentity.NewIntegrationClientBinding(subject, tenantID, entry.SourceIdentity, reference,
		*entry.CertificateBoundTokenRequired, entry.Basis)
}

func integrationClientGrantBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document integrationClientGrantBatchDocument
	if err := decodeStrict(raw, &document, "集成客户端授予登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Grants), "集成客户端授予登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Grants))
	for _, entry := range document.Grants {
		grant, err := integrationClientGrantFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("grants/%s：%w", entry.GrantID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("授予 %s（%s · %s）", grant.GrantID(), grant.Subject().Subject(), grant.FactType()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.clients.RegisterIntegrationClientGrant(ctx, grant)
				return clientOutcome(answer), err
			},
		})
	}
	return items, nil
}

func integrationClientGrantFrom(tenantID string, entry integrationClientGrantItemDocument) (accessidentity.IntegrationClientGrant, error) {
	subject, err := accessidentity.NewIntegrationClientSubject(entry.Issuer, entry.Subject)
	if err != nil {
		return accessidentity.IntegrationClientGrant{}, err
	}
	fact, err := accessidentity.ParseExternalFactType(entry.FactType)
	if err != nil {
		return accessidentity.IntegrationClientGrant{}, fmt.Errorf("factType %q：%w", entry.FactType, err)
	}
	interval, err := intervalFrom(entry.EffectiveStartsAt, entry.EffectiveEndsAt)
	if err != nil {
		return accessidentity.IntegrationClientGrant{}, err
	}
	return accessidentity.NewIntegrationClientGrant(tenantID, entry.GrantID, subject, fact, interval, entry.Basis)
}

func integrationClientRevocationBatchFromJSON(raw []byte) ([]batchItem, error) {
	revocations, err := revocationsFromJSON(raw)
	if err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(revocations))
	for _, revocation := range revocations {
		items = append(items, batchItem{
			label: fmt.Sprintf("撤销 %s", revocation.GrantID()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.clients.RegisterIntegrationClientRevocation(ctx, revocation)
				return clientOutcome(answer), err
			},
		})
	}
	return items, nil
}
