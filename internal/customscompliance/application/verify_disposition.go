package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ErrUnexpectedVerificationSave 说明核对库交回了封闭集合以外的写入结果。
var ErrUnexpectedVerificationSave = errors.New("customs compliance: unexpected verification save outcome")

// VerifyDispositionOutcome 是处置执行核对请求的应用处理结果。
type VerifyDispositionOutcome uint8

const (
	VerifyDispositionOutcomeInvalid VerifyDispositionOutcome = iota
	VerificationRecorded
	VerificationExistingResult
	VerificationNotAccepted
	VerificationUndecided
)

func (outcome VerifyDispositionOutcome) String() string {
	switch outcome {
	case VerificationRecorded:
		return "RECORDED"
	case VerificationExistingResult:
		return "EXISTING_RESULT"
	case VerificationNotAccepted:
		return "NOT_ACCEPTED"
	case VerificationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// VerifyDispositionReason 指名核对停在哪一步。
type VerifyDispositionReason uint8

const (
	VerifyDispositionReasonNone VerifyDispositionReason = iota
	ExecutionFactsUnavailable
	VerificationStoreUnavailable
)

func (reason VerifyDispositionReason) String() string {
	switch reason {
	case ExecutionFactsUnavailable:
		return "EXECUTION_FACTS_UNAVAILABLE"
	case VerificationStoreUnavailable:
		return "VERIFICATION_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// VerifyDispositionCommand 携带一次核对请求：租户与已接收的监管决定本体。
type VerifyDispositionCommand struct {
	TenantID domain.TenantID
	Decision domain.RegulatoryDecision
}

type VerifyDispositionResult struct {
	outcome      VerifyDispositionOutcome
	verification domain.DispositionVerification
	hasRecord    bool
	reason       VerifyDispositionReason
	handoffRef   string
}

func (result VerifyDispositionResult) Outcome() VerifyDispositionOutcome {
	return result.outcome
}

func (result VerifyDispositionResult) Verification() (domain.DispositionVerification, bool) {
	return result.verification, result.hasRecord
}

func (result VerifyDispositionResult) UndecidedReason() VerifyDispositionReason {
	return result.reason
}

// HandoffReference 非空说明核对已提交但意图还没交出去，重放会重发同一份。
func (result VerifyDispositionResult) HandoffReference() string {
	return result.handoffRef
}

type VerifyDispositionDeps struct {
	Facts      ports.ExecutionFactView
	Store      ports.DispositionVerificationStore
	Downstream ports.VerificationHandoff
	Clock      ports.Clock
}

type VerifyDispositionHandler struct {
	deps VerifyDispositionDeps
}

func NewVerifyDispositionHandler(deps VerifyDispositionDeps) *VerifyDispositionHandler {
	return &VerifyDispositionHandler{deps: deps}
}

// Handle 把一份监管决定推进到处置执行核对：读执行事实（空清单如实进核对——证据不足
// 是领域答案不是依赖故障）→ 幂等（同决定同事实集指纹只出一版，新事实到达换指纹换版）
// → VerifyDispositionExecution 领域核对（五值分立）→ 提交与发布意图。核对完成不自动
// 终结监管义务——那半句在领域类型上，编排不复述。
func (handler *VerifyDispositionHandler) Handle(
	ctx context.Context,
	command VerifyDispositionCommand,
) (VerifyDispositionResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		strings.TrimSpace(command.Decision.ID().String()) == "" {
		return VerifyDispositionResult{outcome: VerificationNotAccepted}, nil
	}

	facts, err := handler.deps.Facts.LoadExecutionFacts(ctx, command.TenantID, command.Decision.ID())
	if err != nil {
		return VerifyDispositionResult{
			outcome: VerificationUndecided,
			reason:  ExecutionFactsUnavailable,
		}, nil
	}

	key := ports.VerificationKey{
		TenantID: command.TenantID,
		Decision: command.Decision.ID(),
		Digest:   factSetDigest(facts),
	}
	existing, found, err := handler.deps.Store.FindByKey(ctx, key)
	if err != nil {
		return VerifyDispositionResult{
			outcome: VerificationUndecided,
			reason:  VerificationStoreUnavailable,
		}, nil
	}
	if found {
		return handler.existingResult(ctx, key, existing), nil
	}

	verification, err := domain.VerifyDispositionExecution(
		command.Decision, facts, handler.deps.Clock.Now())
	if err != nil {
		return VerifyDispositionResult{outcome: VerificationNotAccepted}, nil
	}

	saved, err := handler.deps.Store.Save(ctx, key, verification)
	if err != nil {
		return VerifyDispositionResult{
			outcome: VerificationUndecided,
			reason:  VerificationStoreUnavailable,
		}, nil
	}
	switch saved {
	case ports.VerificationSaved:
		result := VerifyDispositionResult{
			outcome:      VerificationRecorded,
			verification: verification,
			hasRecord:    true,
		}
		result.handoffRef = handler.handOff(ctx, key, verification)
		return result, nil
	case ports.VerificationAlreadyRecorded:
		winner, found, err := handler.deps.Store.FindByKey(ctx, key)
		if err != nil || !found {
			return VerifyDispositionResult{
				outcome: VerificationUndecided,
				reason:  VerificationStoreUnavailable,
			}, nil
		}
		return handler.existingResult(ctx, key, winner), nil
	default:
		return VerifyDispositionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedVerificationSave, saved)
	}
}

// existingResult 按已有核对作答并重发同一份意图。
func (handler *VerifyDispositionHandler) existingResult(
	ctx context.Context,
	key ports.VerificationKey,
	verification domain.DispositionVerification,
) VerifyDispositionResult {
	result := VerifyDispositionResult{
		outcome:      VerificationExistingResult,
		verification: verification,
		hasRecord:    true,
	}
	result.handoffRef = handler.handOff(ctx, key, verification)
	return result
}

// handOff 交发布意图。失败不翻核对，留续办引用重发同一份。
func (handler *VerifyDispositionHandler) handOff(
	ctx context.Context,
	key ports.VerificationKey,
	verification domain.DispositionVerification,
) string {
	if err := handler.deps.Downstream.HandOffVerification(ctx, ports.VerificationHandoffIntent{
		Key:          key,
		Verification: verification,
	}); err == nil {
		return ""
	}
	return "CONT-VERIFICATION/" + key.Decision.String() + "/" + key.Digest[:8]
}

// factSetDigest 是事实集的稳定指纹：逐事实引用排序后拼接——同一集合无论装载顺序
// 如何指纹恒定，新事实到达自然换指纹。
func factSetDigest(facts []domain.ExecutionFact) string {
	references := make([]string, 0, len(facts))
	for _, fact := range facts {
		references = append(references, fact.Fact().String())
	}
	sort.Strings(references)
	digest := sha256.Sum256([]byte(strings.Join(references, "\x00")))
	return hex.EncodeToString(digest[:])
}
