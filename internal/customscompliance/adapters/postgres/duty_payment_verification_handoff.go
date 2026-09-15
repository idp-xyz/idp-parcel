package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// dutyPaymentVerificationEventType 是税费付款核对形成意图的事件类型（UC-CC-009 步 8 向
// settlement-accounting 的结算交接，票 sa-cc/05）。
const dutyPaymentVerificationEventType = "customs-compliance.duty-payment-verification.formed"

// OutboxDutyPaymentVerificationHandoff 把一版已形成的核对写入 Outbox，实现
// ports.DutyPaymentVerificationHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxDutyPaymentVerificationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxDutyPaymentVerificationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxDutyPaymentVerificationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxDutyPaymentVerificationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.DutyPaymentVerificationHandoff = (*OutboxDutyPaymentVerificationHandoff)(nil)

// dutyPaymentVerificationPayload 是意图载荷的传输形状：只有下游 FindVerification 所需的幂等键
// ——租户、申报范围、税费引用、资金事实引用与版本指纹。覆盖 / 差额 / 有效性三轴与关联依据
// 一律不进载荷（票面红线「信封不带金额结论」）：CC 是核对的权威，SA 按这些引用回读那一版；
// 载荷带上结论就是第二处口径，SA 的实际代垫判断也不得由任一单项输入直接推导。
type dutyPaymentVerificationPayload struct {
	TenantID string `json:"tenantId"`
	Scope    string `json:"scope"`
	Duty     string `json:"duty"`
	Funds    string `json:"funds"`
	Digest   string `json:"digest"`
}

// dutyPaymentVerificationPartitionKey 取（租户 + 口名段 + 申报范围），不取整个幂等键。
//
// ID 管幂等、分区键管顺序（ADR-0069 决定二）。主体是「租户 / 申报范围」（票 sa-cc/05 裁决 1）：
// 核对版本按申报范围逐笔形成，迟到事实换指纹换版，同一范围的版本链要排一条队；税费版本只是
// 新核对版本的来源之一，不是有版本链的主体。指纹进分区键每版自成一区，SA 读到的核对版本从此
// 没有先后可言。
//
// 口名段的理由与 ADR-0074 决定二同一条：restriction_handoff 也按（租户 + 同一个
// DecisionScopeReference）分区，不带段时两口的键值字符串就相同，一封停在未决的限制信封会把同一
// 范围的核对信封堵在分区头；而两口之间没有任何消费方依赖跨口到达序（SA 按引用回读、案件链不建
// 跨口保序，ADR-0069 决定一），共队只有成本没有收益。主体名登在 internal/architecture 的分区
// 主体登记表（ADR-0074 决定五）。
func dutyPaymentVerificationPartitionKey(key ports.DutyVerificationKey) string {
	return key.TenantID.String() + "/duty-payment-verification/" + key.Scope.String()
}

// dutyPaymentVerificationEventIDPort 是本口在信封 ID 上的口名前缀，与分区键里的口名段同词。
const dutyPaymentVerificationEventIDPort = "duty-payment-verification"

// dutyPaymentVerificationEventID 把幂等键五维（租户、范围、税费、资金、版本指纹）折成 fingerprintEventID 的定长形：
// 同一范围的每一版核对各自一封，第二版不被 outboxintent.EnqueueOnce 的先查后插当成首版的重放静默吞掉；五维全进
// 哈希，少一维就有两版同 ID。
func dutyPaymentVerificationEventID(key ports.DutyVerificationKey) string {
	return fingerprintEventID(dutyPaymentVerificationEventIDPort,
		key.TenantID.String(), key.Scope.String(), key.Duty.String(), key.Funds.String(), key.Digest)
}

// HandOffDutyPaymentVerification 把一份意图入队。幂等键任一维空白、核对时刻缺席是装配缺陷，
// 响亮报错不入队——编排只在核对形成那一格调它，交进来的键与对象就是刚落册的那一版。
func (handoff *OutboxDutyPaymentVerificationHandoff) HandOffDutyPaymentVerification(
	ctx context.Context,
	intent ports.DutyPaymentVerificationHandoffIntent,
) error {
	key := intent.Key
	if strings.TrimSpace(key.TenantID.String()) == "" ||
		strings.TrimSpace(key.Scope.String()) == "" ||
		strings.TrimSpace(key.Duty.String()) == "" ||
		strings.TrimSpace(key.Funds.String()) == "" ||
		strings.TrimSpace(key.Digest) == "" {
		return fmt.Errorf("hand off duty payment verification: verification key is required")
	}
	if intent.Verification.VerifiedAt().IsZero() {
		return fmt.Errorf("hand off duty payment verification: the verification instant is required")
	}

	payload, err := json.Marshal(dutyPaymentVerificationPayload{
		TenantID: key.TenantID.String(),
		Scope:    key.Scope.String(),
		Duty:     key.Duty.String(),
		Funds:    key.Funds.String(),
		Digest:   key.Digest,
	})
	if err != nil {
		return fmt.Errorf("hand off duty payment verification: %w", err)
	}

	eventID := dutyPaymentVerificationEventID(key)
	partitionKey := dutyPaymentVerificationPartitionKey(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ccEventSource,
		Type:         dutyPaymentVerificationEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Scope.String() + "/" + key.Duty.String(),
		PartitionKey: partitionKey,
		OccurredAt:   intent.Verification.VerifiedAt().UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		// 框架 Envelope.Validate 的拒收是确定性的（同一份输入重投永远同一个结果），与存储不可用分两格交出去
		// （ports.ErrHandoffEnvelopeRejected 头注）；ID 已改指纹形，今天能走到这里的是 Subject / PartitionKey
		// 超上限之类的形状错，留这一格是不让「静默提交」那条路重新长出来（票 sa-cc/29 裁决 2）。
		if errors.Is(err, eventing.ErrInvalidEnvelope) {
			return fmt.Errorf("hand off duty payment verification: %w: %w", ports.ErrHandoffEnvelopeRejected, err)
		}
		return fmt.Errorf("hand off duty payment verification: %w", err)
	}
	return nil
}
