package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pshandoff "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/productionhandoff"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 审计票 13——`/shipment-requests` 的第二参是真编排：整条链在真实 PostgreSQL
// 上装得起来；治理目录未配置时归属如实答`权威未确定`、提交停在 OWNERSHIP_UNRESOLVED
// 且带着归属决定；来源保全确实落库——重放同一份输入答`已有结果`，证明首笔事务真的
// 提交了，而不是装配在某个替身上悄悄成立。测试输入是隔离合成，只记 `S`，不进生产装配。
//
// 隔离形态入参传 nil，因此本用例同时是 ADR-0091「生产形态一字未变」那条的钉子：
// 隔离形态哪天漏进生产装配，停在 OWNERSHIP_UNRESOLVED 这一格会在这里先变绿再被发现。
func TestTheWiredSubmissionAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	submission, err := buildSubmissionOrchestration(db, nil)
	if err != nil {
		t.Fatalf("装配提交编排：%v", err)
	}

	command := submissionCommand(t)
	first, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("首次提交：%v", err)
	}
	if got := first.Outcome(); got != shipmentapp.OutcomeOwnershipUnresolved {
		t.Fatalf("outcome = %v, want OWNERSHIP_UNRESOLVED——目录未配置时归属只能答未确定", got)
	}
	if _, has := first.OwnershipDecision(); !has {
		t.Fatalf("被拦下的提交要带归属决定，调用方才有续办引用")
	}

	replay, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放同一份输入：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.OutcomeExistingResult {
		t.Fatalf("outcome = %v, want EXISTING_RESULT——重放靠的是首笔事务已提交的来源保全", got)
	}
}

// Covers: 两段事务边界的第一段（简报「事务边界」；ADR-0081）——来源保全在自己的事务里
// 独立提交，编排随后给出业务性拒绝也动不了它，重放因此答`已有结果`。「被拒的提交也
// 保全来源」由 preservationBoundary 的边界位置兑现，不再依赖「答案即提交」的大事务。
//
// 建单事务回滚的半边不在此重证：WithinTransaction 对返回错误即回滚是框架合同，
// 建单与信封同事务的原子性由 `tests/bentocontract` 的 PBC-04/05/07 在真库上取证。
func TestARefusedSubmissionStillPreservesItsSource(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	submission, err := buildSubmissionOrchestration(db, nil)
	if err != nil {
		t.Fatalf("装配提交编排：%v", err)
	}

	// 跨客户指名按编排规则不查库直接答`查无原委托`——它发生在 Preserve 之后，正好
	// 钉住「业务拒绝不回滚保全」这一格。
	command := submissionCommand(t)
	command.Link = shipmentapp.PriorRequestClaim{
		PriorIdentity:  otherIdentity(t),
		PriorRequestID: mustValue(t, domain.NewShipmentRequestID, "request-absent"),
		Kind:           domain.LinkWithdrawnResubmission,
	}
	blocked, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("指名跨客户原委托的提交：%v", err)
	}
	if got := blocked.Outcome(); got != shipmentapp.OutcomePriorRequestNotFound {
		t.Fatalf("outcome = %v, want PRIOR_REQUEST_NOT_FOUND", got)
	}

	replay, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放被拒提交：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.OutcomeExistingResult {
		t.Fatalf("outcome = %v, want EXISTING_RESULT——被拒的提交也保全了来源", got)
	}
}

// Covers: ADR-0081 出站半边的装配证据——本包的生产边界壳 submissionBoundary 携真实
// Outbox 意图适配器：建单成功即落下恰好一份「委托已提交」信封，重放同一份输入答
// `已有结果`且不入队第二份。原子性与信封内容由 `tests/bentocontract` 的 PBC-03/04/05
// 取证，但那边跑的是夹具副本；这里钉的是 cmd/parcel-api 自己的壳真的把信封接上了——
// 谁改坏本包的 submissionBoundary，夹具那边不会红，这里会。
//
// 归属权威用放行替身（隔离合成 `S`，不进生产装配）：真实治理桥在目录未配置时把提交
// 停在 OWNERSHIP_UNRESOLVED，建单一段走不到，而要取证的恰是建单一段。除这一读口外，
// 仓储、边界壳、Outbox Store、意图适配器与标识工厂全是生产实现。
func TestASubmittedRequestHandsOffExactlyOneEnvelope(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submission, _ := envelopeMintingSubmission(t, db)

	command := submissionCommand(t)
	submitted, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("首次提交：%v", err)
	}
	if got := submitted.Outcome(); got != shipmentapp.OutcomeSubmitted {
		t.Fatalf("outcome = %v, want SUBMITTED——归属替身已放行，建单该成立", got)
	}
	if got := submittedEnvelopeCount(t, pool); got != 1 {
		t.Fatalf("建单后「委托已提交」信封 = %d 份, want 恰好 1", got)
	}

	replay, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放同一份输入：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.OutcomeExistingResult {
		t.Fatalf("outcome = %v, want EXISTING_RESULT", got)
	}
	if got := submittedEnvelopeCount(t, pool); got != 1 {
		t.Fatalf("重放后信封 = %d 份——重放不得入队第二份意图", got)
	}
}

// Covers: 发布侧铸出的那一封，生产消费门真的译得出（ADR-0081 把这条缝从死接到活：
// 在此之前发布侧零生产调用方，两边对不上也不会有任何一封信真的走过这条路）。
//
// 两半各写各的载荷标签——消费门明写不导入发布侧的未导出结构，免得消费方依赖提供方的
// 内部形状。代价是**字段名漂开时的症状是静默的**：译不出即毒丸，消费门显式拒收入账
// 并交回 nil，那一封于是被记成发布成功，而接受判断链一次都没跑过。它与「实例半边还
// 没配置」在库里长着同一张脸——两边都是链没往前走，都不报错。
//
// 既有用例守的是各自那一侧对自己抄本的忠诚，不是两侧彼此对得上：发布侧由 PBC-05 的
// 镜像结构守，消费侧由 `adapters/inbox` 的字面量守，`cmd/parcel-dispatch` 那份同样是
// 手抄。**改发布侧标签时顺手改它自己的镜像**是最自然的一步，而那一步之后没有任何东西
// 会红——实测于 `46dbb90`：把 `submissionVersionId` 连同 PBC-05 镜像一起改名（消费侧
// 不动），上述四个包全绿，只有本用例红。它走的是真字节：建单落下的那一封按派发一拍
// 的同一条认领路径取回来，喂给生产消费门。
func TestTheMintedEnvelopeDecodesIntoTheAcceptanceChainCommand(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submission, store := envelopeMintingSubmission(t, db)

	command := submissionCommand(t)
	submitted, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("首次提交：%v", err)
	}
	if got := submitted.Outcome(); got != shipmentapp.OutcomeSubmitted {
		t.Fatalf("outcome = %v, want SUBMITTED", got)
	}

	envelope := claimSubmittedEnvelope(t, store)

	inboxStore, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	advancer := &recordingAdvancer{err: errAdvancerProbe}
	consumer, err := psinbox.NewShipmentRequestSubmittedConsumer(db.Transactor(), inboxStore, advancer)
	if err != nil {
		t.Fatalf("构造接受链消费门：%v", err)
	}

	// 探针错误让编排那一层不必真装起来，同时把「译到了」与「毒丸」分成两个可观察结果：
	// 毒丸时消费门交回 nil，probe 时它把错误原样上抛。
	err = consumer.Consume(t.Context(), envelope)
	if errors.Is(err, psinbox.ErrPoisonEnvelope) || err == nil {
		t.Fatalf("生产消费门译不出生产发布侧铸的信封：err = %v；两侧载荷字段名已漂开", err)
	}
	if !errors.Is(err, errAdvancerProbe) {
		t.Fatalf("err = %v, want 探针错误——译码之后该走到编排", err)
	}
	if len(advancer.commands) != 1 {
		t.Fatalf("推进次数 = %d, want 1", len(advancer.commands))
	}

	// 译得出还不够：四维互换也照样译得出，而错位的租户维会让接受链去问另一个租户的
	// 权威，答出来的`无适用依据`与本租户没登记一模一样。逐维对回提交时那一份。
	got := advancer.commands[0]
	if got.Identity != command.Identity {
		t.Fatalf("来源身份 = %+v, want %+v", got.Identity, command.Identity)
	}
	if got.ShipmentRequestID != command.ShipmentRequestID {
		t.Fatalf("委托 = %q, want %q", got.ShipmentRequestID, command.ShipmentRequestID)
	}
	if len(got.DeclaredParcelIDs) != len(command.DeclaredParcelIDs) {
		t.Fatalf("声明成员 = %v, want %v", got.DeclaredParcelIDs, command.DeclaredParcelIDs)
	}
	for index, parcel := range command.DeclaredParcelIDs {
		if got.DeclaredParcelIDs[index] != parcel {
			t.Fatalf("声明成员[%d] = %q, want %q", index, got.DeclaredParcelIDs[index], parcel)
		}
	}
	// 提交版本由标识工厂现签，命令里没有可对的原件；译得出非空即证这一维没漂——
	// 标签对不上时它是空串，而空串过不了 NewSubmissionVersionID，上面那格就已经红了。
	if got.SubmissionVersion.String() == "" {
		t.Fatal("提交版本为空——发布侧没写或消费侧没译")
	}
}

// Covers: ADR-0091 决定二——隔离形态接上合成目录之后，归属**真的读了**治理登记册：
// 册里有一条匹配的合成权威区间时，提交越过 OWNERSHIP_UNRESOLVED 直到建单成立。
//
// 期望修订不由本用例照着 revisionFor 再算一遍——那样断言会按构造成立，改坏派生规则
// 它也跟着变。改为走调用方真实拿得到的那条路：先提交一次拿回归属决定，从**决定自己**
// 读出修订，再以另一份来源身份提交。这也顺带钉住了被拦的提交确实带回了续办所需的
// 决定（`UC-PS-001` 结果语义要求归属未决要给得出续办引用）。
func TestIsolatedSubmissionCrossesOwnershipOnceTheRegisterHasAMatchingInterval(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	appendIsolatedAuthorityInterval(t, db)

	submission, err := buildSubmissionOrchestration(db, isolatedWriteAdmissionForTest(t))
	if err != nil {
		t.Fatalf("装配隔离形态提交编排：%v", err)
	}

	probe, err := submission.Handle(t.Context(), submissionCommand(t))
	if err != nil {
		t.Fatalf("首次提交：%v", err)
	}
	decision, has := probe.OwnershipDecision()
	if !has {
		t.Fatal("被拦的提交没带归属决定——调用方无从知道该带哪个修订回来")
	}

	command := submissionCommand(t)
	command.Identity = secondIdentity(t)
	command.ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "syn-request-2")
	command.ExpectedRevision = decision.Revision()

	admitted, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("带着决定给出的修订再提交：%v", err)
	}
	if got := admitted.Outcome(); got != shipmentapp.OutcomeSubmitted {
		t.Fatalf("outcome = %v, want SUBMITTED——合成目录已配、登记册有匹配区间，归属该确定；门禁拦截理由 = %v",
			got, admitted.GateBlockReasons())
	}
}

// Covers: 票 02「必须守住的一格」——答案的来源变了，结论不许被抄近路。合成目录只交
// 坐标，登记册为空时归属照旧答`权威未确定`。这条是「假目录」的反证：一个直接返回
// 「已确定」的目录会让本用例变绿，而它在库里与真实放行不可分辨。
func TestIsolatedSubmissionStillAnswersUnresolvedOnAnEmptyRegister(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	submission, err := buildSubmissionOrchestration(db, isolatedWriteAdmissionForTest(t))
	if err != nil {
		t.Fatalf("装配隔离形态提交编排：%v", err)
	}

	result, err := submission.Handle(t.Context(), submissionCommand(t))
	if err != nil {
		t.Fatalf("空册上提交：%v", err)
	}
	if got := result.Outcome(); got != shipmentapp.OutcomeOwnershipUnresolved {
		t.Fatalf("outcome = %v, want OWNERSHIP_UNRESOLVED——空册上合成目录不得替登记册作答", got)
	}
	decision, has := result.OwnershipDecision()
	if !has {
		t.Fatal("空册上的`权威未确定`没带决定")
	}
	if decision.Authority() != domain.ProductionAuthorityUnresolved {
		t.Fatalf("authority = %s, want UNRESOLVED", decision.Authority())
	}
}

// isolatedWriteAdmissionForTest 走的是进程自己那道门（buildIsolatedWriteAdmission），
// 不在测试里另拼一个注入值：要证的正是装配点交出来的那一组坐标与登记的那一行对得上，
// 自己拼等于把被测的那半换成手抄本。
func isolatedWriteAdmissionForTest(t *testing.T) *isolatedWriteAdmission {
	t.Helper()
	admission, err := buildIsolatedWriteAdmission(fakeGetenv(map[string]string{
		isolatedWriteTenantEnv: "SYN-TENANT-01",
	}))
	if err != nil {
		t.Fatalf("解析隔离写路径准入：%v", err)
	}
	if admission == nil {
		t.Fatal("隔离写路径准入未启用")
	}
	return admission
}

// appendIsolatedAuthorityInterval 往治理登记册里放一条匹配的合成权威区间。四维取装配点
// 那组常量而不是字面量重抄一遍：常量与登记行对不上的后果不是报错，是查不到那一行。
func appendIsolatedAuthorityInterval(t *testing.T, db *bentopg.DB) {
	t.Helper()
	intervals, err := pgpostgres.NewAuthorityIntervals(db)
	if err != nil {
		t.Fatalf("构造权威区间仓储：%v", err)
	}
	interval := pgdomain.AuthorityInterval{
		ObjectScope: isolatedGovernanceObjectScope,
		Capability:  isolatedGovernanceCapability,
		FactKind:    isolatedGovernanceFactKind,
		Authority:   isolatedSelfAuthority,
		From:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return intervals.Append(txCtx, interval)
	})
	if err != nil {
		t.Fatalf("登记合成权威区间：%v", err)
	}
}

// secondIdentity 是同一客户的第二份来源身份：换身份是为了绕开重放判重，不是为了换客户。
func secondIdentity(t *testing.T) domain.SourceIdentity {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
		mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-1"),
		mustValue(t, domain.NewSource, "SYN-SOURCE-A"),
		mustValue(t, domain.NewSourceRequestKey, "SYN-KEY-3"),
	)
	if err != nil {
		t.Fatalf("new second source identity: %v", err)
	}
	return identity
}

// envelopeMintingSubmission 装配「会真发信封」的提交编排：仓储、边界壳、Outbox Store、
// 意图适配器与标识工厂全是生产实现，只有归属权威换成放行替身（隔离合成 `S`，不进生产
// 装配）——真实治理桥在目录未配置时把提交停在 OWNERSHIP_UNRESOLVED，建单一段走不到，
// 而这两个用例要取证的恰是建单一段。
func envelopeMintingSubmission(
	t *testing.T,
	db *bentopg.DB,
) (*shipmentapp.SubmitShipmentRequestHandler, *outbox.Store) {
	t.Helper()

	sources, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		t.Fatalf("构造来源保全仓储：%v", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	clock := fixedClock{at: envelopeProofAnchor}
	handoff, err := pspostgres.NewOutboxShipmentRequestSubmittedHandoff(db, store, clock)
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	identities, err := psidentity.NewSubmissionIdentities()
	if err != nil {
		t.Fatalf("构造标识工厂：%v", err)
	}
	return shipmentapp.NewSubmitShipmentRequestHandler(
		preservationBoundary{transactor: db.Transactor(), inner: sources},
		submissionBoundary{transactor: db.Transactor(), inner: requests, handoff: handoff},
		permittingOwnership{anchor: envelopeProofAnchor},
		pshandoff.UnconfiguredOtherProductionAuthorityChannel{},
		identities,
		clock,
	), store
}

// claimSubmittedEnvelope 按派发一拍的同一条认领路径把信封取回来。走 Claim 而不是自己
// 拼一个 Envelope：要证的正是「库里那一封」，自己拼等于把发布侧那半换成手抄本。
func claimSubmittedEnvelope(t *testing.T, store *outbox.Store) eventing.Envelope {
	t.Helper()

	deliveries, err := store.Claim(t.Context(), eventing.OutboxClaim{
		Now:         envelopeProofAnchor,
		Limit:       10,
		LeaseFor:    time.Minute,
		MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("认领待发信封：%v", err)
	}
	for _, delivery := range deliveries {
		if string(delivery.Envelope.Type) == submittedEnvelopeType {
			return delivery.Envelope
		}
	}
	t.Fatalf("认领到 %d 封，其中没有「委托已提交」", len(deliveries))
	return eventing.Envelope{}
}

// errAdvancerProbe 让接受链编排那一层不必在本包真装起来：本用例问的是「载荷译成了
// 什么」，不是「链会答什么」。
var errAdvancerProbe = errors.New("advancer probe")

type recordingAdvancer struct {
	commands []shipmentapp.AdvanceAcceptanceChainCommand
	err      error
}

func (advancer *recordingAdvancer) Handle(
	_ context.Context,
	command shipmentapp.AdvanceAcceptanceChainCommand,
) (shipmentapp.AdvanceAcceptanceChainResult, error) {
	advancer.commands = append(advancer.commands, command)
	return shipmentapp.AdvanceAcceptanceChainResult{}, advancer.err
}

// envelopeProofAnchor 是信封取证的固定时钟读数：门禁评估时刻必须落在归属替身声明的
// 有效区间内，真实时钟会让这层证据随运行时刻漂移。
var envelopeProofAnchor = time.Date(2026, 8, 21, 10, 0, 5, 0, time.UTC)

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// permittingOwnership 是放行的归属权威替身（只记 `S`，不进生产装配）：对任何拟受理
// 范围答「本产品即当前生产权威、准入开放」。修订与 submissionCommand 的期望修订同值、
// 有效区间罩住 envelopeProofAnchor，门禁因此确定性放行。
type permittingOwnership struct{ anchor time.Time }

func (authority permittingOwnership) DecideProductionOwnership(
	_ context.Context,
	scope domain.AdmissionScope,
) (domain.ProductionOwnershipDecision, error) {
	interval, err := domain.NewOwnershipValidityInterval(
		authority.anchor.Add(-time.Hour),
		authority.anchor.Add(24*time.Hour),
	)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	decisionID, err := domain.NewProductionOwnershipDecisionID("SYN-OWN-DEC-1")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	ruleVersion, err := domain.NewProductionOwnershipRuleVersion("SYN-OWN-RULE-1")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	revision, err := domain.NewProductionOwnershipRevision("syn-rev-1")
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	return domain.NewProductionOwnershipDecision(domain.ProductionOwnershipDecisionSpec{
		DecisionID:       decisionID,
		Scope:            scope,
		Authority:        domain.ProductionAuthorityIDPParcel,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      ruleVersion,
		AsOf:             authority.anchor.Add(-time.Minute),
		Validity:         interval,
		Revision:         revision,
		DecisionAt:       authority.anchor.Add(-time.Minute),
	})
}

// submittedEnvelopeType 与意图适配器的类型常量同字面（`tests/bentocontract` 同款拍法）：
// 按类型统计而不只按 EventID，一个意外身份的第二份信封才逃不掉。
const submittedEnvelopeType = "parcel-shipment.shipment-request.submitted"

func submittedEnvelopeCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		submittedEnvelopeType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计信封行数：%v", err)
	}
	return count
}

func submissionCommand(t *testing.T) shipmentapp.SubmitShipmentRequestCommand {
	t.Helper()
	scope, err := domain.NewAdmissionScope(
		mustValue(t, domain.NewAdmissionScopeReference, "scope-ref-syn-1"),
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-syn-1"),
	)
	if err != nil {
		t.Fatalf("new admission scope: %v", err)
	}
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
		mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-1"),
		mustValue(t, domain.NewSource, "SYN-SOURCE-A"),
		mustValue(t, domain.NewSourceRequestKey, "SYN-KEY-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return shipmentapp.SubmitShipmentRequestCommand{
		Identity:          identity,
		PayloadDigest:     mustValue(t, domain.NewPayloadDigest, "syn-digest-1"),
		OccurredAt:        time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC),
		ReceivedAt:        time.Date(2026, 8, 21, 10, 0, 1, 0, time.UTC),
		BatchID:           mustValue(t, domain.NewSubmissionBatchID, "syn-batch-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "syn-request-1"),
		DeclaredParcelIDs: []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "syn-parcel-1")},
		AdmissionScope:    scope,
		ExpectedRevision:  mustValue(t, domain.NewProductionOwnershipRevision, "syn-rev-1"),
	}
}

// otherIdentity 是另一位客户的来源身份：跨客户指名按编排规则不查库直接答`查无原委托`。
func otherIdentity(t *testing.T) domain.SourceIdentity {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
		mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-2"),
		mustValue(t, domain.NewSource, "SYN-SOURCE-A"),
		mustValue(t, domain.NewSourceRequestKey, "SYN-KEY-2"),
	)
	if err != nil {
		t.Fatalf("new other source identity: %v", err)
	}
	return identity
}

func mustValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %T from %q: %v", value, raw, err)
	}
	return value
}
