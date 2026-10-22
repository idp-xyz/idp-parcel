package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrUnexpectedCancellationSave 说明取消库交回了封闭集合以外的写入结果。
var ErrUnexpectedCancellationSave = errors.New("parcel shipment: unexpected cancellation save outcome")

// CancelParcelOutcome 是取消请求的应用处理结果，对应 UC-PS-006 元数据的四种结束加
// 重复/冲突/不受理三格，以及决定矩阵「已有终局服务结果 → 返回既有终局」那一格。
type CancelParcelOutcome uint8

const (
	CancelParcelOutcomeInvalid CancelParcelOutcome = iota
	ParcelCancelled
	DispositionPending
	CancellationRefused
	CancellationUndecided
	CancellationExistingResult
	CancellationConflict
	CancellationNotAccepted
	// CancellationFinalOutcomeStands：包裹已有终局（非取消终局，或另一请求已成立的取消），交既有
	// 终局、不形成取消（AT-PS-086）。它与`已有结果`分开：那一格说的是同一请求身份的重放，这一格
	// 说的是包裹本身已经终局，与请求是谁无关。不落记录：它说的是此刻在册的终局；终局失效后重放，
	// 按失效后的边界重判。
	CancellationFinalOutcomeStands
)

func (outcome CancelParcelOutcome) String() string {
	switch outcome {
	case ParcelCancelled:
		return "PARCEL_CANCELLED"
	case DispositionPending:
		return "DISPOSITION_PENDING"
	case CancellationRefused:
		return "CANCELLATION_REFUSED"
	case CancellationUndecided:
		return "CANCELLATION_UNDECIDED"
	case CancellationExistingResult:
		return "EXISTING_RESULT"
	case CancellationConflict:
		return "REQUEST_CONFLICT"
	case CancellationNotAccepted:
		return "REQUEST_NOT_ACCEPTED"
	case CancellationFinalOutcomeStands:
		return "FINAL_OUTCOME_STANDS"
	default:
		return ""
	}
}

// CancelUndecidedReason 指名取消判断停在哪一步。
type CancelUndecidedReason uint8

const (
	CancelUndecidedReasonNone CancelUndecidedReason = iota
	CancelRequestUnavailable
	CancelAuthorityUnavailable
	CancelAuthorityUnconfigured
	CancelAdoptionStoreUnavailable
	CancelStoreUnavailable
	CancelIdentityUnavailable
	CancelFinalStoreUnavailable
	CancelCarrierPickupUnavailable
	// CancelCarrierPickupPending：面单渠道服务的首次有效收寄判过了但待确认。它不结束取消权，也不能
	// 当作没有——UC-PS-006 决定矩阵「收寄事实存在但有效性待确认 → 取消/处置未决」。
	CancelCarrierPickupPending
)

func (reason CancelUndecidedReason) String() string {
	switch reason {
	case CancelRequestUnavailable:
		return "REQUEST_UNAVAILABLE"
	case CancelAuthorityUnavailable:
		return "AUTHORITY_UNAVAILABLE"
	case CancelAuthorityUnconfigured:
		return "AUTHORITY_UNCONFIGURED"
	case CancelAdoptionStoreUnavailable:
		return "ADOPTION_STORE_UNAVAILABLE"
	case CancelStoreUnavailable:
		return "CANCELLATION_STORE_UNAVAILABLE"
	case CancelIdentityUnavailable:
		return "CANCELLATION_IDENTITY_UNAVAILABLE"
	case CancelFinalStoreUnavailable:
		return "FINAL_OUTCOME_STORE_UNAVAILABLE"
	case CancelCarrierPickupUnavailable:
		return "CARRIER_PICKUP_UNAVAILABLE"
	case CancelCarrierPickupPending:
		return "CARRIER_PICKUP_PENDING"
	default:
		return ""
	}
}

// CancellationIdentityFactory 签发取消决定标识。
type CancellationIdentityFactory interface {
	NextParcelCancellationID(ctx context.Context) (domain.ParcelCancellationID, error)
}

// CancelParcelCommand 携带一件包裹的取消请求。批量请求由调用方逐包裹分发——批量只
// 归组不拥有共同状态（UC-PS-006 步骤 1），部分成功不以整单状态覆盖成员差异。
type CancelParcelCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	Parcel            domain.DeclaredParcelID
	Requester         domain.CancellationRequesterReference
	Reason            domain.CancellationReasonReference
	RequestedAt       time.Time
}

type CancelParcelResult struct {
	outcome      CancelParcelOutcome
	record       ports.CancellationRecord
	hasRecord    bool
	basis        domain.CheckReason
	reason       CancelUndecidedReason
	continuation domain.OwnershipContinuationReference
	handoff      domain.OwnershipContinuationReference
}

func (result CancelParcelResult) Outcome() CancelParcelOutcome {
	return result.outcome
}

func (result CancelParcelResult) Record() (ports.CancellationRecord, bool) {
	return result.record, result.hasRecord
}

// Basis 在`拒绝`时携带规则依据，在`既有终局`时指名那份终局。
func (result CancelParcelResult) Basis() domain.CheckReason {
	return result.basis
}

func (result CancelParcelResult) UndecidedReason() CancelUndecidedReason {
	return result.reason
}

func (result CancelParcelResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// CancellationHandoffReference 非空说明决定已提交但意图还没交出去，重放会重发同一份。
func (result CancelParcelResult) CancellationHandoffReference() domain.OwnershipContinuationReference {
	return result.handoff
}

// CancelParcelDeps 是取消编排的依赖。提交边界上读的边界事实各一个口：当前有效终局与已成立的取消
// （Finals、Cancellations）、网络服务的责任起点（Adoptions）、面单渠道服务的首次有效收寄
// （CarrierPickups）。都不按服务形态挑着读：本上下文不在包裹上存服务形态的判别位，而任一样在册
// 都使取消不成立（PS CONTEXT「此刻已在册的收寄或终局使取消不成立」），全读是它的直译。
type CancelParcelDeps struct {
	Requests       ports.ShipmentRequestRepository
	Authority      ports.CancellationAuthorityView
	Adoptions      ports.IntakeAdoptionStore
	Finals         ports.CurrentFinalView
	Cancellations  ports.ParcelCancellationView
	CarrierPickups ports.CarrierPickupBoundaryView
	Store          ports.ParcelCancellationStore
	Identities     CancellationIdentityFactory
	Downstream     ports.ParcelCancellationHandoff
	Clock          ports.Clock
}

type CancelParcelHandler struct {
	deps CancelParcelDeps
}

func NewCancelParcelHandler(deps CancelParcelDeps) *CancelParcelHandler {
	return &CancelParcelHandler{deps: deps}
}

// Handle 把一件包裹的取消请求推进到判断：受理与委托核验（统一不可见）→ 幂等/冲突 →
// 授权（未配置即未决，不写默认授权）→ 在决定提交边界重读终局与收寄 → 已有终局交既有终局、
// 网络收寄已在册登记待处置、面单渠道服务首次有效收寄已在册不可取消（都明确不能回退取消）、
// 边界前取消成立 → 提交与发布意图。已发生事实全程不删。
func (handler *CancelParcelHandler) Handle(
	ctx context.Context,
	command CancelParcelCommand,
) (CancelParcelResult, error) {
	if strings.TrimSpace(command.Parcel.String()) == "" ||
		strings.TrimSpace(command.Requester.String()) == "" ||
		strings.TrimSpace(command.Reason.String()) == "" ||
		command.RequestedAt.IsZero() {
		return CancelParcelResult{outcome: CancellationNotAccepted}, nil
	}

	// 委托按来源身份加编号双重指名。查无、编号不符、成员出界与跨租户同答`统一不可见结果`——可区分即可
	// 枚举别人的包裹（AT-PS-090 的隔离面）。它不是`请求未受理`：那一格只说字段缺、请求立不起来（UC-PS-006
	// 结果语义），并进去，客户就分不清该补字段还是该核对自己指名的委托与包裹。
	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return handler.undecided(command, CancelRequestUnavailable), nil
	}
	if !found || request.ShipmentRequestID() != command.ShipmentRequestID ||
		!memberOfCurrentVersion(request, command.Parcel) {
		return CancelParcelResult{}, fmt.Errorf("cancel parcel: %w", ErrShipmentRequestNotVisible)
	}

	key := ports.CancellationRequestKey{
		TenantID:   command.Identity.TenantID(),
		RequestKey: command.Identity.RequestKey(),
		Parcel:     command.Parcel,
	}
	digest := cancellationContentDigest(command)
	existing, found, err := handler.deps.Store.FindByKey(ctx, key)
	if err != nil {
		return handler.undecided(command, CancelStoreUnavailable), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一请求身份内容冲突不覆盖（UC-PS-006 一致性节）。
			return CancelParcelResult{outcome: CancellationConflict}, nil
		}
		return handler.existingResult(ctx, existing), nil
	}

	judgment, configured, err := handler.deps.Authority.JudgeCancellationAuthority(
		ctx, command.Identity, command.Requester, command.Parcel)
	if err != nil {
		return handler.undecided(command, CancelAuthorityUnavailable), nil
	}
	if !configured {
		// 授权目录未配置保持未决（PAR-COM-17 实例半边）：不写默认授权。
		return handler.undecided(command, CancelAuthorityUnconfigured), nil
	}
	if !judgment.Granted {
		record := ports.CancellationRecord{
			Key:           key,
			ContentDigest: digest,
			Kind:          ports.RecordCancellationRefused,
			RefusalBasis:  judgment.Basis,
			DecidedAt:     handler.deps.Clock.Now(),
		}
		return handler.commit(ctx, record)
	}

	// 决定提交边界上已在册的边界事实使取消不成立，不比时间（ADR-0254 决定二第 1 格）。终局先读：
	// 已终局的包裹无论哪种服务形态都交既有终局（AT-PS-086），不落到待处置或拒绝那几格去。
	if result, settled := handler.standingFinal(ctx, command, key.TenantID); settled {
		return result, nil
	}

	// 网络服务的收寄重读（AT-PS-079）：先合法成立的收寄赢，取消不形成、转待处置。
	started, crossed, err := handler.deps.Adoptions.FindResponsibilityStart(
		ctx, key.TenantID, command.Parcel)
	if err != nil {
		return handler.undecided(command, CancelAdoptionStoreUnavailable), nil
	}
	if !crossed {
		// 面单渠道服务的边界是实际承运商首次有效收寄（PS CONTEXT）。已形成即不可取消，拒绝带上越过的
		// 那一代——面单渠道服务没有收寄后处置这条路，承运商收寄就是它的终局边界。
		boundary, err := handler.deps.CarrierPickups.FindCurrentCarrierPickup(ctx, key.TenantID, command.Parcel)
		if err != nil {
			return handler.undecided(command, CancelCarrierPickupUnavailable), nil
		}
		switch boundary.State {
		case ports.CarrierPickupBoundaryAbsent:
		case ports.CarrierPickupBoundaryPending:
			return handler.undecided(command, CancelCarrierPickupPending), nil
		case ports.CarrierPickupBoundaryFormed:
			basis, err := domain.NewCheckReason("CARRIER_FIRST_EFFECTIVE_PICKUP/" +
				boundary.Pickup.Fact.String() + "@" + boundary.Pickup.Version.String())
			if err != nil {
				return CancelParcelResult{}, fmt.Errorf("carrier pickup boundary basis: %w", err)
			}
			return handler.commit(ctx, ports.CancellationRecord{
				Key:           key,
				ContentDigest: digest,
				Kind:          ports.RecordCancellationRefused,
				RefusalBasis:  basis,
				DecidedAt:     handler.deps.Clock.Now(),
			})
		default:
			return CancelParcelResult{}, fmt.Errorf("parcel shipment: unhandled carrier pickup boundary state %d", boundary.State)
		}
	}

	intake := domain.CurrentIntakeFact{}
	if crossed {
		intake = domain.CurrentIntakeFact{
			Present:    true,
			OccurredAt: started.Intake.Source().OccurredAt(),
		}
	}
	cancellationID, err := handler.deps.Identities.NextParcelCancellationID(ctx)
	if err != nil {
		return handler.undecided(command, CancelIdentityUnavailable), nil
	}
	// 授权答复两向都必须带规则依据——允许时它就是取消决定引用的授权；答了允许却说
	// 不出依据的答复不完整，按依赖不可用未决。
	authority, err := domain.NewCancellationAuthorityReference(judgment.Basis.String())
	if err != nil {
		return handler.undecided(command, CancelAuthorityUnavailable), nil
	}
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          cancellationID,
		Parcel:      command.Parcel,
		Requester:   command.Requester,
		Authority:   authority,
		Reason:      command.Reason,
		RequestedAt: command.RequestedAt,
	}, intake)
	if errors.Is(err, domain.ErrIntakeBoundaryCrossed) {
		// 已收寄后只发「取消」消息：明确不能回退取消；没有授权处置决定时保持待处置
		// （AT-PS-082）。越过的收寄版本随记录保全，处置判断由后续独立请求形成。
		record := ports.CancellationRecord{
			Key:           key,
			ContentDigest: digest,
			Kind:          ports.RecordDispositionPending,
			IntakeVersion: started.Key.Version,
			DecidedAt:     handler.deps.Clock.Now(),
		}
		return handler.commit(ctx, record)
	}
	if err != nil {
		return CancelParcelResult{}, fmt.Errorf("decide parcel cancellation: %w", err)
	}

	record := ports.CancellationRecord{
		Key:           key,
		ContentDigest: digest,
		Kind:          ports.RecordParcelCancelled,
		Cancellation:  cancellation,
		DecidedAt:     handler.deps.Clock.Now(),
	}
	return handler.commit(ctx, record)
}

// standingFinal 读包裹当前有效终局：非取消终局与已成立的取消都是终局。settled 为 true 时 result
// 就是答复——既有终局，或读不动的未决（读不动不放行取消）。
func (handler *CancelParcelHandler) standingFinal(
	ctx context.Context,
	command CancelParcelCommand,
	tenant domain.TenantID,
) (CancelParcelResult, bool) {
	final, found, err := handler.deps.Finals.FindCurrentFinal(ctx, tenant, command.Parcel)
	if err != nil {
		return handler.undecided(command, CancelFinalStoreUnavailable), true
	}
	if found && final.Finalized {
		return handler.finalStands("FINAL_OUTCOME/", final.Final.Version().String(), command)
	}
	cancellation, cancelled, err := handler.deps.Cancellations.FindCancellation(ctx, tenant, command.Parcel)
	if err != nil {
		return handler.undecided(command, CancelStoreUnavailable), true
	}
	if cancelled {
		return handler.finalStands("PARCEL_CANCELLATION/", cancellation.ID().String(), command)
	}
	return CancelParcelResult{}, false
}

func (handler *CancelParcelHandler) finalStands(
	kind string,
	identifier string,
	command CancelParcelCommand,
) (CancelParcelResult, bool) {
	if strings.TrimSpace(identifier) == "" {
		// 指名不出那份终局：读口交回的终局缺标识，是读口坏了；不放行取消。
		return handler.undecided(command, CancelFinalStoreUnavailable), true
	}
	basis, err := domain.NewCheckReason(kind + identifier)
	if err != nil {
		return handler.undecided(command, CancelFinalStoreUnavailable), true
	}
	return CancelParcelResult{outcome: CancellationFinalOutcomeStands, basis: basis}, true
}

func memberOfCurrentVersion(request domain.ShipmentRequest, parcel domain.DeclaredParcelID) bool {
	for _, member := range request.CurrentSubmissionVersion().DeclaredParcelIDs() {
		if member == parcel {
			return true
		}
	}
	return false
}

// commit 提交取消判断并交发布意图；并发下另一方先提交时读回赢家。
func (handler *CancelParcelHandler) commit(
	ctx context.Context,
	record ports.CancellationRecord,
) (CancelParcelResult, error) {
	saved, err := handler.deps.Store.Save(ctx, record)
	if err != nil {
		return CancelParcelResult{
			outcome:      CancellationUndecided,
			reason:       CancelStoreUnavailable,
			continuation: cancellationContinuation(record.Key, CancelStoreUnavailable),
		}, nil
	}
	switch saved {
	case ports.CancellationSaved:
		result := resultForCancellation(record)
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.CancellationAlreadyRecorded:
		winner, found, err := handler.deps.Store.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return CancelParcelResult{
				outcome:      CancellationUndecided,
				reason:       CancelStoreUnavailable,
				continuation: cancellationContinuation(record.Key, CancelStoreUnavailable),
			}, nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return CancelParcelResult{}, fmt.Errorf("%w: %d", ErrUnexpectedCancellationSave, saved)
	}
}

// existingResult 按已有记录作答并重发同一份意图（重复请求返回原逐包裹结果）。
func (handler *CancelParcelHandler) existingResult(
	ctx context.Context,
	record ports.CancellationRecord,
) CancelParcelResult {
	result := resultForCancellation(record)
	result.outcome = CancellationExistingResult
	result.handoff = handler.handOff(ctx, record)
	return result
}

func resultForCancellation(record ports.CancellationRecord) CancelParcelResult {
	switch record.Kind {
	case ports.RecordParcelCancelled:
		return CancelParcelResult{outcome: ParcelCancelled, record: record, hasRecord: true}
	case ports.RecordDispositionPending:
		return CancelParcelResult{outcome: DispositionPending, record: record, hasRecord: true}
	case ports.RecordCancellationRefused:
		return CancelParcelResult{
			outcome:   CancellationRefused,
			record:    record,
			hasRecord: true,
			basis:     record.RefusalBasis,
		}
	default:
		return CancelParcelResult{}
	}
}

// handOff 交发布意图。只有取消成立才有下游要释放的东西——待处置与拒绝没有：处置
// 请求由后续独立判断发出，不在取消这份意图里。失败不翻决定，留续办引用重发同一份。
func (handler *CancelParcelHandler) handOff(
	ctx context.Context,
	record ports.CancellationRecord,
) domain.OwnershipContinuationReference {
	if record.Kind != ports.RecordParcelCancelled {
		return domain.OwnershipContinuationReference{}
	}
	if err := handler.deps.Downstream.HandOffParcelCancellation(
		ctx, ports.ParcelCancellationHandoffIntent{Record: record}); err == nil {
		return domain.OwnershipContinuationReference{}
	}
	return derivedContinuation(
		"PARCEL_CANCELLATION_HANDOFF",
		record.Key.TenantID.String(),
		record.Key.RequestKey.String(),
		record.Key.Parcel.String(),
	)
}

func (handler *CancelParcelHandler) undecided(
	command CancelParcelCommand,
	reason CancelUndecidedReason,
) CancelParcelResult {
	key := ports.CancellationRequestKey{
		TenantID:   command.Identity.TenantID(),
		RequestKey: command.Identity.RequestKey(),
		Parcel:     command.Parcel,
	}
	return CancelParcelResult{
		outcome:      CancellationUndecided,
		reason:       reason,
		continuation: cancellationContinuation(key, reason),
	}
}

func cancellationContinuation(
	key ports.CancellationRequestKey,
	reason CancelUndecidedReason,
) domain.OwnershipContinuationReference {
	return derivedContinuation(
		reason.String(),
		key.TenantID.String(),
		key.RequestKey.String(),
		key.Parcel.String(),
	)
}

// cancellationContentDigest 是同一请求身份的内容比对锚。
func cancellationContentDigest(command CancelParcelCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Parcel.String(),
		command.Requester.String(),
		command.Reason.String(),
		command.RequestedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
