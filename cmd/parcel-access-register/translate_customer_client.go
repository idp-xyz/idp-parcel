package main

// 本文件把客户渠道册的三种登记批译成领域类型（票 first-party-api/00，ADR-0139 决定二），纪律同 translate.go：未知字段、缺件
// 与形状错在触库之前拒收，一格不代填。批形照集成客户端那三种，只把来源身份换成客户账户、事实类型换成客户面能力。

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

type customerClientBatchDocument struct {
	TenantID string                       `json:"tenantId"`
	Clients  []customerClientItemDocument `json:"clients"`
}

// customerClientItemDocument 里没有凭据本体的位置：客户端密钥与私钥不进批文，未知字段拒收会把误填的一格当场拦下。
type customerClientItemDocument struct {
	Issuer            string `json:"issuer"`
	Subject           string `json:"subject"`
	CustomerAccountID string `json:"customerAccountId"`
	CredentialRef     string `json:"credentialRef"`
	// CertificateBoundTokenRequired 用指针，是为了分得开「写了 false」与「没写」：后者不代填成 false。
	CertificateBoundTokenRequired *bool  `json:"certificateBoundTokenRequired"`
	Basis                         string `json:"basis"`
}

type customerClientGrantBatchDocument struct {
	TenantID string                            `json:"tenantId"`
	Grants   []customerClientGrantItemDocument `json:"grants"`
}

type customerClientGrantItemDocument struct {
	GrantID           string     `json:"grantId"`
	Issuer            string     `json:"issuer"`
	Subject           string     `json:"subject"`
	Capability        string     `json:"capability"`
	EffectiveStartsAt time.Time  `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time `json:"effectiveEndsAt,omitempty"`
	Basis             string     `json:"basis"`
}

func customerClientBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document customerClientBatchDocument
	if err := decodeStrict(raw, &document, "首方客户端登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Clients), "首方客户端登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Clients))
	for _, entry := range document.Clients {
		binding, err := customerClientBindingFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("clients/%s：%w", entry.Subject, err)
		}
		subject := binding.Subject()
		items = append(items, batchItem{
			label: fmt.Sprintf("首方客户端 %s %s → %s", subject.Issuer(), subject.Subject(), binding.CustomerAccountID()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.customers.RegisterCustomerClient(ctx, binding)
				return customerOutcome(answer), err
			},
		})
	}
	return items, nil
}

func customerClientBindingFrom(tenantID string, entry customerClientItemDocument) (accessidentity.CustomerClientBinding, error) {
	subject, err := accessidentity.NewCustomerClientSubject(entry.Issuer, entry.Subject)
	if err != nil {
		return accessidentity.CustomerClientBinding{}, err
	}
	reference, err := accessidentity.NewCredentialReference(entry.CredentialRef)
	if err != nil {
		return accessidentity.CustomerClientBinding{}, fmt.Errorf("credentialRef：%w", err)
	}
	// 要不要求证书绑定令牌是租户对这个客户端的取值，产品不替它选（ADR-0139 决定二、参数登记册 PAR-INT-13）。
	if entry.CertificateBoundTokenRequired == nil {
		return accessidentity.CustomerClientBinding{}, fmt.Errorf("certificateBoundTokenRequired 缺席：要不要求证书绑定令牌不代填")
	}
	return accessidentity.NewCustomerClientBinding(subject, tenantID, entry.CustomerAccountID, reference,
		*entry.CertificateBoundTokenRequired, entry.Basis)
}

func customerClientGrantBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document customerClientGrantBatchDocument
	if err := decodeStrict(raw, &document, "首方客户端授予登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Grants), "首方客户端授予登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Grants))
	for _, entry := range document.Grants {
		grant, err := customerClientGrantFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("grants/%s：%w", entry.GrantID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("授予 %s（%s · %s）", grant.GrantID(), grant.Subject().Subject(), grant.Capability()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.customers.RegisterCustomerClientGrant(ctx, grant)
				return customerOutcome(answer), err
			},
		})
	}
	return items, nil
}

func customerClientGrantFrom(tenantID string, entry customerClientGrantItemDocument) (accessidentity.CustomerClientGrant, error) {
	subject, err := accessidentity.NewCustomerClientSubject(entry.Issuer, entry.Subject)
	if err != nil {
		return accessidentity.CustomerClientGrant{}, err
	}
	capability, err := accessidentity.ParseCustomerCapability(entry.Capability)
	if err != nil {
		return accessidentity.CustomerClientGrant{}, fmt.Errorf("capability %q：%w", entry.Capability, err)
	}
	interval, err := intervalFrom(entry.EffectiveStartsAt, entry.EffectiveEndsAt)
	if err != nil {
		return accessidentity.CustomerClientGrant{}, err
	}
	return accessidentity.NewCustomerClientGrant(tenantID, entry.GrantID, subject, capability, interval, entry.Basis)
}

func customerClientRevocationBatchFromJSON(raw []byte) ([]batchItem, error) {
	revocations, err := revocationsFromJSON(raw)
	if err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(revocations))
	for _, revocation := range revocations {
		items = append(items, batchItem{
			label: fmt.Sprintf("撤销 %s", revocation.GrantID()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.customers.RegisterCustomerClientRevocation(ctx, revocation)
				return customerOutcome(answer), err
			},
		})
	}
	return items, nil
}
