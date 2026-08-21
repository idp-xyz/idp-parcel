package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psnodeops "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件验的是组合根这一道缝：intakeQualificationEvidence 交回的证据口，装配点交什么
// 实例配置就答什么。适配器本体的行为由 psnodeops 自己的用例守，这里只证「线接通了」与
// 「今天认领的是空」——两者在判断结果上都答未证明，光看结果分不出，所以得分别钉住。
//
// 用真库而不是替身：这道缝的下半截是 nopostgres.ExecutionFacts，替身换掉它就绕开了本
// 文件唯一要证的那一段——生产仓储真的插得进适配器的窄口并读得回事实。

// 前缀与引用都是实例取值：生产装配一段不认领、一条不登记，这里给值只为让缝走得到底。
const (
	wiringAuthorityPrefix = "NODE-OPS"
	wiringPresentRule     = wiringAuthorityPrefix + "/customs-presentation"
	wiringOutsideRule     = "INTAKE-QUAL/customs-precheck"
)

// wiringIntakeAt 是收寄业务时点。执行事实取更早的时刻——晚于收寄的执行证不了收寄当时
// 已满足，那一格由 psnodeops 的用例守，这里不要撞上它。
var wiringIntakeAt = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

// Covers: 票 10 红线「nil 与显式未配置都不得默认 ESTABLISHED」在装配这一层的落点——
// 生产装配交零值，交回的是诚实未配置口，段内段外一律答未证明。
//
// db 传 nil 是断言的一部分而不是省事：认领了才会去建 nopostgres.ExecutionFacts，而那个
// 构造器拒 nil。这一条因此同时证明了零值分支根本没有碰库。
func TestTheProductionAssemblyClaimsNoIntakeQualificationAuthority(t *testing.T) {
	evidence, err := intakeQualificationEvidence(nil, nodeQualificationAuthority{})
	if err != nil {
		t.Fatalf("未认领任何权威段却装不起来：%v", err)
	}
	if _, ok := evidence.(pspartycommercial.UnconfiguredIntakeQualificationEvidence); !ok {
		t.Fatalf("evidence 类型 = %T，want UnconfiguredIntakeQualificationEvidence", evidence)
	}
	for _, rule := range []string{wiringPresentRule, wiringOutsideRule} {
		if proof := wiringProve(t, evidence, rule); proof != psports.IntakeQualificationUnproven {
			t.Fatalf("%s 的 proof = %q, want UNPROVEN", rule, proof)
		}
	}
}

// Covers: ADR-0063 第二条的消费侧权威口真的接到了生产仓储上——同一个证据口，库里没有
// 那件执行事实时答未证明，事实落库后答已证明。
//
// 前后两问是一条用例而不是两条：只断言「有事实就已证明」证不出答案跟着库走，零值分支
// 也可能因为别的原因恰好答对；只断言「没事实就未证明」则与未配置分不开。
func TestAClaimedAuthorityAnswersFromRealNodeExecutionFacts(t *testing.T) {
	db := wiringDB(t)
	evidence, err := intakeQualificationEvidence(db, nodeQualificationAuthority{
		prefix:     wiringAuthorityPrefix,
		qualifying: wiringRegistry(t),
	})
	if err != nil {
		t.Fatalf("认领权威段后装不起来：%v", err)
	}

	if proof := wiringProve(t, evidence, wiringPresentRule); proof != psports.IntakeQualificationUnproven {
		t.Fatalf("事实未登记时 proof = %q, want UNPROVEN", proof)
	}

	seedPresentedExecutionFact(t, db)

	if proof := wiringProve(t, evidence, wiringPresentRule); proof != psports.IntakeQualificationProven {
		t.Fatalf("事实已登记时 proof = %q, want PROVEN——组合口没把段内引用路由到节点权威口", proof)
	}
}

// Covers: ADR-0063「未知前缀答未证明，不得已证明」在登记键这一维——认领 NODE-OPS 不等
// 于认领 INTAKE-QUAL。装配若把节点口登成通配，段外那条引用就会拿一件呈验事实兑出关务
// 已预检，那正是 Consequences 点名禁的假关务身份。
func TestAClaimedAuthorityDoesNotProveAnotherAuthoritySegment(t *testing.T) {
	db := wiringDB(t)
	evidence, err := intakeQualificationEvidence(db, nodeQualificationAuthority{
		prefix:     wiringAuthorityPrefix,
		qualifying: wiringRegistry(t),
	})
	if err != nil {
		t.Fatalf("认领权威段后装不起来：%v", err)
	}
	seedPresentedExecutionFact(t, db)

	if proof := wiringProve(t, evidence, wiringOutsideRule); proof != psports.IntakeQualificationUnproven {
		t.Fatalf("段外引用的 proof = %q, want UNPROVEN", proof)
	}
}

// Covers: 前缀与登记表要一起才立得住。填了表却没认领段是配置写了一半，放过去会得到一
// 张永远问不到的表——判断结果与真的没配置一模一样，而要人做的事相反（一个是去补前缀，
// 一个是本来就该留白）。
func TestAQualificationRegistryWithoutAClaimedAuthorityIsRefused(t *testing.T) {
	evidence, err := intakeQualificationEvidence(nil, nodeQualificationAuthority{
		qualifying: wiringRegistry(t),
	})
	if err == nil {
		t.Fatalf("填了登记表却没认领前缀，装配仍然成立：%T", evidence)
	}
}

func wiringDB(t *testing.T) *bentopg.DB {
	t.Helper()

	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db
}

// wiringRegistry 把 wiringPresentRule 登记到「item-1 上的呈验」。生产装配给空表。
func wiringRegistry(t *testing.T) map[string]psnodeops.QualifyingNodeExecution {
	t.Helper()
	return map[string]psnodeops.QualifyingNodeExecution{
		wiringPresentRule: {
			Item:   wiringValue(t, nodomain.NewCollaborationItemReference, "item-1"),
			Action: nodomain.PresentAction,
		},
	}
}

// seedPresentedExecutionFact 经领域造一件「已呈验」的执行事实并落库。不手搓行塞表：
// RecordExecutionFact 会拦掉越出承接范围的实物与动作，绕开它测到的就不是生产读得回的
// 那种事实。
func seedPresentedExecutionFact(t *testing.T, db *bentopg.DB) {
	t.Helper()

	acceptance, err := nodomain.DecideCollaborationAcceptance(nodomain.CollaborationAcceptanceSpec{
		TenantID:        wiringValue(t, nodomain.NewTenantID, "tenant-1"),
		Node:            wiringValue(t, nodomain.NewNodeReference, "node-origin"),
		Item:            wiringValue(t, nodomain.NewCollaborationItemReference, "item-1"),
		Decision:        nodomain.CollaborationAccepted,
		AcceptedUnits:   []nodomain.HandlingUnitID{wiringValue(t, nodomain.NewHandlingUnitID, "unit-1")},
		AcceptedActions: []nodomain.CollaborationActionKind{nodomain.PresentAction},
		Authority:       wiringValue(t, nodomain.NewAcceptanceAuthorityReference, "authority-1"),
		DecidedAt:       wiringIntakeAt.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造承接以登记事实：%v", err)
	}
	performedAt := wiringIntakeAt.Add(-time.Hour)
	fact, err := nodomain.RecordExecutionFact(
		acceptance,
		wiringValue(t, nodomain.NewHandlingUnitID, "unit-1"),
		nodomain.PresentAction,
		wiringValue(t, nodomain.NewExecutionEvidenceReference, "evidence-1"),
		performedAt,
	)
	if err != nil {
		t.Fatalf("构造执行事实：%v", err)
	}

	facts, err := nopostgres.NewExecutionFacts(db)
	if err != nil {
		t.Fatalf("构造执行事实库：%v", err)
	}
	record := noports.ExecutionFactRecord{
		Key: noports.ExecutionFactKey{
			TenantID: acceptance.TenantID(),
			Item:     acceptance.Item(),
			Unit:     fact.Unit(),
			Action:   nodomain.PresentAction,
		},
		ContentDigest: "digest-evidence-1",
		Fact:          fact,
		RecordedAt:    performedAt,
	}
	var outcome noports.ExecutionFactSaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, err := facts.Save(txCtx, record)
		outcome = saved
		return err
	}); err != nil {
		t.Fatalf("保存执行事实：%v", err)
	}
	if outcome != noports.ExecutionFactSaved {
		t.Fatalf("保存执行事实结果 = %d，应为 SAVED——同键已有记录时读回的是先到者，不是本用例种的那件", outcome)
	}
}

// wiringProve 按端口的四件问一次：身份、来源、引用、收寄业务时点。
func wiringProve(
	t *testing.T,
	evidence psports.IntakeQualificationEvidenceView,
	rule string,
) psports.IntakeQualificationProof {
	t.Helper()

	source := wiringNodeIntakeSource(t)
	proof, err := evidence.ProveIntakeQualification(
		t.Context(),
		wiringIdentity(t),
		source,
		wiringValue(t, psdomain.NewQualificationRuleReference, rule),
		source.OccurredAt(),
	)
	if err != nil {
		t.Fatalf("取证 %q：%v", rule, err)
	}
	return proof
}

// wiringIdentity 的租户与 seedPresentedExecutionFact 的租户同名：取证键的租户维由
// SourceIdentity 译过去，两边不同名就永远取不回事实，而那会表现成一个看不出原因的未证明。
func wiringIdentity(t *testing.T) psdomain.SourceIdentity {
	t.Helper()

	identity, err := psdomain.NewSourceIdentity(
		wiringValue(t, psdomain.NewTenantID, "tenant-1"),
		wiringValue(t, psdomain.NewCustomerAccountID, "customer-1"),
		wiringValue(t, psdomain.NewSource, "source-a"),
		wiringValue(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("构造来源身份：%v", err)
	}
	return identity
}

// wiringNodeIntakeSource 的来源对象与承接里的作业实物同名，理由同租户维。
func wiringNodeIntakeSource(t *testing.T) psdomain.IntakeSource {
	t.Helper()

	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind:       psdomain.NodeIntakeSource,
		Object:     wiringValue(t, psdomain.NewSourceObjectReference, "unit-1"),
		Parcel:     wiringValue(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Place:      wiringValue(t, psdomain.NewIntakePlaceReference, "node-origin"),
		Control:    wiringValue(t, psdomain.NewIntakeControlReference, "NODE-INTAKE/SIGN-7"),
		Version:    wiringValue(t, psdomain.NewSourceResultVersion, "intake-result/v1"),
		OccurredAt: wiringIntakeAt,
	})
	if err != nil {
		t.Fatalf("构造收寄来源：%v", err)
	}
	return source
}

func wiringValue[T interface{ String() string }](
	t *testing.T,
	construct func(string) (T, error),
	raw string,
) T {
	t.Helper()

	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}
