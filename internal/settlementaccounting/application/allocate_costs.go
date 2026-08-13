package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUnexpectedOperatingSave 说明分摊/指标库交回了封闭集合以外的写入结果。
var ErrUnexpectedOperatingSave = errors.New("settlement accounting: unexpected operating save outcome")

// OperatingOutcome 是成本分摊与经营结果编排的应用处理结果。
type OperatingOutcome uint8

const (
	OperatingOutcomeInvalid OperatingOutcome = iota
	CostAllocated
	AllocationExistingResult
	AllocationConflict
	Reallocated
	ResultDerived
	ResultExistingResult
	ResultConflict
	ResultRederived
	OperatingNotAccepted
	OperatingUndecided
)

func (outcome OperatingOutcome) String() string {
	switch outcome {
	case CostAllocated:
		return "COST_ALLOCATED"
	case AllocationExistingResult:
		return "EXISTING_ALLOCATION"
	case AllocationConflict:
		return "ALLOCATION_CONFLICT"
	case Reallocated:
		return "REALLOCATED"
	case ResultDerived:
		return "RESULT_DERIVED"
	case ResultExistingResult:
		return "EXISTING_RESULT"
	case ResultConflict:
		return "RESULT_CONFLICT"
	case ResultRederived:
		return "RESULT_REDERIVED"
	case OperatingNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case OperatingUndecided:
		return "OPERATING_UNDECIDED"
	default:
		return ""
	}
}

// OperatingUndecidedReason 指名提交停在哪一步等谁。分摊规则目录未配置是实例半边的
// 一格——无规则不分摊、不默认均摊。
type OperatingUndecidedReason uint8

const (
	OperatingUndecidedReasonNone OperatingUndecidedReason = iota
	AllocationStoreUnavailable
	ResultStoreUnavailable
	RuleViewUnavailable
	RuleUnconfigured
)

func (reason OperatingUndecidedReason) String() string {
	switch reason {
	case AllocationStoreUnavailable:
		return "ALLOCATION_STORE_UNAVAILABLE"
	case ResultStoreUnavailable:
		return "RESULT_STORE_UNAVAILABLE"
	case RuleViewUnavailable:
		return "RULE_VIEW_UNAVAILABLE"
	case RuleUnconfigured:
		return "RULE_UNCONFIGURED"
	default:
		return ""
	}
}

// PortionDirective 是一条分摊份额指令。
type PortionDirective struct {
	Target      string
	AmountMinor int64
}

// AllocateCostCommand 携带一次成本分摊。刻意没有规则字段——规则版本由视图给出（无
// 规则不分摊），份额是规则引擎算好的输入，守恒由领域把门。
type AllocateCostCommand struct {
	TenantID    domain.TenantID
	Allocation  string
	Source      string
	SourceMinor int64
	Currency    string
	Portions    []PortionDirective
	Version     string
	AllocatedAt time.Time
}

// ReallocateCommand 携带一次重分摊：换版本保留原分摊（版本链在本体上）。
type ReallocateCommand struct {
	TenantID    domain.TenantID
	Allocation  string
	Portions    []PortionDirective
	NewVersion  string
	AllocatedAt time.Time
}

// ComponentDirective 是一条指标组成项指令。
type ComponentDirective struct {
	Source      string
	Effect      domain.ComponentEffect
	AmountMinor int64
}

// DeriveResultCommand 携带一次经营结果派生：净额只由组成项算出，命令里没有可以直接
// 写毛利的字段（指标是派生不可编辑，领域已钉）。
type DeriveResultCommand struct {
	TenantID   domain.TenantID
	Scope      string
	Period     string
	Basis      domain.OperatingBasis
	Currency   string
	Components []ComponentDirective
	Version    string
	AsOf       time.Time
}

// RederiveResultCommand 携带一次重派生：迟到成本换新版本关联原截点，原快照不变
// （AT-SA-137）。
type RederiveResultCommand struct {
	TenantID   domain.TenantID
	Scope      string
	Period     string
	Basis      domain.OperatingBasis
	Components []ComponentDirective
	NewVersion string
	AsOf       time.Time
}

type OperatingResult struct {
	outcome      OperatingOutcome
	reason       OperatingUndecidedReason
	allocation   ports.AllocationRecord
	result       ports.OperatingResultRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result OperatingResult) Outcome() OperatingOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result OperatingResult) UndecidedReason() OperatingUndecidedReason {
	return result.reason
}

func (result OperatingResult) Allocation() (ports.AllocationRecord, bool) {
	return result.allocation, result.hasRecord && result.allocation.Key.Allocation.String() != ""
}

func (result OperatingResult) Result() (ports.OperatingResultRecord, bool) {
	return result.result, result.hasRecord && result.result.Key.Scope.String() != ""
}

func (result OperatingResult) ContinuationReference() string {
	return result.continuation
}

// OperatingHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result OperatingResult) OperatingHandoffReference() string {
	return result.handoff
}

type AllocateCostsDeps struct {
	Allocations ports.CostAllocationStore
	Results     ports.OperatingResultStore
	Rules       ports.AllocationRuleView
	Downstream  ports.OperatingHandoff
	Clock       ports.Clock
}

type AllocateCostsHandler struct {
	deps AllocateCostsDeps
}

func NewAllocateCostsHandler(deps AllocateCostsDeps) *AllocateCostsHandler {
	return &AllocateCostsHandler{deps: deps}
}

// Allocate 形成一次成本分摊：规则版本由视图核对（未配置→未决不默认均摊）→
// FormCostAllocation（守恒领域把门：份额+余额恒等来源、超额拒、来源只引用不修改）
// → 幂等按（租户+分摊标识）→ 意图交分析。
func (handler *AllocateCostsHandler) Allocate(
	ctx context.Context,
	command AllocateCostCommand,
) (OperatingResult, error) {
	allocationID, err := domain.NewAllocationID(command.Allocation)
	if err != nil {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}
	source, err := domain.NewAllocationSourceReference(command.Source)
	if err != nil {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}

	rule, configured, err := handler.deps.Rules.LoadAllocationRule(ctx, command.TenantID, source)
	if err != nil {
		return operatingUndecided(RuleViewUnavailable, command.Allocation), nil
	}
	if !configured {
		// 分摊规则目录是实例半边：未配置停在未决——无规则不分摊、不默认均摊。
		return operatingUndecided(RuleUnconfigured, command.Allocation), nil
	}

	allocation, err := allocationFrom(command, allocationID, source, rule)
	if err != nil {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}

	key := ports.AllocationKey{TenantID: command.TenantID, Allocation: allocationID}
	digest := allocateDigest(command)
	existing, found, err := handler.deps.Allocations.FindByKey(ctx, key)
	if err != nil {
		return operatingUndecided(AllocationStoreUnavailable, command.Allocation), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一分摊标识携带不同份额：冲突保留原分摊——改法走重分摊换版本。
			return OperatingResult{outcome: AllocationConflict}, nil
		}
		return handler.existingAllocation(ctx, existing), nil
	}

	record := ports.AllocationRecord{Key: key, ContentDigest: digest, Allocation: allocation, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Allocations.Save(ctx, record)
	if err != nil {
		return operatingUndecided(AllocationStoreUnavailable, command.Allocation), nil
	}
	switch saved {
	case ports.AllocationSaved:
		result := OperatingResult{outcome: CostAllocated, allocation: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.OperatingIntent{Allocation: record}, command.Allocation)
		return result, nil
	case ports.AllocationAlreadyFormed:
		winner, found, err := handler.deps.Allocations.FindByKey(ctx, key)
		if err != nil || !found {
			return operatingUndecided(AllocationStoreUnavailable, command.Allocation), nil
		}
		return handler.existingAllocation(ctx, winner), nil
	default:
		return OperatingResult{}, fmt.Errorf("%w: %d", ErrUnexpectedOperatingSave, saved)
	}
}

// Reallocate 换版本重分摊：规则重新核对、原分摊保留（版本链回指前版）、来源不动。
func (handler *AllocateCostsHandler) Reallocate(
	ctx context.Context,
	command ReallocateCommand,
) (OperatingResult, error) {
	allocationID, err := domain.NewAllocationID(command.Allocation)
	if err != nil {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}
	newVersion, err := domain.NewAllocationVersion(command.NewVersion)
	if err != nil {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}

	key := ports.AllocationKey{TenantID: command.TenantID, Allocation: allocationID}
	existing, found, err := handler.deps.Allocations.FindByKey(ctx, key)
	if err != nil {
		return operatingUndecided(AllocationStoreUnavailable, command.Allocation), nil
	}
	if !found {
		// 没有可重分的分摊：重分不出无中生有的归因。
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}

	rule, configured, err := handler.deps.Rules.LoadAllocationRule(ctx, command.TenantID, existing.Allocation.Source())
	if err != nil {
		return operatingUndecided(RuleViewUnavailable, command.Allocation), nil
	}
	if !configured {
		return operatingUndecided(RuleUnconfigured, command.Allocation), nil
	}

	portions, bad := portionsFrom(command.Portions)
	if bad {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}
	reallocated, err := existing.Allocation.Reallocate(rule, portions, newVersion, command.AllocatedAt)
	if err != nil {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}

	existing.Allocation = reallocated
	existing.ContentDigest = reallocateDigest(command)
	existing.RecordedAt = handler.deps.Clock.Now()
	ok, err := handler.deps.Allocations.Replace(ctx, existing)
	if err != nil {
		return operatingUndecided(AllocationStoreUnavailable, command.Allocation), nil
	}
	if !ok {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}
	result := OperatingResult{outcome: Reallocated, allocation: existing, hasRecord: true}
	result.handoff = handler.handOff(ctx, ports.OperatingIntent{Allocation: existing}, command.Allocation)
	return result, nil
}

// Derive 派生一份经营结果快照：净额只由组成项算出（DeriveOperatingResult 领域把门），
// 同一（口径+账期+基准）一版一登，重放返原、异内容冲突。
func (handler *AllocateCostsHandler) Derive(
	ctx context.Context,
	command DeriveResultCommand,
) (OperatingResult, error) {
	key, bad := handler.resultKey(command.TenantID, command.Scope, command.Period, command.Basis)
	if bad != nil {
		return *bad, nil
	}
	digest := deriveDigest(command)
	existing, found, err := handler.deps.Results.FindByKey(ctx, key)
	if err != nil {
		return operatingUndecided(ResultStoreUnavailable, command.Scope), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一口径携带不同组成：冲突保留原快照——迟到成本走重派生换版本。
			return OperatingResult{outcome: ResultConflict}, nil
		}
		result := OperatingResult{outcome: ResultExistingResult, result: existing, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.OperatingIntent{Result: existing}, command.Scope)
		return result, nil
	}

	derived, err := deriveFrom(command, key)
	if err != nil {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}
	record := ports.OperatingResultRecord{Key: key, ContentDigest: digest, Result: derived, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Results.Save(ctx, record)
	if err != nil {
		return operatingUndecided(ResultStoreUnavailable, command.Scope), nil
	}
	switch saved {
	case ports.OperatingResultSaved:
		result := OperatingResult{outcome: ResultDerived, result: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.OperatingIntent{Result: record}, command.Scope)
		return result, nil
	case ports.OperatingResultAlreadyDerived:
		winner, found, err := handler.deps.Results.FindByKey(ctx, key)
		if err != nil || !found {
			return operatingUndecided(ResultStoreUnavailable, command.Scope), nil
		}
		result := OperatingResult{outcome: ResultExistingResult, result: winner, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.OperatingIntent{Result: winner}, command.Scope)
		return result, nil
	default:
		return OperatingResult{}, fmt.Errorf("%w: %d", ErrUnexpectedOperatingSave, saved)
	}
}

// Rederive 迟到成本换新版本重派生：原快照不变（版本链回指原版，AT-SA-137）。
func (handler *AllocateCostsHandler) Rederive(
	ctx context.Context,
	command RederiveResultCommand,
) (OperatingResult, error) {
	key, bad := handler.resultKey(command.TenantID, command.Scope, command.Period, command.Basis)
	if bad != nil {
		return *bad, nil
	}
	newVersion, err := domain.NewOperatingResultVersion(command.NewVersion)
	if err != nil {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}

	existing, found, err := handler.deps.Results.FindByKey(ctx, key)
	if err != nil {
		return operatingUndecided(ResultStoreUnavailable, command.Scope), nil
	}
	if !found {
		// 没有可重派的快照：重派不出无中生有的指标。
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}

	components, bad2 := componentsFrom(command.Components)
	if bad2 {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}
	rederived, err := existing.Result.Rederive(components, newVersion, command.AsOf)
	if err != nil {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}

	existing.Result = rederived
	existing.ContentDigest = rederiveDigest(command)
	existing.RecordedAt = handler.deps.Clock.Now()
	ok, err := handler.deps.Results.Replace(ctx, existing)
	if err != nil {
		return operatingUndecided(ResultStoreUnavailable, command.Scope), nil
	}
	if !ok {
		return OperatingResult{outcome: OperatingNotAccepted}, nil
	}
	result := OperatingResult{outcome: ResultRederived, result: existing, hasRecord: true}
	result.handoff = handler.handOff(ctx, ports.OperatingIntent{Result: existing}, command.Scope)
	return result, nil
}

func (handler *AllocateCostsHandler) resultKey(
	tenant domain.TenantID,
	scope string,
	period string,
	basis domain.OperatingBasis,
) (ports.OperatingResultKey, *OperatingResult) {
	notAccepted := &OperatingResult{outcome: OperatingNotAccepted}
	scopeRef, err := domain.NewOperatingScopeReference(scope)
	if err != nil {
		return ports.OperatingResultKey{}, notAccepted
	}
	periodRef, err := domain.NewBillingPeriodReference(period)
	if err != nil {
		return ports.OperatingResultKey{}, notAccepted
	}
	return ports.OperatingResultKey{TenantID: tenant, Scope: scopeRef, Period: periodRef, Basis: basis}, nil
}

func allocationFrom(
	command AllocateCostCommand,
	allocationID domain.AllocationID,
	source domain.AllocationSourceReference,
	rule domain.AllocationRuleVersionReference,
) (domain.CostAllocation, error) {
	spec := domain.CostAllocationSpec{
		ID:          allocationID,
		Source:      source,
		SourceMinor: command.SourceMinor,
		Rule:        rule,
		AllocatedAt: command.AllocatedAt,
	}
	var err error
	if spec.Currency, err = domain.NewCurrencyCode(command.Currency); err != nil {
		return domain.CostAllocation{}, err
	}
	if spec.Version, err = domain.NewAllocationVersion(command.Version); err != nil {
		return domain.CostAllocation{}, err
	}
	portions, bad := portionsFrom(command.Portions)
	if bad {
		return domain.CostAllocation{}, domain.ErrInvalidCostAllocation
	}
	spec.Portions = portions
	return domain.FormCostAllocation(spec)
}

func portionsFrom(directives []PortionDirective) ([]domain.AllocationPortion, bool) {
	portions := make([]domain.AllocationPortion, 0, len(directives))
	for _, directive := range directives {
		target, err := domain.NewAllocationTargetReference(directive.Target)
		if err != nil {
			return nil, true
		}
		portions = append(portions, domain.AllocationPortion{Target: target, AmountMinor: directive.AmountMinor})
	}
	return portions, false
}

func componentsFrom(directives []ComponentDirective) ([]domain.ResultComponent, bool) {
	components := make([]domain.ResultComponent, 0, len(directives))
	for _, directive := range directives {
		source, err := domain.NewComponentSourceReference(directive.Source)
		if err != nil {
			return nil, true
		}
		components = append(components, domain.ResultComponent{
			Source:      source,
			Effect:      directive.Effect,
			AmountMinor: directive.AmountMinor,
		})
	}
	return components, false
}

func deriveFrom(command DeriveResultCommand, key ports.OperatingResultKey) (domain.OperatingResult, error) {
	currency, err := domain.NewCurrencyCode(command.Currency)
	if err != nil {
		return domain.OperatingResult{}, err
	}
	version, err := domain.NewOperatingResultVersion(command.Version)
	if err != nil {
		return domain.OperatingResult{}, err
	}
	components, bad := componentsFrom(command.Components)
	if bad {
		return domain.OperatingResult{}, domain.ErrInvalidOperatingResult
	}
	return domain.DeriveOperatingResult(key.Scope, key.Period, command.Basis, currency, components, version, command.AsOf)
}

func operatingUndecided(reason OperatingUndecidedReason, subject string) OperatingResult {
	return OperatingResult{
		outcome:      OperatingUndecided,
		reason:       reason,
		continuation: operatingContinuation(reason.String(), subject),
	}
}

// existingAllocation 按已有分摊作答并重发同一份意图。
func (handler *AllocateCostsHandler) existingAllocation(
	ctx context.Context,
	record ports.AllocationRecord,
) OperatingResult {
	return OperatingResult{
		outcome:    AllocationExistingResult,
		allocation: record,
		hasRecord:  true,
		handoff:    handler.handOff(ctx, ports.OperatingIntent{Allocation: record}, record.Key.Allocation.String()),
	}
}

// handOff 交发布意图。投递失败不翻结果，留续办引用重放时重发同一份。
func (handler *AllocateCostsHandler) handOff(
	ctx context.Context,
	intent ports.OperatingIntent,
	subject string,
) string {
	if err := handler.deps.Downstream.HandOffOperating(ctx, intent); err == nil {
		return ""
	}
	return operatingContinuation("OPERATING_HANDOFF", subject)
}

func operatingContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

func portionsDigestPart(portions []PortionDirective) []string {
	parts := make([]string, 0, len(portions))
	for _, portion := range portions {
		parts = append(parts, fmt.Sprintf("%s|%d", portion.Target, portion.AmountMinor))
	}
	sort.Strings(parts)
	return parts
}

func componentsDigestPart(components []ComponentDirective) []string {
	parts := make([]string, 0, len(components))
	for _, component := range components {
		parts = append(parts, fmt.Sprintf("%s|%d|%d", component.Source, component.Effect, component.AmountMinor))
	}
	sort.Strings(parts)
	return parts
}

func allocateDigest(command AllocateCostCommand) string {
	parts := append([]string{
		command.Source,
		fmt.Sprintf("%d", command.SourceMinor),
		command.Currency,
		command.Version,
		command.AllocatedAt.UTC().Format(time.RFC3339Nano),
	}, portionsDigestPart(command.Portions)...)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func reallocateDigest(command ReallocateCommand) string {
	parts := append([]string{
		command.NewVersion,
		command.AllocatedAt.UTC().Format(time.RFC3339Nano),
	}, portionsDigestPart(command.Portions)...)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func deriveDigest(command DeriveResultCommand) string {
	parts := append([]string{
		command.Currency,
		command.Version,
		command.AsOf.UTC().Format(time.RFC3339Nano),
	}, componentsDigestPart(command.Components)...)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func rederiveDigest(command RederiveResultCommand) string {
	parts := append([]string{
		command.NewVersion,
		command.AsOf.UTC().Format(time.RFC3339Nano),
	}, componentsDigestPart(command.Components)...)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}
