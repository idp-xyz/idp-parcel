package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var resultReceivedAt = time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)

func externalResult(t *testing.T, layer domain.ResultLayer, semantics string) domain.ExternalResult {
	t.Helper()
	result, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer:        layer,
		SourceID:     "channel-response/771",
		Role:         mustValue(t, domain.NewSourceAuthorityRole, "CUSTOMS_AUTHORITY"),
		RawSemantics: semantics,
		Rule:         mustValue(t, domain.NewInterpretationRuleReference, "interpretation/v3"),
		Version:      mustValue(t, domain.NewSubmissionVersionID, "submission-1/v1"),
		Attempt:      1,
		Scope:        mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		OccurredAt:   resultReceivedAt.Add(-time.Hour),
		ReceivedAt:   resultReceivedAt,
	})
	if err != nil {
		t.Fatalf("interpret external result (%s): %v", layer, err)
	}
	return result
}

// Covers: CC CONTEXT「监管接收、业务受理、监管过程决定、监管核定税费、放行结果和监管处置决定必须分层保存……不能使用一个“清关成功”状态覆盖各层事实」
// 「每项外部结果必须保存来源身份、来源在该结果层的权威角色、原始业务语义、业务发生或适用时间、接收时间、实际采用的解释规则及其与提交版本、提交尝试和明确结果范围的关系」
// ——六层封闭各自成立、八件缺一立不起、
// 类型上没有层间派生方法（前层成功造不出后层）。
func TestSixLayersStandAloneWithTheirEightAnchors(t *testing.T) {
	receipt := externalResult(t, domain.RegulatoryReceiptLayer, "MANIFEST_RECEIVED")
	if receipt.Layer() != domain.RegulatoryReceiptLayer || receipt.RawSemantics() != "MANIFEST_RECEIVED" {
		t.Fatalf("receipt = %#v", receipt)
	}

	missingRole := domain.ExternalResultSpec{
		Layer:        domain.ReleaseResultLayer,
		SourceID:     "channel-response/772",
		RawSemantics: "RELEASED",
		Rule:         mustValue(t, domain.NewInterpretationRuleReference, "interpretation/v3"),
		Version:      mustValue(t, domain.NewSubmissionVersionID, "submission-1/v1"),
		Attempt:      1,
		Scope:        mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		OccurredAt:   resultReceivedAt.Add(-time.Hour),
		ReceivedAt:   resultReceivedAt,
	}
	if _, err := domain.InterpretExternalResult(missingRole); !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v; 没有权威角色的结果被收下了——中介响应只有语义明确代表监管机构才成监管事实", err)
	}

	unattributable := missingRole
	unattributable.Role = mustValue(t, domain.NewSourceAuthorityRole, "CUSTOMS_AUTHORITY")
	unattributable.Version = domain.SubmissionVersionID{}
	if _, err := domain.InterpretExternalResult(unattributable); !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v; 关联不上原提交的结果不得猜测成层", err)
	}
}

// layerFact 造一份同一提交版本、同一范围上的在册或新到事实；放行层按 kind 另拆放行事实，
// kind 为零值即放行事实落点建立之前那批说不出种类的存量。
func layerFact(
	t *testing.T,
	source string,
	layer domain.ResultLayer,
	semantics string,
	kind domain.ReleaseKind,
	occurredAt time.Time,
) domain.LayerFact {
	t.Helper()
	scope := mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1")
	result, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer:        layer,
		SourceID:     source,
		Role:         mustValue(t, domain.NewSourceAuthorityRole, "CUSTOMS_AUTHORITY"),
		RawSemantics: semantics,
		Rule:         mustValue(t, domain.NewInterpretationRuleReference, "interpretation/v3"),
		Version:      mustValue(t, domain.NewSubmissionVersionID, "submission-1/v1"),
		Attempt:      1,
		Scope:        scope,
		OccurredAt:   occurredAt,
		ReceivedAt:   resultReceivedAt,
	})
	if err != nil {
		t.Fatalf("interpret external result (%s): %v", source, err)
	}
	fact := domain.LayerFact{Result: result}
	if kind != domain.ReleaseKindInvalid {
		condition := ""
		if kind == domain.ConditionalRelease {
			condition = "post-release inspection within 7 days"
		}
		release, err := domain.ReceiveReleaseOutcome(kind,
			mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
			scope, condition, resultReceivedAt)
		if err != nil {
			t.Fatalf("receive release outcome (%s): %v", source, err)
		}
		fact.Release = &release
	}
	return fact
}

var (
	earlierAt = resultReceivedAt.Add(-3 * time.Hour)
	laterAt   = resultReceivedAt.Add(-time.Hour)
)

func release(t *testing.T, source string, kind domain.ReleaseKind, at time.Time) domain.LayerFact {
	t.Helper()
	return layerFact(t, source, domain.ReleaseResultLayer, "RELEASE/"+kind.String(), kind, at)
}

// Covers: AT-CC-431、AT-CC-432、AT-CC-434、AT-CC-435 的领域半边——放行层按种类偏序随业务时间只进不退：
// 先部分或附条件、后全部是推进，同种类再到也是；全部之后回退、部分与附条件之间改换、同一时刻两种种类
// 才是矛盾。乱序到达只看业务时间：全部放行先到、更早的部分放行后到，照样是推进。
func TestReleasesAdvanceByKindAlongBusinessTime(t *testing.T) {
	for name, test := range map[string]struct {
		existing, incoming domain.LayerFact
		conflict           bool
	}{
		"先部分、后全部":          {release(t, "r-1", domain.PartialRelease, earlierAt), release(t, "r-2", domain.FullRelease, laterAt), false},
		"全部先到、更早的部分后到":     {release(t, "r-2", domain.FullRelease, laterAt), release(t, "r-1", domain.PartialRelease, earlierAt), false},
		"先附条件、后全部":         {release(t, "r-1", domain.ConditionalRelease, earlierAt), release(t, "r-2", domain.FullRelease, laterAt), false},
		"部分之后再部分（剩余范围）":    {release(t, "r-1", domain.PartialRelease, earlierAt), release(t, "r-2", domain.PartialRelease, laterAt), false},
		"同一时刻两份部分放行，措辞不同":  {release(t, "r-1", domain.PartialRelease, laterAt), layerFact(t, "r-2", domain.ReleaseResultLayer, "PARTIAL/LINES-3-5", domain.PartialRelease, laterAt), false},
		"全部之后回退成部分":        {release(t, "r-1", domain.FullRelease, earlierAt), release(t, "r-2", domain.PartialRelease, laterAt), true},
		"全部之后回退成附条件":       {release(t, "r-1", domain.FullRelease, earlierAt), release(t, "r-2", domain.ConditionalRelease, laterAt), true},
		"部分先到、业务时间更早的全部后到": {release(t, "r-2", domain.PartialRelease, laterAt), release(t, "r-1", domain.FullRelease, earlierAt), true},
		"部分之后改成附条件":        {release(t, "r-1", domain.PartialRelease, earlierAt), release(t, "r-2", domain.ConditionalRelease, laterAt), true},
		"附条件之后改成部分":        {release(t, "r-1", domain.ConditionalRelease, earlierAt), release(t, "r-2", domain.PartialRelease, laterAt), true},
		"同一时刻全部与部分":        {release(t, "r-1", domain.FullRelease, laterAt), release(t, "r-2", domain.PartialRelease, laterAt), true},
		"说不出种类的存量，原始语义相同":  {layerFact(t, "r-1", domain.ReleaseResultLayer, "RELEASE/FULL", domain.ReleaseKindInvalid, earlierAt), release(t, "r-2", domain.FullRelease, laterAt), false},
		"说不出种类的存量，原始语义不同":  {layerFact(t, "r-1", domain.ReleaseResultLayer, "RELEASED", domain.ReleaseKindInvalid, earlierAt), release(t, "r-2", domain.FullRelease, laterAt), true},
	} {
		t.Run(name, func(t *testing.T) {
			err := domain.CheckLayerConsistency([]domain.LayerFact{test.existing}, test.incoming)
			if test.conflict && !errors.Is(err, domain.ErrLayerConflict) {
				t.Fatalf("err = %v; 这是矛盾，却没判冲突——后到的一方会按最后到达改掉当前判断", err)
			}
			if !test.conflict && err != nil {
				t.Fatalf("err = %v; 这是同层后续事件，判了冲突就把它吞了", err)
			}
		})
	}
}

// Covers: AT-CC-433、AT-CC-434、AT-CC-435 的领域半边——监管过程决定按业务时间成序列，先查验、后补资料
// 两项都成立，乱序到达也一样；同一时刻说两样排不出先后，判冲突。其余四层对一版提交只给一个结论，
// 换一种说法就是冲突，与业务时间先后无关。
func TestProcessDecisionsFormASequenceAndOtherLayersHoldOneConclusion(t *testing.T) {
	process := func(source, semantics string, at time.Time) domain.LayerFact {
		return layerFact(t, source, domain.ProcessDecisionLayer, semantics, domain.ReleaseKindInvalid, at)
	}
	single := func(layer domain.ResultLayer, source, semantics string, at time.Time) domain.LayerFact {
		return layerFact(t, source, layer, semantics, domain.ReleaseKindInvalid, at)
	}
	for name, test := range map[string]struct {
		existing, incoming domain.LayerFact
		conflict           bool
	}{
		"先查验、后补资料":        {process("p-1", "INSPECTION_ORDERED", earlierAt), process("p-2", "DOCUMENTS_REQUESTED", laterAt), false},
		"补资料先到、更早的查验后到":   {process("p-2", "DOCUMENTS_REQUESTED", laterAt), process("p-1", "INSPECTION_ORDERED", earlierAt), false},
		"同一时刻同一决定重复到达":    {process("p-1", "INSPECTION_ORDERED", laterAt), process("p-2", "INSPECTION_ORDERED", laterAt), false},
		"同一时刻两种决定":        {process("p-1", "INSPECTION_ORDERED", laterAt), process("p-2", "DOCUMENTS_REQUESTED", laterAt), true},
		"业务受理之后又不受理":      {single(domain.BusinessAcceptanceLayer, "a-1", "ACCEPTED", earlierAt), single(domain.BusinessAcceptanceLayer, "a-2", "REFUSED", laterAt), true},
		"监管接收重复到达":        {single(domain.RegulatoryReceiptLayer, "m-1", "RECEIVED", earlierAt), single(domain.RegulatoryReceiptLayer, "m-2", "RECEIVED", laterAt), false},
		"核定税费换了金额（无更正关系）": {single(domain.AssessedDutyLayer, "d-1", "DUTY/120USD", earlierAt), single(domain.AssessedDutyLayer, "d-2", "DUTY/150USD", laterAt), true},
		"处置决定换了一种":        {single(domain.DispositionDecisionLayer, "x-1", "RETURN", earlierAt), single(domain.DispositionDecisionLayer, "x-2", "DESTROY", laterAt), true},
	} {
		t.Run(name, func(t *testing.T) {
			err := domain.CheckLayerConsistency([]domain.LayerFact{test.existing}, test.incoming)
			if test.conflict && !errors.Is(err, domain.ErrLayerConflict) {
				t.Fatalf("err = %v, want ErrLayerConflict", err)
			}
			if !test.conflict && err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}
}

// Covers: AT-CC-432 的领域半边——判断对两份事实对称，所以一组两两不矛盾的同层事实不论按什么顺序到达都
// 一份也不记冲突；不同层、不同范围、不同提交版本的事实各归各位。
func TestTheJudgmentIsSymmetricSoArrivalOrderCannotMatter(t *testing.T) {
	pool := []domain.LayerFact{
		release(t, "r-1", domain.PartialRelease, earlierAt),
		release(t, "r-2", domain.FullRelease, laterAt),
		release(t, "r-3", domain.ConditionalRelease, earlierAt),
		release(t, "r-4", domain.FullRelease, earlierAt),
		release(t, "r-5", domain.PartialRelease, laterAt),
		layerFact(t, "p-1", domain.ProcessDecisionLayer, "INSPECTION_ORDERED", domain.ReleaseKindInvalid, earlierAt),
	}
	for _, one := range pool {
		for _, other := range pool {
			forward := domain.CheckLayerConsistency([]domain.LayerFact{one}, other)
			backward := domain.CheckLayerConsistency([]domain.LayerFact{other}, one)
			if errors.Is(forward, domain.ErrLayerConflict) != errors.Is(backward, domain.ErrLayerConflict) {
				t.Fatalf("%s 与 %s 的判断随到达顺序翻转：%v / %v",
					one.Result.SourceID(), other.Result.SourceID(), forward, backward)
			}
		}
	}

	// 部分 → 全部 → 全部（更晚的措辞）两两不矛盾：六种到达顺序都一份冲突不记。
	chain := []domain.LayerFact{
		release(t, "c-1", domain.PartialRelease, earlierAt),
		release(t, "c-2", domain.FullRelease, laterAt),
		layerFact(t, "c-3", domain.ReleaseResultLayer, "RELEASE/FINAL", domain.FullRelease, laterAt.Add(time.Minute)),
	}
	for _, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		var booked []domain.LayerFact
		for _, index := range order {
			if err := domain.CheckLayerConsistency(booked, chain[index]); err != nil {
				t.Fatalf("到达顺序 %v 上 %s 被判 %v", order, chain[index].Result.SourceID(), err)
			}
			booked = append(booked, chain[index])
		}
	}

	regression := release(t, "o-1", domain.PartialRelease, laterAt)
	elsewhere := layerFact(t, "o-2", domain.ReleaseResultLayer, "RELEASE/FULL", domain.FullRelease, earlierAt)
	otherScope := mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-2")
	movedResult, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer:        domain.ReleaseResultLayer,
		SourceID:     "o-2",
		Role:         elsewhere.Result.Role(),
		RawSemantics: elsewhere.Result.RawSemantics(),
		Rule:         elsewhere.Result.Rule(),
		Version:      elsewhere.Result.Version(),
		Attempt:      1,
		Scope:        otherScope,
		OccurredAt:   earlierAt,
		ReceivedAt:   resultReceivedAt,
	})
	if err != nil {
		t.Fatalf("interpret: %v", err)
	}
	movedRelease, err := domain.ReceiveReleaseOutcome(domain.FullRelease, elsewhere.Release.Authority(), otherScope, "", resultReceivedAt)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := domain.CheckLayerConsistency(
		[]domain.LayerFact{{Result: movedResult, Release: &movedRelease}}, regression); err != nil {
		t.Fatalf("err = %v; 别的范围上的全部放行不约束这一范围", err)
	}
	if err := domain.CheckLayerConsistency(
		[]domain.LayerFact{layerFact(t, "o-3", domain.ProcessDecisionLayer, "INSPECTION_ORDERED", domain.ReleaseKindInvalid, laterAt)},
		layerFact(t, "o-4", domain.ReleaseResultLayer, "RELEASE/PARTIAL", domain.PartialRelease, laterAt)); err != nil {
		t.Fatalf("err = %v; 不同层的事实各归各位", err)
	}
}

// 放行事实与层次配不上即输入不成形：新到的放行层必带放行事实，非放行层不得带，放行范围就是
// 结果范围；在册那一侧配不上同样不成形，不当作「没有冲突」放过去。
func TestMismatchedReleaseShapesAreInvalidNotSilentlyConsistent(t *testing.T) {
	partial := release(t, "s-1", domain.PartialRelease, earlierAt)
	bare := layerFact(t, "s-2", domain.ReleaseResultLayer, "RELEASE/FULL", domain.ReleaseKindInvalid, laterAt)
	if err := domain.CheckLayerConsistency(nil, bare); !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v; 新到的放行层说不出种类，排不进放行的先后", err)
	}
	smuggled := layerFact(t, "s-3", domain.BusinessAcceptanceLayer, "ACCEPTED", domain.ReleaseKindInvalid, laterAt)
	smuggled.Release = partial.Release
	if err := domain.CheckLayerConsistency(nil, smuggled); !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v; 非放行层夹带放行事实", err)
	}
	if err := domain.CheckLayerConsistency([]domain.LayerFact{smuggled}, partial); !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v; 在册事实配不上层次也不得当作没有冲突", err)
	}
	strayScope, err := domain.ReceiveReleaseOutcome(domain.FullRelease, partial.Release.Authority(),
		mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-9"), "", resultReceivedAt)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	stray := release(t, "s-4", domain.FullRelease, laterAt)
	stray.Release = &strayScope
	if err := domain.CheckLayerConsistency(nil, stray); !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v; 放行范围与结果范围不符", err)
	}
}
