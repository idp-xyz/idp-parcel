package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 层内更正关系的落库（迁移 0027）：更正指向是来源内容，与接收记录同一行；更正是否生效是接收那一刻的判断，
// 同样只建立不改写。库里再守一遍形状——生效的更正只能指在册事实、一份事实至多被一份生效的更正取代、记了
// 冲突的更正不生效——放行读口按这层关系排掉被取代的那份。

// correctingRecord 把 record 改成指名 corrected 的更正；inForce 是接收时判出的更正是否生效。
func correctingRecord(t *testing.T, record ports.ExternalResultRecord, corrected string, inForce bool) ports.ExternalResultRecord {
	t.Helper()
	result := record.Result
	rebuilt, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer:        result.Layer(),
		SourceID:     result.SourceID(),
		Role:         result.Role(),
		RawSemantics: result.RawSemantics(),
		Rule:         result.Rule(),
		Version:      result.Version(),
		Attempt:      result.Attempt(),
		Scope:        result.Scope(),
		OccurredAt:   result.OccurredAt(),
		ReceivedAt:   result.ReceivedAt(),
		Corrects:     corrected,
	})
	if err != nil {
		t.Fatalf("构造更正：%v", err)
	}
	record.Result = rebuilt
	record.CorrectionInForce = inForce
	return record
}

func saveAll(t *testing.T, repository *adapter.ExternalResults, transactor bentoapp.Transactor, records ...ports.ExternalResultRecord) {
	t.Helper()
	inTx2(t, transactor, t.Context(), func(txCtx context.Context) error {
		for _, record := range records {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})
}

// Covers: 更正指向与更正是否生效随记录原样落库、原样读回——FindByKey 与同层一致性读面两处都带着；不带更正
// 指向的记录读回如实没有。
func TestACorrectionRelationRoundTripsThroughBothReads(t *testing.T) {
	repository, transactor := newExternalResults(t)
	original := attributedRecord(t, "tenant-a", "resp-1", "submission-1")
	correction := correctingRecord(t, attributedRecord(t, "tenant-a", "resp-2", "submission-1"), "resp-1", true)
	saveAll(t, repository, transactor, original, correction)

	found, exists, err := repository.FindByKey(t.Context(), correction.Key)
	if err != nil || !exists {
		t.Fatalf("取回更正：%v exists=%v", err, exists)
	}
	if corrected, has := found.Result.Corrects(); !has || corrected != "resp-1" || !found.CorrectionInForce {
		t.Fatalf("更正关系没读回：corrects=%q in force=%v", corrected, found.CorrectionInForce)
	}
	plain, _, err := repository.FindByKey(t.Context(), original.Key)
	if err != nil {
		t.Fatalf("取回原结果：%v", err)
	}
	if _, has := plain.Result.Corrects(); has || plain.CorrectionInForce {
		t.Fatalf("原结果读回凭空带了更正关系：%+v", plain)
	}

	facts := loadForSubmission(t, repository, transactor, make2(t, domain.NewTenantID, "tenant-a"), "submission-1")
	if len(facts) != 2 {
		t.Fatalf("读回 %d 份，want 2", len(facts))
	}
	for _, fact := range facts {
		corrected, has := fact.Result.Corrects()
		switch fact.Result.SourceID() {
		case "resp-1":
			if has || fact.CorrectionInForce {
				t.Fatalf("原结果在比对读面上带了更正关系：%+v", fact)
			}
		case "resp-2":
			if !has || corrected != "resp-1" || !fact.CorrectionInForce {
				t.Fatalf("更正在比对读面上没带更正关系：%+v", fact)
			}
		}
	}
}

// Covers: 库内守更正关系的形状，旁路写入用显式 SQL 绕开一切 Go 侧校验——指向不在册的那份进不来（外键）；同一份
// 事实已被一份生效的更正取代，第二份生效的更正进不来（只许更正链尾）；记了冲突却生效、生效却无指向、归属不上
// 的留存带指向、指向自己，都被 CHECK 挡住。
func TestTheRegisterRejectsMalformedCorrectionRelations(t *testing.T) {
	fixture := newViewFixture(t)
	repository, err := adapter.NewExternalResults(fixture.db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	saveAll(t, repository, fixture.db.Transactor(),
		attributedRecord(t, "tenant-a", "resp-1", "submission-1"),
		correctingRecord(t, attributedRecord(t, "tenant-a", "resp-2", "submission-1"), "resp-1", true),
	)
	insert := `INSERT INTO customs_compliance.external_result
		(tenant_id, source_id, content_digest, unattributable, raw_semantics, claimed_version,
		 layer_conflict, result, submission_version, recorded_at, corrects_source_id, correction_in_force)
		VALUES ('tenant-a', $1, 'digest', $2, '', '', $3, $4, $5, $6, $7, $8)`
	body := func(source string) string {
		return `{"layer":"BUSINESS_ACCEPTANCE","sourceId":"` + source + `","role":"REGULATOR","rawSemantics":"X","rule":"interpretation/v1","version":"submission-1","attempt":1,"scope":"declaration/full","occurredAt":"2026-08-13T15:00:00Z","receivedAt":"2026-08-13T16:00:00Z"}`
	}
	version := "submission-1"
	fixture.rejects(t, "指向不在册的那份", insert,
		"resp-x", false, false, body("resp-x"), version, externalReceivedAt, "resp-nowhere", false)
	fixture.rejects(t, "第二份生效的更正", insert,
		"resp-3", false, false, body("resp-3"), version, externalReceivedAt, "resp-1", true)
	fixture.rejects(t, "记了冲突却生效", insert,
		"resp-4", false, true, body("resp-4"), version, externalReceivedAt, "resp-2", true)
	fixture.rejects(t, "生效却无指向", insert,
		"resp-5", false, false, body("resp-5"), version, externalReceivedAt, nil, true)
	fixture.rejects(t, "归属不上的留存带指向", insert,
		"resp-6", true, false, nil, nil, externalReceivedAt, "resp-1", false)
	fixture.rejects(t, "指向自己", insert,
		"resp-8", false, false, body("resp-8"), version, externalReceivedAt, "resp-8", false)

	// 对照：记了冲突、不生效的第二份更正进得来——冲突关系要双方留存。
	fixture.seed(t, insert, "resp-7", false, true, body("resp-7"), version, externalReceivedAt, "resp-1", false)
}

// Covers: AT-CC-156 与 CC CONTEXT「后续监管更正形成新的外部事实及当前判断，不覆盖原放行结果」的读口半边——放行读口把
// 被生效更正取代的那份照样交出，带上更正那份的业务发生时间作取代时点，更正那份另作一份、带它自己的业务发生时间；
// 哪个时点算哪一份归领域。原行仍在库里。更正不生效（记了冲突）时原放行不带取代时点。
func TestTheReleaseViewCarriesWhenACorrectionSupersededARelease(t *testing.T) {
	fixture := newViewFixture(t)
	repository, err := adapter.NewExternalResults(fixture.db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	full := releaseRecord(t, "tenant-a", "resp-full", domain.FullRelease, "")
	narrowed := correctingRecord(t, releaseRecord(t, "tenant-a", "resp-partial", domain.PartialRelease, ""), "resp-full", true)
	narrowed.Result = occurringAt(t, narrowed.Result, full.Result.OccurredAt().Add(30*time.Minute))
	saveAll(t, repository, fixture.db.Transactor(), full, narrowed)

	view, err := adapter.NewReleaseOutcomeView(fixture.db)
	if err != nil {
		t.Fatalf("构造放行读口：%v", err)
	}
	readings := releaseReadings(t, view, full.Result.Scope())
	if len(readings) != 2 {
		t.Fatalf("放行读口交出 %+v，want 原放行与更正两份", readings)
	}
	if got := readings["resp-full"]; !got.OccurredAt.Equal(full.Result.OccurredAt()) || !got.SupersededAt.Equal(narrowed.Result.OccurredAt()) {
		t.Fatalf("原放行 occurred=%s superseded=%s，want %s / %s", got.OccurredAt, got.SupersededAt,
			full.Result.OccurredAt(), narrowed.Result.OccurredAt())
	}
	if got := readings["resp-partial"]; !got.OccurredAt.Equal(narrowed.Result.OccurredAt()) || !got.SupersededAt.IsZero() {
		t.Fatalf("更正 occurred=%s superseded=%s——更正按它自己的业务发生时间算数、没被取代", got.OccurredAt, got.SupersededAt)
	}

	other := newViewFixture(t)
	otherRepository, err := adapter.NewExternalResults(other.db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	inert := correctingRecord(t, releaseRecord(t, "tenant-a", "resp-partial", domain.PartialRelease, ""), "resp-full", false)
	inert.LayerConflict = true
	saveAll(t, otherRepository, other.db.Transactor(), full, inert)
	otherView, err := adapter.NewReleaseOutcomeView(other.db)
	if err != nil {
		t.Fatalf("构造放行读口：%v", err)
	}
	got := releaseReadings(t, otherView, full.Result.Scope())
	if len(got) != 1 || !got["resp-full"].SupersededAt.IsZero() {
		t.Fatalf("不生效的更正取代了原放行：%+v", got)
	}
}

// occurringAt 换掉一份结果的业务发生时间，其余各件照旧。
func occurringAt(t *testing.T, result domain.ExternalResult, at time.Time) domain.ExternalResult {
	t.Helper()
	corrected, _ := result.Corrects()
	rebuilt, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer: result.Layer(), SourceID: result.SourceID(), Role: result.Role(), RawSemantics: result.RawSemantics(),
		Rule: result.Rule(), Version: result.Version(), Attempt: result.Attempt(), Scope: result.Scope(),
		OccurredAt: at, ReceivedAt: result.ReceivedAt(), Corrects: corrected,
	})
	if err != nil {
		t.Fatalf("改业务发生时间：%v", err)
	}
	return rebuilt
}

func releaseReadings(t *testing.T, view *adapter.ReleaseOutcomeView, scope domain.DecisionScopeReference) map[string]domain.ReleaseReading {
	t.Helper()
	readings, err := view.ListReleaseOutcomes(t.Context(), make2(t, domain.NewTenantID, "tenant-a"), scope)
	if err != nil {
		t.Fatalf("放行读口：%v", err)
	}
	bySource := make(map[string]domain.ReleaseReading, len(readings))
	for _, reading := range readings {
		bySource[reading.Source] = reading
	}
	return bySource
}
