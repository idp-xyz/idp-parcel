package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证按后继收尾旧版的两口（ADR-0233，票 demo-chain-gaps/50）：同范围新版登记后旧版未收尾时
// 闭包答`适用冲突`；截止后按时点两边各唯一、同起点退役后只选后继；已固定的解析记录不被改写；重放不重登；不成立的收尾
// 一行不写；客户服务规则这一类写得进更正册（迁移 0040）；同一旧版上并发的两次截止按版本行锁排队；供应商协议两个点读口
// 与内部成本政策点读口折进截止与退役。

var (
	cutRowPredecessorStart = time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	cutRowSuccessorStart   = time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	cutRowLaterStart       = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	cutRowEnd              = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	cutRowCorrectedAt      = time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	cutRowRegisteredAt     = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	cutRowBefore           = time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	cutRowAfter            = time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
)

// Covers: `AT-PC-054` 在真库上的那一半——现状复现、截止、按时点两边各唯一、原版本与已固定的解析不被改写、修订推进。
func TestASuccessorCutEndsTheConflictInTheStore(t *testing.T) {
	fixture := newCutFixture(t)
	ctx := t.Context()
	fixture.save(t, cutVersion(t, domain.AcceptanceRulePackageObject, "rules-1", "v1", cutRowPredecessorStart))
	old := cutVersion(t, domain.CustomerContractObject, "contract-1", "v1", cutRowPredecessorStart)
	fixture.save(t, old)

	// 新版发布之前固定的那一次解析：它是历史，截止不得改写它。
	fixed := fixture.closureAt(t, cutRowBefore)
	if fixed.Outcome() != domain.UniquelyResolved {
		t.Fatalf("新版发布前 outcome = %q, want UNIQUELY_RESOLVED", fixed.Outcome())
	}
	mustSaveResolution(t, fixture.transactor, ctx, fixture.resolutions, fixed)

	next := cutVersion(t, domain.CustomerContractObject, "contract-1", "v2", cutRowSuccessorStart)
	fixture.save(t, next)
	if got := fixture.closureAt(t, cutRowAfter); got.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("旧版未截止时 outcome = %q, want APPLICABILITY_CONFLICT（现状复现）", got.Outcome())
	}
	viewBefore := fixture.viewRevision(t)

	result := fixture.cut(t, cutCommand(t, old, "v2", "SYN-CUT-1"))
	if result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("cut outcome = %q (cause %v), want REGISTERED", result.Outcome(), result.Cause())
	}
	correction, _ := result.Correction()
	if end, bounded := correction.CorrectedInterval().EndsAt(); !bounded || !end.Equal(cutRowSuccessorStart) {
		t.Fatalf("截止点 = %v, want %v", end, cutRowSuccessorStart)
	}

	for at, want := range map[time.Time]string{cutRowAfter: "v2", cutRowSuccessorStart: "v2", cutRowBefore: "v1"} {
		got := fixture.closureAt(t, at)
		if got.Outcome() != domain.UniquelyResolved {
			t.Fatalf("at %v outcome = %q, want UNIQUELY_RESOLVED", at, got.Outcome())
		}
		adopted, _ := got.AdoptedFor(domain.CustomerContractObject)
		if adopted.Version().Version().String() != want {
			t.Fatalf("at %v adopted contract = %q, want %q", at, adopted.Version().Version(), want)
		}
	}
	if fixture.viewRevision(t) == viewBefore {
		t.Fatal("截止后修订标识没有推进")
	}

	registry := fixture.load(t)
	stored, _ := registry.Lookup(old.Tenant(), old.Kind(), old.ObjectID(), old.Version())
	if stored.Effective() != old.Effective() || stored.Status() != domain.CommercialVersionEffective {
		t.Fatal("截止改写了旧版在版本册上的原区间或生命周期位置")
	}

	reloaded, found, err := fixture.resolutions.LoadResolution(ctx, old.Tenant(), fixed.ResolutionID())
	if err != nil || !found {
		t.Fatalf("已固定的解析读不回：found=%v err=%v", found, err)
	}
	adopted, _ := reloaded.AdoptedFor(domain.CustomerContractObject)
	if reloaded.ResolutionID() != fixed.ResolutionID() || adopted.Version().Version() != old.Version() {
		t.Fatal("截止改写了已固定的解析记录")
	}
	if stale := domain.ValidateClosureBeforeDecision(registry, fixed, nil); stale.Outcome() != domain.ResolutionStale {
		t.Fatalf("已固定的解析提交前重校 outcome = %q, want RESOLUTION_STALE——视图已变，要重解", stale.Outcome())
	}
}

// Covers: `AT-PC-056` 截止在真库上的那一半——同一次截止重放答已登记，册上仍只一条，修订不变。
func TestReplayingASuccessorCutWritesNothingMore(t *testing.T) {
	fixture := newCutFixture(t)
	old := cutVersion(t, domain.CustomerContractObject, "contract-1", "v1", cutRowPredecessorStart)
	fixture.save(t, old)
	fixture.save(t, cutVersion(t, domain.CustomerContractObject, "contract-1", "v2", cutRowSuccessorStart))

	command := cutCommand(t, old, "v2", "SYN-CUT-1")
	if first := fixture.cut(t, command); first.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("first outcome = %q (cause %v)", first.Outcome(), first.Cause())
	}
	view := fixture.viewRevision(t)
	replay := fixture.cut(t, command)
	if replay.Outcome() != application.VersionClosureAlreadyRegistered {
		t.Fatalf("replay outcome = %q (cause %v), want ALREADY_REGISTERED", replay.Outcome(), replay.Cause())
	}
	if got := fixture.correctionRows(t); got != 1 {
		t.Fatalf("更正册 %d 行, want 1——重放追加了一条", got)
	}
	if fixture.viewRevision(t) != view {
		t.Fatal("重放推动了修订标识")
	}
}

// Covers: `AT-PC-057` 截止在真库上的那一半——后继在另一对象下、后继与旧版同一起点、后继不在这一范围、旧版不在本租户，
// 均答未受理且更正册一行不写。
func TestASuccessorCutThatDoesNotHoldWritesNothing(t *testing.T) {
	fixture := newCutFixture(t)
	old := cutVersion(t, domain.CustomerContractObject, "contract-1", "v1", cutRowPredecessorStart)
	fixture.save(t, old)
	fixture.save(t, cutVersion(t, domain.CustomerContractObject, "contract-2", "v2", cutRowSuccessorStart))
	fixture.save(t, cutVersion(t, domain.CustomerContractObject, "contract-1", "v-same-start", cutRowPredecessorStart))

	otherScope := cutCommand(t, old, "v-same-start", "SYN-CUT-1")
	otherScope.Scope = pcValue(t, domain.NewCommercialScopeReference, "scope-other")
	otherTenant := cutCommand(t, old, "v-same-start", "SYN-CUT-1")
	otherTenant.Tenant = pcTenant(t, "tenant-2")

	for name, tc := range map[string]struct {
		command application.CutCommercialVersionAtSuccessorCommand
		want    error
	}{
		"后继只在另一对象下":        {command: cutCommand(t, old, "v2", "SYN-CUT-1"), want: domain.ErrClosureVersionNotRegistered},
		"后继与旧版同一起点：走同起点退役": {command: cutCommand(t, old, "v-same-start", "SYN-CUT-1"), want: domain.ErrSuccessorCutOutsideInterval},
		"指名的范围不是这两版的范围":    {command: otherScope, want: domain.ErrClosureVersionNotRegistered},
		"旧版不在本租户":          {command: otherTenant, want: domain.ErrClosureVersionNotRegistered},
	} {
		t.Run(name, func(t *testing.T) {
			result := fixture.cut(t, tc.command)
			if result.Outcome() != application.VersionClosureNotAccepted || !errors.Is(result.Cause(), tc.want) {
				t.Fatalf("outcome = %q cause = %v, want NOT_ACCEPTED / %v", result.Outcome(), result.Cause(), tc.want)
			}
			if got := fixture.correctionRows(t); got != 0 {
				t.Fatalf("更正册 %d 行, want 0", got)
			}
		})
	}
}

// Covers: 迁移 party_commercial/0040——客户服务规则这一类的截止写得进更正册，整册读回后挂产品那一层按时点唯一。
func TestACustomerServiceRuleCutLandsInTheStore(t *testing.T) {
	fixture := newCutFixture(t)
	old := cutVersion(t, domain.CustomerServiceRuleObject, "service-rule-1", "v1", cutRowPredecessorStart)
	fixture.save(t, old)
	fixture.save(t, cutVersion(t, domain.CustomerServiceRuleObject, "service-rule-1", "v2", cutRowSuccessorStart))

	result := fixture.cut(t, cutCommand(t, old, "v2", "SYN-CUT-RULE"))
	if result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("outcome = %q (cause %v), want REGISTERED", result.Outcome(), result.Cause())
	}
	anchor, err := domain.NewSelectionAnchor(cutRowAfter, pcValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatal(err)
	}
	version, outcome := fixture.load(t).CustomerServiceRuleProductBase(old.Tenant(), old.Scope(), anchor, domain.CommercialObjectID{})
	if outcome != domain.UniquelyResolved || version.Version().String() != "v2" {
		t.Fatalf("产品层 = %q / %q, want UNIQUELY_RESOLVED / v2", outcome, version.Version())
	}
}

// Covers: `ports.VersionClosureRegistry` 的锁先于读——同一旧版上两次截止并发：先到者截在 v2 的生效时点并握着版本行锁，后到者
// 停在锁上；先到者提交后，后到者读到已截止的区间，v3 的生效时点已不在其内，答未受理。若判在锁外，后到者会按未截止
// 的快照判过、在写入那一刻排队后照样落下，成为选用更正，两版在 v2 与 v3 之间重新重叠。
func TestConcurrentCutsOnOnePredecessorQueueOnItsRow(t *testing.T) {
	fixture := newCutFixture(t)
	ctx := t.Context()
	old := cutVersion(t, domain.CustomerContractObject, "contract-1", "v1", cutRowPredecessorStart)
	fixture.save(t, old)
	fixture.save(t, cutVersion(t, domain.CustomerContractObject, "contract-1", "v2", cutRowSuccessorStart))
	fixture.save(t, cutVersion(t, domain.CustomerContractObject, "contract-1", "v3", cutRowLaterStart))

	firstCut := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	var first, second application.VersionClosureResult

	go func() {
		firstDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			first, err = fixture.handler.Cut(txCtx, cutCommand(t, old, "v2", "SYN-CUT-V2"))
			close(firstCut)
			<-release
			return err
		})
	}()
	<-firstCut
	go func() {
		secondDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			second, err = fixture.handler.Cut(txCtx, cutCommand(t, old, "v3", "SYN-CUT-V3"))
			return err
		})
	}()

	select {
	case err := <-secondDone:
		close(release)
		<-firstDone
		t.Fatalf("后到者在先到者提交前就返回了（err=%v）——没有停在旧版的版本行锁上", err)
	case <-time.After(150 * time.Millisecond):
	}
	close(release)

	if err := <-firstDone; err != nil {
		t.Fatalf("先到者事务失败：%v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("后到者事务失败：%v", err)
	}
	if first.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("先到者 outcome = %q (cause %v), want REGISTERED", first.Outcome(), first.Cause())
	}
	if second.Outcome() != application.VersionClosureNotAccepted || !errors.Is(second.Cause(), domain.ErrSuccessorCutOutsideInterval) {
		t.Fatalf("后到者 outcome = %q cause = %v, want NOT_ACCEPTED / ErrSuccessorCutOutsideInterval", second.Outcome(), second.Cause())
	}
	if got := fixture.correctionRows(t); got != 1 {
		t.Fatalf("更正册 %d 行, want 1", got)
	}
}

// ---- 夹具 ----

type cutFixture struct {
	db           *bentopg.DB
	publications *adapter.CommercialPublications
	resolutions  *adapter.CommercialResolutions
	transactor   bentoapp.Transactor
	pool         *pgxpool.Pool
	handler      *application.CloseCommercialVersionHandler
}

func newCutFixture(t *testing.T) cutFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	publications, err := adapter.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	resolutions, err := adapter.NewCommercialResolutions(db)
	if err != nil {
		t.Fatalf("构造解析册：%v", err)
	}
	return cutFixture{
		db:           db,
		publications: publications,
		resolutions:  resolutions,
		transactor:   db.Transactor(),
		pool:         pool,
		handler:      application.NewCloseCommercialVersionHandler(publications, fixedClock{at: cutRowRegisteredAt}),
	}
}

func (fixture cutFixture) save(t *testing.T, version domain.CommercialVersion) {
	t.Helper()
	mustSaveVersion(t, fixture.transactor, t.Context(), fixture.publications, version)
}

func (fixture cutFixture) cut(t *testing.T, command application.CutCommercialVersionAtSuccessorCommand) application.VersionClosureResult {
	t.Helper()
	var result application.VersionClosureResult
	mustWithinPublicationTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.handler.Cut(txCtx, command)
		return err
	})
	return result
}

func (fixture cutFixture) load(t *testing.T) *domain.CommercialRegistry {
	t.Helper()
	registry, err := fixture.publications.LoadForScope(t.Context(), pcTenant(t, "tenant-1"), pcScope(t))
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	return registry
}

func (fixture cutFixture) viewRevision(t *testing.T) string {
	t.Helper()
	return fixture.load(t).ViewRevision(pcTenant(t, "tenant-1"), pcScope(t)).String()
}

func (fixture cutFixture) closureAt(t *testing.T, at time.Time) domain.CommercialClosure {
	t.Helper()
	anchor, err := domain.NewSelectionAnchor(at, pcValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("选择锚点：%v", err)
	}
	return domain.ResolveCommercialClosure(fixture.load(t), domain.ClosureResolutionKey{
		TenantID:             pcTenant(t, "tenant-1"),
		CustomerAccountID:    pcValue(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                pcScope(t),
		Purpose:              domain.AcceptanceControlPurpose,
		Anchor:               anchor,
		RequiredBases:        []domain.CommercialObjectKind{domain.CustomerContractObject, domain.AcceptanceRulePackageObject},
	}, nil)
}

func (fixture cutFixture) correctionRows(t *testing.T) int {
	t.Helper()
	var rows int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM party_commercial.commercial_validity_correction`).Scan(&rows); err != nil {
		t.Fatalf("数更正册：%v", err)
	}
	return rows
}

func cutCommand(t *testing.T, old domain.CommercialVersion, successor, reference string) application.CutCommercialVersionAtSuccessorCommand {
	t.Helper()
	return application.CutCommercialVersionAtSuccessorCommand{
		VersionClosureTarget: closureTarget(t, old, successor),
		Reference:            pcValue(t, domain.NewValidityCorrectionReference, reference),
		CorrectedAt:          cutRowCorrectedAt,
	}
}

func retireCommand(t *testing.T, old domain.CommercialVersion, successor, reference string) application.RetireCommercialVersionAtSameStartCommand {
	t.Helper()
	return application.RetireCommercialVersionAtSameStartCommand{
		VersionClosureTarget: closureTarget(t, old, successor),
		Reference:            pcValue(t, domain.NewRetirementReference, reference),
	}
}

func closureTarget(t *testing.T, old domain.CommercialVersion, successor string) application.VersionClosureTarget {
	t.Helper()
	return application.VersionClosureTarget{
		Tenant:    old.Tenant(),
		Scope:     old.Scope(),
		Kind:      old.Kind(),
		ObjectID:  old.ObjectID(),
		Version:   old.Version(),
		Successor: pcValue(t, domain.NewCommercialVersionLabel, successor),
	}
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// cutVersion 经领域重建门造一份从 startsAt 起`已生效`的版本，终点统一在 cutRowEnd。客户合同照 effectiveVersionOfKind 指名
// rules-1，闭包据它核指名引用。
func cutVersion(t *testing.T, kind domain.CommercialObjectKind, objectID, label string, startsAt time.Time) domain.CommercialVersion {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(startsAt, cutRowEnd)
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := domain.NewApprovalBasis(
		pcValue(t, domain.NewApprovalReference, "approval-"+objectID+"-"+label),
		pcValue(t, domain.NewCommercialSourceReference, "source-1"),
		approvedAtFixture,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	spec := domain.RehydrateCommercialVersionSpec{
		TenantID:      pcTenant(t, "tenant-1"),
		Kind:          kind,
		ObjectID:      pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:       pcValue(t, domain.NewCommercialVersionLabel, label),
		Scope:         pcScope(t),
		ContentDigest: pcValue(t, domain.NewCommercialContentDigest, "digest-"+objectID+"-"+label),
		Effective:     interval,
		Status:        domain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   publishedAtRow,
		EffectiveAt:   startsAt,
	}
	if kind == domain.CustomerContractObject {
		spec.References = map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.AcceptanceRulePackageObject: pcValue(t, domain.NewCommercialObjectID, "rules-1"),
		}
	}
	version, err := domain.RehydrateCommercialVersion(spec)
	if err != nil {
		t.Fatalf("重建 %s 版本：%v", kind, err)
	}
	return version
}

// Covers: `AT-PC-055` 在真库上的那一半——同起点重发两版重叠成`适用冲突`，退役旧版后任何锚点只选后继；版本行快照不被
// 改写（列上仍是已生效），退役另册一行；已退役的旧版再截止答未受理。`AT-PC-056` 退役那一半——同一个决定重放答已登记、
// 回显原退役时刻、册上仍只一行；换一份决定引用再退答未受理。
func TestASameStartRetirementLeavesOnlyTheSuccessorInTheStore(t *testing.T) {
	fixture := newCutFixture(t)
	fixture.save(t, cutVersion(t, domain.AcceptanceRulePackageObject, "rules-1", "v1", cutRowPredecessorStart))
	old := cutVersion(t, domain.CustomerContractObject, "contract-1", "v1", cutRowPredecessorStart)
	fixture.save(t, old)
	fixture.save(t, cutVersion(t, domain.CustomerContractObject, "contract-1", "v2", cutRowPredecessorStart))
	if got := fixture.closureAt(t, cutRowAfter); got.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("同起点重发未收尾时 outcome = %q, want APPLICABILITY_CONFLICT（现状复现）", got.Outcome())
	}

	result := fixture.retire(t, retireCommand(t, old, "v2", "SYN-REISSUE-1"))
	if result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("retire outcome = %q (cause %v), want REGISTERED", result.Outcome(), result.Cause())
	}
	retirement, _ := result.Retirement()
	if !retirement.RetiredAt().Equal(cutRowRegisteredAt) || retirement.Successor().String() != "v2" {
		t.Fatalf("退役 = %+v, want 登记时刻与后继 v2", retirement)
	}
	for _, at := range []time.Time{cutRowBefore, cutRowAfter} {
		got := fixture.closureAt(t, at)
		adopted, _ := got.AdoptedFor(domain.CustomerContractObject)
		if got.Outcome() != domain.UniquelyResolved || adopted.Version().Version().String() != "v2" {
			t.Fatalf("at %v outcome = %q adopted = %q, want 唯一解析到 v2", at, got.Outcome(), adopted.Version().Version())
		}
	}
	stored, _ := fixture.load(t).Lookup(old.Tenant(), old.Kind(), old.ObjectID(), old.Version())
	if stored.Status() != domain.CommercialVersionRetired || stored.Effective() != old.Effective() {
		t.Fatalf("装载出的旧版 status = %q, want RETIRED 且原区间不变", stored.Status())
	}
	var column int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT status FROM party_commercial.commercial_version WHERE object_id = 'contract-1' AND version_label = 'v1'`).Scan(&column); err != nil {
		t.Fatal(err)
	}
	if domain.CommercialVersionStatus(column) != domain.CommercialVersionEffective {
		t.Fatalf("版本行的状态列被改写成 %d", column)
	}

	replay := fixture.retire(t, retireCommand(t, old, "v2", "SYN-REISSUE-1"))
	replayed, _ := replay.Retirement()
	if replay.Outcome() != application.VersionClosureAlreadyRegistered || !replayed.RetiredAt().Equal(cutRowRegisteredAt) {
		t.Fatalf("replay outcome = %q at = %v, want ALREADY_REGISTERED 与原退役时刻", replay.Outcome(), replayed.RetiredAt())
	}
	if other := fixture.retire(t, retireCommand(t, old, "v2", "SYN-REISSUE-OTHER")); other.Outcome() != application.VersionClosureNotAccepted ||
		!errors.Is(other.Cause(), domain.ErrVersionRetiredOtherwise) {
		t.Fatalf("换依据再退 outcome = %q cause = %v, want NOT_ACCEPTED / ErrVersionRetiredOtherwise", other.Outcome(), other.Cause())
	}
	if cut := fixture.cut(t, cutCommand(t, old, "v2", "SYN-CUT-1")); cut.Outcome() != application.VersionClosureNotAccepted ||
		!errors.Is(cut.Cause(), domain.ErrClosureNotEffective) {
		t.Fatalf("已退役的旧版再截止 outcome = %q cause = %v, want NOT_ACCEPTED / ErrClosureNotEffective", cut.Outcome(), cut.Cause())
	}
	var rows int
	if err := fixture.pool.QueryRow(t.Context(), `SELECT count(*) FROM party_commercial.commercial_version_retirement`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("退役册 %d 行, want 1", rows)
	}
}

// Covers: `AT-PC-058` 在真库上的 PC 那一半——供应商协议的版本点读口与正文点读口都折进截止与退役（代决复核 M16）：截止后
// 截止点之前支持采购、之后不支持；退役后版本点读交回`已退役`的壳、只交壳，正文点读的协议一律不支持、终止时刻读作退役时刻。
func TestSupplierAgreementPointReadsFoldTheVersionClosure(t *testing.T) {
	fixture := newCutFixture(t)
	ctx := t.Context()
	contents, err := adapter.NewSupplierAgreementContents(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	v1 := cutVersion(t, domain.SupplierAgreementObject, "agreement-1", "v1", cutRowPredecessorStart)
	v2 := cutVersion(t, domain.SupplierAgreementObject, "agreement-1", "v2", cutRowSuccessorStart)
	v3 := cutVersion(t, domain.SupplierAgreementObject, "agreement-1", "v3", cutRowSuccessorStart)
	for _, version := range []domain.CommercialVersion{v1, v2, v3} {
		fixture.saveAgreement(t, version)
	}

	if result := fixture.cut(t, cutCommand(t, v1, "v2", "SYN-CUT-AGREEMENT")); result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("cut outcome = %q (cause %v)", result.Outcome(), result.Cause())
	}
	read, err := contents.LoadSupplierAgreementVersion(ctx, v1.Tenant(), v1.ObjectID(), v1.Version())
	if err != nil || !read.HasAgreement {
		t.Fatalf("版本点读 v1：read=%+v err=%v", read, err)
	}
	if read.Agreement.SupportsProcurementAt(cutRowAfter) || !read.Agreement.SupportsProcurementAt(cutRowBefore) {
		t.Fatal("版本点读交回的 v1 没有按截止点分开支不支持采购")
	}
	content, found, err := contents.LoadSupplierAgreement(ctx, v1.Tenant(), v1)
	if err != nil || !found || content.SupportsProcurementAt(cutRowAfter) || !content.SupportsProcurementAt(cutRowBefore) {
		t.Fatalf("正文点读 v1 没有折进截止：found=%v err=%v", found, err)
	}

	if result := fixture.retire(t, retireCommand(t, v2, "v3", "SYN-REISSUE-AGREEMENT")); result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("retire outcome = %q (cause %v)", result.Outcome(), result.Cause())
	}
	retired, err := contents.LoadSupplierAgreementVersion(ctx, v2.Tenant(), v2.ObjectID(), v2.Version())
	if err != nil || !retired.HasVersion || retired.HasAgreement || retired.Version.Status() != domain.CommercialVersionRetired {
		t.Fatalf("版本点读 v2：has=%v agreement=%v status=%q err=%v, want 只交已退役的壳", retired.HasVersion, retired.HasAgreement, retired.Version.Status(), err)
	}
	ended, found, err := contents.LoadSupplierAgreement(ctx, v2.Tenant(), v2)
	if err != nil || !found || ended.SupportsProcurementAt(cutRowAfter) {
		t.Fatalf("正文点读 v2 仍支持采购：found=%v err=%v", found, err)
	}
	if terminatedAt, terminated := ended.TerminatedAt(); !terminated || !terminatedAt.Equal(cutRowRegisteredAt) {
		t.Fatalf("正文点读 v2 的终止时刻 = %v %v, want 退役时刻", terminatedAt, terminated)
	}
	successor, err := contents.LoadSupplierAgreementVersion(ctx, v3.Tenant(), v3.ObjectID(), v3.Version())
	if err != nil || !successor.HasAgreement || !successor.Agreement.SupportsProcurementAt(cutRowAfter) {
		t.Fatalf("后继 v3 不支持采购：%+v %v", successor, err)
	}
}

// Covers: ADR-0233 决定四在内部成本政策点读口上的那一格——同起点退役的那一版价格政策此后答「没这份正文」，后继照常交回。
func TestInternalCostPolicyReadFoldsTheRetirement(t *testing.T) {
	fixture := newCutFixture(t)
	old := cutVersion(t, domain.PriceRuleObject, "internal-cost-1", "v1", cutRowPredecessorStart)
	next := cutVersion(t, domain.PriceRuleObject, "internal-cost-1", "v2", cutRowPredecessorStart)
	for _, version := range []domain.CommercialVersion{old, next} {
		fixture.save(t, version)
		fixture.savePricePolicy(t, version)
	}
	if result := fixture.retire(t, retireCommand(t, old, "v2", "SYN-REISSUE-COST")); result.Outcome() != application.VersionClosureRegistered {
		t.Fatalf("retire outcome = %q (cause %v)", result.Outcome(), result.Cause())
	}
	if _, found, err := fixture.publications.LoadInternalCostPolicy(t.Context(), old.Tenant(), old.ObjectID(), old.Version()); err != nil || found {
		t.Fatalf("已退役的 v1：found=%v err=%v, want 没这份正文", found, err)
	}
	if _, found, err := fixture.publications.LoadInternalCostPolicy(t.Context(), next.Tenant(), next.ObjectID(), next.Version()); err != nil || !found {
		t.Fatalf("后继 v2：found=%v err=%v", found, err)
	}
}

func (fixture cutFixture) retire(t *testing.T, command application.RetireCommercialVersionAtSameStartCommand) application.VersionClosureResult {
	t.Helper()
	var result application.VersionClosureResult
	mustWithinPublicationTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.handler.RetireAtSameStart(txCtx, command)
		return err
	})
	return result
}

func (fixture cutFixture) saveAgreement(t *testing.T, version domain.CommercialVersion) {
	t.Helper()
	fixture.save(t, version)
	agreement, err := domain.NewSupplierAgreement(version,
		pcValue(t, domain.NewPartyID, "supplier-1"),
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcScope(t),
		pcValue(t, domain.NewPricingPlanReference, "plan-buy-1"),
		version.Effective())
	if err != nil {
		t.Fatalf("协议正文：%v", err)
	}
	mustWithinPublicationTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		_, err := fixture.publications.SaveSupplierAgreement(txCtx, agreement)
		return err
	})
}

func (fixture cutFixture) savePricePolicy(t *testing.T, version domain.CommercialVersion) {
	t.Helper()
	policy, err := domain.NewCommercialPricePolicy(version, domain.InternalDirection,
		pcValue(t, domain.NewPricingPlanReference, "plan-internal-"+version.Version().String()),
		domain.InternalDirection, domain.PlanBindingConversionNone, pcScope(t), version.Effective())
	if err != nil {
		t.Fatalf("价格政策：%v", err)
	}
	mustWithinPublicationTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		_, err := fixture.publications.SavePricePolicy(txCtx, policy, domain.InternalDirection, domain.PlanBindingConversionNone)
		return err
	})
}
