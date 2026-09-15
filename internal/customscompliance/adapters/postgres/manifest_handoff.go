package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// manifestEventType 是舱单引用意图的事件类型。
const manifestEventType = "customs-compliance.manifest.recorded"

// OutboxManifestHandoff 把舱单引用写入 Outbox，实现 ports.ManifestHandoff。入队一
// 步由 outboxintent.EnqueueOnce 承担。
type OutboxManifestHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxManifestHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxManifestHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxManifestHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ManifestHandoff = (*OutboxManifestHandoff)(nil)

// manifestPayload 是意图载荷的传输形状：只有下游 FindByManifest 所需的租户与舱单
// 标识，不带关联或来源版本明细。
type manifestPayload struct {
	TenantID   string `json:"tenantId"`
	ManifestID string `json:"manifestId"`
}

// manifestEventIDPort 是本口在信封 ID 上的口名前缀。取「承运商舱单」的英文形而不取事件类型里的 manifest 段：
// 前缀的唯一职责是让七只口在同一组维度上算出互不相交的 ID，只要在七只之间互异且稳定就够——它不参与路由、
// 不进分区主体登记（登的是分区键与 Subject 的形），改它只会让已入 Inbox 的旧 ID 与新 ID 对不上。
const manifestEventIDPort = "carrier-manifest"

// manifestEventID 把租户、舱单身份**再加来源版本**折成 outboxintent.FingerprintEventID 的定长形（票 sa-cc/34 裁决 3）。
//
// 版本必须在里面。ADR-0043 说意图由结果标识认领，而一份舱单引用的结果标识是它的来源
// 版本不是舱单身份：承运商更正推进版本走的是同一个舱单（Revise 换版本、指回前身），
// ID 少了版本两版就算出同一份，而 outboxintent.EnqueueOnce 先查后插——修订版
// 于是静默不入队，编排却收到「交接成功」。
func manifestEventID(tenant, manifest, version string) eventing.EventID {
	return outboxintent.FingerprintEventID(manifestEventIDPort, tenant, manifest, version)
}

// manifestPartitionKey 取（租户+舱单），不取版本。
//
// ID 管幂等、分区键管顺序，两者不是一回事。同一份舱单的来源版本是一条链（首版，此后
// 每次承运商更正一版），版本进分区键每版就自成一区，修订版可能先于首版送达——下游
// 读到的引用从此没有先后可言。
func manifestPartitionKey(tenant, manifest string) string {
	return tenant + "/" + manifest
}

// HandOffManifest 把一份意图入队。信封 ID 取舱单身份加来源版本——意图由来源版本认领
// （ADR-0043）。载荷仍是指针式的（只带舱单身份，库只管当前来源版本，下游按身份重读，
// 版本进载荷也指不到单独的一行）。租户、舱单标识或来源版本空白是装配缺陷，响亮报错
// 不入队。
func (handoff *OutboxManifestHandoff) HandOffManifest(
	ctx context.Context,
	intent ports.ManifestHandoffIntent,
) error {
	if intent.TenantID.String() == "" || intent.Reference.Manifest().String() == "" {
		return fmt.Errorf("hand off manifest: tenant and manifest id are required")
	}
	if intent.Reference.Version().String() == "" {
		// 没有版本就分不出首版与修订：两版在 ID 上算出同一个字符串，第二份被静默吞掉。
		return fmt.Errorf("hand off manifest: manifest source version is required")
	}

	payload, err := json.Marshal(manifestPayload{
		TenantID:   intent.TenantID.String(),
		ManifestID: intent.Reference.Manifest().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off manifest: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := manifestEventID(
		intent.TenantID.String(),
		intent.Reference.Manifest().String(),
		intent.Reference.Version().String(),
	)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventID,
		Source:       ccEventSource,
		Type:         manifestEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Reference.Manifest().String(),
		PartitionKey: manifestPartitionKey(intent.TenantID.String(), intent.Reference.Manifest().String()),
		OccurredAt:   intent.Reference.AcceptedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off manifest: %w", err)
	}
	return nil
}
