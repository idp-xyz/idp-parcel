package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// SummarizeHandoverScopeOutcome 是交接范围汇总的应用处理结果。
//
// `不成立汇总`独立成格而不复用「一份三个零的汇总」：零说的是「这个范围有交接，只是这一格
// 没有」，不成立说的是「这个范围还没有交接」，两者要调用方做的事不同。领域侧
// SummarizeHandovers 对空集本就拒绝派生，本格是它在编排层的对应答案。
type SummarizeHandoverScopeOutcome uint8

const (
	SummarizeHandoverScopeOutcomeInvalid SummarizeHandoverScopeOutcome = iota
	HandoverScopeSummarized
	HandoverScopeNotSummarizable
	HandoverScopeUndecided
	HandoverScopeInputNotAccepted
)

func (outcome SummarizeHandoverScopeOutcome) String() string {
	switch outcome {
	case HandoverScopeSummarized:
		return "SCOPE_SUMMARIZED"
	case HandoverScopeNotSummarizable:
		return "SCOPE_NOT_SUMMARIZABLE"
	case HandoverScopeUndecided:
		return "SCOPE_UNDECIDED"
	case HandoverScopeInputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// HandoverScopeUndecidedReason 指名汇总停在哪一步等谁。
type HandoverScopeUndecidedReason uint8

const (
	HandoverScopeUndecidedReasonNone HandoverScopeUndecidedReason = iota
	HandoverScopeRegistryUnavailable
)

func (reason HandoverScopeUndecidedReason) String() string {
	switch reason {
	case HandoverScopeRegistryUnavailable:
		return "HANDOVER_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// SummarizeHandoverScopeQuery 携带一次范围汇总查询。租户显式入参，不从 ctx 里补。
type SummarizeHandoverScopeQuery struct {
	TenantID domain.TenantID
	Scope    string
}

// SummarizeHandoverScopeResult 交回一次汇总的结果。汇总本体只在`已汇总`时在场。
type SummarizeHandoverScopeResult struct {
	outcome      SummarizeHandoverScopeOutcome
	reason       HandoverScopeUndecidedReason
	summary      domain.HandoverScopeSummary
	hasSummary   bool
	continuation string
}

func (result SummarizeHandoverScopeResult) Outcome() SummarizeHandoverScopeOutcome {
	return result.outcome
}

func (result SummarizeHandoverScopeResult) Reason() HandoverScopeUndecidedReason {
	return result.reason
}

func (result SummarizeHandoverScopeResult) Summary() (domain.HandoverScopeSummary, bool) {
	if !result.hasSummary {
		return domain.HandoverScopeSummary{}, false
	}
	return result.summary, true
}

func (result SummarizeHandoverScopeResult) Continuation() string {
	return result.continuation
}

// SummarizeHandoverScopeHandler 派生一个交接范围的汇总。
type SummarizeHandoverScopeHandler struct {
	scopes ports.HandoverScopeView
}

func NewSummarizeHandoverScopeHandler(scopes ports.HandoverScopeView) *SummarizeHandoverScopeHandler {
	return &SummarizeHandoverScopeHandler{scopes: scopes}
}

// Summarize 取回该范围的成员再交给领域派生。
//
// 计数不在这里数，也不下沉到 SQL：SummarizeHandovers 自带跨范围与跨租户的成员校验，
// 绕过它去数就等于为同一形状立第二个口径，领域改一次判据、那一份会悄悄漂移。
func (handler *SummarizeHandoverScopeHandler) Summarize(
	ctx context.Context,
	query SummarizeHandoverScopeQuery,
) (SummarizeHandoverScopeResult, error) {
	// 最小身份先于任何权威读取：身份立不住时不去读库。
	if strings.TrimSpace(query.TenantID.String()) == "" {
		return SummarizeHandoverScopeResult{outcome: HandoverScopeInputNotAccepted}, nil
	}
	scope, err := domain.NewHandoverScopeReference(query.Scope)
	if err != nil {
		return SummarizeHandoverScopeResult{outcome: HandoverScopeInputNotAccepted}, nil
	}

	records, err := handler.scopes.ListByScope(ctx, query.TenantID, scope)
	if err != nil {
		return SummarizeHandoverScopeResult{
			outcome:      HandoverScopeUndecided,
			reason:       HandoverScopeRegistryUnavailable,
			continuation: scopeSummaryContinuation(query.TenantID.String(), query.Scope),
		}, nil
	}

	members := make([]domain.TransportHandover, 0, len(records))
	for _, record := range records {
		members = append(members, record.Handover)
	}
	summary, err := domain.SummarizeHandovers(members)
	if err != nil {
		// 空集与成员不同源都由领域拒。两者都不是本上下文的失败，是「这个范围派生不出
		// 一份汇总」这个业务答案。
		return SummarizeHandoverScopeResult{outcome: HandoverScopeNotSummarizable}, nil
	}
	return SummarizeHandoverScopeResult{
		outcome:    HandoverScopeSummarized,
		summary:    summary,
		hasSummary: true,
	}, nil
}

func scopeSummaryContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}
