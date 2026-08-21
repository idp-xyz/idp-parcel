package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// caseClosureEventType 是关务案件关闭意图的事件类型。
const caseClosureEventType = "customs-compliance.case-closure.recorded"

// OutboxCaseClosureHandoff 把关闭决定写入 Outbox，实现 ports.CaseClosureHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxCaseClosureHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxCaseClosureHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxCaseClosureHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxCaseClosureHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.CaseClosureHandoff = (*OutboxCaseClosureHandoff)(nil)

// caseClosurePayload 是意图载荷的传输形状：只有下游 FindByCase 所需的租户与案件引用。
type caseClosurePayload struct {
	TenantID string `json:"tenantId"`
	CaseRef  string `json:"caseRef"`
}

// caseClosureEventID 取（租户+案件引用+关闭周期序数）。
//
// 周期序数必须在里面。UC-CC-010 要求重开后再次关闭形成新的关闭决定周期、历史周期永久
// 保留，而 ID 少了这一维，第二个周期与第一个算出同一个字符串，outboxintent.EnqueueOnce
// 先查后插——C2 于是静默不入队，编排却收到「交接成功」。今天应用层没有重开入口，序数
// 恒为 1，本公式无行为差异；它拆的是多周期落地那天的引信（ADR-0069）。
func caseClosureEventID(tenant, caseRef string, closureCycle int) string {
	return tenant + "/" + caseRef + "/" + strconv.Itoa(closureCycle)
}

// caseClosureCycle 给出当次关闭是本案第几个关闭周期：首次为 1，每次受控重开后再次关闭
// 递增。序数从关闭记录自身推出而不由意图注入——意图注入要加宽 CaseClosureHandoffIntent，
// 而今天没有任何调用方给得出 1 以外的值，那只是把常量挪进编排（ADR-0069 决定二）。
//
// 本推法是 ADR-0069 决定一的第四条成立前提：多周期若改成一案多条关闭记录、新记录从零条
// 重开起算，序数会退回 1、撞 ID 复活，那时须同时给出新的序数来源，否则重裁。
func caseClosureCycle(closure *domain.CustomsCaseClosure) int {
	return len(closure.Reopenings()) + 1
}

// caseClosurePartitionKey 取（租户+案件引用），不取整个信封 ID。
//
// 与交接登记同一条理由：ID 管幂等（每个关闭周期一份意图，C2 因而不丢），分区键管顺序
// （同一案件的各关闭周期先后排队）。把 ID 直接当分区键会让每个周期自成一个分区，框架的
// 顺序保证于是落空——重开后的关闭可以先于它之前的那一次送达。
//
// 只收窄到案件、不跨口对齐：建立与申报两口各留各的键公式，案件链不建立跨口保序，乱序由
// 指针载荷、按键重读与「不可见即可重试」消化（ADR-0069 决定一）。
func caseClosurePartitionKey(tenant, caseRef string) string {
	return tenant + "/" + caseRef
}

// HandOffClosure 把一份意图入队。租户或案件引用空白、关闭缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxCaseClosureHandoff) HandOffClosure(
	ctx context.Context,
	intent ports.CaseClosureHandoffIntent,
) error {
	if intent.Closure == nil || intent.TenantID.String() == "" || intent.Closure.CaseRef() == "" {
		return fmt.Errorf("hand off case closure: tenant and case ref are required")
	}

	payload, err := json.Marshal(caseClosurePayload{
		TenantID: intent.TenantID.String(),
		CaseRef:  intent.Closure.CaseRef(),
	})
	if err != nil {
		return fmt.Errorf("hand off case closure: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := caseClosureEventID(
		intent.TenantID.String(),
		intent.Closure.CaseRef(),
		caseClosureCycle(intent.Closure),
	)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ccEventSource,
		Type:         caseClosureEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Closure.CaseRef(),
		PartitionKey: caseClosurePartitionKey(intent.TenantID.String(), intent.Closure.CaseRef()),
		OccurredAt:   intent.Closure.ClosedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off case closure: %w", err)
	}
	return nil
}
