package customscompliance_test

import (
	"context"
	"testing"
	"time"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/customscompliance"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉 routing-first-cut/12 轮次四的映射口径：可用 → 满足；不可用 → 适用限制（指回
// 判断标识与理由）；状态未知 → 逐类缺口。出处逐候选带回。投影（租户、时点、两端国家）照
// 样翻译过去，答案缺格或对不齐响亮上抛，不折成一条像样的硬约束。

var assessAsOf = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

func customsTenant(t *testing.T) ccdomain.TenantID {
	t.Helper()
	tenant, err := ccdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatalf("NewTenantID: %v", err)
	}
	return tenant
}

func customsCandidate(t *testing.T, ref string) ccdomain.RouteCandidateReference {
	t.Helper()
	candidate, err := ccdomain.NewRouteCandidateReference(ref)
	if err != nil {
		t.Fatalf("NewRouteCandidateReference: %v", err)
	}
	return candidate
}

func ccPort(t *testing.T, ref string, from time.Time) ccdomain.CustomsPortRecord {
	t.Helper()
	port, err := ccdomain.NewCustomsPortReference(ref)
	if err != nil {
		t.Fatalf("NewCustomsPortReference: %v", err)
	}
	return ccdomain.CustomsPortRecord{Port: port, AppliesFrom: from}
}

func ccPath(t *testing.T, ref, port, direction, mode string, from time.Time) ccdomain.CustomsPathRecord {
	t.Helper()
	path, err := ccdomain.NewDeclarationPathReference(ref)
	if err != nil {
		t.Fatalf("NewDeclarationPathReference: %v", err)
	}
	portRef, err := ccdomain.NewCustomsPortReference(port)
	if err != nil {
		t.Fatalf("NewCustomsPortReference: %v", err)
	}
	modeRef, err := ccdomain.NewDeclarationModeReference(mode)
	if err != nil {
		t.Fatalf("NewDeclarationModeReference: %v", err)
	}
	var dir ccdomain.ManifestDirection
	switch direction {
	case "EXPORT":
		dir = ccdomain.ExportManifest
	case "IMPORT":
		dir = ccdomain.ImportManifest
	default:
		t.Fatalf("测试夹具不认识的申报方向 %q", direction)
	}
	route, err := ccdomain.NewDeclarationPathRoute(portRef, dir, modeRef)
	if err != nil {
		t.Fatalf("NewDeclarationPathRoute: %v", err)
	}
	return ccdomain.CustomsPathRecord{Path: path, Route: route, AppliesFrom: from}
}

var synStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// fullEntries 是两岸齐全的一份目录：CN 出口经 SZX、SG 进口经 SIN。
func fullEntries(t *testing.T) ccdomain.CustomsApplicabilityEntries {
	t.Helper()
	return ccdomain.CustomsApplicabilityEntries{
		Ports: []ccdomain.CustomsPortRecord{
			ccPort(t, "SYN-PORT-SZX-01", synStart),
			ccPort(t, "SYN-PORT-SIN-01", synStart),
		},
		Paths: []ccdomain.CustomsPathRecord{
			ccPath(t, "SYN-PATH-CN-EXPORT-01", "SYN-PORT-SZX-01", "EXPORT", "SYN-MODE-MANIFEST-01", synStart),
			ccPath(t, "SYN-PATH-SG-IMPORT-01", "SYN-PORT-SIN-01", "IMPORT", "SYN-MODE-FORMAL-01", synStart),
		},
	}
}

// judgeDouble 按候选交回答卷；omit 里的候选不答，用来演练「来源对不齐」。
type judgeDouble struct {
	judgments map[string]ccdomain.CustomsApplicabilityJudgment
	omit      map[string]bool
	short     bool

	gotQuery ccports.CustomsApplicabilityQuery
}

func (double *judgeDouble) Handle(
	_ context.Context,
	query ccports.CustomsApplicabilityQuery,
) ([]ccdomain.CustomsApplicabilityJudgment, error) {
	double.gotQuery = query
	answers := make([]ccdomain.CustomsApplicabilityJudgment, 0, len(query.Candidates))
	for _, candidate := range query.Candidates {
		if double.omit[candidate.Candidate.String()] {
			continue
		}
		answers = append(answers, double.judgments[candidate.Candidate.String()])
	}
	if double.short {
		answers = answers[:0]
	}
	return answers, nil
}

func newAssessor(t *testing.T, judge *judgeDouble) *adapter.CustomsApplicabilityAssessor {
	t.Helper()
	assessor, err := adapter.NewCustomsApplicabilityAssessor(judge)
	if err != nil {
		t.Fatalf("NewCustomsApplicabilityAssessor: %v", err)
	}
	return assessor
}

func nrQuery(t *testing.T, candidates ...string) nrports.CustomsApplicabilityQuery {
	t.Helper()
	tenant, err := nrdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatalf("NewTenantID: %v", err)
	}
	query := nrports.CustomsApplicabilityQuery{Tenant: tenant, AsOf: assessAsOf}
	for _, ref := range candidates {
		candidate, err := nrdomain.NewCandidateID(ref)
		if err != nil {
			t.Fatalf("NewCandidateID: %v", err)
		}
		query.Candidates = append(query.Candidates, nrports.CustomsCandidate{
			Candidate: candidate, Origin: "CN", HasOrigin: true,
			Destination: "SG", HasDestination: true,
		})
	}
	return query
}

func foldJudgment(
	t *testing.T,
	tenant ccdomain.TenantID,
	candidate, origin, destination string,
	hasOrigin, hasDestination bool,
	entries ccdomain.CustomsApplicabilityEntries,
) ccdomain.CustomsApplicabilityJudgment {
	t.Helper()
	judgment, err := ccdomain.FoldCustomsApplicability(
		tenant, customsCandidate(t, candidate), origin, hasOrigin, destination, hasDestination,
		assessAsOf, entries)
	if err != nil {
		t.Fatalf("FoldCustomsApplicability: %v", err)
	}
	return judgment
}

func TestAvailableJudgmentTurnsIntoSatisfiedWithCitation(t *testing.T) {
	tenant := customsTenant(t)
	judgment := foldJudgment(t, tenant, "cand-cn-sg", "CN", "SG", true, true, fullEntries(t))
	judge := &judgeDouble{judgments: map[string]ccdomain.CustomsApplicabilityJudgment{
		"cand-cn-sg": judgment,
	}}
	assessor := newAssessor(t, judge)

	assessment, err := assessor.AssessCustomsApplicability(t.Context(), nrQuery(t, "cand-cn-sg"))
	if err != nil {
		t.Fatalf("AssessCustomsApplicability: %v", err)
	}
	if len(assessment.Findings) != 1 || assessment.Findings[0].Outcome() != nrdomain.ConstraintSatisfied {
		t.Fatalf("findings = %+v，可用该译成满足", assessment.Findings)
	}
	if len(assessment.Citations) != 1 ||
		assessment.Citations[0].Judgment() != judgment.JudgmentID() ||
		len(assessment.Citations[0].Versions()) != 4 {
		t.Fatalf("citations = %+v，出处该带判断标识与目录版本引用", assessment.Citations)
	}
	if judge.gotQuery.Tenant.String() != "SYN-TENANT-01" || !judge.gotQuery.AsOf.Equal(assessAsOf) {
		t.Fatalf("翻译过去的查询 = %+v", judge.gotQuery)
	}
	translated := judge.gotQuery.Candidates[0]
	if translated.Origin != "CN" || !translated.HasOrigin ||
		translated.Destination != "SG" || !translated.HasDestination {
		t.Fatalf("翻译过去的两端国家 = %q/%v → %q/%v", translated.Origin, translated.HasOrigin, translated.Destination, translated.HasDestination)
	}
}

func TestUnavailableReasonsMapOntoRestrictions(t *testing.T) {
	tenant := customsTenant(t)
	cases := []struct {
		name    string
		entries func(*testing.T) ccdomain.CustomsApplicabilityEntries
		reason  string
	}{
		{"口岸未登记", func(t *testing.T) ccdomain.CustomsApplicabilityEntries {
			entries := fullEntries(t)
			entries.Ports = entries.Ports[1:] // 出口路径经的 SZX 不在册
			return entries
		}, "PORT_NOT_REGISTERED"},
		{"口岸未生效", func(t *testing.T) ccdomain.CustomsApplicabilityEntries {
			entries := fullEntries(t)
			entries.Ports[0].AppliesFrom = assessAsOf.AddDate(0, 3, 0)
			return entries
		}, "PORT_NOT_EFFECTIVE"},
		{"缺进口方向", func(t *testing.T) ccdomain.CustomsApplicabilityEntries {
			entries := fullEntries(t)
			entries.Paths = entries.Paths[:1]
			return entries
		}, "PATH_DIRECTION_NOT_COVERED"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			judgment := foldJudgment(t, tenant, "cand-blocked", "CN", "SG", true, true, test.entries(t))
			judge := &judgeDouble{judgments: map[string]ccdomain.CustomsApplicabilityJudgment{
				"cand-blocked": judgment,
			}}
			assessment, err := newAssessor(t, judge).AssessCustomsApplicability(t.Context(), nrQuery(t, "cand-blocked"))
			if err != nil {
				t.Fatalf("AssessCustomsApplicability: %v", err)
			}
			if len(assessment.Findings) != 1 ||
				assessment.Findings[0].Outcome() != nrdomain.RestrictionApplies {
				t.Fatalf("findings = %+v，不可用该译成适用限制", assessment.Findings)
			}
			evaluated, _, err := nrdomain.EvaluateHardConstraints(
				[]nrdomain.RouteCandidate{mustQualified(t, "cand-blocked")}, assessment.Findings)
			if err != nil {
				t.Fatalf("EvaluateHardConstraints: %v", err)
			}
			want := "HARD_CONSTRAINT_RESTRICTION/CUSTOMS_APPLICABILITY/" + judgment.JudgmentID() + "/" + test.reason
			if evaluated[0].Outcome() != nrdomain.CandidateEliminated || evaluated[0].Reason().String() != want {
				t.Fatalf("候选 = %s/%q，想要确定性淘汰且原因指回判断标识与理由 %q",
					evaluated[0].Outcome(), evaluated[0].Reason(), want)
			}
		})
	}
}

func mustQualified(t *testing.T, ref string) nrdomain.RouteCandidate {
	t.Helper()
	candidate, err := nrdomain.NewCandidateID(ref)
	if err != nil {
		t.Fatalf("NewCandidateID: %v", err)
	}
	built, err := nrdomain.NewRouteCandidate(candidate, nrdomain.CandidateQualified, nrdomain.CandidateReason{})
	if err != nil {
		t.Fatalf("NewRouteCandidate: %v", err)
	}
	return built
}

func TestUnknownReasonsMapOntoNamedGaps(t *testing.T) {
	tenant := customsTenant(t)
	cases := []struct {
		name    string
		ref     string
		entry   func(*testing.T) ccdomain.CustomsApplicabilityJudgment
		missing string
		again   string
	}{
		{"缺寄件国", "cand-no-origin", func(t *testing.T) ccdomain.CustomsApplicabilityJudgment {
			return foldJudgment(t, tenant, "cand-no-origin", "", "SG", false, true, fullEntries(t))
		}, "CUSTOMS_ENDPOINT_COUNTRY_MISSING/ORIGIN", "CUSTOMS_ENDPOINT_COUNTRY_SUPPLEMENTED"},
		{"目录为空", "cand-empty-catalog", func(t *testing.T) ccdomain.CustomsApplicabilityJudgment {
			return foldJudgment(t, tenant, "cand-empty-catalog", "CN", "SG", true, true, ccdomain.CustomsApplicabilityEntries{})
		}, "CUSTOMS_PORT_PATH_CATALOG_EMPTY", "CUSTOMS_PORT_PATH_CATALOG_REGISTERED"},
		{"目录读不到", "cand-unreadable", func(t *testing.T) ccdomain.CustomsApplicabilityJudgment {
			judgment, err := ccdomain.FoldCustomsApplicabilityUnreadable(
				tenant, customsCandidate(t, "cand-unreadable"), assessAsOf)
			if err != nil {
				t.Fatalf("FoldCustomsApplicabilityUnreadable: %v", err)
			}
			return judgment
		}, "CUSTOMS_PORT_PATH_CATALOG_UNREADABLE", "CUSTOMS_PORT_PATH_CATALOG_RESTORED"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			judgment := test.entry(t)
			judge := &judgeDouble{judgments: map[string]ccdomain.CustomsApplicabilityJudgment{
				test.ref: judgment,
			}}
			assessment, err := newAssessor(t, judge).AssessCustomsApplicability(t.Context(), nrQuery(t, test.ref))
			if err != nil {
				t.Fatalf("AssessCustomsApplicability: %v", err)
			}
			if assessment.Findings[0].Outcome() != nrdomain.ConstraintStatusUnknown {
				t.Fatalf("outcome = %s，状态未知该译成状态未知", assessment.Findings[0].Outcome())
			}
			_, gaps, err := nrdomain.EvaluateHardConstraints(
				[]nrdomain.RouteCandidate{mustQualified(t, test.ref)}, assessment.Findings)
			if err != nil {
				t.Fatalf("EvaluateHardConstraints: %v", err)
			}
			if len(gaps) != 1 || gaps[0].Reference().String() != test.missing ||
				gaps[0].ReassessmentCondition().String() != test.again {
				t.Fatalf("gap = %+v，想要 %q / %q", gaps, test.missing, test.again)
			}
		})
	}
}

func TestTheAdapterRefusesMisalignedAnswers(t *testing.T) {
	tenant := customsTenant(t)
	judgment := foldJudgment(t, tenant, "cand-a", "CN", "SG", true, true, fullEntries(t))
	judge := &judgeDouble{judgments: map[string]ccdomain.CustomsApplicabilityJudgment{
		"cand-a": judgment,
	}}

	omitted := &judgeDouble{omit: map[string]bool{"cand-a": true}, judgments: judge.judgments}
	if _, err := newAssessor(t, omitted).AssessCustomsApplicability(t.Context(), nrQuery(t, "cand-a")); err == nil {
		t.Fatal("回答少一条必须响亮上抛，不把它读成满足")
	}

	short := &judgeDouble{short: true, judgments: judge.judgments}
	if _, err := newAssessor(t, short).AssessCustomsApplicability(t.Context(), nrQuery(t, "cand-a")); err == nil {
		t.Fatal("回答条数与候选数对不齐必须响亮上抛")
	}

	if _, err := adapter.NewCustomsApplicabilityAssessor(nil); err == nil {
		t.Fatal("nil 判断服务应在构造期被拒")
	}
}

func TestAssessingNoCandidatesAsksForNothing(t *testing.T) {
	judge := &judgeDouble{}
	assessment, err := newAssessor(t, judge).AssessCustomsApplicability(t.Context(), nrQuery(t))
	if err != nil {
		t.Fatalf("AssessCustomsApplicability: %v", err)
	}
	if len(assessment.Findings) != 0 || len(assessment.Citations) != 0 || judge.gotQuery.Candidates != nil {
		t.Fatal("候选空间为空时不问、不答——没有要作答的东西")
	}
}
