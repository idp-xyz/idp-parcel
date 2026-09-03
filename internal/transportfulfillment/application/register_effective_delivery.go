package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedDeliverySave 说明交付库交回了封闭集合以外的写入结果。
var ErrUnexpectedDeliverySave = errors.New("transport fulfillment: unexpected effective delivery save outcome")

// DeliveryRegistrationOutcome 是交付生效提交的应用处理结果。`未生效`是业务负向格
// ——对象本次没有交付成，登不出生效交付（领域硬句，编排不绕），恢复动作是改约再派
// 而不是改单或重试。
type DeliveryRegistrationOutcome uint8

const (
	DeliveryRegistrationOutcomeInvalid DeliveryRegistrationOutcome = iota
	DeliveryRegistered
	DeliveryExistingVersion
	DeliveryRegistrationConflict
	DeliveryNotEffective
	DeliveryCorrected
	DeliveryNotAccepted
	DeliveryUndecided
)

func (outcome DeliveryRegistrationOutcome) String() string {
	switch outcome {
	case DeliveryRegistered:
		return "DELIVERY_REGISTERED"
	case DeliveryExistingVersion:
		return "EXISTING_VERSION"
	case DeliveryRegistrationConflict:
		return "SOURCE_CONFLICT"
	case DeliveryNotEffective:
		return "NOT_EFFECTIVE"
	case DeliveryCorrected:
		return "DELIVERY_CORRECTED"
	case DeliveryNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case DeliveryUndecided:
		return "DELIVERY_UNDECIDED"
	default:
		return ""
	}
}

// DeliveryUndecidedReason 指名提交停在哪一步等谁。
type DeliveryUndecidedReason uint8

const (
	DeliveryUndecidedReasonNone DeliveryUndecidedReason = iota
	DeliveryStoreUnavailable
	DeliveryViewUnavailable
	DeliveryIdentityUnavailable
)

func (reason DeliveryUndecidedReason) String() string {
	switch reason {
	case DeliveryStoreUnavailable:
		return "DELIVERY_STORE_UNAVAILABLE"
	case DeliveryViewUnavailable:
		return "DELIVERY_VIEW_UNAVAILABLE"
	case DeliveryIdentityUnavailable:
		return "DELIVERY_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// RegisterEffectiveDeliveryCommand 携带首登入口的全部输入。POD 必备——没有符合当时
// 规则的交付证明就没有生效交付（CONTEXT 交付节）。
type RegisterEffectiveDeliveryCommand struct {
	TenantID  domain.TenantID
	Attempt   string
	Object    string
	Method    string
	Recipient string
	Proof     string
}

// CorrectDeliveryProofCommand 携带更正入口的全部输入：更正走 CorrectProof 换新版本
// 回指前版，原版本与原判断不动（领域已钉）。
type CorrectDeliveryProofCommand struct {
	TenantID    domain.TenantID
	Attempt     string
	Object      string
	NewProof    string
	CorrectedAt time.Time
}

type RegisterEffectiveDeliveryResult struct {
	outcome          DeliveryRegistrationOutcome
	reason           DeliveryUndecidedReason
	record           ports.EffectiveDeliveryRecord
	hasRecord        bool
	continuation     string
	handoff          string
	participationEnd ParticipationEndOutcome
}

// ParticipationEnd 是交付落库后同事务触发的「结束参与」那一半的答案（票 06 裁决 (i)：交付→结束参与是
// TF 自己的生命周期规则，归编排）。只在首登成立时非零；`NO_ACTIVE_PARTICIPATION` 是可观察的一格而不是
// 静默——交付登上了、但没有任何参与被它结束，调用方要知道。
func (result RegisterEffectiveDeliveryResult) ParticipationEnd() ParticipationEndOutcome {
	return result.participationEnd
}

func (result RegisterEffectiveDeliveryResult) Outcome() DeliveryRegistrationOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result RegisterEffectiveDeliveryResult) UndecidedReason() DeliveryUndecidedReason {
	return result.reason
}

func (result RegisterEffectiveDeliveryResult) Record() (ports.EffectiveDeliveryRecord, bool) {
	return result.record, result.hasRecord
}

func (result RegisterEffectiveDeliveryResult) ContinuationReference() string {
	return result.continuation
}

// DeliveryHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result RegisterEffectiveDeliveryResult) DeliveryHandoffReference() string {
	return result.handoff
}

// ParticipationEnder 是结束参与那条编排在两条来源编排（交付、交接）眼里的形状。它是接口而不是具体
// 处理器，只为了测试能用替身把「触发发生了没有」与「结束参与自己的规则」分开测。
type ParticipationEnder interface {
	End(ctx context.Context, command EndFulfillmentParticipationCommand) (EndFulfillmentParticipationResult, error)
}

type RegisterEffectiveDeliveryDeps struct {
	Attempts   ports.DeliveryAttemptView
	Deliveries ports.EffectiveDeliveryStore
	Versions   ports.DeliveryIdentityFactory
	Downstream ports.EffectiveDeliveryHandoff
	Clock      ports.Clock
	// ParticipationEnds 让交付落库后同事务结束该对象的履约参与（票 06 裁决 (i)）。**生产装配必须交入**：
	// 「有效交付→结束参与」是 CONTEXT 生命周期③。缺席时不 panic 也不静默——交付照登，结果答
	// ParticipationEndNotWired 那一格；装配点有没有交入由真库装配测试钉（它断言的是 ENDED / NO_ACTIVE）。
	ParticipationEnds ParticipationEnder
}

type RegisterEffectiveDeliveryHandler struct {
	deps RegisterEffectiveDeliveryDeps
}

func NewRegisterEffectiveDeliveryHandler(deps RegisterEffectiveDeliveryDeps) *RegisterEffectiveDeliveryHandler {
	return &RegisterEffectiveDeliveryHandler{deps: deps}
}

// Register 首登一次交付生效：幂等按（租户+对象+尝试）分重放/冲突 → 读回尝试与对象
// 结果 → FormEffectiveDelivery（失败结果进不来，领域硬句编排不绕）→ 原子提交 →
// 意图交 PS 终局。
func (handler *RegisterEffectiveDeliveryHandler) Register(
	ctx context.Context,
	command RegisterEffectiveDeliveryCommand,
) (RegisterEffectiveDeliveryResult, error) {
	key, attemptRef, objectRef, result := handler.deliveryKey(command.TenantID, command.Attempt, command.Object)
	if result != nil {
		return *result, nil
	}
	digest := deliveryContentDigest(command)
	existing, found, err := handler.deps.Deliveries.FindByKey(ctx, key)
	if err != nil {
		return deliveryStoreUndecided(command.Object), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一（对象+尝试）携带不同 POD/方式/接收方：首登不顶替，修 POD 走更正入口。
			return RegisterEffectiveDeliveryResult{outcome: DeliveryRegistrationConflict}, nil
		}
		return handler.existingResult(ctx, existing), nil
	}

	attempt, attemptResult, resultFound, err := handler.deps.Attempts.LoadDeliveryResult(ctx, command.TenantID, attemptRef, objectRef)
	if err != nil {
		return RegisterEffectiveDeliveryResult{outcome: DeliveryUndecided, reason: DeliveryViewUnavailable,
			continuation: deliveryContinuation("DELIVERY_VIEW_UNAVAILABLE", command.Object)}, nil
	}
	if !resultFound {
		return RegisterEffectiveDeliveryResult{outcome: DeliveryNotAccepted}, nil
	}

	spec, badInput := deliverySpecFrom(command)
	if badInput {
		return RegisterEffectiveDeliveryResult{outcome: DeliveryNotAccepted}, nil
	}
	version, err := handler.deps.Versions.NextDeliveryResultVersion(ctx)
	if err != nil {
		return RegisterEffectiveDeliveryResult{outcome: DeliveryUndecided, reason: DeliveryIdentityUnavailable,
			continuation: deliveryContinuation("DELIVERY_IDENTITY_UNAVAILABLE", command.Object)}, nil
	}
	spec.Version = version

	delivery, err := domain.FormEffectiveDelivery(attempt, attemptResult, spec)
	if errors.Is(err, domain.ErrNotAnEffectiveDelivery) {
		// 无人签收、地址错误、拒收：对象本次没有交付成——业务负向，登不出生效交付。
		return RegisterEffectiveDeliveryResult{outcome: DeliveryNotEffective}, nil
	}
	if err != nil {
		return RegisterEffectiveDeliveryResult{outcome: DeliveryNotAccepted}, nil
	}

	record := ports.EffectiveDeliveryRecord{
		Key:           key,
		ContentDigest: digest,
		Delivery:      delivery,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Deliveries.Save(ctx, record)
	if err != nil {
		return deliveryStoreUndecided(command.Object), nil
	}
	switch saved {
	case ports.DeliverySaved:
		out := RegisterEffectiveDeliveryResult{outcome: DeliveryRegistered, record: record, hasRecord: true}
		out.handoff = handler.handOff(ctx, record)
		if out.participationEnd, err = handler.endParticipation(ctx, command); err != nil {
			return RegisterEffectiveDeliveryResult{}, err
		}
		return out, nil
	case ports.DeliveryAlreadyRegistered:
		winner, found, err := handler.deps.Deliveries.FindByKey(ctx, key)
		if err != nil || !found {
			return deliveryStoreUndecided(command.Object), nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return RegisterEffectiveDeliveryResult{}, fmt.Errorf("%w: %d", ErrUnexpectedDeliverySave, saved)
	}
}

// endParticipation 在交付落库后同事务结束该对象的履约参与（CONTEXT 生命周期③，票 06 裁决 (i)）。
//
// 命令不带段：交付是关于对象的事实，段由结束参与那条编排按对象找。**结束失败则整笔不落**——编排返回
// error，或依赖没应上的`未决`，都作 error 交回，让事务边界把交付一起回滚：不要「交付落了参与没结」的半成品。
// 零个在场参与不是失败，那一格原样透出。
func (handler *RegisterEffectiveDeliveryHandler) endParticipation(
	ctx context.Context,
	command RegisterEffectiveDeliveryCommand,
) (ParticipationEndOutcome, error) {
	if handler.deps.ParticipationEnds == nil {
		return ParticipationEndNotWired, nil
	}
	ended, err := handler.deps.ParticipationEnds.End(ctx, EndFulfillmentParticipationCommand{
		TenantID: command.TenantID,
		Object:   command.Object,
		Source:   ParticipationEndedByDelivery,
		Attempt:  command.Attempt,
	})
	if err != nil {
		return ParticipationEndOutcomeInvalid, fmt.Errorf("end participation on delivery: %w", err)
	}
	if ended.Outcome() == ParticipationEndUndecided {
		return ParticipationEndOutcomeInvalid, fmt.Errorf("end participation on delivery: %w: %s", ErrParticipationEndUnsettled, ended.ContinuationReference())
	}
	return ended.Outcome(), nil
}

// ErrParticipationEndUnsettled 说明结束参与那一半停在`未决`：与来源保全那侧不同，这里按票 06 裁决整笔回滚，
// 调用方重试整次交付。
var ErrParticipationEndUnsettled = errors.New("transport fulfillment: the participation end did not settle")

// Correct 对已登记的交付生效落 POD 更正版本：CorrectProof 换新版回指前版（领域已钉
// 同版本覆盖拒、更正早于交付拒），原版本链随本体保全；新版本随意图重新交付下游。
func (handler *RegisterEffectiveDeliveryHandler) Correct(
	ctx context.Context,
	command CorrectDeliveryProofCommand,
) (RegisterEffectiveDeliveryResult, error) {
	key, _, _, bad := handler.deliveryKey(command.TenantID, command.Attempt, command.Object)
	if bad != nil {
		return *bad, nil
	}
	newProof, err := domain.NewDeliveryProofReference(command.NewProof)
	if err != nil {
		return RegisterEffectiveDeliveryResult{outcome: DeliveryNotAccepted}, nil
	}

	existing, found, err := handler.deps.Deliveries.FindByKey(ctx, key)
	if err != nil {
		return deliveryStoreUndecided(command.Object), nil
	}
	if !found {
		// 没有可更正的登记：更正不出无中生有的交付。
		return RegisterEffectiveDeliveryResult{outcome: DeliveryNotAccepted}, nil
	}

	version, err := handler.deps.Versions.NextDeliveryResultVersion(ctx)
	if err != nil {
		return RegisterEffectiveDeliveryResult{outcome: DeliveryUndecided, reason: DeliveryIdentityUnavailable,
			continuation: deliveryContinuation("DELIVERY_IDENTITY_UNAVAILABLE", command.Object)}, nil
	}
	corrected, err := existing.Delivery.CorrectProof(newProof, version, command.CorrectedAt)
	if err != nil {
		return RegisterEffectiveDeliveryResult{outcome: DeliveryNotAccepted}, nil
	}

	record := ports.EffectiveDeliveryRecord{
		Key:           key,
		ContentDigest: existing.ContentDigest,
		Delivery:      corrected,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	superseded, err := handler.deps.Deliveries.Supersede(ctx, record)
	if err != nil {
		return deliveryStoreUndecided(command.Object), nil
	}
	if !superseded {
		return RegisterEffectiveDeliveryResult{outcome: DeliveryNotAccepted}, nil
	}
	out := RegisterEffectiveDeliveryResult{outcome: DeliveryCorrected, record: record, hasRecord: true}
	out.handoff = handler.handOff(ctx, record)
	return out, nil
}

// deliveryKey 受理两入口共用的最小指名；立不起引用即未受理。
func (handler *RegisterEffectiveDeliveryHandler) deliveryKey(
	tenant domain.TenantID,
	attempt string,
	object string,
) (ports.EffectiveDeliveryKey, domain.AttemptReference, domain.CarriedObjectReference, *RegisterEffectiveDeliveryResult) {
	notAccepted := &RegisterEffectiveDeliveryResult{outcome: DeliveryNotAccepted}
	if strings.TrimSpace(tenant.String()) == "" {
		return ports.EffectiveDeliveryKey{}, domain.AttemptReference{}, domain.CarriedObjectReference{}, notAccepted
	}
	attemptRef, err := domain.NewAttemptReference(attempt)
	if err != nil {
		return ports.EffectiveDeliveryKey{}, domain.AttemptReference{}, domain.CarriedObjectReference{}, notAccepted
	}
	objectRef, err := domain.NewCarriedObjectReference(object)
	if err != nil {
		return ports.EffectiveDeliveryKey{}, domain.AttemptReference{}, domain.CarriedObjectReference{}, notAccepted
	}
	return ports.EffectiveDeliveryKey{TenantID: tenant, Object: objectRef, Attempt: attemptRef}, attemptRef, objectRef, nil
}

func deliverySpecFrom(command RegisterEffectiveDeliveryCommand) (domain.EffectiveDeliverySpec, bool) {
	method, err := domain.NewDeliveryMethodReference(command.Method)
	if err != nil {
		return domain.EffectiveDeliverySpec{}, true
	}
	recipient, err := domain.NewReceivingPartyReference(command.Recipient)
	if err != nil {
		return domain.EffectiveDeliverySpec{}, true
	}
	proof, err := domain.NewDeliveryProofReference(command.Proof)
	if err != nil {
		// POD 必备：没有符合当时规则的交付证明就没有生效交付。
		return domain.EffectiveDeliverySpec{}, true
	}
	return domain.EffectiveDeliverySpec{Method: method, Recipient: recipient, Proof: proof}, false
}

func deliveryStoreUndecided(object string) RegisterEffectiveDeliveryResult {
	return RegisterEffectiveDeliveryResult{
		outcome:      DeliveryUndecided,
		reason:       DeliveryStoreUnavailable,
		continuation: deliveryContinuation("DELIVERY_STORE_UNAVAILABLE", object),
	}
}

// existingResult 按已有登记作答并重发同一份意图。
func (handler *RegisterEffectiveDeliveryHandler) existingResult(
	ctx context.Context,
	record ports.EffectiveDeliveryRecord,
) RegisterEffectiveDeliveryResult {
	return RegisterEffectiveDeliveryResult{
		outcome:   DeliveryExistingVersion,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record),
	}
}

// handOff 把交付生效交给 parcel-shipment 终局判断。投递失败不翻结果，留续办引用
// 重放时重发同一份。
func (handler *RegisterEffectiveDeliveryHandler) handOff(
	ctx context.Context,
	record ports.EffectiveDeliveryRecord,
) string {
	if err := handler.deps.Downstream.HandOffEffectiveDelivery(ctx, ports.EffectiveDeliveryHandoffIntent{Record: record}); err == nil {
		return ""
	}
	return deliveryContinuation("EFFECTIVE_DELIVERY_HANDOFF", record.Key.TenantID.String(), record.Key.Object.String())
}

func deliveryContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// deliveryContentDigest 是同一（对象+尝试）首登的内容比对锚：方式、接收方与 POD 任一
// 不同即是另一份内容。
func deliveryContentDigest(command RegisterEffectiveDeliveryCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Method,
		command.Recipient,
		command.Proof,
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
