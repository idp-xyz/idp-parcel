package main

// 本文件把三种登记批译成领域类型：逐字段过 accessidentity 的构造门，未知字段、缺件与形状错在触库
// 之前拒收，一格不代填——没有缺省的生效起点或撤销时刻，也没有缺省依据。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

type operatorBatchDocument struct {
	TenantID  string                 `json:"tenantId"`
	Operators []operatorItemDocument `json:"operators"`
}

type operatorItemDocument struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
	Basis   string `json:"basis"`
}

type grantBatchDocument struct {
	TenantID string              `json:"tenantId"`
	Grants   []grantItemDocument `json:"grants"`
}

type grantItemDocument struct {
	GrantID           string     `json:"grantId"`
	Issuer            string     `json:"issuer"`
	Subject           string     `json:"subject"`
	CapabilityFace    string     `json:"capabilityFace"`
	EffectiveStartsAt time.Time  `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time `json:"effectiveEndsAt,omitempty"`
	Basis             string     `json:"basis"`
}

type revocationBatchDocument struct {
	TenantID    string                   `json:"tenantId"`
	Revocations []revocationItemDocument `json:"revocations"`
}

type revocationItemDocument struct {
	GrantID   string    `json:"grantId"`
	RevokedAt time.Time `json:"revokedAt"`
	Basis     string    `json:"basis"`
}

func operatorBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document operatorBatchDocument
	if err := decodeStrict(raw, &document, "主体登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Operators), "主体登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Operators))
	for _, entry := range document.Operators {
		subject, err := accessidentity.NewOperatorSubject(entry.Issuer, entry.Subject)
		if err != nil {
			return nil, fmt.Errorf("operators/%s：%w", entry.Subject, err)
		}
		binding, err := accessidentity.NewOperatorBinding(subject, document.TenantID, entry.Basis)
		if err != nil {
			return nil, fmt.Errorf("operators/%s：%w", entry.Subject, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("操作者 %s %s", subject.Issuer(), subject.Subject()),
			apply: func(ctx context.Context, registrar accessidentity.OperatorRegistrar) (accessidentity.OperatorRegistrationOutcome, error) {
				return registrar.RegisterOperator(ctx, binding)
			},
		})
	}
	return items, nil
}

func grantBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document grantBatchDocument
	if err := decodeStrict(raw, &document, "授予登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Grants), "授予登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Grants))
	for _, entry := range document.Grants {
		grant, err := grantFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("grants/%s：%w", entry.GrantID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("授予 %s（%s · %s）", grant.GrantID(), grant.Subject().Subject(), grant.Face()),
			apply: func(ctx context.Context, registrar accessidentity.OperatorRegistrar) (accessidentity.OperatorRegistrationOutcome, error) {
				return registrar.RegisterGrant(ctx, grant)
			},
		})
	}
	return items, nil
}

func grantFrom(tenantID string, entry grantItemDocument) (accessidentity.OperatorGrant, error) {
	subject, err := accessidentity.NewOperatorSubject(entry.Issuer, entry.Subject)
	if err != nil {
		return accessidentity.OperatorGrant{}, err
	}
	face, err := accessidentity.ParseCapabilityFace(entry.CapabilityFace)
	if err != nil {
		return accessidentity.OperatorGrant{}, fmt.Errorf("capabilityFace %q：%w", entry.CapabilityFace, err)
	}
	if entry.EffectiveStartsAt.IsZero() {
		return accessidentity.OperatorGrant{}, fmt.Errorf("effectiveStartsAt 缺席：生效起点不代填")
	}
	endsAt := time.Time{}
	if entry.EffectiveEndsAt != nil {
		// 显式给零值与缺席在领域里同义（不设终点），批文里却是两种写法；拒掉前者，免得一个写坏的
		// 终点被静静读成无限期。
		if entry.EffectiveEndsAt.IsZero() {
			return accessidentity.OperatorGrant{}, fmt.Errorf("effectiveEndsAt 给了零值：不设终点请省略该字段")
		}
		endsAt = *entry.EffectiveEndsAt
	}
	interval, err := accessidentity.NewEffectiveInterval(entry.EffectiveStartsAt, endsAt)
	if err != nil {
		return accessidentity.OperatorGrant{}, err
	}
	return accessidentity.NewOperatorGrant(tenantID, entry.GrantID, subject, face, interval, entry.Basis)
}

func revocationBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document revocationBatchDocument
	if err := decodeStrict(raw, &document, "撤销登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Revocations), "撤销登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Revocations))
	for _, entry := range document.Revocations {
		if entry.RevokedAt.IsZero() {
			return nil, fmt.Errorf("revocations/%s：revokedAt 缺席：撤销时刻不代填", entry.GrantID)
		}
		revocation, err := accessidentity.NewGrantRevocation(document.TenantID, entry.GrantID, entry.RevokedAt, entry.Basis)
		if err != nil {
			return nil, fmt.Errorf("revocations/%s：%w", entry.GrantID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("撤销 %s", revocation.GrantID()),
			apply: func(ctx context.Context, registrar accessidentity.OperatorRegistrar) (accessidentity.OperatorRegistrationOutcome, error) {
				return registrar.RegisterRevocation(ctx, revocation)
			},
		})
	}
	return items, nil
}

func decodeStrict(raw []byte, target any, what string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s不是本入口的形状：%w", what, err)
	}
	return nil
}

func requireBatch(tenantID string, items int, what string) error {
	if strings.TrimSpace(tenantID) == "" {
		return fmt.Errorf("%s缺 tenantId", what)
	}
	if items == 0 {
		return fmt.Errorf("%s没有任何项", what)
	}
	return nil
}
