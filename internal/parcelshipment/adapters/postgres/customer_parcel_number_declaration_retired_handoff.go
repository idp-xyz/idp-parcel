package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// 本文件是客户包裹件号声明撤除时交出的门铃（ADR-0211 决定三；CONTEXT-MAP `parcel-shipment → node-operations` 客户包裹件号声明
// 撤除的交接）：一份带客户包裹件号的委托由`已提交`转为`已拒绝`或`已撤回`的那一笔保存，按它声明的每个号各交一封只带（租户，
// 号码）的信封。那份声明从此不再算歧义信号（PS CONTEXT「歧义信号」），node-operations 收到后照旧经身份核对读口问本上下文，
// 只因歧义信号落下的身份冲突由此可能答出唯一命中。

// CustomerParcelNumberDeclarationRetiredEventType 是声明撤除门铃的事件类型。导出给组合根的对照用例：消费方自己写一份同字面的串，
// 两串由那条用例钉住。
const CustomerParcelNumberDeclarationRetiredEventType = "parcel-shipment.customer-parcel-number-declaration.retired"

// customerParcelNumberDeclarationRetiredEventIDPort 是声明撤除门铃信封 ID 的口名：ID 由（租户，委托，号码）认领——一份委托只会
// 离开`已提交`一次，同一份委托同一个号至多一封。
const customerParcelNumberDeclarationRetiredEventIDPort = "customer-parcel-number-declaration-retired"

// customerParcelNumberDeclarationRetiredPayload 是门铃载荷的传输形状：只有租户与号码，同关联已形成那一只。不带委托、客户账户、
// 声明包裹或撤下的原因——CONTEXT-MAP 那一条明写不交号码的分配方、客户账户或委托，NO 也用不上。
type customerParcelNumberDeclarationRetiredPayload struct {
	TenantID string `json:"tenantId"`
	Number   string `json:"number"`
}

// declarationsRetiredBy 交回这一次保存撤下的客户包裹件号：委托由`已提交`转为`已拒绝`或`已撤回`时，它声明的每个号去重后按首次
// 出现定序——领域不拒两件声明包裹报同一个号（ADR-0184 越权风险点「同一委托内两件包裹给同一个号不拒收」），门铃仍只一封；其余
// 保存交空。
//
// 判据是保存前后两个状态，不是保存后的状态：已终局的委托再保存一次（读回已拒绝的委托补一格留痕之类）不是撤下，不该再响。接受
// 不在此列：接受形成关联，关联已形成那一只门铃会响，NO 重核时那件或成多候选、或成唯一（ADR-0211 决定三）。
func declarationsRetiredBy(prior domain.ShipmentRequestState, request domain.ShipmentRequest) []domain.ExternalIdentifierNumber {
	if prior != domain.ShipmentRequestSubmitted {
		return nil
	}
	if state := request.State(); state != domain.ShipmentRequestRejected && state != domain.ShipmentRequestWithdrawn {
		return nil
	}
	var numbers []domain.ExternalIdentifierNumber
	seen := map[string]struct{}{}
	for _, declaration := range request.CustomerParcelNumbers() {
		if _, twice := seen[declaration.Number().String()]; twice {
			continue
		}
		seen[declaration.Number().String()] = struct{}{}
		numbers = append(numbers, declaration.Number())
	}
	return numbers
}

// retiredAt 是撤下那一刻的业务时间：拒绝取决定时刻，撤回取撤回决定时刻。两者都在委托离开`已提交`的那一次转移里写下。
func retiredAt(request domain.ShipmentRequest) time.Time {
	if withdrawal, withdrawn := request.Withdrawal(); withdrawn {
		return withdrawal.DecidedAt()
	}
	if decision, decided := request.AcceptanceDecision(); decided {
		return decision.DecidedAt()
	}
	return time.Time{}
}

// RingDeclarationsRetired 给这一笔撤下的每个号入队一封门铃，与委托状态变化同一笔事务。挂在关联已形成那一只门铃上：两者是同一个
// 库、同一本 Outbox、同一种只带（租户，号码）的铃，委托仓储已经持有它。走 RequireExecutor：门铃与状态变化同生共死。
func (doorbell *ExternalIdentifierDoorbell) RingDeclarationsRetired(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.ShipmentRequest,
	numbers []domain.ExternalIdentifierNumber,
	recordedAt time.Time,
) error {
	if _, err := doorbell.db.RequireExecutor(ctx); err != nil {
		return fmt.Errorf("ring declaration retired doorbell: %w", err)
	}
	for _, number := range numbers {
		envelope, err := declarationRetiredEnvelope(tenant, request, number, recordedAt)
		if err != nil {
			return err
		}
		if err := outboxintent.EnqueueOnce(ctx, doorbell.db, doorbell.store, envelope); err != nil {
			return fmt.Errorf("ring declaration retired doorbell: %w", err)
		}
	}
	return nil
}

// declarationRetiredEnvelope 组一封声明撤除门铃。
//
//   - ID 是（租户，委托，号码）的定长指纹：重放撞同一个 ID，EnqueueOnce 不出第二封。
//   - 分区键与关联已形成那一只同一个式子、同一个口名：同一个号的两种门铃排一条队。两种门铃都叫 NO 按这个号重核同一批待识别，
//     分两条队并发处理，两笔重核会在同一行上抢着追加。
//   - Subject 原样带号码，越界或带控制字符时换成号码的摘要，与关联已形成那一只同一个做法（externalIdentifierSubject）。
//   - Scope 是租户，与各上下文的信封同一口径。
func declarationRetiredEnvelope(
	tenant domain.TenantID,
	request domain.ShipmentRequest,
	number domain.ExternalIdentifierNumber,
	recordedAt time.Time,
) (eventing.Envelope, error) {
	payload, err := json.Marshal(customerParcelNumberDeclarationRetiredPayload{TenantID: tenant.String(), Number: number.String()})
	if err != nil {
		return eventing.Envelope{}, fmt.Errorf("ring declaration retired doorbell: %w", err)
	}
	return eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: outboxintent.FingerprintEventID(customerParcelNumberDeclarationRetiredEventIDPort,
			tenant.String(), request.ShipmentRequestID().String(), number.String()),
		Source:       eventSource,
		Type:         CustomerParcelNumberDeclarationRetiredEventType,
		Version:      1,
		Scope:        tenant.String(),
		Subject:      externalIdentifierSubject(number.String()),
		PartitionKey: string(outboxintent.FingerprintEventID(externalIdentifierNumberPartitionPort, tenant.String(), number.String())),
		OccurredAt:   retiredAt(request).UTC(),
		RecordedAt:   recordedAt.UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}, nil
}
