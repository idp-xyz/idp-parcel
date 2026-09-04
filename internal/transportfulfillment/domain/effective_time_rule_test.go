package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var (
	ruleOccurredAt = time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	ruleReceivedAt = time.Date(2026, 9, 4, 8, 45, 0, 0, time.UTC)
)

func ruleSpec(t *testing.T) domain.EffectiveTimeRuleSpec {
	t.Helper()
	return domain.EffectiveTimeRuleSpec{
		TenantID: mustRef(t, domain.NewTenantID, "tenant-1"),
		Source:   mustRef(t, domain.NewTrackingSourceReference, "aggregator-a"),
		Version:  mustRef(t, domain.NewEffectiveTimeRuleVersion, "ETR-1"),
		Content: domain.EffectiveTimeRuleContent{
			SourceTimeMeaning: domain.SourceTimeIsEventOccurrence,
			Anchor:            domain.AnchoredAtOccurrence,
		},
	}
}

func TestRegisteringARuleKeepsTheThreeThingsAndMarksNoPredecessor(t *testing.T) {
	spec := ruleSpec(t)
	rule, err := domain.RegisterEffectiveTimeRule(spec)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if rule.TenantID() != spec.TenantID || rule.Source() != spec.Source || rule.Version() != spec.Version {
		t.Fatal("身份三件没有原样保留")
	}
	if rule.Content() != spec.Content {
		t.Fatalf("规则正文没有原样保留：%+v", rule.Content())
	}
	if _, has := rule.Supersedes(); has {
		t.Fatal("首版不该回指任何前版")
	}
	reference := rule.Reference()
	if reference.Rule() != "aggregator-a" || reference.Version() != "ETR-1" {
		t.Fatalf("规则引用应指名源与版本：%q / %q", reference.Rule(), reference.Version())
	}
}

func TestRegisteringRefusesAnIncompleteRule(t *testing.T) {
	cases := map[string]func(*domain.EffectiveTimeRuleSpec){
		"缺租户":  func(spec *domain.EffectiveTimeRuleSpec) { spec.TenantID = domain.TenantID{} },
		"缺轨迹源": func(spec *domain.EffectiveTimeRuleSpec) { spec.Source = domain.TrackingSourceReference{} },
		"缺版本":  func(spec *domain.EffectiveTimeRuleSpec) { spec.Version = domain.EffectiveTimeRuleVersion{} },
		"时间字段含义缺席": func(spec *domain.EffectiveTimeRuleSpec) {
			spec.Content.SourceTimeMeaning = domain.SourceTimeMeaningInvalid
		},
		"锚点缺席": func(spec *domain.EffectiveTimeRuleSpec) { spec.Content.Anchor = domain.EffectiveTimeAnchorInvalid },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := ruleSpec(t)
			mutate(&spec)
			if _, err := domain.RegisterEffectiveTimeRule(spec); !errors.Is(err, domain.ErrInvalidEffectiveTimeRule) {
				t.Fatalf("err = %v, want ErrInvalidEffectiveTimeRule", err)
			}
		})
	}
}

// Covers: ADR-0102 决定三——「有效时间＝发生时间」与「有效时间＝接收时间」两种朴素规则都是
// 显式登记的结果；判断带版本回指规则，依据是 JUDGED_BY_RULE 而不是显式判断。
func TestARuleFormsTheEffectiveTimeFromItsAnchorAndOffset(t *testing.T) {
	cases := map[string]struct {
		content domain.EffectiveTimeRuleContent
		want    time.Time
	}{
		"锚在发生时间、零偏移": {
			content: domain.EffectiveTimeRuleContent{SourceTimeMeaning: domain.SourceTimeIsEventOccurrence, Anchor: domain.AnchoredAtOccurrence},
			want:    ruleOccurredAt,
		},
		"锚在接收时间、零偏移": {
			content: domain.EffectiveTimeRuleContent{SourceTimeMeaning: domain.SourceTimeIsSourceProcessing, Anchor: domain.AnchoredAtReception},
			want:    ruleReceivedAt,
		},
		"锚在发生时间、正偏移两小时": {
			content: domain.EffectiveTimeRuleContent{SourceTimeMeaning: domain.SourceTimeIsEventOccurrence, Anchor: domain.AnchoredAtOccurrence, Offset: 2 * time.Hour},
			want:    ruleOccurredAt.Add(2 * time.Hour),
		},
		"锚在接收时间、负偏移三十分钟": {
			content: domain.EffectiveTimeRuleContent{SourceTimeMeaning: domain.SourceTimeIsSourceProcessing, Anchor: domain.AnchoredAtReception, Offset: -30 * time.Minute},
			want:    ruleReceivedAt.Add(-30 * time.Minute),
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			spec := ruleSpec(t)
			spec.Content = testCase.content
			rule, err := domain.RegisterEffectiveTimeRule(spec)
			if err != nil {
				t.Fatalf("register: %v", err)
			}
			judgment, err := rule.EffectiveTimeFor(ruleOccurredAt, ruleReceivedAt)
			if err != nil {
				t.Fatalf("judge: %v", err)
			}
			at, judged := judgment.EffectiveAt()
			if !judged || !at.Equal(testCase.want) {
				t.Fatalf("effectiveAt = %s judged=%v, want %s", at, judged, testCase.want)
			}
			if judgment.Basis() != domain.EffectiveTimeJudgedByRule {
				t.Fatalf("basis = %s, want JUDGED_BY_RULE", judgment.Basis())
			}
			reference, byRule := judgment.Rule()
			if !byRule || reference != rule.Reference() {
				t.Fatalf("判断应回指本规则版本：%+v byRule=%v", reference, byRule)
			}
		})
	}
}

func TestARuleRefusesToJudgeWithoutTheAnchoredTime(t *testing.T) {
	rule, err := domain.RegisterEffectiveTimeRule(ruleSpec(t))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := rule.EffectiveTimeFor(time.Time{}, ruleReceivedAt); !errors.Is(err, domain.ErrInvalidEffectiveTimeJudgment) {
		t.Fatalf("锚在发生时间却没有发生时间：err = %v", err)
	}
}

func TestRevisingARuleFormsANewVersionThatPointsBackAndLeavesTheOriginalUntouched(t *testing.T) {
	first, err := domain.RegisterEffectiveTimeRule(ruleSpec(t))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	revisedContent := domain.EffectiveTimeRuleContent{
		SourceTimeMeaning: domain.SourceTimeIsSourceProcessing,
		Anchor:            domain.AnchoredAtReception,
		Offset:            -15 * time.Minute,
	}
	second, err := first.Revise(mustRef(t, domain.NewEffectiveTimeRuleVersion, "ETR-2"), revisedContent)
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if second.Version().String() != "ETR-2" || second.Content() != revisedContent {
		t.Fatalf("新版本没有带上新正文：%+v", second.Content())
	}
	if prior, has := second.Supersedes(); !has || prior != first.Version() {
		t.Fatalf("新版本应回指首版：%q has=%v", prior, has)
	}
	if second.TenantID() != first.TenantID() || second.Source() != first.Source() {
		t.Fatal("换版不得改租户或轨迹源")
	}
	if first.Content() != ruleSpec(t).Content {
		t.Fatal("原版本被改写——值语义被破了")
	}
	if _, has := first.Supersedes(); has {
		t.Fatal("原版本凭空长出了前版")
	}
}

func TestRevisingRefusesToReuseTheCurrentVersionOrAnEmptyContent(t *testing.T) {
	first, err := domain.RegisterEffectiveTimeRule(ruleSpec(t))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := first.Revise(first.Version(), first.Content()); !errors.Is(err, domain.ErrInvalidEffectiveTimeRule) {
		t.Fatalf("沿用当前版本号就是覆盖：err = %v", err)
	}
	if _, err := first.Revise(mustRef(t, domain.NewEffectiveTimeRuleVersion, "ETR-2"), domain.EffectiveTimeRuleContent{}); !errors.Is(err, domain.ErrInvalidEffectiveTimeRule) {
		t.Fatalf("空正文不是规则：err = %v", err)
	}
}

func TestEqualComparesTheBusinessContentOfTwoVersions(t *testing.T) {
	first, err := domain.RegisterEffectiveTimeRule(ruleSpec(t))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	same, err := domain.RegisterEffectiveTimeRule(ruleSpec(t))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !first.Equal(same) {
		t.Fatal("同内容两次登记应相等")
	}
	spec := ruleSpec(t)
	spec.Content.Offset = time.Minute
	different, err := domain.RegisterEffectiveTimeRule(spec)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if first.Equal(different) {
		t.Fatal("偏移不同的两个版本不该相等")
	}
}

func TestParsingTheClosedWordsRefusesAnythingOutsideTheSet(t *testing.T) {
	for raw, want := range map[string]domain.SourceTimeMeaning{
		"EVENT_OCCURRENCE":  domain.SourceTimeIsEventOccurrence,
		"SOURCE_PROCESSING": domain.SourceTimeIsSourceProcessing,
	} {
		got, err := domain.ParseSourceTimeMeaning(raw)
		if err != nil || got != want || got.String() != raw {
			t.Fatalf("ParseSourceTimeMeaning(%q) = %v, %v", raw, got, err)
		}
	}
	if _, err := domain.ParseSourceTimeMeaning("GUESSED"); !errors.Is(err, domain.ErrInvalidEffectiveTimeRule) {
		t.Fatalf("集合外的词应拒：%v", err)
	}
	for raw, want := range map[string]domain.EffectiveTimeAnchor{
		"OCCURRED_AT": domain.AnchoredAtOccurrence,
		"RECEIVED_AT": domain.AnchoredAtReception,
	} {
		got, err := domain.ParseEffectiveTimeAnchor(raw)
		if err != nil || got != want || got.String() != raw {
			t.Fatalf("ParseEffectiveTimeAnchor(%q) = %v, %v", raw, got, err)
		}
	}
	if _, err := domain.ParseEffectiveTimeAnchor("NOW"); !errors.Is(err, domain.ErrInvalidEffectiveTimeRule) {
		t.Fatalf("集合外的锚点应拒：%v", err)
	}
}

func TestRehydratingARuleValidatesWithoutRecomputing(t *testing.T) {
	spec := domain.RehydrateEffectiveTimeRuleSpec{
		TenantID:          mustRef(t, domain.NewTenantID, "tenant-1"),
		Source:            mustRef(t, domain.NewTrackingSourceReference, "aggregator-a"),
		Version:           mustRef(t, domain.NewEffectiveTimeRuleVersion, "ETR-2"),
		SourceTimeMeaning: domain.SourceTimeIsSourceProcessing,
		Anchor:            domain.AnchoredAtReception,
		Offset:            -15 * time.Minute,
		Supersedes:        mustRef(t, domain.NewEffectiveTimeRuleVersion, "ETR-1"),
	}
	rule, err := domain.RehydrateEffectiveTimeRule(spec)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if prior, has := rule.Supersedes(); !has || prior != spec.Supersedes {
		t.Fatalf("前版没有装回：%q has=%v", prior, has)
	}
	if rule.Content().Offset != -15*time.Minute || rule.Content().Anchor != domain.AnchoredAtReception {
		t.Fatalf("正文没有装回：%+v", rule.Content())
	}

	t.Run("前版指向自己是一条读不动的链", func(t *testing.T) {
		bad := spec
		bad.Supersedes = bad.Version
		if _, err := domain.RehydrateEffectiveTimeRule(bad); !errors.Is(err, domain.ErrInvalidRehydratedEffectiveTimeRule) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("锚点不在封闭集合内", func(t *testing.T) {
		bad := spec
		bad.Anchor = domain.EffectiveTimeAnchorInvalid
		if _, err := domain.RehydrateEffectiveTimeRule(bad); !errors.Is(err, domain.ErrInvalidRehydratedEffectiveTimeRule) {
			t.Fatalf("err = %v", err)
		}
	})
}
