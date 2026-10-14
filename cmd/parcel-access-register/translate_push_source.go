package main

// 本文件把推送源册的三种登记批译成领域类型（票 operator-channel/12），纪律同 translate.go：未知字段、缺件与形状错在
// 触库之前拒收，一格不代填。批文里没有密钥本体的位置：共享密钥与对方公钥不进批文，未知字段拒收会把误填的一格当场拦下。

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

type pushSourceBatchDocument struct {
	TenantID string                   `json:"tenantId"`
	Sources  []pushSourceItemDocument `json:"sources"`
}

type pushSourceItemDocument struct {
	SourceID string `json:"sourceId"`
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
	// WindowSeconds 与 ClockSkewSeconds 用指针，是为了分得开「写了 0」与「没写」：时钟偏差为零是租户可以作的选择，缺席不代填成零。
	WindowSeconds    *int64 `json:"windowSeconds"`
	ClockSkewSeconds *int64 `json:"clockSkewSeconds"`
	Basis            string `json:"basis"`
}

type pushSigningKeyBatchDocument struct {
	TenantID string                       `json:"tenantId"`
	Keys     []pushSigningKeyItemDocument `json:"keys"`
}

type pushSigningKeyItemDocument struct {
	SourceID          string     `json:"sourceId"`
	KeyID             string     `json:"keyId"`
	Form              string     `json:"form"`
	KeyRef            string     `json:"keyRef"`
	EffectiveStartsAt time.Time  `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time `json:"effectiveEndsAt,omitempty"`
	Basis             string     `json:"basis"`
}

type pushSigningKeyRetirementBatchDocument struct {
	TenantID    string                                 `json:"tenantId"`
	Retirements []pushSigningKeyRetirementItemDocument `json:"retirements"`
}

type pushSigningKeyRetirementItemDocument struct {
	SourceID  string    `json:"sourceId"`
	KeyID     string    `json:"keyId"`
	RetiresAt time.Time `json:"retiresAt"`
	Basis     string    `json:"basis"`
}

func pushSourceBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document pushSourceBatchDocument
	if err := decodeStrict(raw, &document, "推送源登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Sources), "推送源登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Sources))
	for _, entry := range document.Sources {
		binding, err := pushSourceBindingFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("sources/%s：%w", entry.SourceID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("推送源 %s", binding.Source()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.pushSources.RegisterPushSource(ctx, binding)
				return pushSourceOutcome(answer), err
			},
		})
	}
	return items, nil
}

func pushSourceBindingFrom(tenantID string, entry pushSourceItemDocument) (accessidentity.PushSourceBinding, error) {
	source, err := accessidentity.NewPushSourceID(entry.SourceID)
	if err != nil {
		return accessidentity.PushSourceBinding{}, err
	}
	client, err := accessidentity.NewIntegrationClientSubject(entry.Issuer, entry.Subject)
	if err != nil {
		return accessidentity.PushSourceBinding{}, err
	}
	if entry.WindowSeconds == nil || entry.ClockSkewSeconds == nil {
		return accessidentity.PushSourceBinding{}, fmt.Errorf("windowSeconds 或 clockSkewSeconds 缺席：窗口与时钟偏差不代填")
	}
	window, err := secondsAsDuration(*entry.WindowSeconds, "windowSeconds")
	if err != nil {
		return accessidentity.PushSourceBinding{}, err
	}
	skew, err := secondsAsDuration(*entry.ClockSkewSeconds, "clockSkewSeconds")
	if err != nil {
		return accessidentity.PushSourceBinding{}, err
	}
	return accessidentity.NewPushSourceBinding(source, client, tenantID, window, skew, entry.Basis)
}

func pushSigningKeyBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document pushSigningKeyBatchDocument
	if err := decodeStrict(raw, &document, "签名密钥登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Keys), "签名密钥登记批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Keys))
	for _, entry := range document.Keys {
		key, err := pushSigningKeyFrom(document.TenantID, entry)
		if err != nil {
			return nil, fmt.Errorf("keys/%s：%w", entry.KeyID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("签名密钥 %s（%s）", key.KeyID(), key.Source()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.pushSources.RegisterPushSigningKey(ctx, key)
				return pushSourceOutcome(answer), err
			},
		})
	}
	return items, nil
}

func pushSigningKeyFrom(tenantID string, entry pushSigningKeyItemDocument) (accessidentity.PushSigningKey, error) {
	source, err := accessidentity.NewPushSourceID(entry.SourceID)
	if err != nil {
		return accessidentity.PushSigningKey{}, err
	}
	form, err := accessidentity.ParsePushSignatureForm(entry.Form)
	if err != nil {
		return accessidentity.PushSigningKey{}, fmt.Errorf("form %q：%w", entry.Form, err)
	}
	reference, err := accessidentity.NewCredentialReference(entry.KeyRef)
	if err != nil {
		return accessidentity.PushSigningKey{}, fmt.Errorf("keyRef：%w", err)
	}
	interval, err := intervalFrom(entry.EffectiveStartsAt, entry.EffectiveEndsAt)
	if err != nil {
		return accessidentity.PushSigningKey{}, err
	}
	return accessidentity.NewPushSigningKey(tenantID, source, entry.KeyID, form, reference, interval, entry.Basis)
}

func pushSigningKeyRetirementBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document pushSigningKeyRetirementBatchDocument
	if err := decodeStrict(raw, &document, "签名密钥停用批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Retirements), "签名密钥停用批"); err != nil {
		return nil, err
	}
	items := make([]batchItem, 0, len(document.Retirements))
	for _, entry := range document.Retirements {
		if entry.RetiresAt.IsZero() {
			return nil, fmt.Errorf("retirements/%s：retiresAt 缺席：停用时刻不代填", entry.KeyID)
		}
		source, err := accessidentity.NewPushSourceID(entry.SourceID)
		if err != nil {
			return nil, fmt.Errorf("retirements/%s：%w", entry.KeyID, err)
		}
		retirement, err := accessidentity.NewPushSigningKeyRetirement(document.TenantID, source, entry.KeyID, entry.RetiresAt, entry.Basis)
		if err != nil {
			return nil, fmt.Errorf("retirements/%s：%w", entry.KeyID, err)
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("停用 %s（%s）", retirement.KeyID(), retirement.Source()),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.pushSources.RegisterPushSigningKeyRetirement(ctx, retirement)
				return pushSourceOutcome(answer), err
			},
		})
	}
	return items, nil
}

func secondsAsDuration(seconds int64, what string) (time.Duration, error) {
	d := time.Duration(seconds) * time.Second
	if int64(d/time.Second) != seconds {
		return 0, fmt.Errorf("%s 超出可表示的时长", what)
	}
	return d, nil
}
