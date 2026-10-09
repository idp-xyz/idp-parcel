package postgres_test

import (
	"context"
	"slices"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 票 routing-first-cut/12 完成判据二、三的对照检查。断言只从票面判据与 CC CONTEXT
// 「关务适用性判断」各条规则推出；登记走生产写口，作答走生产快照读口与判断服务。各情形
// 各用一个合成租户、共用一个库，于是租户隔离也一并受检——别的租户在册，不能让目录为空
// 的租户答可用。

var criterionAsOf = time.Date(2026, 10, 9, 4, 0, 0, 0, time.UTC)

type criterionHarness struct {
	fixture  *viewFixture
	registry *adapter.PortsPathsRegistrations
	handler  *application.CustomsApplicabilityHandler
}

func newCriterionHarness(t *testing.T) *criterionHarness {
	t.Helper()
	fixture := newViewFixture(t)
	registry, err := adapter.NewPortsPathsRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造口岸路径写口：%v", err)
	}
	snapshots, err := adapter.NewPortsPathsSnapshots(fixture.db)
	if err != nil {
		t.Fatalf("构造口岸路径快照读口：%v", err)
	}
	return &criterionHarness{
		fixture:  fixture,
		registry: registry,
		handler: application.NewCustomsApplicabilityHandler(
			application.CustomsApplicabilityDeps{Catalog: snapshots}),
	}
}

func (harness *criterionHarness) port(t *testing.T, tenant, port string, appliesFrom time.Time) {
	t.Helper()
	harness.mustRegister(t, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return harness.registry.RegisterCandidatePort(ctx,
			viewValue(t, domain.NewTenantID, tenant),
			viewValue(t, domain.NewCustomsPortReference, port), appliesFrom)
	})
}

func (harness *criterionHarness) path(
	t *testing.T,
	tenant, path, port string,
	direction domain.ManifestDirection,
	appliesFrom time.Time,
) {
	t.Helper()
	harness.mustRegister(t, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return harness.registry.RegisterDeclarationPath(ctx,
			viewValue(t, domain.NewTenantID, tenant),
			viewValue(t, domain.NewDeclarationPathReference, path),
			synRoute(t, port, "SYN-MODE-01", direction), appliesFrom)
	})
}

// fullCatalog 给租户登一套两侧都过得去的目录：出口、进口各一条生效路径，所经口岸在册且生效。
func (harness *criterionHarness) fullCatalog(t *testing.T, tenant string) {
	t.Helper()
	past := criterionAsOf.AddDate(0, 0, -30)
	harness.port(t, tenant, "SYN-PORT-CN", past)
	harness.port(t, tenant, "SYN-PORT-SG", past)
	harness.path(t, tenant, "SYN-PATH-EXP", "SYN-PORT-CN", domain.ExportManifest, past)
	harness.path(t, tenant, "SYN-PATH-IMP", "SYN-PORT-SG", domain.ImportManifest, past)
}

func (harness *criterionHarness) mustRegister(
	t *testing.T,
	call func(context.Context) (ports.CaseConfigurationSaveOutcome, error),
) {
	t.Helper()
	outcome, err := register(t, harness.fixture, call)
	if err != nil {
		t.Fatalf("登记目录行：%v", err)
	}
	if outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("登记目录行回执 = %v，要 CaseConfigurationRegistered：夹具没写进去，后面的断言证不了什么", outcome)
	}
}

func (harness *criterionHarness) judge(
	t *testing.T,
	tenant string,
	candidates ...ports.CustomsApplicabilityCandidate,
) map[string]domain.CustomsApplicabilityJudgment {
	t.Helper()
	return harness.judgeAt(t, tenant, criterionAsOf, candidates...)
}

// judgeAt 按候选引用收答案：逐候选作答，漏答、多答或答非所问都直接判失败。
func (harness *criterionHarness) judgeAt(
	t *testing.T,
	tenant string,
	asOf time.Time,
	candidates ...ports.CustomsApplicabilityCandidate,
) map[string]domain.CustomsApplicabilityJudgment {
	t.Helper()
	judgments, err := harness.handler.Handle(t.Context(), ports.CustomsApplicabilityQuery{
		Tenant:     viewValue(t, domain.NewTenantID, tenant),
		AsOf:       asOf,
		Candidates: candidates,
	})
	if err != nil {
		t.Fatalf("作答：%v", err)
	}
	if len(judgments) != len(candidates) {
		t.Fatalf("作答 %d 份，交来候选 %d 条", len(judgments), len(candidates))
	}
	byCandidate := make(map[string]domain.CustomsApplicabilityJudgment, len(judgments))
	for _, judgment := range judgments {
		byCandidate[judgment.Candidate().String()] = judgment
	}
	for _, candidate := range candidates {
		if _, ok := byCandidate[candidate.Candidate.String()]; !ok {
			t.Fatalf("候选 %s 没有得到作答", candidate.Candidate)
		}
	}
	return byCandidate
}

// criterionCandidate 造一条候选投影；国家/地区码给空串即那一端缺码。
func criterionCandidate(t *testing.T, id, origin, destination string) ports.CustomsApplicabilityCandidate {
	t.Helper()
	return ports.CustomsApplicabilityCandidate{
		Candidate:      viewValue(t, domain.NewRouteCandidateReference, id),
		Origin:         origin,
		HasOrigin:      origin != "",
		Destination:    destination,
		HasDestination: destination != "",
	}
}

func expectAvailable(t *testing.T, label string, judgment domain.CustomsApplicabilityJudgment) {
	t.Helper()
	if judgment.Outcome() != domain.CustomsAvailable {
		t.Errorf("%s：结论 %v（不可用理由 %v，未知理由 %v），要 AVAILABLE",
			label, judgment.Outcome(), judgment.UnavailableReason(), judgment.UnknownReason())
	}
	expectJudgmentID(t, label, judgment)
}

func expectUnavailablePort(
	t *testing.T,
	label string,
	judgment domain.CustomsApplicabilityJudgment,
	reason domain.CustomsUnavailableReason,
	port string,
) {
	t.Helper()
	if judgment.Outcome() != domain.CustomsUnavailable || judgment.UnavailableReason() != reason {
		t.Errorf("%s：结论 %v / 理由 %v，要 UNAVAILABLE / %v",
			label, judgment.Outcome(), judgment.UnavailableReason(), reason)
	}
	if named, ok := judgment.UnavailablePort(); !ok || named.String() != port {
		t.Errorf("%s：指名口岸 %q（在场 %v），要 %q", label, named, ok, port)
	}
	expectCatalogCitation(t, label, judgment)
}

func expectUnavailableDirection(
	t *testing.T,
	label string,
	judgment domain.CustomsApplicabilityJudgment,
	direction domain.ManifestDirection,
) {
	t.Helper()
	if judgment.Outcome() != domain.CustomsUnavailable || judgment.UnavailableReason() != domain.PathDirectionNotCovered {
		t.Errorf("%s：结论 %v / 理由 %v，要 UNAVAILABLE / PATH_DIRECTION_NOT_COVERED",
			label, judgment.Outcome(), judgment.UnavailableReason())
	}
	if named, ok := judgment.UnavailableDirection(); !ok || named != direction {
		t.Errorf("%s：指名方向 %v（在场 %v），要 %v", label, named, ok, direction)
	}
	expectCatalogCitation(t, label, judgment)
}

func expectUnknown(
	t *testing.T,
	label string,
	judgment domain.CustomsApplicabilityJudgment,
	reason domain.CustomsUnknownReason,
) {
	t.Helper()
	if judgment.Outcome() != domain.CustomsStatusUnknown || judgment.UnknownReason() != reason {
		t.Errorf("%s：结论 %v / 未知理由 %v，要 STATUS_UNKNOWN / %v",
			label, judgment.Outcome(), judgment.UnknownReason(), reason)
	}
	expectJudgmentID(t, label, judgment)
}

func expectMissingSide(t *testing.T, label string, judgment domain.CustomsApplicabilityJudgment, side domain.CustomsEndpointSide) {
	t.Helper()
	expectUnknown(t, label, judgment, domain.EndpointCountryMissing)
	if named, ok := judgment.EndpointSide(); !ok || named != side {
		t.Errorf("%s：指名缺码一端 %v（在场 %v），要 %v", label, named, ok, side)
	}
}

func expectJudgmentID(t *testing.T, label string, judgment domain.CustomsApplicabilityJudgment) {
	t.Helper()
	if judgment.JudgmentID() == "" {
		t.Errorf("%s：答案没有判断标识，出处缺席", label)
	}
}

// expectCatalogCitation 用于凭目录行得出的答案：没有目录版本引用的可用或不可用无从复核当时凭的是哪一版目录。
func expectCatalogCitation(t *testing.T, label string, judgment domain.CustomsApplicabilityJudgment) {
	t.Helper()
	expectJudgmentID(t, label, judgment)
	if len(judgment.Versions()) == 0 {
		t.Errorf("%s：凭目录作答却没有目录版本引用", label)
	}
}

// Covers: 判据二——合成目录上可用、不可用三因（口岸未登记、口岸未生效、路径方向对不上）、
// 状态未知（目录为空、缺国家码）各得其格；状态未知不折成可用。
func TestApplicabilityCriterionRealCatalogAnswersEveryCell(t *testing.T) {
	harness := newCriterionHarness(t)
	past := criterionAsOf.AddDate(0, 0, -30)
	future := criterionAsOf.AddDate(0, 0, 10)

	harness.fullCatalog(t, "SYN-TENANT-OK")

	// 出口路径指名的口岸在口岸目录里任何时点都没有行。
	harness.port(t, "SYN-TENANT-UNREG", "SYN-PORT-SG", past)
	harness.path(t, "SYN-TENANT-UNREG", "SYN-PATH-EXP", "SYN-PORT-CN", domain.ExportManifest, past)
	harness.path(t, "SYN-TENANT-UNREG", "SYN-PATH-IMP", "SYN-PORT-SG", domain.ImportManifest, past)

	// 出口路径所经口岸在册，但版本在判断时点之后才起。
	harness.port(t, "SYN-TENANT-LATE", "SYN-PORT-CN", future)
	harness.port(t, "SYN-TENANT-LATE", "SYN-PORT-SG", past)
	harness.path(t, "SYN-TENANT-LATE", "SYN-PATH-EXP", "SYN-PORT-CN", domain.ExportManifest, past)
	harness.path(t, "SYN-TENANT-LATE", "SYN-PATH-IMP", "SYN-PORT-SG", domain.ImportManifest, past)

	// 进口一侧一条路径都没有。
	harness.port(t, "SYN-TENANT-NOIMP", "SYN-PORT-CN", past)
	harness.path(t, "SYN-TENANT-NOIMP", "SYN-PATH-EXP", "SYN-PORT-CN", domain.ExportManifest, past)

	// 进口路径在册，但在判断时点尚未生效：未生效的路径不覆盖它的方向。
	harness.port(t, "SYN-TENANT-IMPLATE", "SYN-PORT-CN", past)
	harness.port(t, "SYN-TENANT-IMPLATE", "SYN-PORT-SG", past)
	harness.path(t, "SYN-TENANT-IMPLATE", "SYN-PATH-EXP", "SYN-PORT-CN", domain.ExportManifest, past)
	harness.path(t, "SYN-TENANT-IMPLATE", "SYN-PATH-IMP", "SYN-PORT-SG", domain.ImportManifest, future)

	// SYN-TENANT-EMPTY 什么都不登记。

	crossing := criterionCandidate(t, "SYN-CANDIDATE-CN-SG", "CN", "SG")
	domestic := criterionCandidate(t, "SYN-CANDIDATE-CN-CN", "CN", "CN")
	noDestination := criterionCandidate(t, "SYN-CANDIDATE-CN-NONE", "CN", "")
	noOrigin := criterionCandidate(t, "SYN-CANDIDATE-NONE-SG", "", "SG")
	neither := criterionCandidate(t, "SYN-CANDIDATE-NONE-NONE", "", "")

	ok := harness.judge(t, "SYN-TENANT-OK", crossing, domestic, noDestination, noOrigin, neither)
	expectAvailable(t, "两侧路径与口岸齐全", ok[crossing.Candidate.String()])
	expectCatalogCitation(t, "两侧路径与口岸齐全", ok[crossing.Candidate.String()])
	expectAvailable(t, "两端同国", ok[domestic.Candidate.String()])
	expectMissingSide(t, "收件国缺码", ok[noDestination.Candidate.String()], domain.EndpointSideDestination)
	expectMissingSide(t, "寄件国缺码", ok[noOrigin.Candidate.String()], domain.EndpointSideOrigin)
	expectMissingSide(t, "两端都缺码", ok[neither.Candidate.String()], domain.EndpointSideBoth)

	expectUnavailablePort(t, "出口路径所经口岸未登记",
		harness.judge(t, "SYN-TENANT-UNREG", crossing)[crossing.Candidate.String()],
		domain.PortNotRegistered, "SYN-PORT-CN")
	expectUnavailablePort(t, "出口路径所经口岸未生效",
		harness.judge(t, "SYN-TENANT-LATE", crossing)[crossing.Candidate.String()],
		domain.PortNotEffective, "SYN-PORT-CN")
	expectUnavailableDirection(t, "进口一侧没有路径",
		harness.judge(t, "SYN-TENANT-NOIMP", crossing)[crossing.Candidate.String()],
		domain.ImportManifest)
	expectUnavailableDirection(t, "进口路径未生效",
		harness.judge(t, "SYN-TENANT-IMPLATE", crossing)[crossing.Candidate.String()],
		domain.ImportManifest)

	empty := harness.judge(t, "SYN-TENANT-EMPTY", crossing, domestic, noDestination)
	expectUnknown(t, "目录为空", empty[crossing.Candidate.String()], domain.CatalogEmpty)
	expectAvailable(t, "目录为空时两端同国", empty[domestic.Candidate.String()])
	expectMissingSide(t, "目录为空时收件国缺码", empty[noDestination.Candidate.String()], domain.EndpointSideDestination)
}

// Covers: 判据二「依赖读不到」——快照读口报错时判断服务照常交回答案，跨境候选答状态未知，
// 不以错误上抛，也不折成可用。
func TestApplicabilityCriterionUnreadableCatalogIsStatusUnknownNotAnError(t *testing.T) {
	harness := newCriterionHarness(t)
	harness.fullCatalog(t, "SYN-TENANT-OK")
	crossing := criterionCandidate(t, "SYN-CANDIDATE-CN-SG", "CN", "SG")

	harness.fixture.pool.Close()

	expectUnknown(t, "目录读不到",
		harness.judge(t, "SYN-TENANT-OK", crossing)[crossing.Candidate.String()],
		domain.CatalogUnreadable)
}

// Covers: CONTEXT「关务适用性判断」两端同国不要求口岸与申报路径在场、缺国家码向发起方要——
// 这两格的答案本不依赖目录，目录读不到时与目录读得到时同答；把它们折成「目录读不到」会让
// 境内候选随 CC 的依赖故障一起停摆，也会让人去催运维修一件修好了也解不开的缺码。
func TestApplicabilityCriterionCellsNeedingNoCatalogIgnoreItsReadability(t *testing.T) {
	harness := newCriterionHarness(t)
	harness.fullCatalog(t, "SYN-TENANT-OK")
	domestic := criterionCandidate(t, "SYN-CANDIDATE-CN-CN", "CN", "CN")
	noDestination := criterionCandidate(t, "SYN-CANDIDATE-CN-NONE", "CN", "")

	harness.fixture.pool.Close()

	unreadable := harness.judge(t, "SYN-TENANT-OK", domestic, noDestination)
	expectAvailable(t, "目录读不到时两端同国", unreadable[domestic.Candidate.String()])
	expectMissingSide(t, "目录读不到时收件国缺码", unreadable[noDestination.Candidate.String()], domain.EndpointSideDestination)
}

// Covers: 判据三——同一租户、同一时点、同一候选投影重复作答一致，答案带出处；判断标识随
// 目录版本、时点与候选而变，换版后的答案不沿用上一版的标识。
func TestApplicabilityCriterionRepeatedJudgmentIsIdenticalAndBoundToItsInputs(t *testing.T) {
	harness := newCriterionHarness(t)
	harness.fullCatalog(t, "SYN-TENANT-OK")
	crossing := criterionCandidate(t, "SYN-CANDIDATE-CN-SG", "CN", "SG")
	key := crossing.Candidate.String()

	first := harness.judge(t, "SYN-TENANT-OK", crossing)[key]
	second := harness.judge(t, "SYN-TENANT-OK", crossing)[key]
	expectAvailable(t, "首答", first)
	expectCatalogCitation(t, "首答", first)
	expectSameJudgment(t, "同输入重答", first, second)

	otherCandidate := criterionCandidate(t, "SYN-CANDIDATE-CN-SG-2", "CN", "SG")
	if got := harness.judge(t, "SYN-TENANT-OK", otherCandidate)[otherCandidate.Candidate.String()]; got.JudgmentID() == first.JudgmentID() {
		t.Errorf("另一候选沿用了同一判断标识 %q", got.JudgmentID())
	}
	if got := harness.judgeAt(t, "SYN-TENANT-OK", criterionAsOf.Add(time.Hour), crossing)[key]; got.JudgmentID() == first.JudgmentID() {
		t.Errorf("另一时点沿用了同一判断标识 %q", got.JudgmentID())
	}

	// 出口路径换版，判断时点落进新版：所依目录版本变了，出处必须跟着变。
	harness.path(t, "SYN-TENANT-OK", "SYN-PATH-EXP", "SYN-PORT-CN", domain.ExportManifest, criterionAsOf.AddDate(0, 0, -10))
	afterSupersede := harness.judge(t, "SYN-TENANT-OK", crossing)[key]
	expectAvailable(t, "换版后", afterSupersede)
	if afterSupersede.JudgmentID() == first.JudgmentID() {
		t.Errorf("出口路径换版后判断标识仍是 %q", first.JudgmentID())
	}
	if slices.Equal(afterSupersede.Versions(), first.Versions()) {
		t.Errorf("出口路径换版后目录版本引用仍是 %v", first.Versions())
	}
	expectSameJudgment(t, "换版后同输入重答", afterSupersede, harness.judge(t, "SYN-TENANT-OK", crossing)[key])
}

// Covers: 判据三「答案带出处」——出处只交判断标识与目录版本引用，所以理由不同的两份答案
// 不能共用一份出处。目录为空与目录读不到是唯一一对目录版本清单必然相同（皆空）的答案，
// 判断标识也撞上的话，复核按出处重算只会算出「目录为空」，把一次依赖故障改写成租户没登记。
func TestApplicabilityCriterionEmptyAndUnreadableCatalogsCiteDifferently(t *testing.T) {
	harness := newCriterionHarness(t)
	crossing := criterionCandidate(t, "SYN-CANDIDATE-CN-SG", "CN", "SG")
	key := crossing.Candidate.String()

	empty := harness.judge(t, "SYN-TENANT-EMPTY", crossing)[key]
	expectUnknown(t, "目录为空", empty, domain.CatalogEmpty)

	harness.fixture.pool.Close()

	unreadable := harness.judge(t, "SYN-TENANT-EMPTY", crossing)[key]
	expectUnknown(t, "目录读不到", unreadable, domain.CatalogUnreadable)
	if unreadable.JudgmentID() == empty.JudgmentID() && slices.Equal(unreadable.Versions(), empty.Versions()) {
		t.Errorf("目录读不到与目录为空交回同一份出处（判断标识 %q、目录版本 %v）", empty.JudgmentID(), empty.Versions())
	}
}

func expectSameJudgment(t *testing.T, label string, want, got domain.CustomsApplicabilityJudgment) {
	t.Helper()
	if got.Outcome() != want.Outcome() ||
		got.UnavailableReason() != want.UnavailableReason() ||
		got.UnknownReason() != want.UnknownReason() ||
		got.JudgmentID() != want.JudgmentID() ||
		!slices.Equal(got.Versions(), want.Versions()) {
		t.Errorf("%s：两次作答不一致\n首次 %v/%v/%v %q %v\n再次 %v/%v/%v %q %v", label,
			want.Outcome(), want.UnavailableReason(), want.UnknownReason(), want.JudgmentID(), want.Versions(),
			got.Outcome(), got.UnavailableReason(), got.UnknownReason(), got.JudgmentID(), got.Versions())
	}
}
