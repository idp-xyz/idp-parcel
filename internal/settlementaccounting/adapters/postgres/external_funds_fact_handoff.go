package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// externalFundsFactEventType 是资金事实采用意图的事件类型。更正 / 撤销不另开类型：在 SA 它们是
// 回指原事实的新版本（SA CONTEXT「外部资金事实撤销、更正或退回时……追加映射更正」），消费方对
// 三者一视同仁地重新核对（CC CONTEXT「只作为重新核对的来源事实」）——票 sa-cc/02 裁决 2。核销
// applied / reversed 两型是两个生命周期态而非同一事实的新版本，不是这里的先例。
const externalFundsFactEventType eventing.EventType = "settlement-accounting.external-funds-fact.adopted"

// externalFundsFactEventIDPort 是信封 ID 指纹的口名前缀，与分区键里的口名段同词。它只求本来源名下几只口的 ID 空间
// 互不相交（EnqueueOnce 按（来源, 事件 ID）查重），不参与路由，也不是分区主体登记表里的名字。
const externalFundsFactEventIDPort = "funds-fact"

// OutboxExternalFundsFactHandoff 把资金事实采用写入 Outbox，实现 ports.ExternalFundsFactHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxExternalFundsFactHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxExternalFundsFactHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxExternalFundsFactHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxExternalFundsFactHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ExternalFundsFactHandoff = (*OutboxExternalFundsFactHandoff)(nil)

// externalFundsFactPayload 只带下游按键读回所需的引用：租户、事实引用、采用版本；更正版另带回指
// 前一版本。金额、币种、付款人一个都不带——事实归银行、支付或财务系统拥有、采用在 SA，信封里再
// 抄一份金额就是第二处权威（票 02 红线；ADR-0137 决定四）。
type externalFundsFactPayload struct {
	TenantID string `json:"tenantId"`
	Fact     string `json:"fact"`
	Version  string `json:"version"`
	Corrects string `json:"corrects,omitempty"`
}

// externalFundsFactPartitionKey 取「其先后必须保序的那个对象」——资金事实，不含版本。
//
// ID 管幂等、分区键管顺序（ADR-0069 决定二「分区键收窄到业务主体」）。更正在 SA 是同一事实的新
// 版本回指前一版本，消费方必须按顺序看到 v1 再看到 v2；版本进分区键两版就各自成区，v2 可能先
// 于它更正的 v1 送达。同范围多笔事实之间没有先后可言（CC 按范围逐笔核对，每笔各自成来源），
// 所以主体不取申报范围——票 02 裁决 1。主体名「租户/资金事实」登在 internal/architecture 的
// 分区主体登记表（ADR-0074 决定五）。
func externalFundsFactPartitionKey(key ports.FundsFactKey) string {
	return key.TenantID.String() + "/funds-fact/" + key.Fact.String()
}

// externalFundsFactEventID 取（租户 / 事实 / 版本）的定长指纹（票 sa-cc/32 裁决 1）：同一事实的每个版本各自成一封、
// 同一版本重算必得同一个 ID。三个引用多长归实例半边，本仓给不出上界，可读串接形会把 eventing.MaxEventIDLength 押在
// 别人的长度上——理由与公式都在 outboxintent.FingerprintEventID 头注，这里不复述。分区键与 Subject 保留可读形。
func externalFundsFactEventID(key ports.FundsFactKey, version string) eventing.EventID {
	return outboxintent.FingerprintEventID(externalFundsFactEventIDPort, key.TenantID.String(), key.Fact.String(), version)
}

// HandOffExternalFundsFact 把一份意图入队。信封 ID 由（租户+事实+版本）认领：同一事实的每个版本各自
// 入队、一个都不丢，重放同一版本被 EnqueueOnce 认领吞掉（ADR-0043）。键缺席是装配缺陷，响亮
// 报错不入队。
func (handoff *OutboxExternalFundsFactHandoff) HandOffExternalFundsFact(
	ctx context.Context,
	intent ports.ExternalFundsFactIntent,
) error {
	key := intent.Record.Key
	version := intent.Record.Fact.Version().String()
	if key.TenantID.String() == "" || key.Fact.String() == "" || version == "" {
		return fmt.Errorf("hand off external funds fact: funds fact key and version are required")
	}

	body := externalFundsFactPayload{
		TenantID: key.TenantID.String(),
		Fact:     key.Fact.String(),
		Version:  version,
	}
	if corrects, corrected := intent.Record.Fact.Corrects(); corrected {
		body.Corrects = corrects.String()
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("hand off external funds fact: %w", err)
	}

	partitionKey := externalFundsFactPartitionKey(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           externalFundsFactEventID(key, version),
		Source:       saEventSource,
		Type:         externalFundsFactEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Fact.String() + "/" + version,
		PartitionKey: partitionKey,
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		// 框架 Envelope.Validate 的拒收是确定性的（同一份输入重投永远同一个结果），与存储不可用分两格交出去
		// （ports.ErrFundsFactHandoffRejected 头注）。ID 已是定长指纹形，今天能走到这里的是 Subject / PartitionKey
		// 取到超长的事实引用——引用多长归实例半边、构造门只查非空；折成续办引用会让 CLI 那句「重跑补发」恒假，
		// 所以要让编排看得见这一格并整笔不作答（票 sa-cc/32 裁决 2）。
		if errors.Is(err, eventing.ErrInvalidEnvelope) {
			return fmt.Errorf("hand off external funds fact: %w: %w", ports.ErrFundsFactHandoffRejected, err)
		}
		return fmt.Errorf("hand off external funds fact: %w", err)
	}
	return nil
}
