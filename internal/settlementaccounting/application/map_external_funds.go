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

// ErrUnexpectedFundsSave 说明资金侧库交回了封闭集合以外的写入结果。
var ErrUnexpectedFundsSave = errors.New("settlement accounting: unexpected funds-side save outcome")

// FundsOutcome 是外部资金映射与核销编排的应用处理结果。四个业务负向格（ADR-0029）：
// 失败付款不可映射、跨币种无换算不核销、分配失衡（部分/超额各自表达）、核销已撤销
// ——都是已知答案。
type FundsOutcome uint8

const (
	FundsOutcomeInvalid FundsOutcome = iota
	FundsFactAdopted
	FundsFactExisting
	FundsFactConflict
	FundsMapped
	FundsMappingExisting
	FundsMappingConflict
	UnfundableFactOutcome
	SettlementApplied
	ApplicationExisting
	ApplicationConflict
	ApplicationImbalanceOutcome
	CrossCurrencyOutcome
	ApplicationReversedOutcome
	ApplicationAlreadyReversed
	FundsNotAccepted
	FundsUndecided
)

func (outcome FundsOutcome) String() string {
	switch outcome {
	case FundsFactAdopted:
		return "FUNDS_FACT_ADOPTED"
	case FundsFactExisting:
		return "EXISTING_FUNDS_FACT"
	case FundsFactConflict:
		return "FUNDS_FACT_CONFLICT"
	case FundsMapped:
		return "FUNDS_MAPPED"
	case FundsMappingExisting:
		return "EXISTING_MAPPING"
	case FundsMappingConflict:
		return "MAPPING_CONFLICT"
	case UnfundableFactOutcome:
		return "UNFUNDABLE_FACT"
	case SettlementApplied:
		return "SETTLEMENT_APPLIED"
	case ApplicationExisting:
		return "EXISTING_APPLICATION"
	case ApplicationConflict:
		return "APPLICATION_CONFLICT"
	case ApplicationImbalanceOutcome:
		return "APPLICATION_IMBALANCE"
	case CrossCurrencyOutcome:
		return "CROSS_CURRENCY"
	case ApplicationReversedOutcome:
		return "APPLICATION_REVERSED"
	case ApplicationAlreadyReversed:
		return "ALREADY_REVERSED"
	case FundsNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case FundsUndecided:
		return "FUNDS_UNDECIDED"
	default:
		return ""
	}
}

// FundsUndecidedReason 指名提交停在哪一步等谁。
type FundsUndecidedReason uint8

const (
	FundsUndecidedReasonNone FundsUndecidedReason = iota
	FundsFactStoreUnavailable
	FundsMappingStoreUnavailable
	ApplicationStoreUnavailable
)

func (reason FundsUndecidedReason) String() string {
	switch reason {
	case FundsFactStoreUnavailable:
		return "FUNDS_FACT_STORE_UNAVAILABLE"
	case FundsMappingStoreUnavailable:
		return "FUNDS_MAPPING_STORE_UNAVAILABLE"
	case ApplicationStoreUnavailable:
		return "APPLICATION_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// AdoptFundsFactCommand 携带一次外部资金事实采用：只形成引用与待匹配入口，不直接
// 成为已核销（AT-SA-101）。
type AdoptFundsFactCommand struct {
	TenantID domain.TenantID
	Fact     string
	Source   string
	// Payer 是来源提供的付款人，可缺席（空即来源未提供）——本上下文只保留不判断，
	// 理由在 domain.FundsPayerReference 头注。
	Payer       string
	Kind        domain.FundsFactKind
	Currency    string
	AmountMinor int64
	Version     string
	OccurredAt  time.Time
}

// CorrectFundsFactCommand 携带一次外部更正的采用：同一事实的新版本回指当前链头，只有金额变
// （UC-SA-001「更正必须形成新来源版本」；AT-SA-114 保留原版本、按新有效版本重算）。更正是采用
// 的一种，走同一个用例（票 sa-cc/20 裁决 2）；来源、付款人、种类、币种与业务发生时刻从链头照抄，
// 命令上不再接——外部更正改的是金额，其余若也变了那是另一条事实，不是更正。
type CorrectFundsFactCommand struct {
	TenantID domain.TenantID
	Fact     string
	// Corrects 是被更正的版本，必须等于当前链头。本上下文是铸造方：链由这里按序铸出，纠正一个不是
	// 当前的版本是调用方编程错误（`未受理`），与 CC 作为接收方容忍乱序到达是两侧各自的纪律。
	Corrects    string
	Version     string
	AmountMinor int64
	CorrectedAt time.Time
}

// MapFundsCommand 携带一次资金映射：显式依据必备——金额相同、同一客户或同一时间都
// 不单独证明映射。
type MapFundsCommand struct {
	TenantID   domain.TenantID
	Mapping    string
	Fact       string
	TargetKind domain.SettlementTargetKind
	Target     string
	Basis      string
	MappedAt   time.Time
}

// AllocationDirective 是一条核销分配指令。
type AllocationDirective struct {
	Mapping     string
	TargetKind  domain.SettlementTargetKind
	Target      string
	Direction   domain.AllocationDirection
	AmountMinor int64
}

// ApplySettlementCommand 携带一次核销：核销是显式判断不是自动推导，分配必须有映射
// 背书。
type ApplySettlementCommand struct {
	TenantID       domain.TenantID
	Application    string
	Fact           string
	TargetCurrency string
	MappingRefs    []string
	Allocations    []AllocationDirective
	Basis          string
	AppliedAt      time.Time
}

// ReverseApplicationCommand 携带一次核销撤销：原分配不删，只登记核销关系失效。
type ReverseApplicationCommand struct {
	TenantID    domain.TenantID
	Application string
	Basis       string
	ReversedAt  time.Time
}

type FundsResult struct {
	outcome      FundsOutcome
	reason       FundsUndecidedReason
	fact         ports.FundsFactRecord
	mapping      ports.FundsMappingRecord
	application  ports.SettlementApplicationRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result FundsResult) Outcome() FundsOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result FundsResult) UndecidedReason() FundsUndecidedReason {
	return result.reason
}

func (result FundsResult) Fact() (ports.FundsFactRecord, bool) {
	return result.fact, result.hasRecord && result.fact.Key.Fact.String() != ""
}

func (result FundsResult) Mapping() (ports.FundsMappingRecord, bool) {
	return result.mapping, result.hasRecord && result.mapping.Key.Mapping.String() != ""
}

func (result FundsResult) Application() (ports.SettlementApplicationRecord, bool) {
	return result.application, result.hasRecord && result.application.Key.Application.String() != ""
}

func (result FundsResult) ContinuationReference() string {
	return result.continuation
}

// FundsHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result FundsResult) FundsHandoffReference() string {
	return result.handoff
}

type MapExternalFundsDeps struct {
	Facts        ports.ExternalFundsFactStore
	Mappings     ports.FundsMappingStore
	Applications ports.SettlementApplicationStore
	Downstream   ports.SettlementApplicationHandoff
	// FactHandoff 把采用成功的资金事实交出去（票 sa-cc/02）——CC 的税费付款核对等的正是这封。
	FactHandoff ports.ExternalFundsFactHandoff
	Clock       ports.Clock
}

type MapExternalFundsHandler struct {
	deps MapExternalFundsDeps
}

// NewMapExternalFundsHandler 构造期逐口拒 nil、缺件包 ErrNilDependency（同包 NewRequestBuyEvaluationHandler 那张表的形；
// 票 sa-cc/27 裁决 4）。这只 handler 自那票起第一次被装进生产，装进去的那一刻就该拒：FactHandoff 漏装时 panic 落在
// Facts.Save 已落行之后——行已落、信封没出、结果没返回——比一次普通的空指针更坏；Clock 漏装 panic 在 Save 之前。
// Mappings / Applications / Downstream 不在采用 / 更正路径上也一并拒：各口在同一个 Deps 上，半装的 handler 会让走不到的
// 方法在被调那天才崩，那不是一条守得住的边界（「生产装配里不放任何替身」的同一条纪律）。
func NewMapExternalFundsHandler(deps MapExternalFundsDeps) (*MapExternalFundsHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"external funds fact store", deps.Facts == nil},
		{"funds mapping store", deps.Mappings == nil},
		{"settlement application store", deps.Applications == nil},
		{"settlement application downstream", deps.Downstream == nil},
		{"external funds fact handoff", deps.FactHandoff == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &MapExternalFundsHandler{deps: deps}, nil
}

// AdoptFact 采用一条外部资金事实的首版：只读引用（无余额/已结清字段，领域已钉），幂等按
// （租户+事实引用+版本）分重放/冲突——先按命令的版本字面查，链头被更正版本占着时重放首版仍答
// `已采用`（0021 起一事实多版，票 sa-cc/20）；同一事实换一个首版字面则是`冲突`。
func (handler *MapExternalFundsHandler) AdoptFact(
	ctx context.Context,
	command AdoptFundsFactCommand,
) (FundsResult, error) {
	fact, err := factFrom(command)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	key := ports.FundsFactKey{TenantID: command.TenantID, Fact: fact.Fact()}
	digest := adoptDigest(command)
	existing, found, err := handler.deps.Facts.FindVersion(ctx, key, fact.Version())
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一版本字面携带不同金额或时间：冲突保留原引用——外部更正走版本链，
			// 不在采用处顶替。冲突的那一份没有被采用，无物可交。
			return FundsResult{outcome: FundsFactConflict}, nil
		}
		return handler.existingFact(ctx, existing), nil
	}
	if _, adopted, err := handler.deps.Facts.FindByKey(ctx, key); err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	} else if adopted {
		// 事实已有版本链、而这个版本字面不在链上：一条事实只有一个首版，第二个首版是冲突不是采用；
		// 更正走 CorrectFact 回指链头。
		return FundsResult{outcome: FundsFactConflict}, nil
	}

	record := ports.FundsFactRecord{Key: key, ContentDigest: digest, Fact: fact, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	switch saved {
	case ports.FundsFactSaved:
		result := FundsResult{outcome: FundsFactAdopted, fact: record, hasRecord: true}
		result.handoff = handler.handOffFact(ctx, record)
		return result, nil
	case ports.FundsFactAlreadyAdopted:
		winner, found, err := handler.deps.Facts.FindByKey(ctx, key)
		if err != nil || !found {
			return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
		}
		return handler.existingFact(ctx, winner), nil
	default:
		return FundsResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFundsSave, saved)
	}
}

// CorrectFact 采用一条外部更正：从当前链头经 domain.CorrectAmount 形成回指它的新版本、落版本行、
// 复用同一交接口再发一封（信封 ID 带新版本、载荷回指前版——票 sa-cc/02 裁决 2 预告的那一格）。
// 幂等按（租户+事实+新版本）分重放 / 冲突，照 AdoptFact 四格；回指非链头与更正未采用的事实都是`未受理`。
//
// 先按新版本查、再查链头，顺序不能反：重放同一次更正时链头已经是新版本本身，先查链头会把一次正当
// 的重放判成「回指非链头」。
func (handler *MapExternalFundsHandler) CorrectFact(
	ctx context.Context,
	command CorrectFundsFactCommand,
) (FundsResult, error) {
	factRef, err := domain.NewFundsFactReference(command.Fact)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	corrects, err := domain.NewFundsFactVersion(command.Corrects)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	version, err := domain.NewFundsFactVersion(command.Version)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	// 命令自己的形先判、不碰库：回指自己不是版本链，非正金额与缺席的更正时刻 CorrectAmount 也会拒，
	// 提前到这里是让「同版本重放」的查询不必为一条坏命令跑一趟。
	if version == corrects || command.AmountMinor <= 0 || command.CorrectedAt.IsZero() {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	key := ports.FundsFactKey{TenantID: command.TenantID, Fact: factRef}
	digest := correctDigest(command)
	existing, found, err := handler.deps.Facts.FindVersion(ctx, key, version)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一新版本字面携带不同金额或回指：冲突保留先到的那一版，不顶替。
			return FundsResult{outcome: FundsFactConflict}, nil
		}
		return handler.existingFact(ctx, existing), nil
	}

	head, found, err := handler.deps.Facts.FindByKey(ctx, key)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	if !found || head.Fact.Version() != corrects {
		// 更正一个未采用的事实，或回指的不是当前链头：提交矛盾，不是库的事。
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	corrected, err := head.Fact.CorrectAmount(command.AmountMinor, version, command.CorrectedAt)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	record := ports.FundsFactRecord{Key: key, ContentDigest: digest, Fact: corrected, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	switch saved {
	case ports.FundsFactSaved:
		result := FundsResult{outcome: FundsFactAdopted, fact: record, hasRecord: true}
		result.handoff = handler.handOffFact(ctx, record)
		return result, nil
	case ports.FundsFactAlreadyAdopted:
		// 两步之间另一位写入方赢了：要么同一新版本先落了，要么链头先被别的版本更正了（库上守链形的
		// 唯一约束把后者也折成`已采用`）。两种都按当前链头作答——它就是此刻被采用的那一版。
		// 同一个「回指的不是当前链头」顺序到达时在上面答的是`未受理`，两答有意不同，理由见
		// ExternalFundsFacts.Save 头注。
		winner, found, err := handler.deps.Facts.FindByKey(ctx, key)
		if err != nil || !found {
			return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
		}
		return handler.existingFact(ctx, winner), nil
	default:
		return FundsResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFundsSave, saved)
	}
}

// existingFact 按已采用的事实作答并再交一次同一份意图：重放交的是同一封（同租户、同事实、
// 同版本），Outbox 按认领键吞掉第二次——「重放不交」在真库上就是这样成立的；不在这里跳过
// 交接，是为了让上一次交接失败留下的那封在重放时补上（与 existingApplication 同形）。
func (handler *MapExternalFundsHandler) existingFact(
	ctx context.Context,
	record ports.FundsFactRecord,
) FundsResult {
	return FundsResult{
		outcome:   FundsFactExisting,
		fact:      record,
		hasRecord: true,
		handoff:   handler.handOffFact(ctx, record),
	}
}

// handOffFact 把采用成功的资金事实交给下游（票 sa-cc/02）。投递失败不翻结果——事实已采用是
// 真的，只是那封信还没出去——留续办引用，重放时重发同一份。
func (handler *MapExternalFundsHandler) handOffFact(
	ctx context.Context,
	record ports.FundsFactRecord,
) string {
	if err := handler.deps.FactHandoff.HandOffExternalFundsFact(ctx, ports.ExternalFundsFactIntent{Record: record}); err == nil {
		return ""
	}
	return fundsContinuation("EXTERNAL_FUNDS_FACT_HANDOFF",
		record.Key.TenantID.String(), record.Key.Fact.String(), record.Fact.Version().String())
}

// Map 建立一条资金映射：显式依据由 MapFundsToTarget 把门（巧合不证明映射）；失败
// 付款 → UNFUNDABLE_FACT 业务负向（ErrUnfundableFact 哨兵分格）。
func (handler *MapExternalFundsHandler) Map(
	ctx context.Context,
	command MapFundsCommand,
) (FundsResult, error) {
	mappingRef, err := domain.NewMappingReference(command.Mapping)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	factRef, err := domain.NewFundsFactReference(command.Fact)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	factRecord, found, err := handler.deps.Facts.FindByKey(ctx,
		ports.FundsFactKey{TenantID: command.TenantID, Fact: factRef})
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	if !found {
		// 映射一个未采用的事实：提交矛盾，先采用再映射。
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	mapping, err := mappingFrom(command, factRecord.Fact, mappingRef)
	if errors.Is(err, domain.ErrUnfundableFact) {
		// 付款失败的事实没有可分配的资金——映射无从谈起。
		return FundsResult{outcome: UnfundableFactOutcome}, nil
	}
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	key := ports.FundsMappingKey{TenantID: command.TenantID, Mapping: mappingRef}
	digest := mapDigest(command)
	existing, alreadyMapped, err := handler.deps.Mappings.FindByKey(ctx, key)
	if err != nil {
		return fundsUndecided(FundsMappingStoreUnavailable, command.Mapping), nil
	}
	if alreadyMapped {
		if existing.ContentDigest != digest {
			return FundsResult{outcome: FundsMappingConflict}, nil
		}
		return FundsResult{outcome: FundsMappingExisting, mapping: existing, hasRecord: true}, nil
	}

	record := ports.FundsMappingRecord{Key: key, ContentDigest: digest, Mapping: mapping, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Mappings.Save(ctx, record)
	if err != nil {
		return fundsUndecided(FundsMappingStoreUnavailable, command.Mapping), nil
	}
	switch saved {
	case ports.FundsMappingSaved:
		return FundsResult{outcome: FundsMapped, mapping: record, hasRecord: true}, nil
	case ports.FundsMappingAlreadyRecorded:
		winner, found, err := handler.deps.Mappings.FindByKey(ctx, key)
		if err != nil || !found {
			return fundsUndecided(FundsMappingStoreUnavailable, command.Mapping), nil
		}
		return FundsResult{outcome: FundsMappingExisting, mapping: winner, hasRecord: true}, nil
	default:
		return FundsResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFundsSave, saved)
	}
}

// Apply 落一次核销：核销是显式判断（依据必备）；分配必须有映射背书（凭巧合分钱在
// 领域被拒）；跨币种无换算 → CROSS_CURRENCY；净额越界 → APPLICATION_IMBALANCE；
// 意图交下游已结视图。
func (handler *MapExternalFundsHandler) Apply(
	ctx context.Context,
	command ApplySettlementCommand,
) (FundsResult, error) {
	applicationRef, err := domain.NewApplicationReference(command.Application)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	factRef, err := domain.NewFundsFactReference(command.Fact)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	targetCurrency, err := domain.NewCurrencyCode(command.TargetCurrency)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	basis, err := domain.NewApplicationBasisReference(command.Basis)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	key := ports.SettlementApplicationKey{TenantID: command.TenantID, Application: applicationRef}
	digest := applyDigest(command)
	existingApplication, alreadyApplied, err := handler.deps.Applications.FindByKey(ctx, key)
	if err != nil {
		return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
	}
	if alreadyApplied {
		if existingApplication.ContentDigest != digest {
			return FundsResult{outcome: ApplicationConflict}, nil
		}
		return handler.existingApplication(ctx, existingApplication), nil
	}

	factRecord, found, err := handler.deps.Facts.FindByKey(ctx,
		ports.FundsFactKey{TenantID: command.TenantID, Fact: factRef})
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	if !found {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	mappings := make([]domain.FundsMapping, 0, len(command.MappingRefs))
	for _, raw := range command.MappingRefs {
		mappingRef, err := domain.NewMappingReference(raw)
		if err != nil {
			return FundsResult{outcome: FundsNotAccepted}, nil
		}
		mappingRecord, mapped, err := handler.deps.Mappings.FindByKey(ctx,
			ports.FundsMappingKey{TenantID: command.TenantID, Mapping: mappingRef})
		if err != nil {
			return fundsUndecided(FundsMappingStoreUnavailable, raw), nil
		}
		if !mapped {
			return FundsResult{outcome: FundsNotAccepted}, nil
		}
		mappings = append(mappings, mappingRecord.Mapping)
	}

	allocations, badDirective := allocationsFrom(command.Allocations)
	if badDirective {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	application, err := domain.ApplySettlementInCurrency(
		factRecord.Fact, targetCurrency, mappings, allocations, applicationRef, basis, command.AppliedAt)
	switch {
	case errors.Is(err, domain.ErrCrossCurrencyApplication):
		// 跨币种没有认可换算依据：核销无从谈起（同 CC 税费核对纪律）。
		return FundsResult{outcome: CrossCurrencyOutcome}, nil
	case errors.Is(err, domain.ErrApplicationImbalance):
		// 净额为零或超过事实金额：部分到账/超额分别表达，不静默截断。
		return FundsResult{outcome: ApplicationImbalanceOutcome}, nil
	case errors.Is(err, domain.ErrUnfundableFact):
		return FundsResult{outcome: UnfundableFactOutcome}, nil
	case err != nil:
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	record := ports.SettlementApplicationRecord{Key: key, ContentDigest: digest, Application: application, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Applications.Save(ctx, record)
	if err != nil {
		return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
	}
	switch saved {
	case ports.SettlementApplicationSaved:
		result := FundsResult{outcome: SettlementApplied, application: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.SettlementApplicationAlreadyApplied:
		winner, found, err := handler.deps.Applications.FindByKey(ctx, key)
		if err != nil || !found {
			return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
		}
		return handler.existingApplication(ctx, winner), nil
	default:
		return FundsResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFundsSave, saved)
	}
}

// Reverse 撤销一次核销：原分配不删、撤销一次为限（ErrApplicationReversed 哨兵分格），
// 撤销版随意图重新交下游。
func (handler *MapExternalFundsHandler) Reverse(
	ctx context.Context,
	command ReverseApplicationCommand,
) (FundsResult, error) {
	applicationRef, err := domain.NewApplicationReference(command.Application)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	basis, err := domain.NewApplicationBasisReference(command.Basis)
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	key := ports.SettlementApplicationKey{TenantID: command.TenantID, Application: applicationRef}
	existing, found, err := handler.deps.Applications.FindByKey(ctx, key)
	if err != nil {
		return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
	}
	if !found {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	reversed, err := existing.Application.Reverse(basis, command.ReversedAt)
	if errors.Is(err, domain.ErrApplicationReversed) {
		// 已撤销重放：返回原撤销，不二撤。
		return FundsResult{outcome: ApplicationAlreadyReversed, application: existing, hasRecord: true}, nil
	}
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	existing.Application = reversed
	existing.RecordedAt = handler.deps.Clock.Now()
	ok, err := handler.deps.Applications.Replace(ctx, existing)
	if err != nil {
		return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
	}
	if !ok {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	result := FundsResult{outcome: ApplicationReversedOutcome, application: existing, hasRecord: true}
	result.handoff = handler.handOff(ctx, existing)
	return result, nil
}

func factFrom(command AdoptFundsFactCommand) (domain.ExternalFundsFact, error) {
	spec := domain.ExternalFundsFactSpec{
		Kind:        command.Kind,
		AmountMinor: command.AmountMinor,
		OccurredAt:  command.OccurredAt,
	}
	var err error
	if spec.Fact, err = domain.NewFundsFactReference(command.Fact); err != nil {
		return domain.ExternalFundsFact{}, err
	}
	if spec.Source, err = domain.NewFundsSourceRegistrationReference(command.Source); err != nil {
		return domain.ExternalFundsFact{}, err
	}
	if strings.TrimSpace(command.Payer) != "" {
		if spec.Payer, err = domain.NewFundsPayerReference(command.Payer); err != nil {
			return domain.ExternalFundsFact{}, err
		}
	}
	if spec.Currency, err = domain.NewCurrencyCode(command.Currency); err != nil {
		return domain.ExternalFundsFact{}, err
	}
	if spec.Version, err = domain.NewFundsFactVersion(command.Version); err != nil {
		return domain.ExternalFundsFact{}, err
	}
	return domain.AdoptExternalFundsFact(spec)
}

func mappingFrom(
	command MapFundsCommand,
	fact domain.ExternalFundsFact,
	mappingRef domain.MappingReference,
) (domain.FundsMapping, error) {
	target, err := domain.NewSettlementTargetReference(command.Target)
	if err != nil {
		return domain.FundsMapping{}, err
	}
	basis := domain.MappingBasisReference{}
	if strings.TrimSpace(command.Basis) != "" {
		if basis, err = domain.NewMappingBasisReference(command.Basis); err != nil {
			return domain.FundsMapping{}, err
		}
	}
	return domain.MapFundsToTarget(fact, mappingRef, command.TargetKind, target, basis, command.MappedAt)
}

func allocationsFrom(directives []AllocationDirective) ([]domain.SettlementAllocation, bool) {
	allocations := make([]domain.SettlementAllocation, 0, len(directives))
	for _, directive := range directives {
		mapping, err := domain.NewMappingReference(directive.Mapping)
		if err != nil {
			return nil, true
		}
		target, err := domain.NewSettlementTargetReference(directive.Target)
		if err != nil {
			return nil, true
		}
		allocations = append(allocations, domain.SettlementAllocation{
			Mapping:     mapping,
			TargetKind:  directive.TargetKind,
			Target:      target,
			Direction:   directive.Direction,
			AmountMinor: directive.AmountMinor,
		})
	}
	return allocations, false
}

func fundsUndecided(reason FundsUndecidedReason, subject string) FundsResult {
	return FundsResult{
		outcome:      FundsUndecided,
		reason:       reason,
		continuation: fundsContinuation(reason.String(), subject),
	}
}

// existingApplication 按已有核销作答并重发同一份意图。
func (handler *MapExternalFundsHandler) existingApplication(
	ctx context.Context,
	record ports.SettlementApplicationRecord,
) FundsResult {
	return FundsResult{
		outcome:     ApplicationExisting,
		application: record,
		hasRecord:   true,
		handoff:     handler.handOff(ctx, record),
	}
}

// handOff 把核销/撤销交给下游已结视图。投递失败不翻结果，留续办引用重放时重发同一份。
func (handler *MapExternalFundsHandler) handOff(
	ctx context.Context,
	record ports.SettlementApplicationRecord,
) string {
	if err := handler.deps.Downstream.HandOffSettlementApplication(ctx, ports.SettlementApplicationIntent{Record: record}); err == nil {
		return ""
	}
	return fundsContinuation("SETTLEMENT_APPLICATION_HANDOFF", record.Key.TenantID.String(), record.Key.Application.String())
}

func fundsContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// adoptDigest 把付款人算进内容：同引用换付款人是另一份内容（冲突），不是重放。
//
// 付款人自 migrations/settlement_accounting/0018_external_funds_fact_payer.sql 起进摘要（票 sa-cc/03）。
// 摘要元素一变，变之前落下的行重投同一内容会撞 ContentDigest 答`内容冲突`而不是`已存在`——幂等
// 不变式在版本边界上断开。0018 之前本上下文的采用没有生产入口、存量为零，故不回算旧行的摘要，
// 靠的只是这一条（与该迁移头注同一句）。日后再改摘要元素，要么回算存量，要么在这里再记一版起点。
func adoptDigest(command AdoptFundsFactCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Source,
		strings.TrimSpace(command.Payer),
		fmt.Sprintf("%d", command.Kind),
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.Version,
		command.OccurredAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// correctDigest 只算命令自己带的四样：回指、新版本、金额、更正时刻。来源 / 付款人 / 种类 / 币种 / 发生时刻
// 从链头照抄，不在命令上，也就不进摘要——同一新版本字面换一个金额或换一个回指是另一份内容（冲突），
// 不是重放。与 adoptDigest 元素不同是有意的：同一（事实、版本）若先经 AdoptFact 作首版落下、再有人拿它
// 当更正版本来提，两份摘要必不相等，答`冲突`而不是把首版当成更正的重放。
func correctDigest(command CorrectFundsFactCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Corrects,
		command.Version,
		fmt.Sprintf("%d", command.AmountMinor),
		command.CorrectedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func mapDigest(command MapFundsCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Fact,
		fmt.Sprintf("%d", command.TargetKind),
		command.Target,
		command.Basis,
		command.MappedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func applyDigest(command ApplySettlementCommand) string {
	mappings := append([]string(nil), command.MappingRefs...)
	sort.Strings(mappings)
	parts := []string{
		command.Fact,
		command.TargetCurrency,
		command.Basis,
		command.AppliedAt.UTC().Format(time.RFC3339Nano),
	}
	parts = append(parts, mappings...)
	for _, allocation := range command.Allocations {
		parts = append(parts, fmt.Sprintf("%s|%d|%s|%d|%d",
			allocation.Mapping, allocation.TargetKind, allocation.Target,
			allocation.Direction, allocation.AmountMinor))
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}
