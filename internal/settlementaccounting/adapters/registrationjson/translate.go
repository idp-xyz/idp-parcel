// Package registrationjson 把外部资金事实的登记输入 JSON 译装成采用用例的命令（票 sa-cc/27 裁决 2）。
//
// 它不放在 `cmd/parcel-settlement-register` 里而落在本上下文，理由与 `customscompliance/adapters/registrationjson`
// 同一条：登记载荷的形状要由**一份**代码把门。受控 CLI 的 `-input` 与后继票要长出的在线登记端点收的是同源
// 载荷（ADR-0085 决定一「CLI 与端点消费同一登记用例，答案代数一致」），而 `cmd` 包不可被 `internal` 导入——
// 译装留在 CLI 里，端点那一侧就只能再拄一份，两份的严格性此后各自漂移，而漂移的那一半不会有任何东西报出来。
//
// 本包不认证、不采信任何自报身份：`tenantId` 在这里只是载荷的一个字段，它是不是调用方有权写的那个租户由调用侧
// 回答——CLI 靠「能进数据库网络」这道运维边界，在线口靠真渠道 Intake（操作者渠道 ADR-0100，未就位）。
//
// 本包也不采用：`AdoptFact` 的答案格与 `CorrectFact` 的链头判断全在 `application.MapExternalFundsHandler`（ADR-0137
// 决定四「SA 采用是事实进产品的唯一入口」），这里连库都不碰。UC-SA-005
// 「不因接收回调自动采用」——本包收的是人工 / 受控批量交进来的载荷，不是任何来源系统的回调形。
package registrationjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 译装严格且零默认：未知字段拒收（打错字段名不得静默变成「没给」）、封闭词表在这里就核（词表外的取值走到
// 领域门才被拒时答的是`未受理`一个词，而它明明是用法错误，该指名哪一格）、有构造门的标识在这里就过一遍同一道
// 构造门并点名字段——命令上这些格是裸串，用例 factFrom 会再过一次，两次过的是同一道门不是第二套口径，这里多
// 做的只是把「哪一格坏了」说出来。金额与时刻原样递给领域：非正金额、零时刻、回指自己、回指非链头都是领域与
// 用例的门（`domain.AdoptExternalFundsFact` / `CorrectFundsFactCommand` 头注），这里绝不代判。

type externalFundsFactDocument struct {
	TenantID    string    `json:"tenantId"`
	FactRef     string    `json:"factRef"`
	SourceRef   string    `json:"sourceRef"`
	PayerRef    string    `json:"payerRef"`
	Kind        string    `json:"kind"`
	Currency    string    `json:"currency"`
	AmountMinor int64     `json:"amountMinor"`
	Version     string    `json:"version"`
	OccurredAt  time.Time `json:"occurredAt"`
}

// ExternalFundsFactFromJSON 译装一次外部资金事实首版的采用。payerRef 可缺席——来源未提供付款人是事实的一个诚实状态
// （`domain.FundsPayerReference` 头注），缺席时命令上是空串；给了却全是空白则拒：那不是「未提供」，是打坏了。
func ExternalFundsFactFromJSON(raw []byte) (application.AdoptFundsFactCommand, error) {
	none := application.AdoptFundsFactCommand{}
	var document externalFundsFactDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("资金事实登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, fmt.Errorf("tenantId：%w", err)
	}
	if _, err := domain.NewFundsFactReference(document.FactRef); err != nil {
		return none, fmt.Errorf("factRef：%w", err)
	}
	if _, err := domain.NewFundsSourceRegistrationReference(document.SourceRef); err != nil {
		return none, fmt.Errorf("sourceRef：%w", err)
	}
	if document.PayerRef != "" {
		if _, err := domain.NewFundsPayerReference(document.PayerRef); err != nil {
			return none, fmt.Errorf("payerRef：%w", err)
		}
	}
	kind, err := fundsFactKindFrom(document.Kind)
	if err != nil {
		return none, err
	}
	if _, err := domain.NewCurrencyCode(document.Currency); err != nil {
		return none, fmt.Errorf("currency：%w", err)
	}
	if _, err := domain.NewFundsFactVersion(document.Version); err != nil {
		return none, fmt.Errorf("version：%w", err)
	}
	return application.AdoptFundsFactCommand{
		TenantID:    tenant,
		Fact:        document.FactRef,
		Source:      document.SourceRef,
		Payer:       document.PayerRef,
		Kind:        kind,
		Currency:    document.Currency,
		AmountMinor: document.AmountMinor,
		Version:     document.Version,
		OccurredAt:  document.OccurredAt,
	}, nil
}

type externalFundsFactCorrectionDocument struct {
	TenantID    string    `json:"tenantId"`
	FactRef     string    `json:"factRef"`
	Corrects    string    `json:"corrects"`
	Version     string    `json:"version"`
	AmountMinor int64     `json:"amountMinor"`
	CorrectedAt time.Time `json:"correctedAt"`
}

// ExternalFundsFactCorrectionFromJSON 译装一次外部更正的采用。载荷上只有更正自己带的格：来源、付款人、种类、币种与
// 发生时刻从链头照抄、命令上不接（`application.CorrectFundsFactCommand` 头注），载荷若带了它们按未知字段拒——外部更正
// 改的是金额，其余若也变了那是另一条事实。
func ExternalFundsFactCorrectionFromJSON(raw []byte) (application.CorrectFundsFactCommand, error) {
	none := application.CorrectFundsFactCommand{}
	var document externalFundsFactCorrectionDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("资金事实更正登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, fmt.Errorf("tenantId：%w", err)
	}
	if _, err := domain.NewFundsFactReference(document.FactRef); err != nil {
		return none, fmt.Errorf("factRef：%w", err)
	}
	if _, err := domain.NewFundsFactVersion(document.Corrects); err != nil {
		return none, fmt.Errorf("corrects：%w", err)
	}
	if _, err := domain.NewFundsFactVersion(document.Version); err != nil {
		return none, fmt.Errorf("version：%w", err)
	}
	return application.CorrectFundsFactCommand{
		TenantID:    tenant,
		Fact:        document.FactRef,
		Corrects:    document.Corrects,
		Version:     document.Version,
		AmountMinor: document.AmountMinor,
		CorrectedAt: document.CorrectedAt,
	}, nil
}

// fundsFactKindFrom 把封闭词表译回领域常量，词表与 `domain.FundsFactKind.String()` 及迁移 CHECK 同字。缺席不放行：
// 资金事实没有「种类未到」这一格（与 CC 协作事项 kind 缺席即义务依据未到不同），缺席就是打漏了。
func fundsFactKindFrom(raw string) (domain.FundsFactKind, error) {
	switch strings.TrimSpace(raw) {
	case "RECEIPT_CONFIRMED":
		return domain.FundsReceiptConfirmed, nil
	case "PAYMENT_FAILED":
		return domain.FundsPaymentFailed, nil
	case "FUNDS_RETURNED":
		return domain.FundsReturned, nil
	default:
		return domain.FundsFactKindInvalid, fmt.Errorf(
			"kind=%q 不在封闭三值（RECEIPT_CONFIRMED / PAYMENT_FAILED / FUNDS_RETURNED），缺席同样拒", raw)
	}
}

// decodeStrict 拒未知字段：打错的键静默丢弃，会让操作员以为登进去的比实际多。
func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
