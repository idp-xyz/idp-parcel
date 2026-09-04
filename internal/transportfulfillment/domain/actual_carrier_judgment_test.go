package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var (
	judgmentSegmentEstablishedAt = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	judgmentFormedAt             = time.Date(2026, 9, 1, 8, 0, 5, 0, time.UTC)
	judgmentEvidenceAt           = time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)
)

func openJudgmentSpec(t *testing.T) domain.OpenActualCarrierJudgmentSpec {
	t.Helper()
	return domain.OpenActualCarrierJudgmentSpec{
		TenantID:      mustRef(t, domain.NewTenantID, "tenant-1"),
		Segment:       mustRef(t, domain.NewFulfillmentSegmentReference, "segment-1"),
		EstablishedAt: judgmentSegmentEstablishedAt,
		FormedAt:      judgmentFormedAt,
	}
}

func mustOpenJudgment(t *testing.T) domain.ActualCarrierJudgment {
	t.Helper()
	judgment, err := domain.OpenActualCarrierJudgment(openJudgmentSpec(t))
	if err != nil {
		t.Fatalf("open judgment: %v", err)
	}
	return judgment
}

func mustCarrierSubject(t *testing.T, kind domain.CarrierSubjectKind, reference string) domain.CarrierSubject {
	t.Helper()
	subject, err := domain.NewCarrierSubject(kind, reference)
	if err != nil {
		t.Fatalf("carrier subject: %v", err)
	}
	return subject
}

// registeredEvidence 是一条指名了在册承运主体的合格依据。
func registeredEvidence(
	t *testing.T,
	source domain.CarrierEvidenceSource,
	reference string,
	occurredAt time.Time,
	subject domain.CarrierSubject,
) domain.CarrierEvidence {
	t.Helper()
	evidence, err := domain.NewCarrierEvidence(domain.CarrierEvidenceSpec{
		Source:     source,
		Reference:  mustRef(t, domain.NewCarrierEvidenceReference, reference),
		OccurredAt: occurredAt,
		Subject:    subject,
	})
	if err != nil {
		t.Fatalf("carrier evidence: %v", err)
	}
	return evidence
}

// unregisteredEvidence 是一条只带名称素材、承运主体身份未在册的合格依据。
func unregisteredEvidence(
	t *testing.T,
	source domain.CarrierEvidenceSource,
	reference string,
	occurredAt time.Time,
	material string,
) domain.CarrierEvidence {
	t.Helper()
	evidence, err := domain.NewCarrierEvidence(domain.CarrierEvidenceSpec{
		Source:     source,
		Reference:  mustRef(t, domain.NewCarrierEvidenceReference, reference),
		OccurredAt: occurredAt,
		Material:   material,
	})
	if err != nil {
		t.Fatalf("carrier evidence: %v", err)
	}
	return evidence
}

// Covers: CONTEXT「实际承运商判断」规则节首条「实际履约段成立时即形成首个判断版本……否则为待确认
// （无合格证据）。段没有『尚无判断』的状态——未知也是一个版本，未知期间从这一版起算」；ADR-0103
// 决定二「待确认是判断值，段成立即有第一版」。
func TestOpeningAJudgmentWithoutEvidenceFormsAPendingFirstVersion(t *testing.T) {
	judgment := mustOpenJudgment(t)

	versions := judgment.Versions()
	if len(versions) != 1 {
		t.Fatalf("段成立即有第一版，实得 %d 版", len(versions))
	}
	current := judgment.Current()
	if current.Sequence() != 1 {
		t.Fatalf("首版序号 = %d, want 1", current.Sequence())
	}
	reason, pending := current.Verdict().Pending()
	if !pending || reason != domain.NoQualifiedCarrierEvidence {
		t.Fatalf("首版应为待确认（无合格证据），实得 pending=%v reason=%s", pending, reason)
	}
	if _, identified := current.Verdict().Identified(); identified {
		t.Fatal("没有证据却识别出了承运主体")
	}
	// 未知期间从段成立起算：首版业务时间是段成立时刻，形成时间是本上下文铸的时钟。
	if !current.BusinessTime().Equal(judgmentSegmentEstablishedAt) {
		t.Fatalf("首版业务时间 = %s, want 段成立时刻 %s", current.BusinessTime(), judgmentSegmentEstablishedAt)
	}
	if !current.FormedAt().Equal(judgmentFormedAt) {
		t.Fatalf("首版形成时间 = %s, want %s", current.FormedAt(), judgmentFormedAt)
	}
	if len(current.Bases()) != 0 {
		t.Fatalf("无合格证据的版本不该带依据，实得 %d 条", len(current.Bases()))
	}
	if judgment.Segment().String() != "segment-1" || judgment.TenantID().String() != "tenant-1" {
		t.Fatal("判断的键（租户，段）没有原样保存")
	}
	if !judgment.SegmentEstablishedAt().Equal(judgmentSegmentEstablishedAt) {
		t.Fatalf("段成立时刻 = %s", judgment.SegmentEstablishedAt())
	}
}

// Covers: CONTEXT 生命周期「待确认 → 已识别：新到合格证据指名在册的承运主体、业务时间不早于段成立、
// 且与既有依据不冲突。原待确认版本保留，未知期间以它为界」；ADR-0103 决定五「业务时间取自依据的来源
// 事实，判断形成时间由本上下文铸」。
func TestQualifiedEvidenceNamingARegisteredSubjectIdentifiesTheCarrier(t *testing.T) {
	judgment := mustOpenJudgment(t)
	carrierX := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
	evidence := registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-1", judgmentEvidenceAt, carrierX)
	consideredAt := judgmentEvidenceAt.Add(2 * time.Minute)

	considered, err := judgment.Consider(evidence, consideredAt)
	if err != nil {
		t.Fatalf("consider: %v", err)
	}

	versions := considered.Versions()
	if len(versions) != 2 {
		t.Fatalf("追加不覆盖：应有两版，实得 %d", len(versions))
	}
	// 原待确认版本一字不动——未知期间以它为界。
	if reason, pending := versions[0].Verdict().Pending(); !pending || reason != domain.NoQualifiedCarrierEvidence {
		t.Fatal("首版被改写了")
	}
	current := considered.Current()
	if current.Sequence() != 2 {
		t.Fatalf("新版序号 = %d, want 2", current.Sequence())
	}
	subject, identified := current.Verdict().Identified()
	if !identified || subject != carrierX {
		t.Fatalf("应识别为 carrier-x，实得 identified=%v subject=%+v", identified, subject)
	}
	if !current.BusinessTime().Equal(judgmentEvidenceAt) {
		t.Fatalf("业务时间应取自依据的来源事实 %s，实得 %s", judgmentEvidenceAt, current.BusinessTime())
	}
	if !current.FormedAt().Equal(consideredAt) {
		t.Fatalf("形成时间应由本上下文铸 %s，实得 %s", consideredAt, current.FormedAt())
	}
	bases := current.Bases()
	if len(bases) != 1 || bases[0].Reference().String() != "SCAN-1" || bases[0].Source() != domain.CarrierDirectPickupScan {
		t.Fatalf("依据没有逐条保留：%+v", bases)
	}
	// 原值不动：判断是值类型。
	if len(judgment.Versions()) != 1 {
		t.Fatal("Consider 改写了调用方手里的原值")
	}
}

// Covers: CONTEXT「业务时间不得早于该段成立时间；早于段成立的证据说的是段成立之前的事，不作为本段承运
// 主体的依据」——票面验收场景「业务时间早于段成立被拒」。
func TestEvidenceOccurringBeforeTheSegmentWasEstablishedIsRefused(t *testing.T) {
	judgment := mustOpenJudgment(t)
	carrierX := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
	early := registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-EARLY",
		judgmentSegmentEstablishedAt.Add(-time.Second), carrierX)

	_, err := judgment.Consider(early, judgmentFormedAt.Add(time.Hour))
	if !errors.Is(err, domain.ErrCarrierEvidencePrecedesSegment) {
		t.Fatalf("error = %v, want ErrCarrierEvidencePrecedesSegment", err)
	}

	// 同刻允许：段成立那一刻的收寄扫描正是段成立的依据本身。
	atEstablishment := registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-AT",
		judgmentSegmentEstablishedAt, carrierX)
	if _, err := judgment.Consider(atEstablishment, judgmentFormedAt.Add(time.Hour)); err != nil {
		t.Fatalf("与段成立同刻的证据应被接受：%v", err)
	}
}

// Covers: CONTEXT「同一段内不同载运对象的证据指向不同承运主体时，不为各对象分别判断，也不挑一个——形成
// 待确认（来源冲突）并保留全部依据」与生命周期「已识别或待确认 → 待确认（来源冲突）」；票面验收场景
// 「同段冲突」「已识别后相反证据→冲突」。
func TestEvidenceNamingADifferentSubjectTurnsTheJudgmentIntoASourceConflict(t *testing.T) {
	carrierX := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
	carrierY := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-y")
	identified, err := mustOpenJudgment(t).Consider(
		registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-A", judgmentEvidenceAt, carrierX),
		judgmentEvidenceAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("consider X: %v", err)
	}

	// 同段另一对象的可信渠道回传指向 Y：不是「两个对象两个承运商」，是证据冲突（ADR-0103 Context 一）。
	conflicting, err := identified.Consider(
		registeredEvidence(t, domain.TrustedChannelCallback, "CALLBACK-B", judgmentEvidenceAt.Add(time.Hour), carrierY),
		judgmentEvidenceAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("consider Y: %v", err)
	}

	current := conflicting.Current()
	reason, pending := current.Verdict().Pending()
	if !pending || reason != domain.CarrierEvidenceSourceConflict {
		t.Fatalf("应为待确认（来源冲突），实得 pending=%v reason=%s", pending, reason)
	}
	if _, stillIdentified := current.Verdict().Identified(); stillIdentified {
		t.Fatal("冲突之下不挑一个——不该仍识别出任何主体")
	}
	if len(current.Bases()) != 2 {
		t.Fatalf("冲突版本要保留全部依据，实得 %d 条", len(current.Bases()))
	}
	// 已识别的那一版保留，未知期间不倒填。
	versions := conflicting.Versions()
	if subject, wasIdentified := versions[1].Verdict().Identified(); !wasIdentified || subject != carrierX {
		t.Fatal("已识别版本被改写了")
	}

	// 冲突之中再来一条指向 X 的证据：仍是冲突——不由证据多少或到达先后自动裁，冲突由人裁。
	stillConflicting, err := conflicting.Consider(
		registeredEvidence(t, domain.CarrierReceiptVoucher, "RECEIPT-C", judgmentEvidenceAt.Add(3*time.Hour), carrierX),
		judgmentEvidenceAt.Add(4*time.Hour))
	if err != nil {
		t.Fatalf("consider X again: %v", err)
	}
	if reason, pending := stillConflicting.Current().Verdict().Pending(); !pending || reason != domain.CarrierEvidenceSourceConflict {
		t.Fatalf("多一条指向 X 的证据不该自动裁掉冲突，实得 pending=%v reason=%s", pending, reason)
	}
}

// 两条依据指向同一在册主体：仍是已识别，且两条都在依据里——「同一个答案的多份证据」不是冲突。
func TestConcurringEvidenceKeepsTheCarrierIdentified(t *testing.T) {
	carrierX := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
	first, err := mustOpenJudgment(t).Consider(
		registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-A", judgmentEvidenceAt, carrierX),
		judgmentEvidenceAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("consider: %v", err)
	}
	second, err := first.Consider(
		registeredEvidence(t, domain.HandedOverToCarrier, "HANDOVER-B", judgmentEvidenceAt.Add(time.Hour), carrierX),
		judgmentEvidenceAt.Add(time.Hour+time.Minute))
	if err != nil {
		t.Fatalf("consider again: %v", err)
	}
	subject, identified := second.Current().Verdict().Identified()
	if !identified || subject != carrierX || len(second.Current().Bases()) != 2 {
		t.Fatalf("两条一致证据应仍识别为 X 且两条都保留：identified=%v bases=%d", identified, len(second.Current().Bases()))
	}
}

// 同一份依据不形成第二个版本：重放是业务答案，判断不变，未知期间不凭空断成两截。
func TestTheSameEvidenceIsNotConsideredTwice(t *testing.T) {
	carrierX := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
	evidence := registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-A", judgmentEvidenceAt, carrierX)
	once, err := mustOpenJudgment(t).Consider(evidence, judgmentEvidenceAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("consider: %v", err)
	}
	if _, err := once.Consider(evidence, judgmentEvidenceAt.Add(2*time.Minute)); !errors.Is(err, domain.ErrCarrierEvidenceAlreadyConsidered) {
		t.Fatalf("error = %v, want ErrCarrierEvidenceAlreadyConsidered", err)
	}
	// 同引用异内容也算同一份依据：引用指名的是那条来源事实，事实本身变了走更正，不在这里悄悄换掉。
	sameReferenceOtherTime := registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-A", judgmentEvidenceAt.Add(time.Hour), carrierX)
	if _, err := once.Consider(sameReferenceOtherTime, judgmentEvidenceAt.Add(2*time.Minute)); !errors.Is(err, domain.ErrCarrierEvidenceAlreadyConsidered) {
		t.Fatalf("error = %v, want ErrCarrierEvidenceAlreadyConsidered", err)
	}
}

// Covers: CONTEXT「证据里的承运方名称在参与方册上找不到对应身份时，不据名称铸身份，形成待确认（承运
// 主体身份未登记），名称作为素材随依据保留」与生命周期「待确认（承运主体身份未登记）→ 已识别：
// `party-commercial` 登记了对应身份后，凭同一份证据形成新版本；不追溯改写未登记期间」；票面验收场景
// 「身份未登记→登记后新版本」。
func TestAnUnregisteredCarrierNameLeavesTheJudgmentPendingUntilTheIdentityIsRecognised(t *testing.T) {
	pending, err := mustOpenJudgment(t).Consider(
		unregisteredEvidence(t, domain.TrustedChannelCallback, "CALLBACK-1", judgmentEvidenceAt, "Y Express"),
		judgmentEvidenceAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("consider: %v", err)
	}
	reason, isPending := pending.Current().Verdict().Pending()
	if !isPending || reason != domain.CarrierIdentityNotRegistered {
		t.Fatalf("应为待确认（承运主体身份未登记），实得 pending=%v reason=%s", isPending, reason)
	}
	basis := pending.Current().Bases()[0]
	if basis.Material() != "Y Express" {
		t.Fatalf("名称素材没有随依据保留：%q", basis.Material())
	}
	if _, hasSubject := basis.Subject(); hasSubject {
		t.Fatal("未登记的名称被铸成了身份")
	}

	// PC 登记之后凭同一份证据补认身份：新版本已识别，业务时间仍是那条证据的时间（事实史），形成时间是
	// 补认那一刻（知识史）——未登记期间原样留在上一版里。
	carrierY := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-y")
	recognisedAt := judgmentEvidenceAt.Add(24 * time.Hour)
	recognised, err := pending.RecogniseCarrierIdentity(
		mustRef(t, domain.NewCarrierEvidenceReference, "CALLBACK-1"), carrierY, recognisedAt)
	if err != nil {
		t.Fatalf("recognise: %v", err)
	}
	if len(recognised.Versions()) != 3 {
		t.Fatalf("补认应追加一版，实得 %d 版", len(recognised.Versions()))
	}
	current := recognised.Current()
	subject, identified := current.Verdict().Identified()
	if !identified || subject != carrierY {
		t.Fatalf("应识别为 carrier-y，实得 identified=%v subject=%+v", identified, subject)
	}
	if !current.BusinessTime().Equal(judgmentEvidenceAt) || !current.FormedAt().Equal(recognisedAt) {
		t.Fatalf("业务时间应仍为证据时间 %s、形成时间应为补认时刻 %s；实得 %s / %s",
			judgmentEvidenceAt, recognisedAt, current.BusinessTime(), current.FormedAt())
	}
	if got := current.Bases(); len(got) != 1 || got[0].Material() != "" {
		t.Fatalf("补认后的依据应指向身份而不再只有素材：%+v", got)
	}
	if recognisedSubject, has := current.Bases()[0].Subject(); !has || recognisedSubject != carrierY {
		t.Fatal("补认后的依据没有带上身份引用")
	}
	// 上一版（未登记期间）一字不动。
	if got := recognised.Versions()[1].Bases()[0].Material(); got != "Y Express" {
		t.Fatalf("未登记期间那一版被追溯改写了：%q", got)
	}
}

// 补认身份之后若与既有在册依据相左，答案照旧是冲突——补认不是裁决。
func TestRecognisingAnIdentityThatContradictsOtherEvidenceFormsAConflict(t *testing.T) {
	carrierX := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
	carrierY := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-y")
	judgment, err := mustOpenJudgment(t).Consider(
		registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-A", judgmentEvidenceAt, carrierX),
		judgmentEvidenceAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("consider X: %v", err)
	}
	judgment, err = judgment.Consider(
		unregisteredEvidence(t, domain.TrustedChannelCallback, "CALLBACK-B", judgmentEvidenceAt.Add(time.Hour), "Y Express"),
		judgmentEvidenceAt.Add(time.Hour+time.Minute))
	if err != nil {
		t.Fatalf("consider unregistered: %v", err)
	}
	// 一条在册、一条未在册：先答身份未登记，不因另一条在册就答已识别。
	if reason, pending := judgment.Current().Verdict().Pending(); !pending || reason != domain.CarrierIdentityNotRegistered {
		t.Fatalf("在册与未在册并存应答身份未登记，实得 pending=%v reason=%s", pending, reason)
	}

	recognised, err := judgment.RecogniseCarrierIdentity(
		mustRef(t, domain.NewCarrierEvidenceReference, "CALLBACK-B"), carrierY, judgmentEvidenceAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("recognise: %v", err)
	}
	if reason, pending := recognised.Current().Verdict().Pending(); !pending || reason != domain.CarrierEvidenceSourceConflict {
		t.Fatalf("补认成 Y 与既有 X 相左应为来源冲突，实得 pending=%v reason=%s", pending, reason)
	}
}

func TestRecognisingAnIdentityRefusesEvidenceItCannotFindOrThatIsAlreadyRecognised(t *testing.T) {
	carrierX := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
	judgment, err := mustOpenJudgment(t).Consider(
		registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-A", judgmentEvidenceAt, carrierX),
		judgmentEvidenceAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("consider: %v", err)
	}
	later := judgmentEvidenceAt.Add(time.Hour)

	if _, err := judgment.RecogniseCarrierIdentity(mustRef(t, domain.NewCarrierEvidenceReference, "NOT-THERE"), carrierX, later); !errors.Is(err, domain.ErrCarrierEvidenceNotConsidered) {
		t.Fatalf("不在依据里的引用：error = %v, want ErrCarrierEvidenceNotConsidered", err)
	}
	if _, err := judgment.RecogniseCarrierIdentity(mustRef(t, domain.NewCarrierEvidenceReference, "SCAN-A"), carrierX, later); !errors.Is(err, domain.ErrCarrierIdentityAlreadyRecognised) {
		t.Fatalf("已指向在册身份的依据：error = %v, want ErrCarrierIdentityAlreadyRecognised", err)
	}
	if _, err := judgment.RecogniseCarrierIdentity(mustRef(t, domain.NewCarrierEvidenceReference, "SCAN-A"), domain.CarrierSubject{}, later); !errors.Is(err, domain.ErrInvalidActualCarrierJudgment) {
		t.Fatalf("零值身份：error = %v, want ErrInvalidActualCarrierJudgment", err)
	}
}

func TestConsideringRequiresAValidEvidenceAndAFormationTime(t *testing.T) {
	judgment := mustOpenJudgment(t)
	carrierX := mustCarrierSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
	evidence := registeredEvidence(t, domain.CarrierDirectPickupScan, "SCAN-1", judgmentEvidenceAt, carrierX)

	if _, err := judgment.Consider(domain.CarrierEvidence{}, judgmentFormedAt); !errors.Is(err, domain.ErrInvalidCarrierEvidence) {
		t.Fatalf("零值依据：error = %v, want ErrInvalidCarrierEvidence", err)
	}
	if _, err := judgment.Consider(evidence, time.Time{}); !errors.Is(err, domain.ErrInvalidActualCarrierJudgment) {
		t.Fatalf("缺形成时间：error = %v, want ErrInvalidActualCarrierJudgment", err)
	}
}

func TestOpeningAJudgmentRequiresItsKeyAndBothTimes(t *testing.T) {
	cases := map[string]func(*domain.OpenActualCarrierJudgmentSpec){
		"缺租户":    func(spec *domain.OpenActualCarrierJudgmentSpec) { spec.TenantID = domain.TenantID{} },
		"缺段":     func(spec *domain.OpenActualCarrierJudgmentSpec) { spec.Segment = domain.FulfillmentSegmentReference{} },
		"缺段成立时刻": func(spec *domain.OpenActualCarrierJudgmentSpec) { spec.EstablishedAt = time.Time{} },
		"缺形成时间":  func(spec *domain.OpenActualCarrierJudgmentSpec) { spec.FormedAt = time.Time{} },
	}
	for name, breakOne := range cases {
		t.Run(name, func(t *testing.T) {
			spec := openJudgmentSpec(t)
			breakOne(&spec)
			if _, err := domain.OpenActualCarrierJudgment(spec); !errors.Is(err, domain.ErrInvalidActualCarrierJudgment) {
				t.Fatalf("error = %v, want ErrInvalidActualCarrierJudgment", err)
			}
		})
	}
}
