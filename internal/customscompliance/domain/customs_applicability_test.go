package domain

import (
	"reflect"
	"testing"
	"time"
)

// 本文件钉票 routing-first-cut/12 的完成判据二、三在领域层的形状：可用 / 不可用（口岸
// 未登记、口岸未生效、申报路径方向缺）三格与状态未知（缺国家码、目录为空、依赖读不到）
// 各成其格，状态未知不折成可用；同输入重复折叠结果一致、出处可重算。

var (
	applicabilityAsOf   = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	applicabilityTenant = mustApplicabilityTenant()
)

func mustApplicabilityTenant() TenantID {
	tenant, err := NewTenantID("SYN-TENANT-01")
	if err != nil {
		panic(err)
	}
	return tenant
}

func portRecord(t *testing.T, ref string, from time.Time, until time.Time) CustomsPortRecord {
	t.Helper()
	port, err := NewCustomsPortReference(ref)
	if err != nil {
		t.Fatalf("NewCustomsPortReference(%q): %v", ref, err)
	}
	return CustomsPortRecord{Port: port, AppliesFrom: from, AppliesUntil: until}
}

func pathRecord(t *testing.T, ref, portRef, direction, mode string, from, until time.Time) CustomsPathRecord {
	t.Helper()
	path, err := NewDeclarationPathReference(ref)
	if err != nil {
		t.Fatalf("NewDeclarationPathReference(%q): %v", ref, err)
	}
	port, err := NewCustomsPortReference(portRef)
	if err != nil {
		t.Fatalf("NewCustomsPortReference(%q): %v", portRef, err)
	}
	var dir ManifestDirection
	switch direction {
	case "EXPORT":
		dir = ExportManifest
	case "IMPORT":
		dir = ImportManifest
	default:
		t.Fatalf("测试夹具不认识的申报方向 %q", direction)
	}
	modeRef, err := NewDeclarationModeReference(mode)
	if err != nil {
		t.Fatalf("NewDeclarationModeReference(%q): %v", mode, err)
	}
	route, err := NewDeclarationPathRoute(port, dir, modeRef)
	if err != nil {
		t.Fatalf("NewDeclarationPathRoute: %v", err)
	}
	return CustomsPathRecord{Path: path, Route: route, AppliesFrom: from, AppliesUntil: until}
}

func candidateRef(t *testing.T, ref string) RouteCandidateReference {
	t.Helper()
	candidate, err := NewRouteCandidateReference(ref)
	if err != nil {
		t.Fatalf("NewRouteCandidateReference(%q): %v", ref, err)
	}
	return candidate
}

func fold(
	t *testing.T,
	candidate RouteCandidateReference,
	origin string, hasOrigin bool,
	destination string, hasDestination bool,
	entries CustomsApplicabilityEntries,
) CustomsApplicabilityJudgment {
	t.Helper()
	judgment, err := FoldCustomsApplicability(
		applicabilityTenant, candidate, origin, hasOrigin, destination, hasDestination,
		applicabilityAsOf, entries)
	if err != nil {
		t.Fatalf("FoldCustomsApplicability: %v", err)
	}
	return judgment
}

// applicableEntries 是按演示租户形状合成的一份完整目录：两侧口岸生效、出口与进口路径
// 各一条且所经口岸过硬——CN→SG 候选应得可用。
func applicableEntries(t *testing.T) CustomsApplicabilityEntries {
	t.Helper()
	return CustomsApplicabilityEntries{
		Ports: []CustomsPortRecord{
			portRecord(t, "SYN-PORT-SZX-01", opensAt("2026-01-01"), time.Time{}),
			portRecord(t, "SYN-PORT-SIN-01", opensAt("2026-01-01"), time.Time{}),
		},
		Paths: []CustomsPathRecord{
			pathRecord(t, "SYN-PATH-CN-EXPORT-01", "SYN-PORT-SZX-01", "EXPORT", "SYN-MODE-MANIFEST-01", opensAt("2026-01-01"), time.Time{}),
			pathRecord(t, "SYN-PATH-SG-IMPORT-01", "SYN-PORT-SIN-01", "IMPORT", "SYN-MODE-FORMAL-01", opensAt("2026-01-01"), time.Time{}),
		},
	}
}

func opensAt(value string) time.Time {
	opened, err := time.Parse(time.RFC3339, value+"T00:00:00Z")
	if err != nil {
		panic(err)
	}
	return opened
}

func TestCrossingCandidateIsAvailableWhenBothSidesHold(t *testing.T) {
	judgment := fold(t, candidateRef(t, "cand-cn-sg"), "CN", true, "SG", true, applicableEntries(t))
	if judgment.Outcome() != CustomsAvailable {
		t.Fatalf("outcome = %q，希望可用；一份两岸都在册的目录折不出可用", judgment.Outcome().String())
	}
	if judgment.JudgmentID() == "" || len(judgment.Versions()) == 0 {
		t.Fatal("可用作答必须带出处：判断标识与目录版本引用")
	}
}

func TestDomesticCandidateIsAvailableWithoutAnyCatalog(t *testing.T) {
	judgment := fold(t, candidateRef(t, "cand-cn-cn"), "CN", true, "CN", true, CustomsApplicabilityEntries{})
	if judgment.Outcome() != CustomsAvailable {
		t.Fatalf("outcome = %q，两端同国的候选不含关务段，无论目录空否都该可用", judgment.Outcome().String())
	}
}

func TestMissingEndpointCountryAnswersUnknownAndNamesTheSide(t *testing.T) {
	origin := fold(t, candidateRef(t, "cand-noorigin"), "", false, "SG", true, applicableEntries(t))
	if origin.Outcome() != CustomsStatusUnknown || origin.UnknownReason() != EndpointCountryMissing {
		t.Fatalf("outcome/unknown = %q/%q，缺寄件国该答状态未知", origin.Outcome().String(), origin.UnknownReason().String())
	}
	if side, ok := origin.EndpointSide(); !ok || side != EndpointSideOrigin {
		t.Fatalf("side = %q/%v，该指名缺寄件国", side.String(), ok)
	}

	both := fold(t, candidateRef(t, "cand-noboth"), "", false, "", false, applicableEntries(t))
	if side, ok := both.EndpointSide(); !ok || side != EndpointSideBoth {
		t.Fatalf("side = %q/%v，两端都缺该是 Both 一格", side.String(), ok)
	}
}

func TestEmptyCatalogAnswersUnknownNotAvailable(t *testing.T) {
	judgment := fold(t, candidateRef(t, "cand-empty"), "CN", true, "SG", true, CustomsApplicabilityEntries{})
	if judgment.Outcome() != CustomsStatusUnknown {
		t.Fatalf("outcome = %q，两本目录皆空该答状态未知，绝不折成可用", judgment.Outcome().String())
	}
	if judgment.UnknownReason() != CatalogEmpty {
		t.Fatalf("unknown = %q，该是目录为空", judgment.UnknownReason().String())
	}
}

func TestPathThroughUnregisteredPortIsUnavailable(t *testing.T) {
	entries := applicableEntries(t)
	entries.Ports = entries.Ports[1:] // 只留 SIN：出口路径经的 SZX 任何时点都不在册
	judgment := fold(t, candidateRef(t, "cand-noport"), "CN", true, "SG", true, entries)
	if judgment.Outcome() != CustomsUnavailable || judgment.UnavailableReason() != PortNotRegistered {
		t.Fatalf("outcome/reason = %q/%q，该是口岸未登记", judgment.Outcome().String(), judgment.UnavailableReason().String())
	}
	if port, ok := judgment.UnavailablePort(); !ok || port.String() != "SYN-PORT-SZX-01" {
		t.Fatalf("port = %q/%v，该指名经不起的那条路径的口岸", port.String(), ok)
	}
}

func TestPathThroughFuturePortIsUnavailable(t *testing.T) {
	entries := applicableEntries(t)
	// SZX 那行改成未来才生效：有行而判断时点不覆盖 → 口岸未生效。
	entries.Ports[0].AppliesFrom = applicabilityAsOf.AddDate(0, 3, 0)
	judgment := fold(t, candidateRef(t, "cand-futureport"), "CN", true, "SG", true, entries)
	if judgment.Outcome() != CustomsUnavailable || judgment.UnavailableReason() != PortNotEffective {
		t.Fatalf("outcome/reason = %q/%q，该是口岸在判断时点未生效", judgment.Outcome().String(), judgment.UnavailableReason().String())
	}
	if port, ok := judgment.UnavailablePort(); !ok || port.String() != "SYN-PORT-SZX-01" {
		t.Fatalf("port = %q/%v，该指名未生效的口岸", port.String(), ok)
	}
}

func TestMissingImportDirectionIsUnavailable(t *testing.T) {
	entries := applicableEntries(t)
	entries.Paths = entries.Paths[:1] // 只剩出口路径：收件国一侧没有生效的进口路径
	judgment := fold(t, candidateRef(t, "cand-noimport"), "CN", true, "SG", true, entries)
	if judgment.Outcome() != CustomsUnavailable || judgment.UnavailableReason() != PathDirectionNotCovered {
		t.Fatalf("outcome/reason = %q/%q，该是申报路径方向没被覆盖", judgment.Outcome().String(), judgment.UnavailableReason().String())
	}
	if direction, ok := judgment.UnavailableDirection(); !ok || direction != ImportManifest {
		t.Fatalf("direction = %q/%v，该指名缺进口方向", direction.String(), ok)
	}
}

func TestASidePassesWhenAnyEffectivePathHolds(t *testing.T) {
	entries := applicableEntries(t)
	// 出口侧再加一条经未登记口岸的路径：有一条过硬路径该侧即过，不被坏行拖住。
	entries.Paths = append(entries.Paths,
		pathRecord(t, "SYN-PATH-CN-EXPORT-BAD", "SYN-PORT-GHOST-01", "EXPORT", "SYN-MODE-MANIFEST-01", opensAt("2026-01-01"), time.Time{}))
	judgment := fold(t, candidateRef(t, "cand-altpath"), "CN", true, "SG", true, entries)
	if judgment.Outcome() != CustomsAvailable {
		t.Fatalf("outcome = %q，坏行不该拦下另一条过硬路径", judgment.Outcome().String())
	}
}

func TestFailedSideReasonIsDeterministic(t *testing.T) {
	entries := applicableEntries(t)
	entries.Ports = entries.Ports[1:] // SZX 未登记；出口侧两条路径都经坏口岸
	entries.Paths = append(entries.Paths,
		pathRecord(t, "SYN-PATH-CN-EXPORT-00", "SYN-PORT-GHOST-02", "EXPORT", "SYN-MODE-MANIFEST-01", opensAt("2026-01-01"), time.Time{}))
	first := fold(t, candidateRef(t, "cand-deterministic"), "CN", true, "SG", true, entries)
	second := fold(t, candidateRef(t, "cand-deterministic"), "CN", true, "SG", true, entries)
	if first.UnavailableReason() != PortNotRegistered {
		t.Fatalf("reason = %q，两条路径都坏口岸，该仍答口岸未登记", first.UnavailableReason().String())
	}
	if port, _ := first.UnavailablePort(); port.String() != "SYN-PORT-GHOST-02" {
		t.Fatalf("port = %q，该按路径引用排序取第一条失败路径的口岸（-00 排在 -01 前）", port.String())
	}
	if first.JudgmentID() != second.JudgmentID() {
		t.Fatal("同一份快照两次折叠的判断标识必须相同")
	}
}

func TestSameInputFoldsIdenticalJudgments(t *testing.T) {
	entries := applicableEntries(t)
	candidate := candidateRef(t, "cand-repeat")
	first := fold(t, candidate, "CN", true, "SG", true, entries)
	second := fold(t, candidate, "CN", true, "SG", true, entries)
	if first.Outcome() != second.Outcome() ||
		first.JudgmentID() != second.JudgmentID() ||
		!reflect.DeepEqual(first.Versions(), second.Versions()) {
		t.Fatal("同租户、同时点、同候选、同目录，重复作答必须逐字节一致（判据三）")
	}
}

func TestJudgmentIDTracksCatalogAndCandidate(t *testing.T) {
	candidate := candidateRef(t, "cand-id")
	base := fold(t, candidate, "CN", true, "SG", true, applicableEntries(t))

	byCandidate := fold(t, candidateRef(t, "cand-other"), "CN", true, "SG", true, applicableEntries(t))
	if byCandidate.JudgmentID() == base.JudgmentID() {
		t.Fatal("换候选必须换判断标识")
	}

	changed := applicableEntries(t)
	changed.Ports = append(changed.Ports, portRecord(t, "SYN-PORT-SH-01", opensAt("2026-06-01"), time.Time{}))
	byCatalog := fold(t, candidate, "CN", true, "SG", true, changed)
	if byCatalog.JudgmentID() == base.JudgmentID() {
		t.Fatal("目录多一行必须换判断标识——出处指到新依据上")
	}
}

func TestUnreadableFoldCarriesJudgmentIDWithoutVersions(t *testing.T) {
	judgment, err := FoldCustomsApplicabilityUnreadable(
		applicabilityTenant, candidateRef(t, "cand-unreadable"), applicabilityAsOf)
	if err != nil {
		t.Fatalf("FoldCustomsApplicabilityUnreadable: %v", err)
	}
	if judgment.Outcome() != CustomsStatusUnknown || judgment.UnknownReason() != CatalogUnreadable {
		t.Fatalf("outcome/unknown = %q/%q，依赖读不到该折成状态未知", judgment.Outcome().String(), judgment.UnknownReason().String())
	}
	if judgment.JudgmentID() == "" || len(judgment.Versions()) != 0 {
		t.Fatal("读不到时出处只有判断标识，没有目录版本可引")
	}
	again, err := FoldCustomsApplicabilityUnreadable(
		applicabilityTenant, candidateRef(t, "cand-unreadable"), applicabilityAsOf)
	if err != nil || again.JudgmentID() != judgment.JudgmentID() {
		t.Fatal("同租户、同时点、同候选的重算判断标识必须一致")
	}
}

func TestFoldRefusesDegenerateInputs(t *testing.T) {
	candidate := candidateRef(t, "cand-degenerate")
	if _, err := FoldCustomsApplicability(
		TenantID{}, candidate, "CN", true, "SG", true, applicabilityAsOf, applicableEntries(t)); err == nil {
		t.Fatal("租户空值必须被拒，而不是折出一份假答案")
	}
	if _, err := FoldCustomsApplicability(
		applicabilityTenant, RouteCandidateReference{}, "CN", true, "SG", true, applicabilityAsOf, applicableEntries(t)); err == nil {
		t.Fatal("候选引用空值必须被拒")
	}
	if _, err := FoldCustomsApplicability(
		applicabilityTenant, candidate, "CN", true, "SG", true, time.Time{}, applicableEntries(t)); err == nil {
		t.Fatal("判断时点零值必须被拒")
	}
}

func TestJudgmentShapeValidationRejectsIncoherentLoads(t *testing.T) {
	candidate := candidateRef(t, "cand-shape")

	coherent := CustomsApplicabilityJudgmentSpec{
		Candidate: candidate, Outcome: CustomsAvailable, JudgmentID: "id-1",
	}
	if _, err := newCustomsApplicabilityJudgment(coherent); err != nil {
		t.Fatalf("可用格不该被拒：%v", err)
	}

	coherent.Reason = PortNotRegistered
	if _, err := newCustomsApplicabilityJudgment(coherent); err == nil {
		t.Fatal("可用却带不可用理由，形状拼不拢必须被拒")
	}

	unavailable := CustomsApplicabilityJudgmentSpec{
		Candidate: candidate, Outcome: CustomsUnavailable, Reason: PathDirectionNotCovered,
		Direction: ImportManifest, JudgmentID: "id-2",
	}
	if _, err := newCustomsApplicabilityJudgment(unavailable); err != nil {
		t.Fatalf("方向格正当负载不该被拒：%v", err)
	}
	unavailable.Direction = ManifestDirectionInvalid
	unavailable.Port, _ = NewCustomsPortReference("P-1")
	if _, err := newCustomsApplicabilityJudgment(unavailable); err == nil {
		t.Fatal("一条事实同时指名方向与口岸读不出它是哪一格，必须被拒")
	}

	unknown := CustomsApplicabilityJudgmentSpec{
		Candidate: candidate, Outcome: CustomsStatusUnknown, Unknown: EndpointCountryMissing,
		Side: EndpointSideOrigin, JudgmentID: "id-3",
	}
	if _, err := newCustomsApplicabilityJudgment(unknown); err != nil {
		t.Fatalf("缺国家码格正当负载不该被拒：%v", err)
	}
	unknown.Unknown = CatalogEmpty
	if _, err := newCustomsApplicabilityJudgment(unknown); err == nil {
		t.Fatal("目录为空却带端点侧负载，必须被拒")
	}

	blankVersion := CustomsApplicabilityJudgmentSpec{
		Candidate: candidate, Outcome: CustomsAvailable, JudgmentID: "id-4",
		Versions: []string{"PORT:P@2026-01-01T00:00:00Z", " "},
	}
	if _, err := newCustomsApplicabilityJudgment(blankVersion); err == nil {
		t.Fatal("目录版本引用里的空白行必须被拒")
	}

	noID := CustomsApplicabilityJudgmentSpec{Candidate: candidate, Outcome: CustomsAvailable}
	if _, err := newCustomsApplicabilityJudgment(noID); err == nil {
		t.Fatal("没有判断标识的答案读不出出处，必须被拒")
	}
}
