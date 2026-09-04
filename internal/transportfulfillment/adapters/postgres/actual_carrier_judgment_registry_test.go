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

// 本文件对真实 PostgreSQL 16 证实际承运商判断登记册（tf-segment-lifecycle-closure/02，0013）：首登头行连首版
// 同笔落并整份往返、版本只追加（撞序号答`版本已在册`、原版本一字不动）、依据逐版本各存一份且在册身份与名称
// 素材两种形状都能往返、没有段就开不了判断（外键）、写口无事务即拒、库内 CHECK 挡住领域造不出的行；最后
// 一条把收寄登记编排接上真段登记册与真判断登记册，证段成立那一笔真的把首版落进了库。夹具全为合成登记（S 级）。

var (
	judgmentEstablishedAt = segmentEnteredAtFixture
	judgmentOpenedAt      = segmentEnteredAtFixture.Add(time.Minute)
	judgmentEvidenceAt    = segmentEnteredAtFixture.Add(2 * time.Hour)
)

type judgmentRegistries struct {
	db         *bentopg.DB
	segments   *adapter.FulfillmentSegments
	judgments  *adapter.ActualCarrierJudgments
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newJudgmentRegistries(t *testing.T) judgmentRegistries {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	segments, err := adapter.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatalf("构造段登记册：%v", err)
	}
	judgments, err := adapter.NewActualCarrierJudgments(db)
	if err != nil {
		t.Fatalf("构造判断登记册：%v", err)
	}
	return judgmentRegistries{db: db, segments: segments, judgments: judgments, transactor: db.Transactor(), pool: pool}
}

// establishSegment 先把段立起来——判断的外键钉在段上，没有段就没有判断。
func (registries judgmentRegistries) establishSegment(t *testing.T, ctx context.Context, segment string) ports.ActualCarrierJudgmentKey {
	t.Helper()
	mustSaveSegment(t, registries.transactor, ctx, registries.segments, segmentRecord(t, segment, activeMember(t, "parcel-1")))
	key := segmentKeyFixture(t, "tenant-1", segment)
	return ports.ActualCarrierJudgmentKey{TenantID: key.TenantID, Segment: key.Segment}
}

func openedJudgment(t *testing.T, key ports.ActualCarrierJudgmentKey) domain.ActualCarrierJudgment {
	t.Helper()
	judgment, err := domain.OpenActualCarrierJudgment(domain.OpenActualCarrierJudgmentSpec{
		TenantID:      key.TenantID,
		Segment:       key.Segment,
		EstablishedAt: judgmentEstablishedAt,
		FormedAt:      judgmentOpenedAt,
	})
	if err != nil {
		t.Fatalf("开判断夹具：%v", err)
	}
	return judgment
}

func (registries judgmentRegistries) mustOpen(t *testing.T, ctx context.Context, record ports.ActualCarrierJudgmentRecord) ports.JudgmentOpenOutcome {
	t.Helper()
	var outcome ports.JudgmentOpenOutcome
	mustWithinSegmentTransaction(t, registries.transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = registries.judgments.Open(txCtx, record)
		return err
	})
	return outcome
}

func (registries judgmentRegistries) mustAppend(
	t *testing.T,
	ctx context.Context,
	key ports.ActualCarrierJudgmentKey,
	version domain.ActualCarrierJudgmentVersion,
	recordedAt time.Time,
) ports.JudgmentVersionAppendOutcome {
	t.Helper()
	var outcome ports.JudgmentVersionAppendOutcome
	mustWithinSegmentTransaction(t, registries.transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = registries.judgments.AppendVersion(txCtx, key, version, recordedAt)
		return err
	})
	return outcome
}

func judgmentSubject(t *testing.T, reference string) domain.CarrierSubject {
	t.Helper()
	subject, err := domain.NewCarrierSubject(domain.ExternalCarrierParty, reference)
	if err != nil {
		t.Fatalf("承运主体夹具：%v", err)
	}
	return subject
}

func judgmentEvidence(t *testing.T, source domain.CarrierEvidenceSource, reference string, subject domain.CarrierSubject, material string) domain.CarrierEvidence {
	t.Helper()
	evidence, err := domain.NewCarrierEvidence(domain.CarrierEvidenceSpec{
		Source:     source,
		Reference:  segmentRef(t, domain.NewCarrierEvidenceReference, reference),
		OccurredAt: judgmentEvidenceAt,
		Subject:    subject,
		Material:   material,
	})
	if err != nil {
		t.Fatalf("依据夹具：%v", err)
	}
	return evidence
}

func TestOpeningAJudgmentRoundTripsItsHeadAndPendingFirstVersion(t *testing.T) {
	registries := newJudgmentRegistries(t)
	ctx := t.Context()
	key := registries.establishSegment(t, ctx, "SEG-ACJ-1")
	record := ports.ActualCarrierJudgmentRecord{Key: key, Judgment: openedJudgment(t, key), RecordedAt: judgmentOpenedAt}

	if outcome := registries.mustOpen(t, ctx, record); outcome != ports.JudgmentOpened {
		t.Fatalf("首登 outcome = %s", outcome)
	}

	found, exists, err := registries.judgments.FindByKey(ctx, key)
	if err != nil || !exists {
		t.Fatalf("取回：%v exists=%v", err, exists)
	}
	if !found.Judgment.SegmentEstablishedAt().Equal(judgmentEstablishedAt) || !found.RecordedAt.Equal(judgmentOpenedAt) {
		t.Fatalf("段成立时刻 %s / 落库时刻 %s 没有原样带回", found.Judgment.SegmentEstablishedAt(), found.RecordedAt)
	}
	versions := found.Judgment.Versions()
	if len(versions) != 1 || versions[0].Sequence() != 1 {
		t.Fatalf("首登应恰一版序号 1：%+v", versions)
	}
	if reason, pending := versions[0].Verdict().Pending(); !pending || reason != domain.NoQualifiedCarrierEvidence {
		t.Fatalf("首版应为待确认（无合格证据）：pending=%v reason=%s", pending, reason)
	}
	if !versions[0].BusinessTime().Equal(judgmentEstablishedAt) || !versions[0].FormedAt().Equal(judgmentOpenedAt) || len(versions[0].Bases()) != 0 {
		t.Fatalf("首版三件没有原样带回：%s / %s / %d 条依据", versions[0].BusinessTime(), versions[0].FormedAt(), len(versions[0].Bases()))
	}
}

func TestOpeningTheSameJudgmentTwiceAnswersAlreadyOpened(t *testing.T) {
	registries := newJudgmentRegistries(t)
	ctx := t.Context()
	key := registries.establishSegment(t, ctx, "SEG-ACJ-2")
	record := ports.ActualCarrierJudgmentRecord{Key: key, Judgment: openedJudgment(t, key), RecordedAt: judgmentOpenedAt}
	registries.mustOpen(t, ctx, record)
	if outcome := registries.mustOpen(t, ctx, record); outcome != ports.JudgmentAlreadyOpened {
		t.Fatalf("撞键应答已开：%s", outcome)
	}
}

// 版本只追加：识别、冲突各成一版，依据逐版本各存一份，原版本一字不动；撞序号答`版本已在册`而不覆盖。
func TestVersionsAppendWithTheirOwnBasesAndNeverOverwrite(t *testing.T) {
	registries := newJudgmentRegistries(t)
	ctx := t.Context()
	key := registries.establishSegment(t, ctx, "SEG-ACJ-3")
	judgment := openedJudgment(t, key)
	registries.mustOpen(t, ctx, ports.ActualCarrierJudgmentRecord{Key: key, Judgment: judgment, RecordedAt: judgmentOpenedAt})

	carrierX, carrierY := judgmentSubject(t, "party/carrier-x"), judgmentSubject(t, "party/carrier-y")
	identified, err := judgment.Consider(judgmentEvidence(t, domain.CarrierDirectPickupScan, "SCAN-A", carrierX, ""), judgmentEvidenceAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("consider X: %v", err)
	}
	if outcome := registries.mustAppend(t, ctx, key, identified.Current(), judgmentEvidenceAt.Add(time.Minute)); outcome != ports.JudgmentVersionAppended {
		t.Fatalf("追加第二版 outcome = %s", outcome)
	}
	conflicting, err := identified.Consider(judgmentEvidence(t, domain.TrustedChannelCallback, "CALLBACK-B", carrierY, ""), judgmentEvidenceAt.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("consider Y: %v", err)
	}
	if outcome := registries.mustAppend(t, ctx, key, conflicting.Current(), judgmentEvidenceAt.Add(2*time.Minute)); outcome != ports.JudgmentVersionAppended {
		t.Fatalf("追加第三版 outcome = %s", outcome)
	}

	// 同一序号再来一份不同内容：撞序号是业务答案，原版本不动。
	rival, err := identified.Consider(judgmentEvidence(t, domain.CarrierReceiptVoucher, "RECEIPT-C", carrierX, ""), judgmentEvidenceAt.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("consider rival: %v", err)
	}
	if outcome := registries.mustAppend(t, ctx, key, rival.Current(), judgmentEvidenceAt.Add(3*time.Minute)); outcome != ports.JudgmentVersionAlreadyRecorded {
		t.Fatalf("撞序号应答版本已在册：%s", outcome)
	}

	found, _, err := registries.judgments.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	versions := found.Judgment.Versions()
	if len(versions) != 3 {
		t.Fatalf("应恰三版，实得 %d", len(versions))
	}
	if subject, ok := versions[1].Verdict().Identified(); !ok || subject != carrierX || len(versions[1].Bases()) != 1 {
		t.Fatalf("第二版应识别 X 且一条依据：%+v", versions[1])
	}
	if reason, ok := versions[2].Verdict().Pending(); !ok || reason != domain.CarrierEvidenceSourceConflict || len(versions[2].Bases()) != 2 {
		t.Fatalf("第三版应来源冲突且两条依据：%+v", versions[2])
	}
	// 第三版那份依据里 RECEIPT-C 不该出现——撞序号的那一版没落进去。
	for _, basis := range versions[2].Bases() {
		if basis.Reference().String() == "RECEIPT-C" {
			t.Fatal("撞序号的版本把依据写进了已在册的版本")
		}
	}
	// 记录的落库时刻跟着最近一次落的那一版走。
	if !found.RecordedAt.Equal(judgmentOpenedAt) {
		t.Fatalf("头行落库时刻 = %s，want 首登时刻 %s——头行只插不改", found.RecordedAt, judgmentOpenedAt)
	}
}

// 名称素材那一格往返：身份未登记的依据只带素材，登记后补认的新版本带身份引用，上一版的素材原样留着。
func TestAnUnregisteredNameRoundTripsAsMaterialAndRecognitionAppendsANewVersion(t *testing.T) {
	registries := newJudgmentRegistries(t)
	ctx := t.Context()
	key := registries.establishSegment(t, ctx, "SEG-ACJ-4")
	judgment := openedJudgment(t, key)
	registries.mustOpen(t, ctx, ports.ActualCarrierJudgmentRecord{Key: key, Judgment: judgment, RecordedAt: judgmentOpenedAt})

	pending, err := judgment.Consider(judgmentEvidence(t, domain.TrustedChannelCallback, "CALLBACK-1", domain.CarrierSubject{}, "Y Express"), judgmentEvidenceAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("consider: %v", err)
	}
	registries.mustAppend(t, ctx, key, pending.Current(), judgmentEvidenceAt.Add(time.Minute))
	recognised, err := pending.RecogniseCarrierIdentity(segmentRef(t, domain.NewCarrierEvidenceReference, "CALLBACK-1"), judgmentSubject(t, "party/carrier-y"), judgmentEvidenceAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("recognise: %v", err)
	}
	registries.mustAppend(t, ctx, key, recognised.Current(), judgmentEvidenceAt.Add(time.Hour))

	found, _, err := registries.judgments.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	versions := found.Judgment.Versions()
	if len(versions) != 3 {
		t.Fatalf("应三版，实得 %d", len(versions))
	}
	if reason, ok := versions[1].Verdict().Pending(); !ok || reason != domain.CarrierIdentityNotRegistered || versions[1].Bases()[0].Material() != "Y Express" {
		t.Fatalf("第二版应身份未登记且素材原样：%+v", versions[1])
	}
	if _, has := versions[1].Bases()[0].Subject(); has {
		t.Fatal("未登记的依据装回来却带了身份")
	}
	subject, ok := versions[2].Verdict().Identified()
	if !ok || subject.Reference() != "party/carrier-y" || versions[2].Bases()[0].Material() != "" {
		t.Fatalf("第三版应识别 Y 且依据带身份不带素材：%+v", versions[2])
	}
	if !versions[2].BusinessTime().Equal(judgmentEvidenceAt) || !versions[2].FormedAt().Equal(judgmentEvidenceAt.Add(time.Hour)) {
		t.Fatalf("补认版本业务时间 %s / 形成时间 %s", versions[2].BusinessTime(), versions[2].FormedAt())
	}
}

// 没有段就开不了判断：外键把「一段一份」钉在库上。
func TestAJudgmentCannotBeOpenedForASegmentThatWasNeverEstablished(t *testing.T) {
	registries := newJudgmentRegistries(t)
	key := ports.ActualCarrierJudgmentKey{
		TenantID: segmentRef(t, domain.NewTenantID, "tenant-1"),
		Segment:  segmentRef(t, domain.NewFulfillmentSegmentReference, "SEG-NEVER"),
	}
	err := registries.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, openErr := registries.judgments.Open(txCtx, ports.ActualCarrierJudgmentRecord{Key: key, Judgment: openedJudgment(t, key), RecordedAt: judgmentOpenedAt})
		return openErr
	})
	if err == nil {
		t.Fatal("没有段的判断落进去了")
	}
}

func TestJudgmentWritesRefuseToRunOutsideATransaction(t *testing.T) {
	registries := newJudgmentRegistries(t)
	ctx := t.Context()
	key := registries.establishSegment(t, ctx, "SEG-ACJ-5")
	judgment := openedJudgment(t, key)

	if _, err := registries.judgments.Open(ctx, ports.ActualCarrierJudgmentRecord{Key: key, Judgment: judgment, RecordedAt: judgmentOpenedAt}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务首登应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := registries.judgments.AppendVersion(ctx, key, judgment.Current(), judgmentOpenedAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestOpenRefusesAKeyThatDisagreesWithTheJudgment(t *testing.T) {
	registries := newJudgmentRegistries(t)
	ctx := t.Context()
	key := registries.establishSegment(t, ctx, "SEG-ACJ-6")
	other := registries.establishSegment(t, ctx, "SEG-ACJ-6B")
	err := registries.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, openErr := registries.judgments.Open(txCtx, ports.ActualCarrierJudgmentRecord{Key: other, Judgment: openedJudgment(t, key), RecordedAt: judgmentOpenedAt})
		return openErr
	})
	if err == nil {
		t.Fatal("键与聚合不一致的写入落进去了")
	}
}

func TestJudgmentsAreInvisibleAcrossTenants(t *testing.T) {
	registries := newJudgmentRegistries(t)
	ctx := t.Context()
	key := registries.establishSegment(t, ctx, "SEG-ACJ-7")
	registries.mustOpen(t, ctx, ports.ActualCarrierJudgmentRecord{Key: key, Judgment: openedJudgment(t, key), RecordedAt: judgmentOpenedAt})

	foreign := key
	foreign.TenantID = segmentRef(t, domain.NewTenantID, "tenant-2")
	if _, exists, err := registries.judgments.FindByKey(ctx, foreign); err != nil || exists {
		t.Fatalf("他租户看见了判断：exists=%v err=%v", exists, err)
	}
}

// 库内 CHECK 是第二道门：判断值恰居其一、来源与分支封闭、依据的身份与素材恰居其一——适配器绕不过任何一道。
func TestJudgmentCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	registries := newJudgmentRegistries(t)
	ctx := t.Context()
	key := registries.establishSegment(t, ctx, "SEG-ACJ-8")
	registries.mustOpen(t, ctx, ports.ActualCarrierJudgmentRecord{Key: key, Judgment: openedJudgment(t, key), RecordedAt: judgmentOpenedAt})

	version := `INSERT INTO transport_fulfillment.actual_carrier_judgment_version
	    (tenant_id, segment_ref, sequence_no, subject_kind, subject_ref, pending_reason, business_time, formed_at, recorded_at) VALUES `
	basis := `INSERT INTO transport_fulfillment.actual_carrier_judgment_basis
	    (tenant_id, segment_ref, sequence_no, evidence_ref, evidence_source, occurred_at, subject_kind, subject_ref, name_material) VALUES `
	for name, statement := range map[string]string{
		"版本既识别又待确认":   version + `('tenant-1','SEG-ACJ-8',2,'EXTERNAL_PARTY','party/x','SOURCE_CONFLICT',now(),now(),now())`,
		"版本既不识别也无原因":  version + `('tenant-1','SEG-ACJ-8',2,NULL,NULL,NULL,now(),now(),now())`,
		"版本只有分支没有引用":  version + `('tenant-1','SEG-ACJ-8',2,'EXTERNAL_PARTY',NULL,NULL,now(),now(),now())`,
		"版本分支不在两支内":   version + `('tenant-1','SEG-ACJ-8',2,'BRAND','party/x',NULL,now(),now(),now())`,
		"版本原因不在三格内":   version + `('tenant-1','SEG-ACJ-8',2,NULL,NULL,'UNKNOWN',now(),now(),now())`,
		"版本序号为零":      version + `('tenant-1','SEG-ACJ-8',0,NULL,NULL,'NO_QUALIFIED_EVIDENCE',now(),now(),now())`,
		"版本引用空白":      version + `('tenant-1','SEG-ACJ-8',2,'EXTERNAL_PARTY','  ',NULL,now(),now(),now())`,
		"依据来源不在四格内":   basis + `('tenant-1','SEG-ACJ-8',1,'E-1','BOOKING_ACCEPTED',now(),NULL,NULL,'X')`,
		"依据既有身份又有素材":  basis + `('tenant-1','SEG-ACJ-8',1,'E-1','CARRIER_PICKUP_SCAN',now(),'EXTERNAL_PARTY','party/x','X')`,
		"依据既无身份也无素材":  basis + `('tenant-1','SEG-ACJ-8',1,'E-1','CARRIER_PICKUP_SCAN',now(),NULL,NULL,NULL)`,
		"依据素材空白":      basis + `('tenant-1','SEG-ACJ-8',1,'E-1','CARRIER_PICKUP_SCAN',now(),NULL,NULL,'  ')`,
		"依据引用空白":      basis + `('tenant-1','SEG-ACJ-8',1,'  ','CARRIER_PICKUP_SCAN',now(),NULL,NULL,'X')`,
		"依据挂在不存在的版本上": basis + `('tenant-1','SEG-ACJ-8',9,'E-1','CARRIER_PICKUP_SCAN',now(),NULL,NULL,'X')`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := registries.pool.Exec(ctx, statement); err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}
}

// 挂点在真库上：收寄登记编排接真段登记册与真判断登记册，段成立那一笔把首版落进库；第二个对象加入不再开第二份。
// 这是生产装配那一行（Judgments 交入）在本包能证到的最近一格——装配点本身归 cmd/parcel-api。
func TestRegisteringAPickupIntoANewSegmentOpensTheJudgmentInTheDatabase(t *testing.T) {
	registries := newJudgmentRegistries(t)
	ctx := t.Context()
	pickups, err := adapter.NewOffsitePickupRegistrations(registries.db)
	if err != nil {
		t.Fatalf("揽收登记册：%v", err)
	}
	handler := application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
		Pickups:    pickups,
		Segments:   registries.segments,
		Judgments:  registries.judgments,
		Versions:   noVersions{},
		Downstream: noPickupHandoff{},
		Clock:      credentialClock{at: judgmentOpenedAt},
	})
	tenant := segmentRef(t, domain.NewTenantID, "tenant-1")
	register := func(object, attempt string) application.RegisterOffsitePickupResult {
		t.Helper()
		var result application.RegisterOffsitePickupResult
		mustWithinSegmentTransaction(t, registries.transactor, ctx, func(txCtx context.Context) error {
			var registerErr error
			result, registerErr = handler.Register(txCtx, application.RegisterOffsitePickupCommand{
				TenantID:   tenant,
				Object:     object,
				Task:       "task-1",
				Attempt:    attempt,
				Place:      "place-1",
				Control:    "TRANSPORT-CONTROL/" + object,
				ExecutedBy: "executor-1",
				OccurredAt: judgmentEstablishedAt,
				Segment:    "SEG-ACJ-9",
			})
			return registerErr
		})
		return result
	}

	first := register("parcel-1", "attempt-1")
	if first.Outcome() != application.PickupRegistered || first.SegmentContinuationReference() != "" {
		t.Fatalf("首个对象：outcome=%s debt=%q", first.Outcome(), first.SegmentContinuationReference())
	}
	key := ports.ActualCarrierJudgmentKey{TenantID: tenant, Segment: segmentRef(t, domain.NewFulfillmentSegmentReference, "SEG-ACJ-9")}
	found, exists, err := registries.judgments.FindByKey(ctx, key)
	if err != nil || !exists {
		t.Fatalf("段成立了库里却没有判断：exists=%v err=%v", exists, err)
	}
	if len(found.Judgment.Versions()) != 1 || !found.Judgment.SegmentEstablishedAt().Equal(judgmentEstablishedAt) {
		t.Fatalf("首版没按段成立时刻落：%+v", found.Judgment)
	}

	second := register("parcel-2", "attempt-2")
	if second.Outcome() != application.PickupRegistered || second.SegmentContinuationReference() != "" {
		t.Fatalf("第二个对象：outcome=%s debt=%q", second.Outcome(), second.SegmentContinuationReference())
	}
	var heads int
	if err := registries.pool.QueryRow(ctx,
		`SELECT count(*) FROM transport_fulfillment.actual_carrier_judgment WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-ACJ-9'`,
	).Scan(&heads); err != nil || heads != 1 {
		t.Fatalf("判断按段一份：heads=%d err=%v", heads, err)
	}
}

type noVersions struct{}

func (noVersions) NextPickupResultVersion(context.Context) (domain.PickupResultVersion, error) {
	return domain.NewPickupResultVersion("pickup-result/v-fixed")
}

type noPickupHandoff struct{}

func (noPickupHandoff) HandOffOffsitePickupRegistration(context.Context, ports.OffsitePickupRegistrationIntent) error {
	return nil
}
