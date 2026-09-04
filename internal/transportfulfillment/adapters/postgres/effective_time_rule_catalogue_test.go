package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证轨迹源有效时间规则目录（label-channel/19）：三件正文原样往返、换版
// 成新版本回指前版而原版本一字不动、当前版按回指派生、规则口无版本答`无`有版本按锚点＋偏移答`已
// 判`且带版本、库内两道唯一索引挡住第二个首版与第二个后继、写口无事务即拒、CHECK 挡住领域造不出的
// 行；末一条把票 16 的收编执行器接上真目录，证「有规则→认领并判断→交 VE」。夹具全为合成登记（S
// 级），不含任何真实轨迹源的规则取值。

var (
	ruleRecordedAt = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	ruleOccurredDB = time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	ruleReceivedDB = time.Date(2026, 9, 4, 8, 45, 0, 0, time.UTC)
)

func newEffectiveTimeRuleCatalogue(t *testing.T) (*adapter.EffectiveTimeRuleCatalogue, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := adapter.NewEffectiveTimeRuleCatalogue(db)
	if err != nil {
		t.Fatalf("构造规则目录：%v", err)
	}
	return catalogue, db.Transactor(), pool
}

type ruleFixtureOptions struct {
	source, version string
	content         domain.EffectiveTimeRuleContent
}

func ruleRecord(t *testing.T, options ruleFixtureOptions) ports.EffectiveTimeRuleRecord {
	t.Helper()
	if options.content.Anchor == domain.EffectiveTimeAnchorInvalid {
		options.content = domain.EffectiveTimeRuleContent{
			SourceTimeMeaning: domain.SourceTimeIsEventOccurrence,
			Anchor:            domain.AnchoredAtOccurrence,
		}
	}
	spec := domain.EffectiveTimeRuleSpec{
		TenantID: segmentRef(t, domain.NewTenantID, "tenant-1"),
		Source:   segmentRef(t, domain.NewTrackingSourceReference, options.source),
		Version:  segmentRef(t, domain.NewEffectiveTimeRuleVersion, options.version),
		Content:  options.content,
	}
	rule, err := domain.RegisterEffectiveTimeRule(spec)
	if err != nil {
		t.Fatalf("形成规则夹具：%v", err)
	}
	return ruleRecordOf(rule, ruleRecordedAt)
}

func ruleRecordOf(rule domain.EffectiveTimeRule, recordedAt time.Time) ports.EffectiveTimeRuleRecord {
	return ports.EffectiveTimeRuleRecord{
		Key:        ports.EffectiveTimeRuleKey{TenantID: rule.TenantID(), Source: rule.Source(), Version: rule.Version()},
		Rule:       rule,
		RecordedAt: recordedAt,
	}
}

func mustSaveRule(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	catalogue *adapter.EffectiveTimeRuleCatalogue,
	record ports.EffectiveTimeRuleRecord,
) ports.EffectiveTimeRuleSaveOutcome {
	t.Helper()
	var outcome ports.EffectiveTimeRuleSaveOutcome
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = catalogue.Save(txCtx, record)
		return err
	})
	return outcome
}

func TestAFirstRuleVersionRoundTripsAndIsCurrent(t *testing.T) {
	catalogue, transactor, _ := newEffectiveTimeRuleCatalogue(t)
	ctx := t.Context()
	record := ruleRecord(t, ruleFixtureOptions{source: "src-round-trip", version: "ETR-1", content: domain.EffectiveTimeRuleContent{
		SourceTimeMeaning: domain.SourceTimeIsSourceProcessing,
		Anchor:            domain.AnchoredAtReception,
		Offset:            -20 * time.Minute,
	}})
	if outcome := mustSaveRule(t, transactor, ctx, catalogue, record); outcome != ports.EffectiveTimeRuleSaved {
		t.Fatalf("首登 outcome = %s", outcome)
	}

	found, exists, err := catalogue.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("取回：%v exists=%v", err, exists)
	}
	if !found.Rule.Equal(record.Rule) {
		t.Fatalf("三件正文没有原样带回：%+v", found.Rule.Content())
	}
	if found.Rule.Content().Offset != -20*time.Minute {
		t.Fatalf("负偏移没有原样带回：%s", found.Rule.Content().Offset)
	}
	if !found.RecordedAt.Equal(ruleRecordedAt) {
		t.Fatalf("登记时刻没有原样带回：%s", found.RecordedAt)
	}

	current, exists, err := catalogue.FindCurrent(ctx, record.Key.TenantID, record.Key.Source)
	if err != nil || !exists || current.Key != record.Key {
		t.Fatalf("当前版：%v exists=%v key=%+v", err, exists, current.Key)
	}
}

func TestSavingTheSameRuleVersionTwiceAnswersAlreadyRegistered(t *testing.T) {
	catalogue, transactor, _ := newEffectiveTimeRuleCatalogue(t)
	ctx := t.Context()
	record := ruleRecord(t, ruleFixtureOptions{source: "src-replay", version: "ETR-1"})
	mustSaveRule(t, transactor, ctx, catalogue, record)
	if outcome := mustSaveRule(t, transactor, ctx, catalogue, record); outcome != ports.EffectiveTimeRuleAlreadyRegistered {
		t.Fatalf("撞键应答已登记：%s", outcome)
	}
}

func TestRevisionFormsANewCurrentVersionAndLeavesTheOriginalUntouched(t *testing.T) {
	catalogue, transactor, _ := newEffectiveTimeRuleCatalogue(t)
	ctx := t.Context()
	first := ruleRecord(t, ruleFixtureOptions{source: "src-revise", version: "ETR-1"})
	mustSaveRule(t, transactor, ctx, catalogue, first)

	revised, err := first.Rule.Revise(segmentRef(t, domain.NewEffectiveTimeRuleVersion, "ETR-2"), domain.EffectiveTimeRuleContent{
		SourceTimeMeaning: domain.SourceTimeIsSourceProcessing,
		Anchor:            domain.AnchoredAtReception,
		Offset:            -5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("换版：%v", err)
	}
	if outcome := mustSaveRule(t, transactor, ctx, catalogue, ruleRecordOf(revised, ruleRecordedAt.Add(time.Hour))); outcome != ports.EffectiveTimeRuleSaved {
		t.Fatalf("新版本应落库：%s", outcome)
	}

	current, exists, err := catalogue.FindCurrent(ctx, first.Key.TenantID, first.Key.Source)
	if err != nil || !exists {
		t.Fatalf("当前版：%v exists=%v", err, exists)
	}
	if current.Key.Version != revised.Version() {
		t.Fatalf("当前版应是未被回指的新版：%q", current.Key.Version)
	}
	if prior, has := current.Rule.Supersedes(); !has || prior != first.Key.Version {
		t.Fatalf("新版应回指首版：%q has=%v", prior, has)
	}

	original, exists, err := catalogue.FindByKey(ctx, first.Key)
	if err != nil || !exists {
		t.Fatalf("原版本应保留：%v exists=%v", err, exists)
	}
	if !original.Rule.Equal(first.Rule) {
		t.Fatal("原版本被回写——只插不改被破了")
	}

	versions, err := catalogue.ListVersions(ctx, first.Key.TenantID, first.Key.Source)
	if err != nil {
		t.Fatalf("列版本：%v", err)
	}
	if len(versions) != 2 || versions[0].Key.Version != first.Key.Version || versions[1].Key.Version != revised.Version() {
		t.Fatalf("版本应按登记先后两条：%+v", versions)
	}
}

// Covers: 库面守住版本链的形状——同一源只能有一个首版、同一版本只能有一个后继。两道都由部分唯一索引
// 守，不靠编排里那次「先读当前版再写」——那一读一写之间另一方落进来时，只有索引拦得住。
func TestTheCatalogueRefusesASecondFirstVersionAndASecondSuccessor(t *testing.T) {
	catalogue, transactor, _ := newEffectiveTimeRuleCatalogue(t)
	ctx := t.Context()
	first := ruleRecord(t, ruleFixtureOptions{source: "src-chain", version: "ETR-1"})
	mustSaveRule(t, transactor, ctx, catalogue, first)

	rival := ruleRecord(t, ruleFixtureOptions{source: "src-chain", version: "ETR-1b"})
	if outcome := mustSaveRule(t, transactor, ctx, catalogue, rival); outcome != ports.EffectiveTimeRuleAlreadyRegistered {
		t.Fatalf("同一源的第二个首版应被索引拦下并答已登记：%s", outcome)
	}

	second, err := first.Rule.Revise(segmentRef(t, domain.NewEffectiveTimeRuleVersion, "ETR-2"), first.Rule.Content())
	if err != nil {
		t.Fatalf("换版：%v", err)
	}
	mustSaveRule(t, transactor, ctx, catalogue, ruleRecordOf(second, ruleRecordedAt.Add(time.Hour)))
	rivalSuccessor, err := first.Rule.Revise(segmentRef(t, domain.NewEffectiveTimeRuleVersion, "ETR-2b"), first.Rule.Content())
	if err != nil {
		t.Fatalf("换版：%v", err)
	}
	if outcome := mustSaveRule(t, transactor, ctx, catalogue, ruleRecordOf(rivalSuccessor, ruleRecordedAt.Add(2*time.Hour))); outcome != ports.EffectiveTimeRuleAlreadyRegistered {
		t.Fatalf("同一版本的第二个后继应被索引拦下并答已登记：%s", outcome)
	}
}

// Covers: ADR-0102 决定三——规则口没有规则答`无`，不默认等于发生时间；有规则按锚点＋偏移形成有效
// 时间并带上规则版本；换版后按新版判。
func TestTheRulesPortAnswersAbsentWithoutARuleAndAppliedWithTheCurrentVersion(t *testing.T) {
	catalogue, transactor, _ := newEffectiveTimeRuleCatalogue(t)
	ctx := t.Context()
	tenant := segmentRef(t, domain.NewTenantID, "tenant-1")
	input := ports.EffectiveTimeRuleInput{
		TenantID:   tenant,
		Source:     segmentRef(t, domain.NewTrackingSourceReference, "src-rules-port"),
		Status:     segmentRef(t, domain.NewRawStatusReference, "DELIVERED"),
		OccurredAt: ruleOccurredDB,
		ReceivedAt: ruleReceivedDB,
	}

	absent, err := catalogue.JudgeEffectiveTime(ctx, input)
	if err != nil || absent.Outcome != ports.EffectiveTimeRuleAbsent {
		t.Fatalf("无规则应答 RULE_ABSENT：%v %s", err, absent.Outcome)
	}
	if !absent.EffectiveAt.IsZero() {
		t.Fatal("无规则却给出了有效时间——那就是「默认等于发生时间」在偷跑")
	}

	first := ruleRecord(t, ruleFixtureOptions{source: "src-rules-port", version: "ETR-1", content: domain.EffectiveTimeRuleContent{
		SourceTimeMeaning: domain.SourceTimeIsEventOccurrence,
		Anchor:            domain.AnchoredAtOccurrence,
	}})
	mustSaveRule(t, transactor, ctx, catalogue, first)
	applied, err := catalogue.JudgeEffectiveTime(ctx, input)
	if err != nil || applied.Outcome != ports.EffectiveTimeRuleApplied {
		t.Fatalf("有规则应答 RULE_APPLIED：%v %s", err, applied.Outcome)
	}
	if !applied.EffectiveAt.Equal(ruleOccurredDB) || applied.Rule.Rule() != "src-rules-port" || applied.Rule.Version() != "ETR-1" {
		t.Fatalf("应按首版锚在发生时间：%s %s/%s", applied.EffectiveAt, applied.Rule.Rule(), applied.Rule.Version())
	}

	revised, err := first.Rule.Revise(segmentRef(t, domain.NewEffectiveTimeRuleVersion, "ETR-2"), domain.EffectiveTimeRuleContent{
		SourceTimeMeaning: domain.SourceTimeIsSourceProcessing,
		Anchor:            domain.AnchoredAtReception,
		Offset:            -10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("换版：%v", err)
	}
	mustSaveRule(t, transactor, ctx, catalogue, ruleRecordOf(revised, ruleRecordedAt.Add(time.Hour)))
	reapplied, err := catalogue.JudgeEffectiveTime(ctx, input)
	if err != nil || reapplied.Outcome != ports.EffectiveTimeRuleApplied {
		t.Fatalf("换版后应仍答 RULE_APPLIED：%v %s", err, reapplied.Outcome)
	}
	if !reapplied.EffectiveAt.Equal(ruleReceivedDB.Add(-10*time.Minute)) || reapplied.Rule.Version() != "ETR-2" {
		t.Fatalf("应按新版锚在接收时间减十分钟：%s 版本 %s", reapplied.EffectiveAt, reapplied.Rule.Version())
	}

	t.Run("另一家源仍然无规则", func(t *testing.T) {
		other := input
		other.Source = segmentRef(t, domain.NewTrackingSourceReference, "src-rules-port-other")
		ruling, err := catalogue.JudgeEffectiveTime(ctx, other)
		if err != nil || ruling.Outcome != ports.EffectiveTimeRuleAbsent {
			t.Fatalf("规则按源登记，别家源不得沾光：%v %s", err, ruling.Outcome)
		}
	})
}

func TestEffectiveTimeRuleWritesRefuseToRunOutsideATransaction(t *testing.T) {
	catalogue, _, _ := newEffectiveTimeRuleCatalogue(t)
	_, err := catalogue.Save(t.Context(), ruleRecord(t, ruleFixtureOptions{source: "src-no-tx", version: "ETR-1"}))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestRuleSaveRefusesAKeyThatDisagreesWithTheRule(t *testing.T) {
	catalogue, transactor, _ := newEffectiveTimeRuleCatalogue(t)
	record := ruleRecord(t, ruleFixtureOptions{source: "src-key", version: "ETR-1"})
	record.Key.Version = segmentRef(t, domain.NewEffectiveTimeRuleVersion, "ETR-other")
	err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, saveErr := catalogue.Save(txCtx, record)
		return saveErr
	})
	if err == nil {
		t.Fatal("键与聚合不一致的写入落进去了")
	}
}

// TestEffectiveTimeRuleCheckConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK 是第二道门。
func TestEffectiveTimeRuleCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	_, _, pool := newEffectiveTimeRuleCatalogue(t)
	ctx := t.Context()
	base := `INSERT INTO transport_fulfillment.effective_time_rule
	    (tenant_id, source_ref, version, source_time_meaning, anchor, offset_seconds, supersedes_version, recorded_at) VALUES `
	for name, values := range map[string]string{
		"时间字段含义不在封闭集合内": `('t','BAD-1','v1','GUESSED','OCCURRED_AT',0,NULL,now())`,
		"锚点不在封闭集合内":     `('t','BAD-2','v1','EVENT_OCCURRENCE','NOW',0,NULL,now())`,
		"前版指向自己":        `('t','BAD-3','v1','EVENT_OCCURRENCE','OCCURRED_AT',0,'v1',now())`,
		"前版为空串":         `('t','BAD-4','v2','EVENT_OCCURRENCE','OCCURRED_AT',0,'  ',now())`,
		"源为空":           `('t','  ','v1','EVENT_OCCURRENCE','OCCURRED_AT',0,NULL,now())`,
		"版本为空":          `('t','BAD-6','','EVENT_OCCURRENCE','OCCURRED_AT',0,NULL,now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}
}

// Covers: 票 16 用例矩阵「有规则→认领并判断→交 VE」在真实现上复现——执行器、事实登记册、身份签发、
// 凭证登记册、规则目录全接真库；只有交接意图用替身以数它被交了几次。反面同证：规则登记前同一
// 家源的素材只到「已认领待判断」，不交 VE。
func TestTheAdoptionExecutorJudgesByTheRealRuleAndHandsOff(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	facts, err := adapter.NewExternalTrackingFacts(db)
	if err != nil {
		t.Fatalf("事实登记册：%v", err)
	}
	identities, err := adapter.NewResultVersions(db)
	if err != nil {
		t.Fatalf("身份签发：%v", err)
	}
	credentials, err := adapter.NewExternalCarrierCredentials(db, credentialClock{at: credentialNow})
	if err != nil {
		t.Fatalf("凭证登记册：%v", err)
	}
	rules, err := adapter.NewEffectiveTimeRuleCatalogue(db)
	if err != nil {
		t.Fatalf("规则目录：%v", err)
	}
	handoffs := &countingHandoff{}
	executor := application.NewAdoptTrackingMaterialHandler(application.AdoptTrackingMaterialDeps{
		Facts:       facts,
		Identities:  identities,
		Credentials: credentials,
		Rules:       rules,
		Ledger:      facts,
		Downstream:  handoffs,
		Clock:       credentialClock{at: credentialNow},
	})
	transactor := db.Transactor()
	ctx := t.Context()
	tenant := segmentRef(t, domain.NewTenantID, "tenant-1")
	mustSaveCredential(t, transactor, ctx, credentials, credentialRecord(t, credentialFixtureOptions{
		credential: "carrier-x/1Z-RULE-1", version: "ECV-1", reference: "PCL-RULE-1",
	}))

	adopt := func(t *testing.T, event string) application.AdoptTrackingMaterialResult {
		t.Helper()
		material := ports.TrackingMaterial{
			Source:          "src-adopt-rule",
			Subject:         ports.TrackingSubject{Tenant: tenant, CredentialReference: "carrier-x/1Z-RULE-1"},
			SourceEventID:   event,
			OccurredAt:      ports.SourceTime{Given: true, At: ruleOccurredDB},
			ReceivedAt:      ruleReceivedDB,
			StatusReference: "IN_TRANSIT",
			PayloadDigest:   "sha256:" + event,
		}
		var result application.AdoptTrackingMaterialResult
		mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var adoptErr error
			result, adoptErr = executor.Adopt(txCtx, material)
			return adoptErr
		})
		return result
	}

	pending := adopt(t, "evt-rule-before")
	if pending.Outcome() != application.TrackingFactAdoptedPendingJudgment || handoffs.count != 0 {
		t.Fatalf("规则登记前应认领为待判断且不交 VE：%s handoffs=%d", pending.Outcome(), handoffs.count)
	}

	mustSaveRule(t, transactor, ctx, rules, ruleRecord(t, ruleFixtureOptions{source: "src-adopt-rule", version: "ETR-1", content: domain.EffectiveTimeRuleContent{
		SourceTimeMeaning: domain.SourceTimeIsSourceProcessing,
		Anchor:            domain.AnchoredAtReception,
		Offset:            -15 * time.Minute,
	}}))
	judged := adopt(t, "evt-rule-after")
	if judged.Outcome() != application.TrackingFactAdopted {
		t.Fatalf("规则登记后应认领并判断：%s reason=%s", judged.Outcome(), judged.UndecidedReason())
	}
	record, has := judged.Record()
	if !has {
		t.Fatal("认领成功却没带回记录")
	}
	at, isJudged := record.Fact.EffectiveAt()
	if !isJudged || !at.Equal(ruleReceivedDB.Add(-15*time.Minute)) {
		t.Fatalf("有效时间应按规则锚在接收时间减十五分钟：%s judged=%v", at, isJudged)
	}
	rule, byRule := record.Fact.Effective().Rule()
	if !byRule || rule.Rule() != "src-adopt-rule" || rule.Version() != "ETR-1" {
		t.Fatalf("事实上应记下按哪一版规则判的：%+v byRule=%v", rule, byRule)
	}
	if handoffs.count != 1 {
		t.Fatalf("判断过的版本应交 VE 恰好一次：%d", handoffs.count)
	}

	stored, exists, err := facts.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("事实应已落库：%v exists=%v", err, exists)
	}
	if stored.Fact.Effective().Basis() != domain.EffectiveTimeJudgedByRule {
		t.Fatalf("库里的依据应是 JUDGED_BY_RULE：%s", stored.Fact.Effective().Basis())
	}
}

type countingHandoff struct{ count int }

func (handoff *countingHandoff) HandOffExternalTrackingFact(context.Context, ports.ExternalTrackingFactHandoffIntent) error {
	handoff.count++
	return nil
}
