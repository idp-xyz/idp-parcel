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

// ErrPostingNotAdvanced 说明在作用域锁下按锁后时刻推进运营余额或信用状况，登记却答未推进：登记上已有更晚的截至时点，
// 只可能是别的进程的时钟在前头。静默当成功会让这次映射或核销答成功而余额停在旧值，所以整笔回滚、交给重试。
var ErrPostingNotAdvanced = errors.New("settlement accounting: posting did not advance under the scope lock; a later as-of is already registered, check clock skew between processes")

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
	ClaimTargetStoreUnavailable
)

func (reason FundsUndecidedReason) String() string {
	switch reason {
	case FundsFactStoreUnavailable:
		return "FUNDS_FACT_STORE_UNAVAILABLE"
	case FundsMappingStoreUnavailable:
		return "FUNDS_MAPPING_STORE_UNAVAILABLE"
	case ApplicationStoreUnavailable:
		return "APPLICATION_STORE_UNAVAILABLE"
	case ClaimTargetStoreUnavailable:
		return "CLAIM_TARGET_STORE_UNAVAILABLE"
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
	// OperationEvidence 是提交方（操作者渠道上是 `OPERATOR/<发行方>#<sub>`），只进审计，理由在 ports.FundsFactRecord 同名字段。
	// 不从载荷收：由 Intake 按认证结果填，集成客户端族与受控 CLI 两路不填。
	OperationEvidence string
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
	// OperationEvidence 同 AdoptFundsFactCommand 那一格：更正可能出自与首版不同的人，记在这一版上。
	OperationEvidence string
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
	unapplied    int64
	hasUnapplied bool
	posting      FactScopePosting
	hasPosting   bool
	overApplied  int64
}

// FactScopePosting 说这一步之后一条资金事实怎样记入客户应收作用域的入账余额（ADR-0283）：Scopes 是它经对账单或账户映射
// 落到的客户应收作用域个数。只落到一个作用域的，除核到应付、贷项或金额本体的部分外都记入那一个；落到几个作用域的按
// 核销记入——每个作用域只记指向它的未撤销分配，收款没核销的那部分不计入任何作用域，退回没核销的那部分在每个作用域都
// 先扣。Kind 让读的人分得清是哪一种。
type FactScopePosting struct {
	Scopes int
	Kind   domain.FundsFactKind
}

// ByApplication 为真即这条事实按核销记入，累计未分配不归任何一个作用域。
func (posting FactScopePosting) ByApplication() bool {
	return posting.Scopes > 1
}

// FactScopePosting 只在映射、核销与撤销碰到客户收款这一支（确认到账或资金退回）时交回；第二值为 false 即这一格没有
// 作用域语境。
func (result FundsResult) FactScopePosting() (FactScopePosting, bool) {
	return result.posting, result.hasPosting
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

// FactUnappliedMinor 是这一步之后该事实的累计未分配：当前有效那一版的金额减去事实上全部未撤销核销的合计。核销、
// 核销重放、撤销与撤销重放交回它；第二值为 false 即这一格没有核销语境。不用 SettlementApplication.RemainderMinor：
// 那只扣本笔，同一事实上的别的核销不在里头。
func (result FundsResult) FactUnappliedMinor() (int64, bool) {
	return result.unapplied, result.hasUnapplied
}

// OverAppliedMinor 是更正采用之后这条事实累计超核的金额：新有效版本的金额低于事实上全部未撤销核销的合计时，差额；否则
// 为 0。更正本身成立——外部系统拥有资金事实，本上下文不拒收它——可已核销的合计从此多于钱，这个数把它说出来，不截成 0，
// 也不自动撤销任何核销：撤哪一笔是人的判断。只有 CorrectFact 的结果交回非零值。
func (result FundsResult) OverAppliedMinor() int64 {
	return result.overApplied
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
	// Books、Balances、Standings 是客户收款映射、核销与核销撤销在各自事务里重算的三口（ADR-0181 决定一、决定二，
	// ADR-0182 决定一与决定三）。Standings 只在账户级映射上用。确认费用不从这里挂。
	Books     ports.CustomerReceiptBooks
	Balances  ports.OperationalBalanceAdvance
	Standings ports.CreditStandingAdvance
	// ClaimTargets 读、锁客户赔付与追偿认可两格金额本体：映射核它在本租户的册上，核销在同一事务里锁住它、按它自己的
	// 金额本体减去已核销净额判这次分配能不能落。赔付付款与追偿到账都不是客户应收作用域上的收付：赔付是运营企业对客户
	// 的应付，追偿是对供应商或保险方的应收，默认不与客户应收共用余额、不做净额抵销——所以 Books、Balances、Standings
	// 不从这两格挂。
	ClaimTargets ports.ClaimSettlementTargets
	// FactLocks 锁一条事实并交回它上面未撤销核销的合计：同一笔钱换个核销引用再交，事实一级的上限在这里拦。
	FactLocks ports.FundsFactApplicationLock
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
		{"customer receipt books", deps.Books == nil},
		{"operational balance advance", deps.Balances == nil},
		{"credit standing advance", deps.Standings == nil},
		{"claim settlement targets", deps.ClaimTargets == nil},
		{"funds fact application lock", deps.FactLocks == nil},
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
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversionedAdoptDigest(command)) {
		case domain.DifferentPayload:
			// 同一版本字面携带不同金额或时间：冲突保留原引用——外部更正走版本链，
			// 不在采用处顶替。冲突的那一份没有被采用，无物可交。
			return FundsResult{outcome: FundsFactConflict}, nil
		case domain.UnknownPayloadShape:
			return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
		}
		return handler.existingFact(ctx, existing)
	}
	if _, adopted, err := handler.deps.Facts.FindByKey(ctx, key); err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	} else if adopted {
		// 事实已有版本链、而这个版本字面不在链上：一条事实只有一个首版，第二个首版是冲突不是采用；
		// 更正走 CorrectFact 回指链头。
		return FundsResult{outcome: FundsFactConflict}, nil
	}

	record := ports.FundsFactRecord{Key: key, ContentDigest: digest, Fact: fact, RecordedAt: handler.deps.Clock.Now(),
		OperationEvidence: strings.TrimSpace(command.OperationEvidence)}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	switch saved {
	case ports.FundsFactSaved:
		return handler.adoptedFact(ctx, record)
	case ports.FundsFactAlreadyAdopted:
		winner, found, err := handler.deps.Facts.FindByKey(ctx, key)
		if err != nil || !found {
			return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
		}
		return handler.existingFact(ctx, winner)
	default:
		return FundsResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFundsSave, saved)
	}
}

// CorrectFact 采用一条外部更正：从当前链头经 domain.CorrectAmount 形成回指它的新版本、落版本行、
// 复用同一交接口再发一封（信封 ID 带新版本、载荷回指前版——票 sa-cc/02 裁决 2 预告的那一格）。
// 幂等按（租户+事实+新版本）分重放 / 冲突，照 AdoptFact 四格；回指非链头与更正未采用的事实都是`未受理`。
//
// 外部系统拥有资金事实，更正低于事实上未撤销核销的合计（超核）时也照采，不拒、不撤任何核销：撤哪一笔是人的判断。
// 超核由答复说出来（FundsResult.OverAppliedMinor），入账余额在同一事务里跟着新金额重算（ADR-0285）。
//
// 先对事实身份行取锁、再读任何东西：锁与核销、撤销、映射同一把，求合计的那句读在锁后才含先到那笔已提交的核销。落版本行时
// 外键对身份行取的 KEY SHARE 来在读之后，只靠它排队，合计读的就是旧账。锁序取资金侧全序（ports.FundsFactApplicationLock
// 头注）的子序列：事实 → 这条事实落到的客户应收作用域（按键序）；先于 Save 取齐，隐式的行锁都落在作用域锁之后。
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
	applied, found, err := handler.deps.FactLocks.LockFundsFactApplications(ctx, key)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	if !found {
		// 更正一个未采用的事实：提交矛盾，不是库的事。
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	digest := correctDigest(command)
	existing, found, err := handler.deps.Facts.FindVersion(ctx, key, version)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	if found {
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversionedCorrectDigest(command)) {
		case domain.DifferentPayload:
			// 同一新版本字面携带不同金额或回指：冲突保留先到的那一版，不顶替。
			return FundsResult{outcome: FundsFactConflict}, nil
		case domain.UnknownPayloadShape:
			return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
		}
		return handler.existingCorrection(ctx, key, existing, applied)
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
	scopes, err := handler.factScopes(ctx, command.TenantID, corrected)
	if err != nil {
		return FundsResult{}, err
	}
	if err := handler.lockScopes(ctx, command.TenantID, scopes); err != nil {
		return FundsResult{}, err
	}

	record := ports.FundsFactRecord{Key: key, ContentDigest: digest, Fact: corrected, RecordedAt: handler.deps.Clock.Now(),
		OperationEvidence: strings.TrimSpace(command.OperationEvidence)}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	switch saved {
	case ports.FundsFactSaved:
		// 金额变了，事实落到的每个作用域的 P 都可能随之变（收款的封顶、退回的 −(d+r)），不论升降都重算。
		if err := handler.recomputeScopes(ctx, command.TenantID, scopes); err != nil {
			return FundsResult{}, err
		}
		result, err := handler.adoptedFact(ctx, record)
		if err != nil {
			return FundsResult{}, err
		}
		result.overApplied = overAppliedBy(applied, record)
		return result, nil
	case ports.FundsFactAlreadyAdopted:
		// 两步之间另一位写入方赢了：要么同一新版本先落了，要么链头先被别的版本更正了（库上守链形的
		// 唯一约束把后者也折成`已采用`）。两种都按当前链头作答——它就是此刻被采用的那一版。
		// 同一个「回指的不是当前链头」顺序到达时答的是`未受理`；两答不一致是缺陷，见票 demo-chain-gaps/125 的
		// SA 同形追记。
		winner, found, err := handler.deps.Facts.FindByKey(ctx, key)
		if err != nil || !found {
			return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
		}
		result, err := handler.existingFact(ctx, winner)
		if err != nil {
			return FundsResult{}, err
		}
		result.overApplied = overAppliedBy(applied, winner)
		return result, nil
	default:
		return FundsResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFundsSave, saved)
	}
}

// existingCorrection 按已采用的更正版本作答并带出此刻的超核：响应丢了、操作员重交同一次更正，看到的信号与第一次一致。
// 只有此刻的链头才有超核可说——被后来的更正取代的版本说的是已经过去的金额。
func (handler *MapExternalFundsHandler) existingCorrection(
	ctx context.Context,
	key ports.FundsFactKey,
	record ports.FundsFactRecord,
	applied int64,
) (FundsResult, error) {
	head, found, err := handler.deps.Facts.FindByKey(ctx, key)
	if err != nil || !found {
		return fundsUndecided(FundsFactStoreUnavailable, key.Fact.String()), nil
	}
	result, err := handler.existingFact(ctx, record)
	if err != nil {
		return FundsResult{}, err
	}
	if head.Fact.Version() == record.Fact.Version() {
		result.overApplied = overAppliedBy(applied, record)
	}
	return result, nil
}

// overAppliedBy 是事实上未撤销核销的合计超出这一版金额的部分；没超出为 0。
func overAppliedBy(applied int64, record ports.FundsFactRecord) int64 {
	_, amount := record.Fact.Amount()
	if applied > amount {
		return applied - amount
	}
	return 0
}

// factScopes 交回一条事实经对账单或账户映射落到的全部客户应收作用域，按键序排好（sortScopes）——更正改的是事实金额，
// 它所及的每个作用域都在重算之列。收付之外的事实不进任何客户应收作用域。
func (handler *MapExternalFundsHandler) factScopes(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFact,
) ([]customerScope, error) {
	switch fact.Kind() {
	case domain.FundsReceiptConfirmed, domain.FundsReturned:
	default:
		return nil, nil
	}
	reached, err := handler.deps.Books.FactCustomerScopes(ctx, tenant, fact.Fact())
	if err != nil {
		return nil, err
	}
	reached = sortScopes(reached)
	scopes := make([]customerScope, 0, len(reached))
	for _, scope := range reached {
		scopes = append(scopes, customerScope{scope: scope})
	}
	return scopes, nil
}

// adoptedFact 按刚落下的那一版作答并交出意图。交接被确定性拒收时不给结果（handOffFact 头注）。
func (handler *MapExternalFundsHandler) adoptedFact(
	ctx context.Context,
	record ports.FundsFactRecord,
) (FundsResult, error) {
	handoff, err := handler.handOffFact(ctx, record)
	if err != nil {
		return FundsResult{}, err
	}
	return FundsResult{outcome: FundsFactAdopted, fact: record, hasRecord: true, handoff: handoff}, nil
}

// existingFact 按已采用的事实作答并再交一次同一份意图：重放交的是同一封（同租户、同事实、
// 同版本），Outbox 按认领键吞掉第二次——「重放不交」在真库上就是这样成立的；不在这里跳过
// 交接，是为了让上一次交接失败留下的那封在重放时补上（与 existingApplication 同形）。
// 重放同一份被确定性拒收的信封同样返 error 不答`已存在`：那一格永远同一个结果，答`已存在`就是把
// 「CC 永远收不到那一版」说成幂等成功。
func (handler *MapExternalFundsHandler) existingFact(
	ctx context.Context,
	record ports.FundsFactRecord,
) (FundsResult, error) {
	handoff, err := handler.handOffFact(ctx, record)
	if err != nil {
		return FundsResult{}, err
	}
	return FundsResult{outcome: FundsFactExisting, fact: record, hasRecord: true, handoff: handoff}, nil
}

// handOffFact 把采用成功的资金事实交给下游（票 sa-cc/02），失败按种类分两格（票 sa-cc/32 裁决 2）：
//
//   - 信封被框架确定性校验拒收（ports.ErrFundsFactHandoffRejected）→ 原样返 error。同一份输入重投永远同一个
//     结果，折成续办引用只会让操作者按「重跑补发」重跑到死；返 error 让事务壳把版本行一起回滚——版本行、信封、
//     结果三者同生同灭。
//   - 其余失败（依赖不可用等）→ 不翻结果、留续办引用：事实已采用是真的，只是那封信还没出去，重放时重发同一份。
//     单事务入口下 Outbox 的 SQL 错会先把事务打进中止态、由事务壳返错，这一格今天实际到不了；留着它是因为它
//     对依赖故障说的话是真的，删它要动 FundsResult 的形与两个入口的答案格，是另一张票的量。
func (handler *MapExternalFundsHandler) handOffFact(
	ctx context.Context,
	record ports.FundsFactRecord,
) (string, error) {
	err := handler.deps.FactHandoff.HandOffExternalFundsFact(ctx, ports.ExternalFundsFactIntent{Record: record})
	if err == nil {
		return "", nil
	}
	if errors.Is(err, ports.ErrFundsFactHandoffRejected) {
		return "", err
	}
	return fundsContinuation("EXTERNAL_FUNDS_FACT_HANDOFF",
		record.Key.TenantID.String(), record.Key.Fact.String(), record.Fact.Version().String()), nil
}

// Map 建立一条资金映射：显式依据由 MapFundsToTarget 把门（巧合不证明映射）；失败
// 付款 → UNFUNDABLE_FACT 业务负向（ErrUnfundableFact 哨兵分格）。
//
// 映到对账单或账户的，先锁事实身份行再读事实、插映射行（锁序见 ports.FundsFactApplicationLock 头注）：一条事实落到
// 几个客户应收作用域决定每个作用域怎么记它（ADR-0283），两笔映射把同一条事实并发映到两个作用域时，不在同一把锁上
// 排队就各自只看见自己那一条、都当自己是唯一的作用域去记没核销的部分。这把锁与核销、撤销同一把，映射与核销也因此排队。
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

	factKey := ports.FundsFactKey{TenantID: command.TenantID, Fact: factRef}
	customerTarget := command.TargetKind == domain.TargetStatement || command.TargetKind == domain.TargetAccount
	alreadyApplied := int64(0)
	if customerTarget {
		applied, found, err := handler.deps.FactLocks.LockFundsFactApplications(ctx, factKey)
		if err != nil {
			return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
		}
		if !found {
			return FundsResult{outcome: FundsNotAccepted}, nil
		}
		alreadyApplied = applied
	}
	factRecord, found, err := handler.deps.Facts.FindByKey(ctx, factKey)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	if !found {
		// 映射一个未采用的事实：提交矛盾，先采用再映射。
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	_, factMinor := factRecord.Fact.Amount()

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
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversionedMapDigest(command)) {
		case domain.DifferentPayload:
			return FundsResult{outcome: FundsMappingConflict}, nil
		case domain.UnknownPayloadShape:
			return fundsUndecided(FundsMappingStoreUnavailable, command.Mapping), nil
		}
		return FundsResult{outcome: FundsMappingExisting, mapping: existing, hasRecord: true}, nil
	}
	if mapping.TargetKind().PointsAtClaimAmount() {
		exists, err := handler.deps.ClaimTargets.FindClaimTarget(ctx, command.TenantID,
			domain.ClaimTargetKey{Kind: mapping.TargetKind(), Target: mapping.Target()})
		if err != nil {
			return fundsUndecided(ClaimTargetStoreUnavailable, command.Target), nil
		}
		if !exists {
			// 不存在与不属本租户同一格：调用方的恢复动作都是改目标，分开答就是回答了一个它无权问的问题（ADR-0029）。
			return FundsResult{outcome: FundsNotAccepted}, nil
		}
	}

	// 这条映射之前事实已落到哪几个作用域：一个变两个的那一笔要连原作用域一起重算（advanceCustomerReceipt）。在事实锁下
	// 读，同一条事实别的映射插不进来。
	var before []domain.SettlementScope
	if customerTarget {
		if before, err = handler.deps.Books.FactCustomerScopes(ctx, command.TenantID, factRef); err != nil {
			return FundsResult{}, err
		}
	}

	record := ports.FundsMappingRecord{Key: key, ContentDigest: digest, Mapping: mapping, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Mappings.Save(ctx, record)
	if err != nil {
		return fundsUndecided(FundsMappingStoreUnavailable, command.Mapping), nil
	}
	switch saved {
	case ports.FundsMappingSaved:
		posting, posted, err := handler.advanceCustomerReceipt(ctx, command, factRecord.Fact, before)
		if err != nil {
			return FundsResult{}, err
		}
		result := FundsResult{outcome: FundsMapped, mapping: record, hasRecord: true}
		if posted {
			result.posting, result.hasPosting = posting, true
			result.unapplied, result.hasUnapplied = factMinor-alreadyApplied, true
		}
		return result, nil
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

// Apply 落一次核销：核销是显式判断（依据必备）；分配必须有映射背书（凭巧合分钱在领域被拒）；跨币种无换算 →
// CROSS_CURRENCY；净额越界、事实上的核销合计越过事实金额、金额本体未核销不够 → APPLICATION_IMBALANCE；指向对账单的分配
// 会让作用域已确认未结为负 → 未受理。落行之后在同一事务里重算这条事实落到的客户应收作用域（ADR-0181 决定一，
// 取哪几个见 applicationScopes），意图交下游已结视图。
//
// 锁按资金侧的全序取：事实 → 金额本体 → 运营余额作用域，作用域之间按键序（ports.FundsFactApplicationLock 头注）。
// 同键与事实上限都在锁后判：并发两份同键，后到的那份在事实锁上等，先到的提交后它查得到那一行，答已存在；在锁前查就会
// 被事实上限判成失衡。
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

	factKey := ports.FundsFactKey{TenantID: command.TenantID, Fact: factRef}
	alreadyApplied, found, err := handler.deps.FactLocks.LockFundsFactApplications(ctx, factKey)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	if !found {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	factRecord, found, err := handler.deps.Facts.FindByKey(ctx, factKey)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, command.Fact), nil
	}
	if !found {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	_, factMinor := factRecord.Fact.Amount()

	key := ports.SettlementApplicationKey{TenantID: command.TenantID, Application: applicationRef}
	digest := applyDigest(command)
	existingApplication, alreadyRecorded, err := handler.deps.Applications.FindByKey(ctx, key)
	if err != nil {
		return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
	}
	if alreadyRecorded {
		switch domain.CompareStoredDigest(existingApplication.ContentDigest, digest, unversionedApplyDigest(command)) {
		case domain.DifferentPayload:
			return FundsResult{outcome: ApplicationConflict}, nil
		case domain.UnknownPayloadShape:
			return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
		}
		result, err := handler.existingApplication(ctx, existingApplication, factMinor-alreadyApplied)
		if err != nil {
			return FundsResult{}, err
		}
		return handler.withFactPosting(ctx, command.TenantID, factRecord.Fact, result)
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
	if err == nil {
		err = application.AdmitWithinFact(alreadyApplied)
	}
	switch {
	case errors.Is(err, domain.ErrCrossCurrencyApplication):
		// 跨币种没有认可换算依据：核销无从谈起（同 CC 税费核对纪律）。
		return FundsResult{outcome: CrossCurrencyOutcome}, nil
	case errors.Is(err, domain.ErrApplicationImbalance):
		// 净额为零、超过事实金额，或与事实上已有的核销合计超过事实金额：超额分别表达，不静默截断。
		return FundsResult{outcome: ApplicationImbalanceOutcome}, nil
	case errors.Is(err, domain.ErrUnfundableFact):
		return FundsResult{outcome: UnfundableFactOutcome}, nil
	case err != nil:
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	refused, stop, err := handler.admitClaimTargets(ctx, command.TenantID, factRecord.Fact, application)
	if err != nil {
		return FundsResult{}, err
	}
	if stop {
		return refused, nil
	}
	scopes, refused, stop, err := handler.admitApplicationScopes(ctx, command.TenantID, factRecord.Fact, application)
	if err != nil {
		return FundsResult{}, err
	}
	if stop {
		return refused, nil
	}

	record := ports.SettlementApplicationRecord{Key: key, ContentDigest: digest, Application: application, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Applications.Save(ctx, record)
	if err != nil {
		return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
	}
	switch saved {
	case ports.SettlementApplicationSaved:
		if err := handler.recomputeScopes(ctx, command.TenantID, scopes); err != nil {
			return FundsResult{}, err
		}
		handoff, err := handler.handOff(ctx, record)
		if err != nil {
			return FundsResult{}, err
		}
		result := FundsResult{outcome: SettlementApplied, application: record, hasRecord: true, handoff: handoff,
			unapplied: factMinor - alreadyApplied - application.AppliedMinor(), hasUnapplied: true}
		return handler.withFactPosting(ctx, command.TenantID, factRecord.Fact, result)
	case ports.SettlementApplicationAlreadyApplied:
		// 同键在事实锁后查过仍会走到这里：同一核销引用、不同事实的两份并发交，各锁各的事实，都查不到对方未提交的那行。
		// 照锁后那段分出冲突与已存在；赢家可能落在另一条事实上，这里手上的已核销合计不是它的，不报累计未分配。
		winner, found, err := handler.deps.Applications.FindByKey(ctx, key)
		if err != nil || !found {
			return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
		}
		switch domain.CompareStoredDigest(winner.ContentDigest, digest, unversionedApplyDigest(command)) {
		case domain.DifferentPayload:
			return FundsResult{outcome: ApplicationConflict}, nil
		case domain.UnknownPayloadShape:
			return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
		}
		handoff, err := handler.handOff(ctx, winner)
		if err != nil {
			return FundsResult{}, err
		}
		return FundsResult{outcome: ApplicationExisting, application: winner, hasRecord: true, handoff: handoff}, nil
	default:
		return FundsResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFundsSave, saved)
	}
}

// Reverse 撤销一次核销：原分配不删、撤销一次为限（ErrApplicationReversed 哨兵分格），撤销版随意图重新交下游；同一事务里
// 重算与核销时同一组作用域（applicationScopes），已核销分配等额回到已确认未结（ADR-0181 决定一、AT-SA-113）；多作用域
// 事实上撤下的那一截回到未分配，照 ADR-0283 的记法离开它原来指向的那个作用域。
//
// 锁取全序（ports.FundsFactApplicationLock 头注）的子序列：事实 → 运营余额作用域（按键序）。锁事实之后再读一次核销：
// 并发两份撤销，后到的那份读到的已是撤销版，答已撤销。
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
	factKey := ports.FundsFactKey{TenantID: command.TenantID, Fact: existing.Application.Fact()}
	alreadyApplied, found, err := handler.deps.FactLocks.LockFundsFactApplications(ctx, factKey)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, existing.Application.Fact().String()), nil
	}
	if !found {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	factRecord, found, err := handler.deps.Facts.FindByKey(ctx, factKey)
	if err != nil {
		return fundsUndecided(FundsFactStoreUnavailable, existing.Application.Fact().String()), nil
	}
	if !found {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	_, factMinor := factRecord.Fact.Amount()
	if existing, found, err = handler.deps.Applications.FindByKey(ctx, key); err != nil {
		return fundsUndecided(ApplicationStoreUnavailable, command.Application), nil
	} else if !found {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}

	reversed, err := existing.Application.Reverse(basis, command.ReversedAt)
	if errors.Is(err, domain.ErrApplicationReversed) {
		// 已撤销重放：返回原撤销，不二撤。
		return handler.withFactPosting(ctx, command.TenantID, factRecord.Fact, FundsResult{
			outcome: ApplicationAlreadyReversed, application: existing, hasRecord: true,
			unapplied: factMinor - alreadyApplied, hasUnapplied: true})
	}
	if err != nil {
		return FundsResult{outcome: FundsNotAccepted}, nil
	}
	scopes, err := handler.applicationScopes(ctx, command.TenantID, factRecord.Fact, existing.Application)
	if err != nil {
		return FundsResult{}, err
	}
	if err := handler.lockScopes(ctx, command.TenantID, scopes); err != nil {
		return FundsResult{}, err
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
	if err := handler.recomputeScopes(ctx, command.TenantID, scopes); err != nil {
		return FundsResult{}, err
	}
	handoff, err := handler.handOff(ctx, existing)
	if err != nil {
		return FundsResult{}, err
	}
	return handler.withFactPosting(ctx, command.TenantID, factRecord.Fact, FundsResult{
		outcome: ApplicationReversedOutcome, application: existing, hasRecord: true, handoff: handoff,
		unapplied: factMinor - (alreadyApplied - reversed.AppliedMinor()), hasUnapplied: true})
}

// withFactPosting 给核销与撤销的答复补上这条事实此刻怎样记入客户应收作用域。
func (handler *MapExternalFundsHandler) withFactPosting(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFact,
	result FundsResult,
) (FundsResult, error) {
	posting, posted, err := handler.factPosting(ctx, tenant, fact)
	if err != nil {
		return FundsResult{}, err
	}
	result.posting, result.hasPosting = posting, posted
	return result, nil
}

// customerScope 是一次核销或撤销要重算的一个客户应收作用域。statement 为真即本次有分配指向它的对账单，writeoff 是那几片
// 的净额（借加贷减，同 W 的求法），落行之前按它判已确认未结会不会为负。
type customerScope struct {
	scope     domain.SettlementScope
	statement bool
	writeoff  int64
}

// applicationScopes 交回一次核销或撤销改了 P 或 W 的客户应收作用域，按键序排好（sortScopes）——锁按这个顺序取。
// 改动只来自这一笔的分配与它挪动的「没核销的部分」r（ADR-0283）：
//
//   - 分配所指的作用域：对账单分配与账户分配各按目标收成作用域（目标与它沿的映射一致），d 在那里动，对账单分配的
//     W 也在那里动。
//   - 事实只落到一个客户应收作用域时，那一个：r 进它的 P，核到应付、贷项或金额本体上的分配也会挪动 r。
//   - 事实是退回时，它落到的全部作用域：退回没核销的部分在每个作用域都扣。
//
// 落到几个作用域的收款，核到别处的分配不改任何作用域的 P，不碰它们——一个作用域账已不平时，别的作用域上的核销
// 照样落下。statement 为真的那几项另带对账单分配的净额，落行之前按它判已确认未结。
func (handler *MapExternalFundsHandler) applicationScopes(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFact,
	application domain.SettlementApplication,
) ([]customerScope, error) {
	byScope := map[domain.SettlementScope]customerScope{}
	for _, allocation := range application.Allocations() {
		var (
			scope domain.SettlementScope
			found bool
			err   error
		)
		switch allocation.TargetKind {
		case domain.TargetStatement:
			scope, found, err = handler.deps.Books.CustomerStatementScope(ctx, tenant, allocation.Target.String())
		case domain.TargetAccount:
			scope, found, err = handler.deps.Books.ReceivableAccountScope(ctx, tenant, allocation.Target.String())
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		entry := byScope[scope]
		if allocation.TargetKind == domain.TargetStatement {
			entry.statement = true
			if allocation.Direction == domain.AllocationDebit {
				entry.writeoff += allocation.AmountMinor
			} else {
				entry.writeoff -= allocation.AmountMinor
			}
		}
		byScope[scope] = entry
	}
	keys := make([]domain.SettlementScope, 0, len(byScope))
	for scope := range byScope {
		keys = append(keys, scope)
	}
	if fact.Kind() == domain.FundsReceiptConfirmed || fact.Kind() == domain.FundsReturned {
		factScopes, err := handler.deps.Books.FactCustomerScopes(ctx, tenant, fact.Fact())
		if err != nil {
			return nil, err
		}
		if fact.Kind() == domain.FundsReturned || len(factScopes) == 1 {
			keys = append(keys, factScopes...)
		}
	}
	keys = sortScopes(keys)
	scopes := make([]customerScope, 0, len(keys))
	for _, scope := range keys {
		entry := byScope[scope]
		entry.scope = scope
		scopes = append(scopes, entry)
	}
	return scopes, nil
}

// admitApplicationScopes 按键序逐个锁住本次核销要重算的作用域，在指向对账单的那几个上按账判这一笔落下之后已确认未结
// 会不会为负：会就答`未受理`、什么都不落——分给对账单的钱超出了该作用域已确认的应收，这是分配写错了，改分配才行，
// 重交同一份永远同一个结果。第三值为真即已作答。
func (handler *MapExternalFundsHandler) admitApplicationScopes(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFact,
	application domain.SettlementApplication,
) ([]customerScope, FundsResult, bool, error) {
	scopes, err := handler.applicationScopes(ctx, tenant, fact, application)
	if err != nil {
		return nil, FundsResult{}, false, err
	}
	if err := handler.lockScopes(ctx, tenant, scopes); err != nil {
		return nil, FundsResult{}, false, err
	}
	for _, scope := range scopes {
		if !scope.statement {
			continue
		}
		postedNet, writeoffNet, confirmed, err := handler.deps.Books.CustomerDirectionNets(ctx, tenant, scope.scope)
		if err != nil {
			return nil, FundsResult{}, false, err
		}
		if _, _, err := domain.PostedAndUnsettled(postedNet, writeoffNet+scope.writeoff, confirmed); errors.Is(err, domain.ErrNegativeConfirmedUnsettled) {
			return nil, FundsResult{outcome: FundsNotAccepted}, true, nil
		}
	}
	return scopes, FundsResult{}, false, nil
}

// lockScopes 按交来的次序（applicationScopes 已排好键序）取作用域咨询锁。
func (handler *MapExternalFundsHandler) lockScopes(
	ctx context.Context,
	tenant domain.TenantID,
	scopes []customerScope,
) error {
	for _, scope := range scopes {
		if err := handler.deps.Balances.LockOperationalBalanceScope(ctx, tenant, scope.scope); err != nil {
			return err
		}
	}
	return nil
}

// recomputeScopes 在调用方已锁住的作用域上逐个重算推进；锁再取一次是重入。
func (handler *MapExternalFundsHandler) recomputeScopes(
	ctx context.Context,
	tenant domain.TenantID,
	scopes []customerScope,
) error {
	for _, scope := range scopes {
		if _, err := handler.recomputeScope(ctx, tenant, scope.scope); err != nil {
			return err
		}
	}
	return nil
}

// factPosting 交回一条事实此刻怎样记入客户应收作用域，给核销与撤销的答复用；收付之外的事实与没落到任何客户应收
// 作用域的事实第二值为 false。
func (handler *MapExternalFundsHandler) factPosting(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFact,
) (FactScopePosting, bool, error) {
	switch fact.Kind() {
	case domain.FundsReceiptConfirmed, domain.FundsReturned:
	default:
		return FactScopePosting{}, false, nil
	}
	scopes, err := handler.deps.Books.FactCustomerScopes(ctx, tenant, fact.Fact())
	if err != nil {
		return FactScopePosting{}, false, err
	}
	scopes = sortScopes(scopes)
	if len(scopes) == 0 {
		return FactScopePosting{}, false, nil
	}
	return FactScopePosting{Scopes: len(scopes), Kind: fact.Kind()}, true, nil
}

// recomputeScope 是入账余额与已确认未结唯一的求法入口（ADR-0181 决定三「函数只有一个」）：映射、核销、核销撤销都从
// 这里推进，交回这次推进用的截至时点。先锁作用域再取时刻、再按账读：锁前取的时刻可能早于占锁那一笔推进时的时刻，
// 「同键只准往前」就会把这一次吞成未推进；锁后的读才含先到那笔已提交的账。已确认未结求出为负是账不平，推进失败、
// 整笔回滚；锁下仍答未推进同样整笔回滚（ErrPostingNotAdvanced）。
func (handler *MapExternalFundsHandler) recomputeScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
) (time.Time, error) {
	if err := handler.deps.Balances.LockOperationalBalanceScope(ctx, tenant, scope); err != nil {
		return time.Time{}, err
	}
	asOf := handler.deps.Clock.Now()
	postedNet, writeoffNet, confirmed, err := handler.deps.Books.CustomerDirectionNets(ctx, tenant, scope)
	if err != nil {
		return time.Time{}, err
	}
	posted, unsettled, err := domain.PostedAndUnsettled(postedNet, writeoffNet, confirmed)
	if err != nil {
		// 带上是哪个作用域：一笔映射、核销或撤销可能一次重算几个作用域，续办要人去核对的是账不平的那一个。
		return time.Time{}, fmt.Errorf("%w：作用域 法人 %s、账户 %s、币种 %s", err,
			scope.LegalEntity().String(), scope.Account().String(), scope.Currency().String())
	}
	advanced, err := handler.deps.Balances.AdvanceOperationalBalance(ctx, tenant, scope, posted, unsettled, asOf)
	if err != nil {
		return time.Time{}, err
	}
	if advanced != ports.OperationalBalanceAdvanced {
		return time.Time{}, fmt.Errorf("%w: operational balance %s", ErrPostingNotAdvanced, advanced)
	}
	return asOf, nil
}

// admitClaimTargets 按目标逐条判本次核销分给客户赔付、追偿认可两格的合计能不能落：在调用方的事务里先锁住那条金额，
// 再读它的已核销净额——锁与随后的 Save 同一笔，并发核销同一条金额时后到的那笔读到的已核销含前一笔。锁在事实之后、
// 作用域之前取，次序取 ClaimTargetDemands 给的顺序。金额本体只读不写：核销改的是核销状态与未分配余额，不是金额。
// 交回的 bool 为真即已作答，调用方把交回的 FundsResult 原样交出、什么都不落；交回的 error 是那一条账不平
// （ErrInvalidClaimTargetStanding），整笔回滚。
func (handler *MapExternalFundsHandler) admitClaimTargets(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFact,
	application domain.SettlementApplication,
) (FundsResult, bool, error) {
	factCurrency, _ := fact.Amount()
	for _, demand := range application.ClaimTargetDemands() {
		standing, found, err := handler.deps.ClaimTargets.LockClaimTarget(ctx, tenant, demand.Key)
		if errors.Is(err, domain.ErrInvalidClaimTargetStanding) {
			// 那一条的已核销已越过它的金额本体（账不平）：续办与作用域已确认未结为负那一格相同——核对指向它的核销、
			// 撤掉多出的那几笔——所以交回同一类确定性错误、整笔回滚，由入口归同一格（ADR-0029），不折成未决。
			return FundsResult{}, false, err
		}
		if err != nil {
			return fundsUndecided(ClaimTargetStoreUnavailable, demand.Key.Target.String()), true, nil
		}
		if !found {
			// 分配都有映射背书、映射时核过在册，金额册又只增不改，正常走不到这里；走到了按映射时那一格答，不放它落下。
			return FundsResult{outcome: FundsNotAccepted}, true, nil
		}
		switch err := standing.Admit(factCurrency, demand.AmountMinor); {
		case errors.Is(err, domain.ErrCrossCurrencyApplication):
			return FundsResult{outcome: CrossCurrencyOutcome}, true, nil
		case errors.Is(err, domain.ErrApplicationImbalance):
			return FundsResult{outcome: ApplicationImbalanceOutcome}, true, nil
		case err != nil:
			return FundsResult{outcome: FundsNotAccepted}, true, nil
		}
	}
	return FundsResult{}, false, nil
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

// existingApplication 按已有核销作答并重发同一份意图。同一份被确定性拒收的信封重放仍返 error，不答`已存在`
// （existingFact 同一条理由）。
func (handler *MapExternalFundsHandler) existingApplication(
	ctx context.Context,
	record ports.SettlementApplicationRecord,
	unappliedMinor int64,
) (FundsResult, error) {
	handoff, err := handler.handOff(ctx, record)
	if err != nil {
		return FundsResult{}, err
	}
	return FundsResult{outcome: ApplicationExisting, application: record, hasRecord: true, handoff: handoff,
		unapplied: unappliedMinor, hasUnapplied: true}, nil
}

// handOff 把核销/撤销交给下游已结视图，失败按种类分两格，判据同 handOffFact：信封被确定性拒收原样返 error，让事务壳
// 把核销行一起回滚；其余失败不翻结果，留续办引用，重放时重发同一份。
func (handler *MapExternalFundsHandler) handOff(
	ctx context.Context,
	record ports.SettlementApplicationRecord,
) (string, error) {
	err := handler.deps.Downstream.HandOffSettlementApplication(ctx, ports.SettlementApplicationIntent{Record: record})
	if err == nil {
		return "", nil
	}
	if errors.Is(err, ports.ErrSettlementApplicationHandoffRejected) {
		return "", err
	}
	return fundsContinuation("SETTLEMENT_APPLICATION_HANDOFF", record.Key.TenantID.String(), record.Key.Application.String()), nil
}

// advanceCustomerReceipt 在客户对账单或应收结算账户上的确认到账与资金退回之后，重算这一笔映射改了 P 的作用域
// （ADR-0283）：映射目标所在的作用域；一条收款由这一笔从落到一个作用域变成两个时，原来那个作用域也算——它的 P 从
// 「d + 没核销的部分」变成只记 d，入账余额会降，不在这一笔里跟上就停在多出来的数上。落到两个以上之后再多一个、或
// 退回多落一个，既有作用域的 P 不变，不重算。作用域锁按键序取（sortScopes）。供应商应付、贷项、两格金额本体与确认
// 费用都不从这里进；核销与撤销从 recomputeScopes 进同一个求法。对账单收不成应收作用域时映射成立、余额不动——那不是
// 这一支。账户目标收不成应收作用域则整笔失败。已确认未结求出为负则整笔失败，调用方的事务一起回滚。信用状况只在
// 账户级映射的那个作用域上重算。before 是插这一条映射之前事实已落到的作用域。第二值为 false 即这条事实没有落到
// 任何客户应收作用域上，交回的记入方式不作数。
func (handler *MapExternalFundsHandler) advanceCustomerReceipt(
	ctx context.Context,
	command MapFundsCommand,
	fact domain.ExternalFundsFact,
	before []domain.SettlementScope,
) (FactScopePosting, bool, error) {
	switch fact.Kind() {
	case domain.FundsReceiptConfirmed, domain.FundsReturned:
	default:
		return FactScopePosting{}, false, nil
	}

	var (
		target domain.SettlementScope
		found  bool
		err    error
	)
	switch command.TargetKind {
	case domain.TargetStatement:
		target, found, err = handler.deps.Books.CustomerStatementScope(ctx, command.TenantID, command.Target)
		if err != nil {
			return FactScopePosting{}, false, err
		}
	case domain.TargetAccount:
		target, found, err = handler.deps.Books.ReceivableAccountScope(ctx, command.TenantID, command.Target)
		if err != nil {
			return FactScopePosting{}, false, err
		}
		if !found {
			return FactScopePosting{}, false, domain.ErrNotReceivableAccount
		}
	default:
		return FactScopePosting{}, false, nil
	}

	before = sortScopes(before)
	after := before
	if found {
		after = sortScopes(append(append([]domain.SettlementScope(nil), before...), target))
	}
	if len(after) == 0 {
		return FactScopePosting{}, false, nil
	}
	posting := FactScopePosting{Scopes: len(after), Kind: fact.Kind()}
	if !found {
		return posting, true, nil
	}

	moved := []domain.SettlementScope{target}
	if fact.Kind() == domain.FundsReceiptConfirmed && len(before) == 1 && len(after) == 2 {
		moved = append(moved, before[0])
	}
	moved = sortScopes(moved)
	for _, scope := range moved {
		if err := handler.deps.Balances.LockOperationalBalanceScope(ctx, command.TenantID, scope); err != nil {
			return FactScopePosting{}, false, err
		}
	}
	var targetAsOf time.Time
	for _, scope := range moved {
		asOf, err := handler.recomputeScope(ctx, command.TenantID, scope)
		if err != nil {
			return FactScopePosting{}, false, err
		}
		if scope == target {
			targetAsOf = asOf
		}
	}
	if command.TargetKind != domain.TargetAccount {
		return posting, true, nil
	}
	if err := handler.advanceCreditStanding(ctx, command.TenantID, target, targetAsOf); err != nil {
		return FactScopePosting{}, false, err
	}
	return posting, true, nil
}

// sortScopes 去重并按（法人、账户、币种）排序：资金侧作用域咨询锁的键序。一笔事务要取几个作用域时都按这个次序取，
// 两笔事务各握一个、等另一个就接不成环（ports.FundsFactApplicationLock 头注）。
func sortScopes(scopes []domain.SettlementScope) []domain.SettlementScope {
	seen := make(map[domain.SettlementScope]bool, len(scopes))
	unique := make([]domain.SettlementScope, 0, len(scopes))
	for _, scope := range scopes {
		if seen[scope] {
			continue
		}
		seen[scope] = true
		unique = append(unique, scope)
	}
	sort.Slice(unique, func(i, j int) bool {
		left, right := unique[i], unique[j]
		if left.LegalEntity() != right.LegalEntity() {
			return left.LegalEntity().String() < right.LegalEntity().String()
		}
		if left.Account() != right.Account() {
			return left.Account().String() < right.Account().String()
		}
		return left.Currency().String() < right.Currency().String()
	})
	return unique
}

// advanceCreditStanding 按已确认费用的到期画像决定这次写不写信用状况。
// 没有已确认费用则逾期为假并写行。已有已确认费用却缺到期日则这次不写。
func (handler *MapExternalFundsHandler) advanceCreditStanding(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	asOf time.Time,
) error {
	picture, err := handler.deps.Books.ConfirmedChargeAging(ctx, tenant, scope, asOf)
	if err != nil {
		return err
	}
	if picture.ConfirmedCount > 0 && picture.MissingDueDate {
		return nil
	}
	overdue := false
	if picture.ConfirmedCount > 0 {
		overdue = picture.Overdue
	}
	advanced, err := handler.deps.Standings.AdvanceCreditStanding(ctx, tenant, scope, overdue, asOf)
	if err != nil {
		return err
	}
	if advanced != ports.CreditStandingAdvanced {
		// 与运营余额同一把作用域锁、同一个锁后时刻：答未推进只可能是时钟回拨，理由同 ErrPostingNotAdvanced。
		return fmt.Errorf("%w: credit standing %s", ErrPostingNotAdvanced, advanced)
	}
	return nil
}

func fundsContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// adoptDigest 把付款人算进内容：同引用换付款人是另一份内容（冲突），不是重放。
//
// 付款人自 migrations/settlement_accounting/0018_external_funds_fact_payer.sql 起进摘要（票 sa-cc/03）；
// 0018 之前本上下文的采用没有生产入口、存量为零（与该迁移头注同一句）。摘要元素一变，变之前落下的行
// 重投同一内容就会撞 ContentDigest 答`内容冲突`——所以 SAC-1 起摘要带形状前缀，之前落下的行按
// unversionedAdoptDigest 那一版比，不回算存量。再改元素就再换形状版本。
func adoptDigest(command AdoptFundsFactCommand) string {
	return settlementCanonicalDigest(domain.CanonicalizeAdoptFundsFactPayload(
		command.Source, command.Payer, int64(command.Kind), command.Currency, command.AmountMinor, command.Version, command.OccurredAt))
}

// correctDigest 只算命令自己带的四样：回指、新版本、金额、更正时刻。来源 / 付款人 / 种类 / 币种 / 发生时刻
// 从链头照抄，不在命令上，也就不进摘要——同一新版本字面换一个金额或换一个回指是另一份内容（冲突），
// 不是重放。与 adoptDigest 元素不同是有意的：同一（事实、版本）若先经 AdoptFact 作首版落下、再有人拿它
// 当更正版本来提，两份摘要必不相等，答`冲突`而不是把首版当成更正的重放。
func correctDigest(command CorrectFundsFactCommand) string {
	return settlementCanonicalDigest(domain.CanonicalizeCorrectFundsFactPayload(command.Corrects, command.Version, command.AmountMinor, command.CorrectedAt))
}

func mapDigest(command MapFundsCommand) string {
	return settlementCanonicalDigest(domain.CanonicalizeMapFundsPayload(command.Fact, int64(command.TargetKind), command.Target, command.Basis, command.MappedAt))
}

func applyDigest(command ApplySettlementCommand) string {
	allocations := make([]domain.SettlementAllocationContent, len(command.Allocations))
	for i, allocation := range command.Allocations {
		allocations[i] = domain.SettlementAllocationContent{
			Mapping: allocation.Mapping, TargetKind: int64(allocation.TargetKind), Target: allocation.Target,
			Direction: int64(allocation.Direction), AmountMinor: allocation.AmountMinor,
		}
	}
	return settlementCanonicalDigest(domain.CanonicalizeApplySettlementPayload(
		command.Fact, command.TargetCurrency, command.Basis, command.AppliedAt, command.MappingRefs, allocations))
}
