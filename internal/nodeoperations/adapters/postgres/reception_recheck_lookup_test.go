package postgres_test

import (
	"context"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件对真实 PostgreSQL 16 证重核用的两问（ADR-0199 决定三，票 no-reception-source/03）：按（租户，号码）找待重核的待识别，
// 按（租户，判断版本）取一行。夹具全为合成引用（S 级）。

// recheckablePending 是一行零候选的待识别：不记身份冲突、带来源文档（号码 mark）、判断版本即收寄结果版本 version。
func recheckablePending(t *testing.T, tenant, sourceID, version, mark string) ports.ReceptionRecord {
	t.Helper()
	record := pendingRecord(t, tenant, sourceID)
	intake, err := domain.FormNodeIntake(domain.NodeIntakeSpec{
		TenantID:    record.Key.TenantID,
		Unit:        record.Intake.Unit(),
		Node:        record.Intake.Node(),
		DeliveredBy: record.Intake.DeliveredBy(),
		Evidence:    record.Intake.Evidence(),
		Version:     ref(t, domain.NewIntakeResultVersion, version),
		ReceivedAt:  record.Intake.ReceivedAt(),
	})
	if err != nil {
		t.Fatalf("构造待识别收寄：%v", err)
	}
	record.Intake = intake
	record.JudgmentVersion = intake.Version()
	record.Candidates = nil
	record.IdentityConflict = false
	record.Source = deliverySource(t, record.Intake.Unit().String(), mark, "RECEIVED")
	return record
}

func sourceIDsOf(records []ports.ReceptionRecord) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.Key.SourceID)
	}
	return ids
}

// Covers: ADR-0199 决定三「重核对象」的存储面——按号只找回当前有效版本是待识别、不带关联、不记身份冲突、来源文档的号逐字
// 相等的那几行，按来源标识定序：记身份冲突的、已有后继的、号差一个空白的、来源未保全的旧行、别的租户的、已形成的都不在
// 其中。号长过 btree 一项的上限也落得下、找得回（索引按号的摘要建，不按号本身）。
func TestPendingUnitsAreFoundByThePreservedMark(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	const tenant, mark = "SYN-TENANT-NRS03", "SYN-MARK-NRS03"
	long := strings.Repeat("SYN-LONG-MARK-", 300)

	conflicted := recheckablePending(t, tenant, "SYN-SCAN-NRS03-CONFLICT", "SYN-INTAKEV-NRS03-CONFLICT", mark)
	conflicted.IdentityConflict = true
	conflicted.Candidates = pendingRecord(t, tenant, "unused").Candidates
	superseded := recheckablePending(t, tenant, "SYN-SCAN-NRS03-SUPERSEDED", "SYN-INTAKEV-NRS03-SUPERSEDED", mark)
	successor := recheckablePending(t, tenant, "SYN-SCAN-NRS03-SUCCESSOR", "SYN-INTAKEV-NRS03-SUCCESSOR", mark)
	successor.Predecessor = superseded.JudgmentVersion
	successor.IdentityConflict = true
	successor.Candidates = conflicted.Candidates
	spaced := recheckablePending(t, tenant, "SYN-SCAN-NRS03-SPACED", "SYN-INTAKEV-NRS03-SPACED", mark+" ")
	sourceless := recheckablePending(t, tenant, "SYN-SCAN-NRS03-OLD", "SYN-INTAKEV-NRS03-OLD", mark)
	sourceless.Source = domain.DeliverySource{}
	elsewhere := recheckablePending(t, "SYN-TENANT-NRS03-OTHER", "SYN-SCAN-NRS03-B", "SYN-INTAKEV-NRS03-ELSEWHERE", mark)
	formed := formedRecord(t, tenant, "SYN-SCAN-NRS03-FORMED")
	formed.JudgmentVersion = formed.Intake.Version()
	formed.Source = deliverySource(t, "unit-1", mark, "RECEIVED")
	second := recheckablePending(t, tenant, "SYN-SCAN-NRS03-B", "SYN-INTAKEV-NRS03-B", mark)
	first := recheckablePending(t, tenant, "SYN-SCAN-NRS03-A", "SYN-INTAKEV-NRS03-A", mark)
	longMarked := recheckablePending(t, tenant, "SYN-SCAN-NRS03-LONG", "SYN-INTAKEV-NRS03-LONG", long)

	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ReceptionRecord{
			conflicted, superseded, successor, spaced, sourceless, elsewhere, formed, second, first, longMarked,
		} {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	found, err := repository.FindRecheckableByMark(ctx, ref(t, domain.NewTenantID, tenant), mark)
	if err != nil {
		t.Fatalf("按号找：%v", err)
	}
	if got := sourceIDsOf(found); strings.Join(got, ",") != "SYN-SCAN-NRS03-A,SYN-SCAN-NRS03-B" {
		t.Fatalf("按号找到 %v, want [SYN-SCAN-NRS03-A SYN-SCAN-NRS03-B]", got)
	}
	if found[0].Source != first.Source || found[0].JudgmentVersion != first.JudgmentVersion || found[0].Control != first.Control {
		t.Fatalf("找回的那一行没有原样读回：%+v", found[0])
	}

	byLong, err := repository.FindRecheckableByMark(ctx, ref(t, domain.NewTenantID, tenant), long)
	if err != nil || strings.Join(sourceIDsOf(byLong), ",") != "SYN-SCAN-NRS03-LONG" {
		t.Fatalf("按长号找到 %v（err %v）, want [SYN-SCAN-NRS03-LONG]", sourceIDsOf(byLong), err)
	}
	none, err := repository.FindRecheckableByMark(ctx, ref(t, domain.NewTenantID, tenant), "SYN-MARK-NRS03-ABSENT")
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("号不在册 = %v（err %v）, want 空切片", none, err)
	}
}

// Covers: 指名重核与票 05 的自投门铃都按（租户，判断版本）取那一件：本租户里取得回、原样读回；别的租户里同名的版本、册上
// 没有的版本都答没有。
func TestAJudgmentIsFoundByItsVersion(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	pending := recheckablePending(t, "SYN-TENANT-NRS03", "SYN-SCAN-NRS03-NAMED", "SYN-INTAKEV-NRS03-NAMED", "SYN-MARK-NRS03")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, pending)
		return err
	})

	found, exists, err := repository.FindByJudgmentVersion(ctx, pending.Key.TenantID, pending.JudgmentVersion)
	if err != nil || !exists || found.Key != pending.Key || found.Source != pending.Source || found.Kind != ports.RecordPendingIdentification {
		t.Fatalf("按版本取回 %+v exists=%v err=%v", found, exists, err)
	}
	for name, lookup := range map[string]struct{ tenant, version string }{
		"别的租户": {"SYN-TENANT-NRS03-OTHER", "SYN-INTAKEV-NRS03-NAMED"},
		"册上没有": {"SYN-TENANT-NRS03", "SYN-INTAKEV-NRS03-ABSENT"},
	} {
		_, exists, err := repository.FindByJudgmentVersion(ctx, ref(t, domain.NewTenantID, lookup.tenant), ref(t, domain.NewIntakeResultVersion, lookup.version))
		if err != nil || exists {
			t.Fatalf("%s：exists=%v err=%v, want 没有", name, exists, err)
		}
	}
}
