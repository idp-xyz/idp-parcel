package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// 本文件是外部标识关联册新形成关联时交出的门铃（ADR-0199 决定三；CONTEXT-MAP `parcel-shipment → node-operations` 外部标识关联
// 已形成的交接）：关联真插入的那一笔经 Outbox 交出只带（租户，号码）的信封，node-operations 收到后照旧经身份核对读口问本上下文，
// 不据信封认定身份。

// ExternalIdentifierRelationFormedEventType 是门铃的事件类型。导出给组合根的对照用例：消费方自己写一份同字面的串，两串由那条
// 用例钉住。
const ExternalIdentifierRelationFormedEventType = "parcel-shipment.external-identifier-relation.formed"

const (
	// externalIdentifierRelationEventIDPort 是门铃信封 ID 的口名：ID 由关联的键认领（ADR-0185：含形成依据）。
	externalIdentifierRelationEventIDPort = "external-identifier-relation"
	// externalIdentifierNumberPartitionPort 是门铃分区键的口名：同一个号的门铃排一条队。
	externalIdentifierNumberPartitionPort = "external-identifier-number"
	// externalIdentifierSubjectDigestMark 标出 Subject 里放的是号码的摘要而不是号码本身。
	externalIdentifierSubjectDigestMark = "NUMBER-SHA256/"
)

// externalIdentifierRelationFormedPayload 是门铃载荷的传输形状：只有租户与号码。不带包裹、分配方、标识类型或委托——CONTEXT-MAP
// 那一条明写不交号码的分配方、客户账户或委托；门铃只叫 NO 再问一次，答案由身份核对读口给。
type externalIdentifierRelationFormedPayload struct {
	TenantID string `json:"tenantId"`
	Number   string `json:"number"`
}

// formedExternalIdentifier 是这一笔真插入的一条关联，带它在册上的记录时刻。
type formedExternalIdentifier struct {
	relation   domain.ExternalIdentifierRelation
	recordedAt time.Time
}

// ExternalIdentifierDoorbell 在关联写入方（委托仓储、面单交易仓储）的事务里入队门铃。写入方各自构造一只，不经它们的构造参数
// 注入：门铃是关联插入这一步本身的一部分，与关联同一笔事务、同一个库，写入方各有几十处装配点，没有哪一处会想要一只不响的
// 门铃。导出只为同包测试能对它举无事务被拒的证据。
type ExternalIdentifierDoorbell struct {
	db    *bentopg.DB
	store *outbox.Store
}

func NewExternalIdentifierDoorbell(db *bentopg.DB) (*ExternalIdentifierDoorbell, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: external identifier doorbell: db is nil")
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel shipment postgres: external identifier doorbell: %w", err)
	}
	return &ExternalIdentifierDoorbell{db: db, store: store}, nil
}

// RingFormed 给这一笔真插入的一条关联入队一封门铃。须在适用状态变化写完之后调：同一笔里形成又作废（或被替代）的关联已是
// 终态，不交门铃（ADR-0199 决定三「适用终态不交信封」）——它的号此刻不标识任何包裹，叫 NO 去问只会问出零候选。走
// RequireExecutor：门铃与关联同生共死，没有事务时关联那一笔本就写不进来。
func (doorbell *ExternalIdentifierDoorbell) RingFormed(
	ctx context.Context,
	relation domain.ExternalIdentifierRelation,
	recordedAt time.Time,
) error {
	executor, err := doorbell.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("ring external identifier doorbell: %w", err)
	}
	var terminal bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1
		       FROM parcel_shipment.external_identifier_relation_change
		      WHERE tenant_id = $1
		        AND identifier_kind = $2
		        AND allocator = $3
		        AND identifier_number = $4
		        AND object_ref = $5
		        AND basis = $6)`,
		relation.Tenant().String(),
		relation.Kind().String(),
		relation.Allocator().String(),
		relation.Number().String(),
		relation.Parcel().String(),
		relation.Basis().String(),
	).Scan(&terminal); err != nil {
		return fmt.Errorf("ring external identifier doorbell: %w", err)
	}
	if terminal {
		return nil
	}
	envelope, err := externalIdentifierDoorbellEnvelope(formedExternalIdentifier{relation: relation, recordedAt: recordedAt})
	if err != nil {
		return err
	}
	if err := outboxintent.EnqueueOnce(ctx, doorbell.db, doorbell.store, envelope); err != nil {
		return fmt.Errorf("ring external identifier doorbell: %w", err)
	}
	return nil
}

// ringFormed 给这一笔真插入的每一条关联响一次门铃。
func ringFormed(ctx context.Context, doorbell *ExternalIdentifierDoorbell, formed []formedExternalIdentifier) error {
	for _, entry := range formed {
		if err := doorbell.RingFormed(ctx, entry.relation, entry.recordedAt); err != nil {
			return err
		}
	}
	return nil
}

// externalIdentifierDoorbellEnvelope 组一封门铃。
//
//   - ID 是关联键（租户，类型，分配方，号码，包裹，形成依据）的定长指纹：一条关联一封，重放撞同一个 ID，EnqueueOnce 不出第二封。
//   - 分区键是（租户，号码）的定长指纹：同一个号的门铃排一条队，重核按号找的正是同一批待识别；不用可读的「租户/号码」，号码
//     进册的字节界与分区键的上限是各自的两条界，拼起来的串不保证收得下。
//   - Subject 原样带号码，越界或带控制字符时换成号码的摘要，见 externalIdentifierSubject。
//   - Scope 是租户，与各上下文的信封同一口径。
func externalIdentifierDoorbellEnvelope(entry formedExternalIdentifier) (eventing.Envelope, error) {
	relation := entry.relation
	tenant, number := relation.Tenant().String(), relation.Number().String()
	payload, err := json.Marshal(externalIdentifierRelationFormedPayload{TenantID: tenant, Number: number})
	if err != nil {
		return eventing.Envelope{}, fmt.Errorf("ring external identifier doorbell: %w", err)
	}
	return eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: outboxintent.FingerprintEventID(externalIdentifierRelationEventIDPort,
			tenant, relation.Kind().String(), relation.Allocator().String(), number,
			relation.Parcel().String(), relation.Basis().String()),
		Source:       eventSource,
		Type:         ExternalIdentifierRelationFormedEventType,
		Version:      1,
		Scope:        tenant,
		Subject:      externalIdentifierSubject(number),
		PartitionKey: string(outboxintent.FingerprintEventID(externalIdentifierNumberPartitionPort, tenant, number)),
		OccurredAt:   relation.FormedAt().UTC(),
		RecordedAt:   entry.recordedAt.UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}, nil
}

// externalIdentifierSubject 是门铃的 Subject：号码原样，给看信封的人读。号码进册前的门（NewExternalIdentifierNumber）只管存不
// 存得下，不拦控制字符（条码规则属租户取值，ADR-0184 决定五）；它的字节界与 Subject 的上限是各自的两条界，这里不假设哪条更紧。
// 信封的 Subject 有上限、不收控制字符，越界的信封被框架确定性拒收。门铃与关联同一笔事务，拒收就把委托接受或面单结果一并
// 回滚，重试多少次都一样——为一封门铃挡住一份接受，代价倒置了。所以这样的号在 Subject 上换成号码的定长摘要，载荷照旧原样
// 带号：NO 按载荷里的号比对，Subject 只是给人看的。
func externalIdentifierSubject(number string) string {
	if len(number) <= eventing.MaxSubjectLength && utf8.ValidString(number) && strings.IndexFunc(number, unicode.IsControl) < 0 {
		return number
	}
	digest := sha256.Sum256([]byte(number))
	return externalIdentifierSubjectDigestMark + hex.EncodeToString(digest[:])
}
