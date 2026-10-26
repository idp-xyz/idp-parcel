package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Covers: `AT-VE-202` 的领域半边与 VE CONTEXT「每个信号必须保存对象、类型、规则版本、判断时间、事实依据、可信度」——缺因果条件、
// 缺依据、依据立不起来的命中开不了发作期；存量那一格的因果条件不许新命中用。
func TestAHitWithoutItsCauseOrFactBasisOpensNoEpisode(t *testing.T) {
	id := mustValue(t, domain.NewEpisodeID, "episode-1")

	noCause := signalHit(t, "movement-fact/parcel-1/MF-1", firstHitAt)
	noCause.Cause = domain.CausalConditionReference{}
	noBasis := signalHit(t, "movement-fact/parcel-1/MF-1", firstHitAt)
	noBasis.Basis = nil
	brokenBasis := signalHit(t, "movement-fact/parcel-1/MF-1", firstHitAt)
	brokenBasis.Basis = []domain.SignalFactBasis{{Source: domain.SourceTransportFulfillment}}
	blankJudgement := signalHit(t, "movement-fact/parcel-1/MF-1", firstHitAt)
	blankJudgement.Basis[0].Judgement = "  "
	legacyCause := signalHit(t, "movement-fact/parcel-1/MF-1", firstHitAt)
	legacyCause.Cause = mustValue(t, domain.NewCausalConditionReference, domain.CausalConditionRecordedBeforeCauses)

	for name, hit := range map[string]domain.SignalHit{
		"缺因果条件":      noCause,
		"缺事实依据":      noBasis,
		"依据缺引用与版本":   brokenBasis,
		"判断标识只有空白":   blankJudgement,
		"新命中借用存量那一格": legacyCause,
	} {
		if hit.Complete() {
			t.Errorf("%s：受理判据放过了它", name)
		}
		if _, err := domain.OpenEpisode(id, hit); !errors.Is(err, domain.ErrInvalidEpisode) {
			t.Errorf("%s：err = %v，开期门放过了它", name, err)
		}
	}

	withJudgement := signalHit(t, "movement-fact/parcel-1/MF-1", firstHitAt)
	withJudgement.Basis[0].Judgement = "CC-JUDGEMENT-1"
	episode, err := domain.OpenEpisode(id, withJudgement)
	if err != nil {
		t.Fatalf("open episode: %v", err)
	}
	basis := episode.Basis()
	if len(basis) != 1 || basis[0].Fact.String() != "movement-fact/parcel-1/MF-1" ||
		basis[0].Version.String() != "v1" || basis[0].Judgement != "CC-JUDGEMENT-1" {
		t.Fatalf("basis = %+v，开期那一次命中的依据没有原样留下", basis)
	}
	basis[0].Judgement = "tampered"
	if episode.Basis()[0].Judgement != "CC-JUDGEMENT-1" {
		t.Fatal("交出去的依据改得动发作期里的那一份")
	}
}

// Covers: VE CONTEXT「同一对象、类型、因果条件和连续影响期……再次发生时建立关联的新发作期」——关联的新一期只认同一个原因；
// 规则版本与依据取再发生那一次的。
func TestAReopenedEpisodeKeepsItsCauseAndTakesTheNewHitsRuleAndBasis(t *testing.T) {
	episode := openEpisode(t)
	if err := episode.End("CONDITION_CLEARED", firstHitAt.Add(time.Hour)); err != nil {
		t.Fatalf("end: %v", err)
	}

	otherCause := signalHit(t, "movement-fact/parcel-1/MF-2", firstHitAt.Add(2*time.Hour))
	if _, err := episode.ReopenAsLinked(mustValue(t, domain.NewEpisodeID, "episode-2"), otherCause); !errors.Is(err, domain.ErrInvalidEpisode) {
		t.Fatalf("err = %v；另一个原因被当成这一期恢复后的再发生", err)
	}

	again := signalHit(t, "movement-fact/parcel-1/MF-1", firstHitAt.Add(2*time.Hour))
	again.Rule = mustValue(t, domain.NewSignalRuleVersionReference, "signal-rules/v4")
	again.Basis[0].Version = mustValue(t, domain.NewSourceFactVersion, "v2")
	linked, err := episode.ReopenAsLinked(mustValue(t, domain.NewEpisodeID, "episode-2"), again)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	snapshot := linked.Snapshot()
	if snapshot.Cause != episode.Cause() {
		t.Fatalf("cause = %q，关联的新一期换了原因", snapshot.Cause)
	}
	if snapshot.Rule.String() != "signal-rules/v4" || snapshot.Basis[0].Version.String() != "v2" {
		t.Fatalf("rule = %q basis = %+v，新一期沿用了前一期的判断", snapshot.Rule, snapshot.Basis)
	}
}

// Covers: `AT-VE-203` 的领域半边——存量发作期回填为「登记于因果条件之前」、依据为空，重建门放它进来；别的因果条件没有依据、
// 存量那一格反倒带着依据，都是写坏了。
func TestOnlyAnEpisodeRecordedBeforeCausesRehydratesWithoutFactBasis(t *testing.T) {
	snapshot := openEpisode(t).Snapshot()

	legacy := snapshot
	legacy.Cause = mustValue(t, domain.NewCausalConditionReference, domain.CausalConditionRecordedBeforeCauses)
	legacy.Basis = nil
	rebuilt, err := domain.RehydrateSignalEpisode(legacy)
	if err != nil {
		t.Fatalf("存量发作期重建不了：%v", err)
	}
	if len(rebuilt.Basis()) != 0 || rebuilt.Cause().String() != domain.CausalConditionRecordedBeforeCauses {
		t.Fatal("存量发作期被补了原因或依据")
	}

	legacyWithBasis := legacy
	legacyWithBasis.Basis = snapshot.Basis
	missingBasis := snapshot
	missingBasis.Basis = nil
	missingCause := snapshot
	missingCause.Cause = domain.CausalConditionReference{}
	for name, broken := range map[string]domain.SignalEpisodeSnapshot{
		"存量那一格带着依据": legacyWithBasis,
		"有因果条件却没依据": missingBasis,
		"没有因果条件":    missingCause,
	} {
		if _, err := domain.RehydrateSignalEpisode(broken); !errors.Is(err, domain.ErrInvalidEpisode) {
			t.Errorf("%s：err = %v，坏写入被读成了合法发作期", name, err)
		}
	}
}

// Covers: 两种产品自带的因果条件写法分得开——同一份源事实作「这一次发生」与作「这条事实流分叉」是两个原因；冲突信号的依据是
// 判断保留下来的每一份事实。
func TestSourceFactAndForkCausesDifferAndAConflictSignalCarriesItsRetainedFacts(t *testing.T) {
	reference := mustValue(t, domain.NewSourceFactReference, "handover/parcel-1/H-1")
	fact, err := domain.SourceFactCausalCondition(domain.SourceTransportFulfillment, reference)
	if err != nil {
		t.Fatalf("source fact cause: %v", err)
	}
	fork, err := domain.SupersessionForkCausalCondition(domain.SourceTransportFulfillment, reference)
	if err != nil {
		t.Fatalf("fork cause: %v", err)
	}
	if fact == fork {
		t.Fatalf("两种原因撞成了同一个：%q", fact)
	}
	if _, err := domain.SupersessionForkCausalCondition(domain.SourceContextInvalid, reference); err == nil {
		t.Fatal("集外源上下文立起了因果条件")
	}

	left := factSpec(t)
	left.Version = mustValue(t, domain.NewSourceFactVersion, "v2")
	left.Supersedes = mustValue(t, domain.NewSourceFactVersion, "v1")
	right := left
	right.Version = mustValue(t, domain.NewSourceFactVersion, "v3")
	judgment, err := domain.ResolveByBusinessTime([]domain.AcceptedSourceFact{
		mustFact(t, left), mustFact(t, right),
	})
	if err != nil || judgment.Resolved() {
		t.Fatalf("同刻两份后继应当裁不了：resolved=%v err=%v", judgment.Resolved(), err)
	}
	signal, err := domain.RaiseConflictSignal(
		mustValue(t, domain.NewExceptionSignalKindReference, "FACT_CONFLICT"), judgment, firstHitAt)
	if err != nil {
		t.Fatalf("raise conflict signal: %v", err)
	}
	basis := signal.Basis()
	if len(basis) != 2 || basis[0].Version.String() != "v2" || basis[1].Version.String() != "v3" ||
		basis[0].Fact != basis[1].Fact || basis[0].Judgement != "" {
		t.Fatalf("basis = %+v，冲突信号的依据不是保留下来的各方", basis)
	}
}

func mustFact(t *testing.T, spec domain.AcceptedSourceFactSpec) domain.AcceptedSourceFact {
	t.Helper()
	fact, err := domain.NewAcceptedSourceFact(spec)
	if err != nil {
		t.Fatalf("accepted fact: %v", err)
	}
	return fact
}
