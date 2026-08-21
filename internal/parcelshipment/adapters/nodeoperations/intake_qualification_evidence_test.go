package nodeoperations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 生产装配用的真实仓储必须能插进本适配器的窄口——B 票要接的就是这一根线，编译期钉住
// 比等装配时才发现形状不合要早。
var _ adapter.ExecutionFactLookup = (*nopostgres.ExecutionFacts)(nil)

// 前缀与引用都是实例取值：生产装配不认领任何段也不登记任何项，这里给值只为验机制。
const (
	nodeAuthorityPrefix  = "NODE-OPS"
	presentQualification = nodeAuthorityPrefix + "/customs-presentation"
	customsQualification = "INTAKE-QUAL/customs-precheck"
)

type executionFactLookupDouble struct {
	record noports.ExecutionFactRecord
	found  bool
	err    error
	keys   []noports.ExecutionFactKey
}

func (double *executionFactLookupDouble) FindByKey(
	_ context.Context,
	key noports.ExecutionFactKey,
) (noports.ExecutionFactRecord, bool, error) {
	double.keys = append(double.keys, key)
	if double.err != nil {
		return noports.ExecutionFactRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

// presentedFact 造一件「已呈验」的节点执行事实，发生于 performedAt。
func presentedFact(t *testing.T, performedAt time.Time) noports.ExecutionFactRecord {
	t.Helper()
	acceptance, err := nodomain.DecideCollaborationAcceptance(nodomain.CollaborationAcceptanceSpec{
		TenantID:        value(t, nodomain.NewTenantID, "tenant-1"),
		Node:            value(t, nodomain.NewNodeReference, "node-origin"),
		Item:            value(t, nodomain.NewCollaborationItemReference, "item-1"),
		Decision:        nodomain.CollaborationAccepted,
		AcceptedUnits:   []nodomain.HandlingUnitID{value(t, nodomain.NewHandlingUnitID, "unit-1")},
		AcceptedActions: []nodomain.CollaborationActionKind{nodomain.PresentAction},
		Authority:       value(t, nodomain.NewAcceptanceAuthorityReference, "authority-1"),
		DecidedAt:       performedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("decide collaboration acceptance: %v", err)
	}
	fact, err := nodomain.RecordExecutionFact(
		acceptance,
		value(t, nodomain.NewHandlingUnitID, "unit-1"),
		nodomain.PresentAction,
		value(t, nodomain.NewExecutionEvidenceReference, "evidence-1"),
		performedAt,
	)
	if err != nil {
		t.Fatalf("record execution fact: %v", err)
	}
	return noports.ExecutionFactRecord{
		Key: noports.ExecutionFactKey{
			TenantID: value(t, nodomain.NewTenantID, "tenant-1"),
			Item:     value(t, nodomain.NewCollaborationItemReference, "item-1"),
			Unit:     value(t, nodomain.NewHandlingUnitID, "unit-1"),
			Action:   nodomain.PresentAction,
		},
		ContentDigest: "digest-1",
		Fact:          fact,
		RecordedAt:    performedAt,
	}
}

// presentationRegistry 是实例半边那张表的测试取值：把一条已声明的资格引用登记到
// 「item-1 上的呈验」。生产装配给空表——这里给值只为验机制。
func presentationRegistry(t *testing.T) map[string]adapter.QualifyingNodeExecution {
	t.Helper()
	return map[string]adapter.QualifyingNodeExecution{
		presentQualification: {
			Item:   value(t, nodomain.NewCollaborationItemReference, "item-1"),
			Action: nodomain.PresentAction,
		},
	}
}

func nodeIntakeSource(t *testing.T) psdomain.IntakeSource {
	t.Helper()
	return intakeSourceOfKind(t, psdomain.NodeIntakeSource)
}

func intakeSourceOfKind(t *testing.T, kind psdomain.IntakeSourceKind) psdomain.IntakeSource {
	t.Helper()
	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind:       kind,
		Object:     value(t, psdomain.NewSourceObjectReference, "unit-1"),
		Parcel:     value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Place:      value(t, psdomain.NewIntakePlaceReference, "node-origin"),
		Control:    value(t, psdomain.NewIntakeControlReference, "NODE-INTAKE/SIGN-7"),
		Version:    value(t, psdomain.NewSourceResultVersion, "intake-result/v1"),
		OccurredAt: receivedAt,
	})
	if err != nil {
		t.Fatalf("new intake source: %v", err)
	}
	return source
}

func qualificationRule(t *testing.T, raw string) psdomain.QualificationRuleReference {
	t.Helper()
	return value(t, psdomain.NewQualificationRuleReference, raw)
}

func evidenceOver(
	t *testing.T,
	facts adapter.ExecutionFactLookup,
	qualifying map[string]adapter.QualifyingNodeExecution,
) adapter.NodeExecutionQualificationEvidence {
	t.Helper()
	view, err := adapter.NewNodeExecutionQualificationEvidence(facts, nodeAuthorityPrefix, qualifying)
	if err != nil {
		t.Fatalf("new node execution qualification evidence: %v", err)
	}
	return view
}

// prove 走一遍取证口，参数按端口的四件：身份、来源、引用、收寄业务时点。
func prove(
	t *testing.T,
	view adapter.NodeExecutionQualificationEvidence,
	source psdomain.IntakeSource,
	rule string,
) (psports.IntakeQualificationProof, error) {
	t.Helper()
	return view.ProveIntakeQualification(
		context.Background(), identity(t), source, qualificationRule(t, rule), source.OccurredAt())
}

// Covers: ADR-0063 第二条「权威方实现落在消费侧适配器」——节点已登记的执行事实证得了
// 挂在节点权威段上的那条硬资格，问的键是（租户+事项+实物+动作）。
func TestARegisteredNodeExecutionProvesTheDeclaredQualification(t *testing.T) {
	facts := &executionFactLookupDouble{record: presentedFact(t, receivedAt.Add(-time.Hour)), found: true}
	view := evidenceOver(t, facts, presentationRegistry(t))

	proof, err := prove(t, view, nodeIntakeSource(t), presentQualification)
	if err != nil {
		t.Fatalf("prove intake qualification: %v", err)
	}
	if proof != psports.IntakeQualificationProven {
		t.Fatalf("proof = %q, want PROVEN", proof)
	}
	if len(facts.keys) != 1 {
		t.Fatalf("lookups = %d, want 1", len(facts.keys))
	}
	asked := facts.keys[0]
	if asked.TenantID.String() != "tenant-1" || asked.Item.String() != "item-1" ||
		asked.Unit.String() != "unit-1" || asked.Action != nodomain.PresentAction {
		t.Fatalf("key = %+v; 四维必须逐维译到位", asked)
	}
}

// Covers: ADR-0063 第四条与票 10 红线「不得默认 ESTABLISHED」——生产装配的空登记表是
// 首发唯一走得到的分支，它答未证明且根本不问 node-operations（未登记不是依赖故障）。
func TestAnUnregisteredReferenceAnswersUnprovenWithoutAskingNodeOperations(t *testing.T) {
	facts := &executionFactLookupDouble{record: presentedFact(t, receivedAt.Add(-time.Hour)), found: true}

	for name, registry := range map[string]map[string]adapter.QualifyingNodeExecution{
		"生产装配的空表": nil,
		"登记了别的引用": presentationRegistry(t),
	} {
		t.Run(name, func(t *testing.T) {
			facts.keys = nil
			view := evidenceOver(t, facts, registry)

			proof, err := prove(t, view, nodeIntakeSource(t), customsQualification)
			if err != nil {
				t.Fatalf("prove intake qualification: %v", err)
			}
			if proof != psports.IntakeQualificationUnproven {
				t.Fatalf("proof = %q, want UNPROVEN；未登记引用不得已证明", proof)
			}
			if len(facts.keys) != 0 {
				t.Fatalf("lookups = %d; 未登记引用不得去问节点", len(facts.keys))
			}
		})
	}
}

// Covers: ADR-0063「未证明」格——声明已在、证明未到。事实还没登记就是未证明，等它到达
// 后重试同一拍，不是依赖不可用。
func TestAMissingExecutionFactAnswersUnproven(t *testing.T) {
	view := evidenceOver(t, &executionFactLookupDouble{found: false}, presentationRegistry(t))

	proof, err := prove(t, view, nodeIntakeSource(t), presentQualification)
	if err != nil {
		t.Fatalf("prove intake qualification: %v", err)
	}
	if proof != psports.IntakeQualificationUnproven {
		t.Fatalf("proof = %q, want UNPROVEN", proof)
	}
}

// Covers: ADR-0063 第二条「时点取自被采用来源的实际发生时间」——证据的有效性按收寄业务
// 时点判：收寄之后才做的执行证不了收寄当时已满足，边界上同一时刻算已满足。
func TestEvidenceIsValidOnlyIfItPrecedesTheIntakeBusinessTime(t *testing.T) {
	for name, performedAt := range map[string]time.Time{
		"收寄之前":   receivedAt.Add(-time.Second),
		"恰在收寄时点": receivedAt,
	} {
		t.Run(name+"·已证明", func(t *testing.T) {
			view := evidenceOver(t,
				&executionFactLookupDouble{record: presentedFact(t, performedAt), found: true},
				presentationRegistry(t))

			proof, err := prove(t, view, nodeIntakeSource(t), presentQualification)
			if err != nil {
				t.Fatalf("prove intake qualification: %v", err)
			}
			if proof != psports.IntakeQualificationProven {
				t.Fatalf("proof = %q, want PROVEN", proof)
			}
		})
	}

	t.Run("收寄之后·未证明", func(t *testing.T) {
		view := evidenceOver(t,
			&executionFactLookupDouble{record: presentedFact(t, receivedAt.Add(time.Second)), found: true},
			presentationRegistry(t))

		proof, err := prove(t, view, nodeIntakeSource(t), presentQualification)
		if err != nil {
			t.Fatalf("prove intake qualification: %v", err)
		}
		if proof != psports.IntakeQualificationUnproven {
			t.Fatalf("proof = %q, want UNPROVEN；晚于收寄的执行证不了收寄当时", proof)
		}
	})
}

// Covers: ADR-0063 第三条「依赖不可用走 error，不并进未证明」——两格恢复动作不同：等依赖
// 恢复 vs 等证据到达。
func TestALookupFailureSurfacesAsErrorInsteadOfUnproven(t *testing.T) {
	unavailable := errors.New("节点执行事实库不可读")
	view := evidenceOver(t, &executionFactLookupDouble{err: unavailable}, presentationRegistry(t))

	proof, err := prove(t, view, nodeIntakeSource(t), presentQualification)
	if !errors.Is(err, unavailable) {
		t.Fatalf("err = %v, want wrapped lookup failure", err)
	}
	if proof == psports.IntakeQualificationProven {
		t.Fatal("读不回却答了已证明")
	}
}

// Covers: ADR-0063「未知前缀答未证明，不得已证明」在来源维上的同一条——场外揽收的来源
// 对象是 transport-fulfillment 的编号，拿它去问节点执行事实会在编号偶然同名时误证。
func TestAnOffsitePickupObjectIsNeverAskedAgainstNodeExecutionFacts(t *testing.T) {
	facts := &executionFactLookupDouble{record: presentedFact(t, receivedAt.Add(-time.Hour)), found: true}
	view := evidenceOver(t, facts, presentationRegistry(t))

	proof, err := prove(t, view, intakeSourceOfKind(t, psdomain.OffsitePickupSource), presentQualification)
	if err != nil {
		t.Fatalf("prove intake qualification: %v", err)
	}
	if proof != psports.IntakeQualificationUnproven {
		t.Fatalf("proof = %q, want UNPROVEN；场外揽收对象不是节点作业实物", proof)
	}
	if len(facts.keys) != 0 {
		t.Fatalf("lookups = %d; 同名编号也不得拿去问节点", len(facts.keys))
	}
}

// Covers: 本包 ErrUntranslatableAnswer 的既有语义「某一侧交出了词汇表之外的内容，是编程
// 错误不是业务答案」——译不过去既不是未证明也不是已证明，上抛。
func TestAnUntranslatableIdentityIsNotFoldedIntoUnproven(t *testing.T) {
	facts := &executionFactLookupDouble{record: presentedFact(t, receivedAt.Add(-time.Hour)), found: true}
	view := evidenceOver(t, facts, presentationRegistry(t))

	proof, err := view.ProveIntakeQualification(
		context.Background(),
		psdomain.SourceIdentity{},
		nodeIntakeSource(t),
		qualificationRule(t, presentQualification),
		receivedAt,
	)
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
	}
	if proof == psports.IntakeQualificationProven {
		t.Fatal("译不过去却答了已证明")
	}
	if len(facts.keys) != 0 {
		t.Fatalf("lookups = %d; 键立不起来不得去问节点", len(facts.keys))
	}
}

// Covers: 装配期拒绝立不起来的登记项——空事项、非法动作与缺仓储都在构造期报错，不留到
// 判断时再答一个看不出原因的未证明。
func TestTheRegistryRefusesEntriesThatCannotStandUpAtAssembly(t *testing.T) {
	facts := &executionFactLookupDouble{}

	t.Run("缺仓储", func(t *testing.T) {
		if _, err := adapter.NewNodeExecutionQualificationEvidence(
			nil, nodeAuthorityPrefix, presentationRegistry(t)); err == nil {
			t.Fatal("没有仓储也构造出了证据口")
		}
	})

	t.Run("空事项", func(t *testing.T) {
		if _, err := adapter.NewNodeExecutionQualificationEvidence(facts, nodeAuthorityPrefix,
			map[string]adapter.QualifyingNodeExecution{
				presentQualification: {Action: nodomain.PresentAction},
			}); err == nil {
			t.Fatal("事项为空也登记成功了")
		}
	})

	t.Run("非法动作", func(t *testing.T) {
		if _, err := adapter.NewNodeExecutionQualificationEvidence(facts, nodeAuthorityPrefix,
			map[string]adapter.QualifyingNodeExecution{
				presentQualification: {
					Item:   value(t, nodomain.NewCollaborationItemReference, "item-1"),
					Action: nodomain.CollaborationActionKindInvalid,
				},
			}); err == nil {
			t.Fatal("非法动作也登记成功了")
		}
	})

	t.Run("空引用", func(t *testing.T) {
		if _, err := adapter.NewNodeExecutionQualificationEvidence(facts, nodeAuthorityPrefix,
			map[string]adapter.QualifyingNodeExecution{
				"": {
					Item:   value(t, nodomain.NewCollaborationItemReference, "item-1"),
					Action: nodomain.PresentAction,
				},
			}); err == nil {
			t.Fatal("空引用也登记成功了")
		}
	})

	for name, prefix := range map[string]string{"空前缀": "", "前缀带斜杠": "NODE-OPS/customs"} {
		t.Run(name, func(t *testing.T) {
			if _, err := adapter.NewNodeExecutionQualificationEvidence(
				facts, prefix, nil); err == nil {
				t.Fatalf("前缀 %q 也认领成功了", prefix)
			}
		})
	}
}

// Covers: ADR-0063 Consequences「不得为了变绿拆假关务身份」——节点权威口只为自己拥有正文
// 的那一段引用作证。把关务段的资格项登记到节点证据口，构造期就得报错：留到判断时才拦，
// 这条配置会一直躺在表里等一次路由变更把它兑成假的已证明。
func TestTheRegistryRefusesReferencesFromAnotherAuthoritySegment(t *testing.T) {
	_, err := adapter.NewNodeExecutionQualificationEvidence(
		&executionFactLookupDouble{}, nodeAuthorityPrefix,
		map[string]adapter.QualifyingNodeExecution{
			customsQualification: {
				Item:   value(t, nodomain.NewCollaborationItemReference, "item-1"),
				Action: nodomain.PresentAction,
			},
		})
	if err == nil {
		t.Fatal("关务段的资格项登进了节点证据口——呈验完成会被兑成关务预检已证明")
	}
}

// Covers: 同一条护栏在判断维上的兜底——装配把本口误登在别的前缀下时，被路由进来的段外
// 引用仍答未证明。装配错不该由证据口兑出一个跨权威段的证明。
func TestAMisroutedSeamStillCannotProveAnotherAuthoritySegment(t *testing.T) {
	facts := &executionFactLookupDouble{record: presentedFact(t, receivedAt.Add(-time.Hour)), found: true}
	misrouted := pspartycommercial.NewKnownPrefixIntakeQualificationEvidence(
		map[string]psports.IntakeQualificationEvidenceView{
			"INTAKE-QUAL": evidenceOver(t, facts, presentationRegistry(t)),
		},
	)

	proof, err := misrouted.ProveIntakeQualification(context.Background(), identity(t),
		nodeIntakeSource(t), qualificationRule(t, customsQualification), receivedAt)
	if err != nil {
		t.Fatalf("prove through the misrouted seam: %v", err)
	}
	if proof != psports.IntakeQualificationUnproven {
		t.Fatalf("proof = %q, want UNPROVEN；段外引用不得由节点作证", proof)
	}
	if len(facts.keys) != 0 {
		t.Fatalf("lookups = %d; 段外引用不得去问节点", len(facts.keys))
	}
}

// Covers: 票 10 落点「给 KnownPrefix 缝后面接真源」——本适配器装进组合口后按前缀路由：
// 节点权威段走到节点证据，关务前缀仍属未登记，答未证明（ADR-0063 结果一栏）。登记键取自
// AuthorityPrefix()，B 票的装配点照这个形状接线就不会两处写岔。
func TestTheNodeEvidencePlugsIntoTheKnownPrefixSeam(t *testing.T) {
	facts := &executionFactLookupDouble{record: presentedFact(t, receivedAt.Add(-time.Hour)), found: true}
	view := evidenceOver(t, facts, presentationRegistry(t))
	seam := pspartycommercial.NewKnownPrefixIntakeQualificationEvidence(
		map[string]psports.IntakeQualificationEvidenceView{
			view.AuthorityPrefix(): view,
		},
	)

	proven, err := seam.ProveIntakeQualification(context.Background(), identity(t),
		nodeIntakeSource(t), qualificationRule(t, presentQualification), receivedAt)
	if err != nil {
		t.Fatalf("prove through the seam: %v", err)
	}
	if proven != psports.IntakeQualificationProven {
		t.Fatalf("proof = %q, want PROVEN；已登记前缀应路由到节点证据", proven)
	}

	customs, err := seam.ProveIntakeQualification(context.Background(), identity(t),
		nodeIntakeSource(t), qualificationRule(t, customsQualification), receivedAt)
	if err != nil {
		t.Fatalf("prove customs through the seam: %v", err)
	}
	if customs != psports.IntakeQualificationUnproven {
		t.Fatalf("proof = %q, want UNPROVEN；关务前缀未登记，节点说不了监管的话", customs)
	}
}
