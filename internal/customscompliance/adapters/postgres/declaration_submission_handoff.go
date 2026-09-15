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

// declarationSubmissionEventType 是申报提交版本意图的事件类型。
const declarationSubmissionEventType = "customs-compliance.declaration-submission.formed"

// OutboxDeclarationSubmissionHandoff 把提交版本写入 Outbox，实现
// ports.DeclarationSubmissionHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxDeclarationSubmissionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxDeclarationSubmissionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxDeclarationSubmissionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxDeclarationSubmissionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.DeclarationSubmissionHandoff = (*OutboxDeclarationSubmissionHandoff)(nil)

// declarationSubmissionPayload 是意图载荷的传输形状：下游 FindByKey 所需的幂等键
// 三维加版本维，再加单元所属案件（ADR-0069 决定四/ADR-0073 决定五：案件引用进载荷
// 不进分区键），不带组成快照或发送尝试明细。
type declarationSubmissionPayload struct {
	TenantID  string `json:"tenantId"`
	UnitID    string `json:"unitId"`
	Procedure string `json:"procedure"`
	VersionID string `json:"versionId"`
	CaseID    string `json:"caseId"`
}

// declarationSubmissionEventIDPort 是本口在信封 ID 上的口名前缀。
const declarationSubmissionEventIDPort = "declaration-submission"

// declarationSubmissionEventID 按（逻辑申报目标+提交版本）四维认领信封，折成 outboxintent.FingerprintEventID 的定长形
// （票 sa-cc/34 裁决 3）。版本维必须在 ID 上：原案内更正在同一逻辑申报目标下形成第二版（CONTEXT「原提交及其结果永久
// 保留」），EnqueueOnce 按（来源+事件 ID）先查后插、查到即静默成功——ID 不带版本维，第二版意图必然被首版
// 信封吞掉且无任何一环报错（declaration-envelope-version-dedup/01 坐实的机制坑）。
// 重放同一版本仍重发同一份，ADR-0043 的幂等语义不变。
func declarationSubmissionEventID(key ports.DeclarationSubmissionKey, version string) eventing.EventID {
	return outboxintent.FingerprintEventID(declarationSubmissionEventIDPort,
		key.TenantID.String(), key.Unit.String(), key.Procedure.String(), version)
}

// declarationSubmissionPartitionKey 是同一逻辑申报目标的分区锚：版本演进必须保序
// （V2 的替代关系指着 V1，乱序消费让下游先见后继再见前身），所以分区键取三维目标键
// 不含版本——同一目标的各版本进同一分区，不同目标互不阻塞。
func declarationSubmissionPartitionKey(key ports.DeclarationSubmissionKey) string {
	return key.TenantID.String() + "/" + key.Unit.String() + "/" + key.Procedure.String()
}

// HandOffDeclarationSubmission 把一份意图入队。信封 ID 由目标键加版本认领（ADR-0043：
// 意图由结果标识认领——提交版本就是这次结果的标识）。键或版本缺席是装配缺陷，响亮
// 报错不入队。
func (handoff *OutboxDeclarationSubmissionHandoff) HandOffDeclarationSubmission(
	ctx context.Context,
	intent ports.DeclarationSubmissionHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Unit.String() == "" || key.Procedure.String() == "" {
		return fmt.Errorf("hand off declaration submission: receive key is required")
	}
	if intent.Record.Version.ID().String() == "" {
		return fmt.Errorf("hand off declaration submission: the submission version is required")
	}
	// 案件维必填（ADR-0073 决定五）：缺席是装配缺陷，响亮报错不入队——静默发出去
	// 会在下游译码处变毒丸。
	if intent.Case.String() == "" {
		return fmt.Errorf("hand off declaration submission: the customs case is required")
	}

	payload, err := json.Marshal(declarationSubmissionPayload{
		TenantID:  key.TenantID.String(),
		UnitID:    key.Unit.String(),
		Procedure: key.Procedure.String(),
		VersionID: intent.Record.Version.ID().String(),
		CaseID:    intent.Case.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off declaration submission: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := declarationSubmissionEventID(key, intent.Record.Version.ID().String())
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventID,
		Source:       ccEventSource,
		Type:         declarationSubmissionEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      intent.Record.Version.ID().String(),
		PartitionKey: declarationSubmissionPartitionKey(key),
		OccurredAt:   intent.Record.Version.FixedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off declaration submission: %w", err)
	}
	return nil
}
