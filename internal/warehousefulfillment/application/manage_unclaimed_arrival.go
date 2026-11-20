package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ClaimUnclaimedArrivalOutcome 是运营认领待认领到货的结果（UC-WF-005 结果语义契约）。
type ClaimUnclaimedArrivalOutcome uint8

const (
	ClaimUnclaimedOutcomeInvalid ClaimUnclaimedArrivalOutcome = iota
	ClaimUnclaimedClaimed
	ClaimUnclaimedExistingResult
	ClaimUnclaimedSourceConflict
	ClaimUnclaimedAlreadyClaimed
	ClaimUnclaimedClaimInvalid
	ClaimUnclaimedStatusMismatch
	ClaimUnclaimedArrivalNotFound
	ClaimUnclaimedNotConfigured
)

func (outcome ClaimUnclaimedArrivalOutcome) String() string {
	switch outcome {
	case ClaimUnclaimedClaimed:
		return "CLAIMED"
	case ClaimUnclaimedExistingResult:
		return "EXISTING_RESULT"
	case ClaimUnclaimedSourceConflict:
		return "SOURCE_CONFLICT"
	case ClaimUnclaimedAlreadyClaimed:
		return "ALREADY_CLAIMED"
	case ClaimUnclaimedClaimInvalid:
		return "CLAIM_INVALID"
	case ClaimUnclaimedStatusMismatch:
		return "STATUS_MISMATCH"
	case ClaimUnclaimedArrivalNotFound:
		return "ARRIVAL_NOT_FOUND"
	case ClaimUnclaimedNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

type ClaimUnclaimedArrivalResult struct {
	outcome   ClaimUnclaimedArrivalOutcome
	claim     domain.UnclaimedArrivalClaim
	hasClaim  bool
	field     string
	status    domain.UnclaimedArrivalStatus
	hasStatus bool
	missing   string
}

func (r ClaimUnclaimedArrivalResult) Outcome() ClaimUnclaimedArrivalOutcome { return r.outcome }
func (r ClaimUnclaimedArrivalResult) Claim() (domain.UnclaimedArrivalClaim, bool) {
	return r.claim, r.hasClaim
}
func (r ClaimUnclaimedArrivalResult) Field() string { return r.field }
func (r ClaimUnclaimedArrivalResult) Status() (domain.UnclaimedArrivalStatus, bool) {
	return r.status, r.hasStatus
}
func (r ClaimUnclaimedArrivalResult) Missing() string { return r.missing }

// ClaimUnclaimedArrivalCommand 是运营认领一件待认领到货。Tenant 与 Operator 取自信封。
type ClaimUnclaimedArrivalCommand struct {
	Tenant     domain.TenantID
	DecisionID domain.RequestKey
	ArrivalID  domain.UnclaimedArrivalID
	Owner      domain.OwnerAccountID
	PreAdvice  domain.PreAdviceID
	Operator   domain.Registrant
}

// RevokeUnclaimedClaimOutcome 是运营撤销认领的结果（UC-WF-005 结果语义契约）。
type RevokeUnclaimedClaimOutcome uint8

const (
	RevokeClaimOutcomeInvalid RevokeUnclaimedClaimOutcome = iota
	RevokeClaimRevoked
	RevokeClaimExistingResult
	RevokeClaimSourceConflict
	RevokeClaimNotAllowed
	RevokeClaimStatusMismatch
	RevokeClaimArrivalNotFound
	RevokeClaimNotConfigured
)

func (outcome RevokeUnclaimedClaimOutcome) String() string {
	switch outcome {
	case RevokeClaimRevoked:
		return "CLAIM_REVOKED"
	case RevokeClaimExistingResult:
		return "EXISTING_RESULT"
	case RevokeClaimSourceConflict:
		return "SOURCE_CONFLICT"
	case RevokeClaimNotAllowed:
		return "REVOCATION_NOT_ALLOWED"
	case RevokeClaimStatusMismatch:
		return "STATUS_MISMATCH"
	case RevokeClaimArrivalNotFound:
		return "ARRIVAL_NOT_FOUND"
	case RevokeClaimNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

type RevokeUnclaimedClaimResult struct {
	outcome       RevokeUnclaimedClaimOutcome
	revocation    domain.UnclaimedArrivalClaimRevocation
	hasRevocation bool
	status        domain.UnclaimedArrivalStatus
	hasStatus     bool
	missing       string
}

func (r RevokeUnclaimedClaimResult) Outcome() RevokeUnclaimedClaimOutcome { return r.outcome }
func (r RevokeUnclaimedClaimResult) Revocation() (domain.UnclaimedArrivalClaimRevocation, bool) {
	return r.revocation, r.hasRevocation
}
func (r RevokeUnclaimedClaimResult) Status() (domain.UnclaimedArrivalStatus, bool) {
	return r.status, r.hasStatus
}
func (r RevokeUnclaimedClaimResult) Missing() string { return r.missing }

// RevokeUnclaimedClaimCommand 是运营撤销一件待认领到货的当前认领。按决定标识判重。
type RevokeUnclaimedClaimCommand struct {
	Tenant     domain.TenantID
	DecisionID domain.RequestKey
	ArrivalID  domain.UnclaimedArrivalID
	Reason     domain.UnclaimedArrivalNote
	Operator   domain.Registrant
}

// DisposeUnclaimedArrivalOutcome 是运营处置一件待认领到货的结果（UC-WF-005 结果语义契约）。
type DisposeUnclaimedArrivalOutcome uint8

const (
	DisposeUnclaimedOutcomeInvalid DisposeUnclaimedArrivalOutcome = iota
	DisposeUnclaimedDisposed
	DisposeUnclaimedExistingResult
	DisposeUnclaimedNotAllowed
	DisposeUnclaimedStatusMismatch
	DisposeUnclaimedArrivalNotFound
	DisposeUnclaimedNotConfigured
)

func (outcome DisposeUnclaimedArrivalOutcome) String() string {
	switch outcome {
	case DisposeUnclaimedDisposed:
		return "DISPOSED"
	case DisposeUnclaimedExistingResult:
		return "EXISTING_RESULT"
	case DisposeUnclaimedNotAllowed:
		return "DISPOSITION_NOT_ALLOWED"
	case DisposeUnclaimedStatusMismatch:
		return "STATUS_MISMATCH"
	case DisposeUnclaimedArrivalNotFound:
		return "ARRIVAL_NOT_FOUND"
	case DisposeUnclaimedNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

// DispositionNotAllowedReason 区分处置不成立是认领期限未到还是方式不在该仓规则里（AT-WF-063）。
type DispositionNotAllowedReason uint8

const (
	DispositionNotAllowedNone DispositionNotAllowedReason = iota
	DispositionClaimWindowNotElapsed
	DispositionMethodNotInRule
)

func (reason DispositionNotAllowedReason) String() string {
	switch reason {
	case DispositionClaimWindowNotElapsed:
		return "CLAIM_WINDOW_NOT_ELAPSED"
	case DispositionMethodNotInRule:
		return "METHOD_NOT_IN_RULE"
	default:
		return ""
	}
}

type DisposeUnclaimedArrivalResult struct {
	outcome        DisposeUnclaimedArrivalOutcome
	disposition    domain.UnclaimedArrivalDisposition
	hasDisposition bool
	reason         DispositionNotAllowedReason
	status         domain.UnclaimedArrivalStatus
	hasStatus      bool
	missing        string
}

func (r DisposeUnclaimedArrivalResult) Outcome() DisposeUnclaimedArrivalOutcome { return r.outcome }
func (r DisposeUnclaimedArrivalResult) Disposition() (domain.UnclaimedArrivalDisposition, bool) {
	return r.disposition, r.hasDisposition
}
func (r DisposeUnclaimedArrivalResult) Reason() (DispositionNotAllowedReason, bool) {
	return r.reason, r.outcome == DisposeUnclaimedNotAllowed
}
func (r DisposeUnclaimedArrivalResult) Status() (domain.UnclaimedArrivalStatus, bool) {
	return r.status, r.hasStatus
}
func (r DisposeUnclaimedArrivalResult) Missing() string { return r.missing }

// DisposeUnclaimedArrivalCommand 是运营处置一件待认领到货。重放身份取到货本身，一件到货只处置一次。
type DisposeUnclaimedArrivalCommand struct {
	Tenant    domain.TenantID
	ArrivalID domain.UnclaimedArrivalID
	Method    domain.UnclaimedDisposalMethod
	Note      domain.UnclaimedArrivalNote
	Operator  domain.Registrant
}

// ManageUnclaimedArrivalHandler 承接认领、认领撤销与处置三个运营决定用例。
// 事务边界在调用方；本处理器不拿预报锁，与收货确认的锁序（预报 → 到货）严格无环。
type ManageUnclaimedArrivalHandler struct {
	arrivals   ports.UnclaimedArrivalStore
	rules      ports.DisposalRuleLookup
	preAdvices ports.PreAdviceStore
	warehouses ports.TenantWarehouseLookup
	owners     ports.OwnerAccountLookup
	clock      ports.Clock
}

func NewManageUnclaimedArrivalHandler(
	arrivals ports.UnclaimedArrivalStore,
	rules ports.DisposalRuleLookup,
	preAdvices ports.PreAdviceStore,
	warehouses ports.TenantWarehouseLookup,
	owners ports.OwnerAccountLookup,
	clock ports.Clock,
) *ManageUnclaimedArrivalHandler {
	return &ManageUnclaimedArrivalHandler{
		arrivals:   arrivals,
		rules:      rules,
		preAdvices: preAdvices,
		warehouses: warehouses,
		owners:     owners,
		clock:      clock,
	}
}

// Claim 认领一件待认领到货（UC-WF-005、AT-WF-057、AT-WF-058、AT-WF-061、AT-WF-066）。
//  1. 本租户无仓：NOT_CONFIGURED（missing=WAREHOUSE）；
//  2. 锁决定标识并按决定标识认重放：同内容回显当初那一笔（不看到货此刻状态），异内容答 SOURCE_CONFLICT；
//  3. 向 party-commercial 核货主账户在本租户：查不到交 ErrOwnerAccountNotInTenant（404），读不动交 error；
//  4. 锁到货并读当前详情：查无答 ARRIVAL_NOT_FOUND；已处置答 STATUS_MISMATCH；已认领时同指名答 EXISTING_RESULT，
//     不同指名答 ALREADY_CLAIMED 并回显在册认领；
//  5. 待认领时核预报：退件带了预报或入库到货没带/预报不属于该货主/不在该仓/不可收货均答 CLAIM_INVALID（field=preAdviceId）；
//  6. 记下认领并推进到货为 CLAIMED，不改库存账与预报累计实收。
func (handler *ManageUnclaimedArrivalHandler) Claim(
	ctx context.Context,
	command ClaimUnclaimedArrivalCommand,
) (ClaimUnclaimedArrivalResult, error) {
	if command.Tenant.String() == "" || command.DecisionID.String() == "" || command.ArrivalID.String() == "" ||
		command.Owner.String() == "" || command.Operator.String() == "" {
		return ClaimUnclaimedArrivalResult{}, errors.New("claim unclaimed arrival: tenant, decisionId, arrivalId, owner and operator are required")
	}
	configured, err := handler.warehouses.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: %w", err)
	}
	if !configured {
		return ClaimUnclaimedArrivalResult{outcome: ClaimUnclaimedNotConfigured, missing: "WAREHOUSE"}, nil
	}

	if err := handler.arrivals.HoldClaimDecision(ctx, command.Tenant, command.DecisionID); err != nil {
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: %w", err)
	}
	prior, found, err := handler.arrivals.LoadClaimByDecision(ctx, command.Tenant, command.DecisionID)
	if err != nil {
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: %w", err)
	}
	if found {
		if prior.SameTargetAs(command.ArrivalID, command.Owner, command.PreAdvice) {
			return ClaimUnclaimedArrivalResult{outcome: ClaimUnclaimedExistingResult, claim: prior, hasClaim: true}, nil
		}
		return ClaimUnclaimedArrivalResult{outcome: ClaimUnclaimedSourceConflict}, nil
	}

	if handler.owners == nil {
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: %w", errOwnerLookupMissing)
	}
	ownerRegistered, err := handler.owners.OwnerAccountRegistered(ctx, command.Tenant, command.Owner)
	if err != nil {
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: %w", err)
	}
	if !ownerRegistered {
		return ClaimUnclaimedArrivalResult{}, ErrOwnerAccountNotInTenant
	}

	if err := handler.arrivals.HoldArrival(ctx, command.Tenant, command.ArrivalID); err != nil {
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: %w", err)
	}
	detail, exists, err := handler.arrivals.LoadDetail(ctx, command.Tenant, command.ArrivalID)
	if err != nil {
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: %w", err)
	}
	if !exists {
		return ClaimUnclaimedArrivalResult{outcome: ClaimUnclaimedArrivalNotFound}, nil
	}

	switch detail.Arrival.Status() {
	case domain.UnclaimedArrivalStatusDisposed:
		return ClaimUnclaimedArrivalResult{
			outcome: ClaimUnclaimedStatusMismatch, status: detail.Arrival.Status(), hasStatus: true,
		}, nil
	case domain.UnclaimedArrivalStatusClaimed:
		active, ok := detail.ActiveClaim()
		if !ok {
			return ClaimUnclaimedArrivalResult{}, errors.New("claim unclaimed arrival: 到货为已认领却读不到生效认领")
		}
		if active.SameTargetAs(command.ArrivalID, command.Owner, command.PreAdvice) {
			return ClaimUnclaimedArrivalResult{outcome: ClaimUnclaimedExistingResult, claim: active, hasClaim: true}, nil
		}
		return ClaimUnclaimedArrivalResult{outcome: ClaimUnclaimedAlreadyClaimed, claim: active, hasClaim: true}, nil
	case domain.UnclaimedArrivalStatusUnclaimed:
	default:
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: unexpected status %s", detail.Arrival.Status())
	}

	switch detail.Arrival.Kind() {
	case domain.UnclaimedArrivalKindReturn:
		if command.PreAdvice.String() != "" {
			return ClaimUnclaimedArrivalResult{outcome: ClaimUnclaimedClaimInvalid, field: "preAdviceId"}, nil
		}
	case domain.UnclaimedArrivalKindInbound:
		if command.PreAdvice.String() == "" {
			return ClaimUnclaimedArrivalResult{outcome: ClaimUnclaimedClaimInvalid, field: "preAdviceId"}, nil
		}
		advice, adviceFound, err := handler.preAdvices.Find(ctx, command.Tenant, command.PreAdvice)
		if err != nil {
			return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: %w", err)
		}
		if !adviceFound || advice.Owner() != command.Owner || advice.Warehouse() != detail.Arrival.Warehouse() || !preAdviceReceivable(advice) {
			return ClaimUnclaimedArrivalResult{outcome: ClaimUnclaimedClaimInvalid, field: "preAdviceId"}, nil
		}
	default:
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: unexpected kind %s", detail.Arrival.Kind())
	}

	claim, err := domain.NewUnclaimedArrivalClaim(
		command.Tenant, command.DecisionID, command.ArrivalID, command.Owner, command.PreAdvice, command.Operator, handler.clock.Now(),
	)
	if err != nil {
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: %w", err)
	}
	if err := handler.arrivals.RecordClaim(ctx, claim); err != nil {
		return ClaimUnclaimedArrivalResult{}, fmt.Errorf("claim unclaimed arrival: %w", err)
	}
	return ClaimUnclaimedArrivalResult{outcome: ClaimUnclaimedClaimed, claim: claim, hasClaim: true}, nil
}

// RevokeClaim 撤销一件待认领到货的当前认领（UC-WF-005、AT-WF-060、AT-WF-066）。
//  1. 无仓：NOT_CONFIGURED；
//  2. 锁决定标识并按决定标识认重放：同内容回显当初那一笔（不看到货此刻状态），异内容答 SOURCE_CONFLICT；
//  3. 锁到货：查无答 ARRIVAL_NOT_FOUND；不在 CLAIMED 答 STATUS_MISMATCH 并回显状态；
//  4. 已有确认收货引用答 REVOCATION_NOT_ALLOWED，库存不动；
//  5. 记下撤销并把到货退回 UNCLAIMED，原认领照留。
func (handler *ManageUnclaimedArrivalHandler) RevokeClaim(
	ctx context.Context,
	command RevokeUnclaimedClaimCommand,
) (RevokeUnclaimedClaimResult, error) {
	if command.Tenant.String() == "" || command.DecisionID.String() == "" || command.ArrivalID.String() == "" || command.Operator.String() == "" {
		return RevokeUnclaimedClaimResult{}, errors.New("revoke unclaimed claim: tenant, decisionId, arrivalId and operator are required")
	}
	configured, err := handler.warehouses.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return RevokeUnclaimedClaimResult{}, fmt.Errorf("revoke unclaimed claim: %w", err)
	}
	if !configured {
		return RevokeUnclaimedClaimResult{outcome: RevokeClaimNotConfigured, missing: "WAREHOUSE"}, nil
	}

	if err := handler.arrivals.HoldRevokeDecision(ctx, command.Tenant, command.DecisionID); err != nil {
		return RevokeUnclaimedClaimResult{}, fmt.Errorf("revoke unclaimed claim: %w", err)
	}
	prior, found, err := handler.arrivals.LoadRevocationByDecision(ctx, command.Tenant, command.DecisionID)
	if err != nil {
		return RevokeUnclaimedClaimResult{}, fmt.Errorf("revoke unclaimed claim: %w", err)
	}
	if found {
		if prior.SameContentAs(command.ArrivalID, command.Reason) {
			return RevokeUnclaimedClaimResult{
				outcome: RevokeClaimExistingResult, revocation: prior, hasRevocation: true,
			}, nil
		}
		return RevokeUnclaimedClaimResult{outcome: RevokeClaimSourceConflict}, nil
	}

	if err := handler.arrivals.HoldArrival(ctx, command.Tenant, command.ArrivalID); err != nil {
		return RevokeUnclaimedClaimResult{}, fmt.Errorf("revoke unclaimed claim: %w", err)
	}
	detail, exists, err := handler.arrivals.LoadDetail(ctx, command.Tenant, command.ArrivalID)
	if err != nil {
		return RevokeUnclaimedClaimResult{}, fmt.Errorf("revoke unclaimed claim: %w", err)
	}
	if !exists {
		return RevokeUnclaimedClaimResult{outcome: RevokeClaimArrivalNotFound}, nil
	}
	if detail.Arrival.Status() != domain.UnclaimedArrivalStatusClaimed {
		return RevokeUnclaimedClaimResult{
			outcome: RevokeClaimStatusMismatch, status: detail.Arrival.Status(), hasStatus: true,
		}, nil
	}
	active, ok := detail.ActiveClaim()
	if !ok {
		return RevokeUnclaimedClaimResult{}, errors.New("revoke unclaimed claim: 到货为已认领却读不到生效认领")
	}
	referenced, err := handler.arrivals.HasConfirmedReceiptReference(ctx, command.Tenant, command.ArrivalID)
	if err != nil {
		return RevokeUnclaimedClaimResult{}, fmt.Errorf("revoke unclaimed claim: %w", err)
	}
	if referenced {
		return RevokeUnclaimedClaimResult{outcome: RevokeClaimNotAllowed}, nil
	}

	revocation, err := domain.NewUnclaimedArrivalClaimRevocation(
		command.Tenant, command.DecisionID, command.ArrivalID, active.DecisionID(), command.Reason, command.Operator, handler.clock.Now(),
	)
	if err != nil {
		return RevokeUnclaimedClaimResult{}, fmt.Errorf("revoke unclaimed claim: %w", err)
	}
	if err := handler.arrivals.RecordRevocation(ctx, revocation); err != nil {
		return RevokeUnclaimedClaimResult{}, fmt.Errorf("revoke unclaimed claim: %w", err)
	}
	return RevokeUnclaimedClaimResult{
		outcome: RevokeClaimRevoked, revocation: revocation, hasRevocation: true,
	}, nil
}

// Dispose 按该仓当前有效的处置规则处置一件待认领到货（UC-WF-005、AT-WF-063、AT-WF-064、AT-WF-065、AT-WF-066）。
//  1. 无仓：NOT_CONFIGURED（missing=WAREHOUSE）；
//  2. 锁到货：查无答 ARRIVAL_NOT_FOUND；
//  3. 已处置：同方式答 EXISTING_RESULT 回显在册处置；换方式答 STATUS_MISMATCH 并回显状态与在册处置；
//  4. 已认领：答 STATUS_MISMATCH 并回显 CLAIMED；
//  5. 待认领：读该仓最新处置规则——没登记答 NOT_CONFIGURED（missing=DISPOSAL_RULE），不借用别的仓；
//     按到货发生时刻经过的时长核认领期限（不依赖仓业务时区），期限未到答 DISPOSITION_NOT_ALLOWED/CLAIM_WINDOW_NOT_ELAPSED；
//     逐字比对方式是否在规则内，不在答 DISPOSITION_NOT_ALLOWED/METHOD_NOT_IN_RULE；
//  6. 记下处置与所据规则修订、推进到货为 DISPOSED，不改库存账。
func (handler *ManageUnclaimedArrivalHandler) Dispose(
	ctx context.Context,
	command DisposeUnclaimedArrivalCommand,
) (DisposeUnclaimedArrivalResult, error) {
	if command.Tenant.String() == "" || command.ArrivalID.String() == "" || command.Method.String() == "" || command.Operator.String() == "" {
		return DisposeUnclaimedArrivalResult{}, errors.New("dispose unclaimed arrival: tenant, arrivalId, method and operator are required")
	}
	configured, err := handler.warehouses.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return DisposeUnclaimedArrivalResult{}, fmt.Errorf("dispose unclaimed arrival: %w", err)
	}
	if !configured {
		return DisposeUnclaimedArrivalResult{outcome: DisposeUnclaimedNotConfigured, missing: "WAREHOUSE"}, nil
	}

	if err := handler.arrivals.HoldArrival(ctx, command.Tenant, command.ArrivalID); err != nil {
		return DisposeUnclaimedArrivalResult{}, fmt.Errorf("dispose unclaimed arrival: %w", err)
	}
	detail, exists, err := handler.arrivals.LoadDetail(ctx, command.Tenant, command.ArrivalID)
	if err != nil {
		return DisposeUnclaimedArrivalResult{}, fmt.Errorf("dispose unclaimed arrival: %w", err)
	}
	if !exists {
		return DisposeUnclaimedArrivalResult{outcome: DisposeUnclaimedArrivalNotFound}, nil
	}

	switch detail.Arrival.Status() {
	case domain.UnclaimedArrivalStatusDisposed:
		if detail.Disposition == nil {
			return DisposeUnclaimedArrivalResult{}, errors.New("dispose unclaimed arrival: 到货为已处置却读不到处置行")
		}
		if detail.Disposition.Method() == command.Method {
			return DisposeUnclaimedArrivalResult{
				outcome: DisposeUnclaimedExistingResult, disposition: *detail.Disposition, hasDisposition: true,
			}, nil
		}
		return DisposeUnclaimedArrivalResult{
			outcome:        DisposeUnclaimedStatusMismatch,
			status:         detail.Arrival.Status(),
			hasStatus:      true,
			disposition:    *detail.Disposition,
			hasDisposition: true,
		}, nil
	case domain.UnclaimedArrivalStatusClaimed:
		return DisposeUnclaimedArrivalResult{
			outcome: DisposeUnclaimedStatusMismatch, status: detail.Arrival.Status(), hasStatus: true,
		}, nil
	case domain.UnclaimedArrivalStatusUnclaimed:
	default:
		return DisposeUnclaimedArrivalResult{}, fmt.Errorf("dispose unclaimed arrival: unexpected status %s", detail.Arrival.Status())
	}

	rule, hasRule, err := handler.rules.LoadLatestDisposalRule(ctx, command.Tenant, detail.Arrival.Warehouse())
	if err != nil {
		return DisposeUnclaimedArrivalResult{}, fmt.Errorf("dispose unclaimed arrival: %w", err)
	}
	if !hasRule {
		return DisposeUnclaimedArrivalResult{outcome: DisposeUnclaimedNotConfigured, missing: "DISPOSAL_RULE"}, nil
	}

	now := handler.clock.Now()
	// 认领期限自到货发生时刻起按经过的秒数算，不按业务日，不依赖仓业务时区（AT-WF-065）。
	if !rule.Content().ClaimWindowElapsed(detail.Arrival.OccurredAt(), now) {
		return DisposeUnclaimedArrivalResult{
			outcome: DisposeUnclaimedNotAllowed,
			reason:  DispositionClaimWindowNotElapsed,
		}, nil
	}
	if !rule.Content().HasMethod(command.Method) {
		return DisposeUnclaimedArrivalResult{
			outcome: DisposeUnclaimedNotAllowed,
			reason:  DispositionMethodNotInRule,
		}, nil
	}

	disposition, err := domain.NewUnclaimedArrivalDisposition(
		command.Tenant, command.ArrivalID, command.Method, rule.Warehouse(), rule.Revision(), command.Note, command.Operator, now,
	)
	if err != nil {
		return DisposeUnclaimedArrivalResult{}, fmt.Errorf("dispose unclaimed arrival: %w", err)
	}
	if err := handler.arrivals.RecordDisposition(ctx, disposition); err != nil {
		return DisposeUnclaimedArrivalResult{}, fmt.Errorf("dispose unclaimed arrival: %w", err)
	}
	return DisposeUnclaimedArrivalResult{
		outcome: DisposeUnclaimedDisposed, disposition: disposition, hasDisposition: true,
	}, nil
}

// QueryUnclaimedArrivalsHandler 是待认领到货的运营查阅用例。客户面不调用它。
type QueryUnclaimedArrivalsHandler struct {
	arrivals ports.UnclaimedArrivalCatalogueRead
}

func NewQueryUnclaimedArrivalsHandler(arrivals ports.UnclaimedArrivalCatalogueRead) *QueryUnclaimedArrivalsHandler {
	return &QueryUnclaimedArrivalsHandler{arrivals: arrivals}
}

func (handler *QueryUnclaimedArrivalsHandler) ListUnclaimedArrivals(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
	receivedFrom, receivedTo time.Time,
) (cataloguepage.Result[domain.UnclaimedArrivalDetail], error) {
	page, err := handler.arrivals.ListUnclaimedArrivals(ctx, tenant, limit, query, receivedFrom, receivedTo)
	if err != nil {
		return cataloguepage.Result[domain.UnclaimedArrivalDetail]{}, fmt.Errorf("list unclaimed arrivals: %w", err)
	}
	return page, nil
}

func (handler *QueryUnclaimedArrivalsHandler) GetUnclaimedArrival(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.UnclaimedArrivalID,
) (domain.UnclaimedArrivalDetail, bool, error) {
	detail, found, err := handler.arrivals.LoadDetail(ctx, tenant, id)
	if err != nil {
		return domain.UnclaimedArrivalDetail{}, false, fmt.Errorf("get unclaimed arrival: %w", err)
	}
	return detail, found, nil
}
