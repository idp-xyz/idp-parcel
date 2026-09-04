package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 放行层记录的往返（0015 建表，票 mechanism-executor-triage/07 CC-b）：放行事实与分层事实
// 同键同事务落地、同一次 FindByKey 一起读回；没有放行三件的记录读回时 Release 为空。库内
// 再守一遍「没有分层事实就没有放行事实」与「条件随种类」。

func releaseRecord(t *testing.T, tenant, sourceID string, kind domain.ReleaseKind, condition string) ports.ExternalResultRecord {
	t.Helper()
	record := attributedRecord(t, tenant, sourceID, "submission-release")
	record.Result = attributedResult(t, sourceID, "submission-release", domain.ReleaseResultLayer)
	release, err := domain.ReceiveReleaseOutcome(kind,
		make2(t, domain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY-01"),
		record.Result.Scope(), condition, record.Result.ReceivedAt())
	if err != nil {
		t.Fatalf("构造放行结果：%v", err)
	}
	record.Release = &release
	return record
}

// Covers: 放行层记录带放行事实落库后，FindByKey 把分层事实与放行事实一起读回，四件如实。
func TestAReleaseRecordRoundTripsWithItsReleaseOutcome(t *testing.T) {
	repository, transactor := newExternalResults(t)
	ctx := t.Context()
	record := releaseRecord(t, "tenant-a", "resp-release-1", domain.ConditionalRelease, "re-export within 30 days")

	inTx2(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.ExternalResultSaved {
			t.Errorf("首次保存该是 Saved：%v", outcome)
		}
		return nil
	})

	found, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("取回放行层记录：%v exists=%v", err, exists)
	}
	if found.Release == nil {
		t.Fatalf("放行事实没随记录读回：%+v", found)
	}
	condition, conditional := found.Release.Condition()
	if found.Release.Kind() != domain.ConditionalRelease || !conditional || condition != "re-export within 30 days" ||
		found.Release.Authority().String() != "SYN-AUTHORITY-01" ||
		found.Release.Scope() != record.Result.Scope() ||
		!found.Release.ReceivedAt().Equal(record.Result.ReceivedAt()) {
		t.Fatalf("放行事实走样：kind=%s condition=%q authority=%s", found.Release.Kind(), condition, found.Release.Authority())
	}
	if found.Result.Layer() != domain.ReleaseResultLayer {
		t.Fatalf("分层事实层位走样：%s", found.Result.Layer())
	}
}

// Covers: 不带放行事实的记录（非放行层）读回 Release 为空——不为别的层凭空造放行。
func TestANonReleaseRecordReadsBackWithoutAReleaseOutcome(t *testing.T) {
	repository, transactor := newExternalResults(t)
	ctx := t.Context()
	record := attributedRecord(t, "tenant-a", "resp-acceptance-1", "submission-1")

	inTx2(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, record)
		return err
	})
	found, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil || !exists || found.Release != nil {
		t.Fatalf("非放行层记录不该带放行事实：err=%v exists=%v release=%v", err, exists, found.Release)
	}
}

// Covers: 同键重放不重写放行事实（写入代数由分层事实那一行的 ON CONFLICT 决定，放行行
// 随之一起不落）——`已有记录`交回，库里仍是首版种类。
func TestReplayingAReleaseRecordKeepsTheFirstReleaseOutcome(t *testing.T) {
	repository, transactor := newExternalResults(t)
	ctx := t.Context()
	first := releaseRecord(t, "tenant-a", "resp-release-2", domain.FullRelease, "")
	second := releaseRecord(t, "tenant-a", "resp-release-2", domain.PartialRelease, "")

	inTx2(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, first); err != nil {
			return err
		}
		outcome, err := repository.Save(txCtx, second)
		if err != nil {
			return err
		}
		if outcome != ports.ExternalResultAlreadyRecorded {
			t.Errorf("同键重放该是 AlreadyRecorded：%v", outcome)
		}
		return nil
	})
	found, _, err := repository.FindByKey(ctx, first.Key)
	if err != nil || found.Release == nil || found.Release.Kind() != domain.FullRelease {
		t.Fatalf("重放顶掉了首版放行事实：err=%v release=%v", err, found.Release)
	}
}

// Covers: 库内守形状——没有分层事实的放行行进不来（外键）；附条件无条件、全部放行带条件、
// 种类集外都被 CHECK 挡住。旁路写入用显式 SQL，绕开一切 Go 侧校验。
func TestTheReleaseOutcomeTableRejectsWhatTheDomainRejects(t *testing.T) {
	fixture := newViewFixture(t)
	insert := `INSERT INTO customs_compliance.release_outcome
		(tenant_id, source_id, kind, authority_ref, scope_ref, condition, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	fixture.rejects(t, "没有分层事实的放行事实", insert,
		"tenant-a", "resp-orphan", "FULL", "SYN-AUTHORITY-01", "declaration/full", "", externalReceivedAt)

	fixture.seed(t, `INSERT INTO customs_compliance.external_result
		(tenant_id, source_id, content_digest, unattributable, raw_semantics, claimed_version,
		 layer_conflict, result, submission_version, recorded_at)
		VALUES ('tenant-a', 'resp-shape', 'digest-shape', false, 'RELEASE', 'submission-1', false,
		        '{"layer":"RELEASE_RESULT","sourceId":"resp-shape","role":"REGULATOR","rawSemantics":"RELEASE","rule":"interpretation/v1","version":"submission-1","attempt":1,"scope":"declaration/full","occurredAt":"2026-08-13T15:00:00Z","receivedAt":"2026-08-13T16:00:00Z"}',
		        'submission-1', $1)`, externalReceivedAt)
	fixture.rejects(t, "附条件放行无条件", insert,
		"tenant-a", "resp-shape", "CONDITIONAL", "SYN-AUTHORITY-01", "declaration/full", "  ", externalReceivedAt)
	fixture.rejects(t, "全部放行带条件", insert,
		"tenant-a", "resp-shape", "FULL", "SYN-AUTHORITY-01", "declaration/full", "surprise", externalReceivedAt)
	fixture.rejects(t, "种类集外", insert,
		"tenant-a", "resp-shape", "MAYBE", "SYN-AUTHORITY-01", "declaration/full", "", externalReceivedAt)
}
