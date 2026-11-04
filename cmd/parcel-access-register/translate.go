package main

// 本文件把操作者册的三种登记批译成领域类型（集成客户端册的在 translate_integration_client.go）：单项的形状与翻译取
// registrationjson（在线写口与本命令行共用同一份），本文件只拆批、取批头的租户；未知字段、缺件与形状错在触库之前拒收，
// 一格不代填——没有缺省的生效起点或撤销时刻，也没有缺省依据。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/registrationjson"
)

type operatorBatchDocument struct {
	TenantID  string                 `json:"tenantId"`
	Operators []operatorItemDocument `json:"operators"`
}

type operatorItemDocument = registrationjson.OperatorItem

type grantBatchDocument struct {
	TenantID string              `json:"tenantId"`
	Grants   []grantItemDocument `json:"grants"`
}

type grantItemDocument = registrationjson.GrantItem

type revocationBatchDocument struct {
	TenantID    string                   `json:"tenantId"`
	Revocations []revocationItemDocument `json:"revocations"`
}

type revocationItemDocument = registrationjson.RevocationItem

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
		binding, err := registrationjson.BindingFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("operators/%s：%w", entry.Subject, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("操作者 %s %s", binding.Subject().Issuer(), binding.Subject().Subject()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.operators.RegisterOperator(ctx, binding)
				return operatorOutcome(answer), err
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
		grant, err := registrationjson.GrantFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("grants/%s：%w", entry.GrantID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("授予 %s（%s · %s）", grant.GrantID(), grant.Subject().Subject(), grant.Face()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.operators.RegisterGrant(ctx, grant)
				return operatorOutcome(answer), err
			},
		})
	}
	return items, nil
}

// intervalFrom 译授予的生效区间，各本册的授予批共用：起点不代填。形状规则在 registrationjson。
func intervalFrom(startsAt time.Time, endsAt *time.Time) (accessidentity.EffectiveInterval, error) {
	return registrationjson.IntervalFrom(startsAt, endsAt)
}

func revocationBatchFromJSON(raw []byte) ([]batchItem, error) {
	revocations, err := revocationsFromJSON(raw)
	if err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(revocations))
	for _, revocation := range revocations {
		items = append(items, batchItem{
			label: fmt.Sprintf("撤销 %s", revocation.GrantID()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.operators.RegisterRevocation(ctx, revocation)
				return operatorOutcome(answer), err
			},
		})
	}
	return items, nil
}

// revocationsFromJSON 译一份撤销批，两本册共用同一形状：撤一笔授予要的就是（租户、授予标识、时刻、依据）。撤的是哪
// 一本册的授予由子命令定，批文里不写。
func revocationsFromJSON(raw []byte) ([]accessidentity.GrantRevocation, error) {
	var document revocationBatchDocument
	if err := decodeStrict(raw, &document, "撤销登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Revocations), "撤销登记批"); err != nil {
		return nil, err
	}
	revocations := make([]accessidentity.GrantRevocation, 0, len(document.Revocations))
	for _, entry := range document.Revocations {
		revocation, err := registrationjson.RevocationFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("revocations/%s：%w", entry.GrantID, err)
		}
		revocations = append(revocations, revocation)
	}
	return revocations, nil
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
