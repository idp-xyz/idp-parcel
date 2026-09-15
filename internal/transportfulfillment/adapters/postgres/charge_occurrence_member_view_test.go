package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证发生项成员的窄只读口（pp-seams/01）：按 ChargeOccurrenceKey 精确到
// 有效性版本答成员载运对象清单（裸引用原样）、业务时点与主要业务范围；键不存在、版本不同、他租
// 一律 found=false 且不造默认成员。夹具复用 charge_occurrence_registry_test.go，全部为合成登记（S 级）。

// 同一结构体同时满足写侧登记册与只读口——只读口不是登记册的再导出，而是同一实现者的第二份契约。
var _ ports.ChargeOccurrenceMemberView = (*adapter.ChargeOccurrences)(nil)

// TestChargeOccurrenceMemberViewAnswersMembersBusinessTimeAndScopeByKey 证按键取到成员清单、
// 业务时点与主要业务范围，成员按登记时的引用原样交回、不带种类。
func TestChargeOccurrenceMemberViewAnswersMembersBusinessTimeAndScopeByKey(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()

	mustSaveOccurrence(t, transactor, ctx, repository,
		occurrenceRecord(t, "SYN-OCC-0101", "v1", "SYN-PARCEL-1", "SYN-UNIT-7", "SYN-PARCEL-2"))

	members, found, err := repository.LoadMembers(ctx, occurrenceKey(t, "tenant-1", "SYN-OCC-0101", "v1"))
	if err != nil || !found {
		t.Fatalf("按键取成员：%v found=%v", err, found)
	}
	if got := members.Members; len(got) != 3 ||
		got[0].String() != "SYN-PARCEL-1" || got[1].String() != "SYN-PARCEL-2" || got[2].String() != "SYN-UNIT-7" {
		t.Fatalf("成员清单没有原样带回：%v", got)
	}
	if !members.OccurredAt.Equal(occurredAtFixture) {
		t.Fatalf("业务时点变形：%v", members.OccurredAt)
	}
	if members.Scope.String() != "scope-1" {
		t.Fatalf("主要业务范围变形：%q", members.Scope)
	}
}

// TestChargeOccurrenceMemberViewAnswersNotFoundForAnUnknownKey 证键不在册答 found=false，
// 不造默认成员、不报错——「没有这个发生项」是业务答案，不是读不回来。
func TestChargeOccurrenceMemberViewAnswersNotFoundForAnUnknownKey(t *testing.T) {
	repository, _, _ := newChargeOccurrences(t)

	members, found, err := repository.LoadMembers(t.Context(), occurrenceKey(t, "tenant-1", "SYN-OCC-NEVER", "v1"))
	if err != nil || found {
		t.Fatalf("不存在的键：err=%v found=%v", err, found)
	}
	if len(members.Members) != 0 || !members.OccurredAt.IsZero() {
		t.Fatalf("不存在的键长出了默认成员或时点：%+v", members)
	}
}

// TestChargeOccurrenceMemberViewKeysOnTheExactValidityVersion 证键精确到有效性版本：只登了 v1
// 时问 v2 答 found=false，不退到「最近一版」；修订落成 v2 后两版各答各的成员。
func TestChargeOccurrenceMemberViewKeysOnTheExactValidityVersion(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()

	first := occurrenceRecord(t, "SYN-OCC-0102", "v1", "SYN-PARCEL-1")
	mustSaveOccurrence(t, transactor, ctx, repository, first)

	if _, found, err := repository.LoadMembers(ctx, occurrenceKey(t, "tenant-1", "SYN-OCC-0102", "v2")); err != nil || found {
		t.Fatalf("尚未登记的版本被答成了在册：err=%v found=%v", err, found)
	}

	revised, err := first.Occurrence.ReviseValidity(
		domain.OccurrenceSuperseded,
		occurrenceRef(t, domain.NewOccurrenceValidityVersion, "v2"),
		occurrenceRef(t, domain.NewOccurrenceBasisReference, "CORRECTION/SYN-SRC-1"),
		occurredAtFixture.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("形成修订：%v", err)
	}
	mustSaveOccurrence(t, transactor, ctx, repository, ports.ChargeOccurrenceRecord{
		Key:        occurrenceKey(t, "tenant-1", "SYN-OCC-0102", "v2"),
		Occurrence: revised,
		RecordedAt: occurredAtFixture,
	})

	for _, version := range []string{"v1", "v2"} {
		members, found, err := repository.LoadMembers(ctx, occurrenceKey(t, "tenant-1", "SYN-OCC-0102", version))
		if err != nil || !found {
			t.Fatalf("版本 %s 取不回：err=%v found=%v", version, err, found)
		}
		if len(members.Members) != 1 || members.Members[0].String() != "SYN-PARCEL-1" {
			t.Fatalf("版本 %s 的成员清单变形：%v", version, members.Members)
		}
	}
}

// TestChargeOccurrenceMemberViewIsInvisibleAcrossTenants 证租户隔离：他租拿同一发生项键问，答 found=false。
func TestChargeOccurrenceMemberViewIsInvisibleAcrossTenants(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()

	mustSaveOccurrence(t, transactor, ctx, repository, occurrenceRecord(t, "SYN-OCC-0103", "v1", "SYN-PARCEL-1"))

	if _, found, err := repository.LoadMembers(ctx, occurrenceKey(t, "tenant-other", "SYN-OCC-0103", "v1")); err != nil || found {
		t.Fatalf("他租看见了这条发生项的成员：err=%v found=%v", err, found)
	}
}

// TestChargeOccurrenceMemberViewRefusesAnOccurrenceRegisteredWithoutMembers 证库面不一致响亮报错：
// 本体在册而成员表为空是领域造不出的行（构造门拒空成员、Save 两表同笔落），本口不把空清单当答案。
func TestChargeOccurrenceMemberViewRefusesAnOccurrenceRegisteredWithoutMembers(t *testing.T) {
	repository, _, pool := newChargeOccurrences(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_charge_occurrence
		    (tenant_id, occurrence_ref, validity_version, journey_ref, legal_entity_ref,
		     provider_ref, agreement_ref, reason, fact_basis, scope_ref, quantity, unit_ref,
		     occurred_at, corrects_version, revision_kind, revision_basis, revised_at, recorded_at)
		 VALUES ('tenant-1','SYN-OCC-0104','v1','j','le','pr','ag','BOOKING','fb','sc',1,'u',now(),NULL,NULL,NULL,NULL,now())`,
	); err != nil {
		t.Fatalf("直插无成员本体：%v", err)
	}

	members, found, err := repository.LoadMembers(ctx, occurrenceKey(t, "tenant-1", "SYN-OCC-0104", "v1"))
	if err == nil {
		t.Fatalf("无成员的本体被当成答案交了出去：found=%v %+v", found, members)
	}
	if found || len(members.Members) != 0 {
		t.Fatalf("报错的同时还交了内容：found=%v %+v", found, members)
	}
}
