package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ADR-0112 在登记册一侧的四件：替代参与是第四个窄写口（只插不改）、
// 「在场」判据从 `ended_at IS NULL` 变为「且无人回指」、按对象反查「在场或已离场的当前参与所在段」、
// 以及迁移 0016 让领域造不出的链形（第二个根、分叉、悬空、自指、跨对象）在库内落不进去。
//
// 这些用例写在读过对方在途代码之前——接手另一个会话的在途实现时先写自己第一片 red，让它成为交叉验证
// 而不是那份实现的镜像（parallel-sessions）。判据全部取自 ADR-0112 与领域层 8b70c052 的读面。

// supersedingMember 是同一对象在同段内回指 root 的替代参与版本：入场依据换成更正后的来源版本，起点随更正。
func supersedingMember(t *testing.T, root domain.RehydrateParticipationSpec, version string, enteredAt time.Time) domain.RehydrateParticipationSpec {
	t.Helper()
	replacement := root
	replacement.EntryBasis = segmentRef(t, domain.NewParticipationBasisReference, root.EntryBasis.String()+"/"+version)
	replacement.EnteredAt = enteredAt
	replacement.Supersedes = root.EntryBasis
	return replacement
}

// rebuiltChainTail 从「根 + 替代版本」两行重建出链，交回链尾那一条领域值——窄写口收的是领域交出的参与，
// 而只有段能交出它。
func rebuiltChainTail(t *testing.T, segment string, root, replacement domain.RehydrateParticipationSpec) domain.FulfillmentParticipation {
	t.Helper()
	built, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       segmentRef(t, domain.NewTenantID, "tenant-1"),
		Segment:        segmentRef(t, domain.NewFulfillmentSegmentReference, segment),
		Participations: []domain.RehydrateParticipationSpec{root, replacement},
	})
	if err != nil {
		t.Fatalf("构造替代链：%v", err)
	}
	tail, present := built.ParticipationFor(root.Object)
	if !present || tail.EntryBasis() != replacement.EntryBasis {
		t.Fatalf("链尾不是替代版本：%+v", tail)
	}
	return tail
}

// TestSupersedingAParticipationIsInsertOnlyAndTheChainTailBecomesCurrent 证第四个窄口与 `Join` 同形：
// 只插一行替代版本，原参与一字不动（ended_at 仍空、不带回指）；重放撞键是业务答案；读回时链尾是当前、
// 根标已被替代、在场只数链尾；同段其他对象不受影响。
func TestSupersedingAParticipationIsInsertOnlyAndTheChainTailBecomesCurrent(t *testing.T) {
	repository, transactor, pool := newSegmentRegistry(t)
	ctx := t.Context()

	key := segmentKeyFixture(t, "tenant-1", "SEG-S001")
	root := activeMember(t, "parcel-1")
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-S001", root, activeMember(t, "parcel-2")))

	replacement := supersedingMember(t, root, "v2", segmentEnteredAtFixture.Add(time.Hour))
	tail := rebuiltChainTail(t, "SEG-S001", root, replacement)

	var first, second ports.ParticipationSupersedeOutcome
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if first, err = repository.Supersede(txCtx, key, tail, segmentEnteredAtFixture.Add(2*time.Hour)); err != nil {
			return err
		}
		second, err = repository.Supersede(txCtx, key, tail, segmentEnteredAtFixture.Add(2*time.Hour))
		return err
	})
	if first != ports.ParticipationSuperseded {
		t.Fatalf("首次替代 outcome = %d, want ParticipationSuperseded", first)
	}
	if second != ports.ParticipationAlreadySuperseded {
		t.Fatalf("重放替代 outcome = %d, want ParticipationAlreadySuperseded", second)
	}

	found, exists, err := repository.FindByKey(ctx, key)
	if err != nil || !exists {
		t.Fatalf("取回：%v exists=%v", err, exists)
	}
	parcel1 := segmentRef(t, domain.NewCarriedObjectReference, "parcel-1")
	current, present := found.Segment.ParticipationFor(parcel1)
	if !present || current.EntryBasis().String() != "OFFSITE-PICKUP/parcel-1/v2" || !current.Active() {
		t.Fatalf("链尾不是替代版本：%+v present=%v", current, present)
	}
	if supersedes, chained := current.Supersedes(); !chained || supersedes != root.EntryBasis {
		t.Fatalf("替代版本的回指没有往返：%v %v", supersedes, chained)
	}
	if !current.EnteredAt().Equal(segmentEnteredAtFixture.Add(time.Hour)) {
		t.Fatalf("替代版本的起点 = %v, want 更正后的时刻", current.EnteredAt())
	}
	history := found.Segment.ParticipationHistory(parcel1)
	if len(history) != 2 || !history[0].Superseded() || history[0].Active() || history[0].EntryBasis() != root.EntryBasis {
		t.Fatalf("历史不是「根已被替代 → 链尾」：%+v", history)
	}
	if !history[0].EnteredAt().Equal(segmentEnteredAtFixture) {
		t.Fatal("原参与被改写了")
	}
	if found.Segment.ActiveParticipations() != 2 {
		t.Fatalf("在场参与 = %d, want 2（parcel-1 只数链尾 + parcel-2）", found.Segment.ActiveParticipations())
	}
	if other, ok := found.Segment.ParticipationFor(segmentRef(t, domain.NewCarriedObjectReference, "parcel-2")); !ok || other.Superseded() || !other.Active() {
		t.Fatalf("同段另一对象被牵连：%+v", other)
	}

	// 库面：parcel-1 两行；根那一行 ended_at 仍空、不带回指——替代不是结束也不是改写。
	var rows int
	var rootEndedAt, rootSupersedes *string
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-S001' AND object_ref = 'parcel-1'`).Scan(&rows); err != nil {
		t.Fatalf("数行：%v", err)
	}
	if rows != 2 {
		t.Fatalf("parcel-1 有 %d 行, want 2（原参与一行 + 替代版本一行）", rows)
	}
	if err := pool.QueryRow(ctx,
		`SELECT ended_at::text, supersedes_entry_basis FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-S001' AND object_ref = 'parcel-1'
		    AND entry_basis = 'OFFSITE-PICKUP/parcel-1'`).Scan(&rootEndedAt, &rootSupersedes); err != nil {
		t.Fatalf("读根行：%v", err)
	}
	if rootEndedAt != nil || rootSupersedes != nil {
		t.Fatalf("根行被改写：ended_at=%v supersedes=%v", rootEndedAt, rootSupersedes)
	}
}

// TestASupersededVersionIsNeitherActiveNorEndable 证 ADR-0112 Consequences 那一句：`EndParticipation` 与
// `FindActiveSegments` 的「在场」判据从 `ended_at IS NULL` 变为「且无人回指」——被替代的版本永远不会被
// 结束（它不再是当前控制），也不算在场；结束落在链尾上之后，根行 ended_at 仍空而对象已不在场。
func TestASupersededVersionIsNeitherActiveNorEndable(t *testing.T) {
	repository, transactor, pool := newSegmentRegistry(t)
	ctx := t.Context()

	key := segmentKeyFixture(t, "tenant-1", "SEG-S002")
	root := activeMember(t, "parcel-1")
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-S002", root))
	replacement := supersedingMember(t, root, "v2", segmentEnteredAtFixture.Add(time.Hour))
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Supersede(txCtx, key, rebuiltChainTail(t, "SEG-S002", root, replacement), segmentEnteredAtFixture)
		return err
	})

	parcel1 := segmentRef(t, domain.NewCarriedObjectReference, "parcel-1")
	active, err := repository.FindActiveSegments(ctx, key.TenantID, parcel1)
	if err != nil || len(active) != 1 {
		t.Fatalf("链尾在场时应恰答一段：%+v err=%v", active, err)
	}

	// 试图结束被替代的根：没有匹配行——答`已离场`那一格而不是改写它。
	endedRoot := root
	endedRoot.EndKind = domain.EndedByControlTermination
	endedRoot.EndBasis = segmentRef(t, domain.NewParticipationBasisReference, "CONTROL-TERMINATION/root")
	endedRoot.EndedAt = segmentEnteredAtFixture.Add(3 * time.Hour)
	var onRoot ports.ParticipationEndOutcome
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		onRoot, err = repository.EndParticipation(txCtx, key, rebuiltParticipation(t, "SEG-S002", endedRoot))
		return err
	})
	if onRoot != ports.ParticipationAlreadyEnded {
		t.Fatalf("结束被替代的根 outcome = %d, want ParticipationAlreadyEnded（被替代的版本永远不会被结束）", onRoot)
	}

	// 结束链尾：唯一会被结束的那一条。
	endedTail := replacement
	endedTail.EndKind = domain.EndedByEffectiveDelivery
	endedTail.EndBasis = segmentRef(t, domain.NewParticipationBasisReference, "EFFECTIVE-DELIVERY/parcel-1")
	endedTail.EndedAt = segmentEnteredAtFixture.Add(8 * time.Hour)
	var onTail ports.ParticipationEndOutcome
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		onTail, err = repository.EndParticipation(txCtx, key, rebuiltChainTail(t, "SEG-S002", root, endedTail))
		return err
	})
	if onTail != ports.ParticipationEnded {
		t.Fatalf("结束链尾 outcome = %d, want ParticipationEnded", onTail)
	}

	active, err = repository.FindActiveSegments(ctx, key.TenantID, parcel1)
	if err != nil || len(active) != 0 {
		t.Fatalf("链尾已离场，对象不该再算在场（根行 ended_at 为空不算数）：%+v err=%v", active, err)
	}

	var rootEndedAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT ended_at FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-S002' AND entry_basis = 'OFFSITE-PICKUP/parcel-1'`).Scan(&rootEndedAt); err != nil {
		t.Fatalf("读根行：%v", err)
	}
	if rootEndedAt != nil {
		t.Fatalf("根行被写上了终点 %v", *rootEndedAt)
	}

	found, _, err := repository.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	tail, _ := found.Segment.ParticipationFor(parcel1)
	if kind, _, at, ended := tail.End(); !ended || kind != domain.EndedByEffectiveDelivery || !at.Equal(endedTail.EndedAt) {
		t.Fatalf("链尾的离场三件没有落库：%+v", tail)
	}
	if found.Segment.ActiveParticipations() != 0 {
		t.Fatalf("在场参与 = %d, want 0", found.Segment.ActiveParticipations())
	}
	if _, _, _, rootEnded := found.Segment.ParticipationHistory(parcel1)[0].End(); rootEnded {
		t.Fatal("根凭空长出了终点")
	}
}

// TestFindSegmentsForObjectAnswersEverySegmentWhereTheObjectHasACurrentParticipation 证按对象的反查口
// （ADR-0112 决定二「更正的对象可能早已离场」）：已离场的段与在场的段都答；一条链只把段答一次；他租户与
// 无关对象不可见。
func TestFindSegmentsForObjectAnswersEverySegmentWhereTheObjectHasACurrentParticipation(t *testing.T) {
	repository, transactor, _ := newSegmentRegistry(t)
	ctx := t.Context()
	tenant := segmentRef(t, domain.NewTenantID, "tenant-1")
	parcel1 := segmentRef(t, domain.NewCarriedObjectReference, "parcel-1")

	// SEG-O001：parcel-1 已按交付离场。SEG-O002：parcel-1 在场且已长成两版的链。SEG-O003：别的对象。
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-O001", deliveredMember(t, "parcel-1")))
	root := activeMember(t, "parcel-1")
	root.EnteredAt = segmentEnteredAtFixture.Add(24 * time.Hour)
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-O002", root))
	mustWithinSegmentTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Supersede(txCtx, segmentKeyFixture(t, "tenant-1", "SEG-O002"),
			rebuiltChainTail(t, "SEG-O002", root, supersedingMember(t, root, "v2", root.EnteredAt.Add(time.Hour))), segmentEnteredAtFixture)
		return err
	})
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-O003", activeMember(t, "parcel-9")))

	keys, err := repository.FindSegmentsForObject(ctx, tenant, parcel1)
	if err != nil {
		t.Fatalf("按对象反查：%v", err)
	}
	if len(keys) != 2 || keys[0].Segment.String() != "SEG-O001" || keys[1].Segment.String() != "SEG-O002" {
		t.Fatalf("keys = %+v, want [SEG-O001 SEG-O002]（已离场也答、一条链答一次、按入场先后）", keys)
	}

	stranger, err := repository.FindSegmentsForObject(ctx, segmentRef(t, domain.NewTenantID, "tenant-2"), parcel1)
	if err != nil || len(stranger) != 0 {
		t.Fatalf("他租户看见了本租户的参与：%+v err=%v", stranger, err)
	}
	unknown, err := repository.FindSegmentsForObject(ctx, tenant, segmentRef(t, domain.NewCarriedObjectReference, "parcel-none"))
	if err != nil || len(unknown) != 0 {
		t.Fatalf("从未入段的对象答了段：%+v err=%v", unknown, err)
	}
}

// TestAChainRoundTripsThroughFirstRegistration 证首登路径也带回指列：`Save` 与窄写口共用同一段 INSERT，
// 加列时不许分叉——一个带链的段首登之后读回仍是链。
func TestAChainRoundTripsThroughFirstRegistration(t *testing.T) {
	repository, transactor, _ := newSegmentRegistry(t)
	ctx := t.Context()

	root := activeMember(t, "parcel-1")
	replacement := supersedingMember(t, root, "v2", segmentEnteredAtFixture.Add(time.Hour))
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-S003", root, replacement))

	found, _, err := repository.FindByKey(ctx, segmentKeyFixture(t, "tenant-1", "SEG-S003"))
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	parcel1 := segmentRef(t, domain.NewCarriedObjectReference, "parcel-1")
	history := found.Segment.ParticipationHistory(parcel1)
	if len(history) != 2 || !history[0].Superseded() || history[1].Superseded() {
		t.Fatalf("链没有往返：%+v", history)
	}
	if supersedes, chained := history[1].Supersedes(); !chained || supersedes != root.EntryBasis {
		t.Fatalf("回指没有往返：%v %v", supersedes, chained)
	}
}

// TestSupersessionChainConstraintsRejectRowsTheDomainCannotProduce 证迁移 0016 是第二道门：绕过领域直接
// INSERT 也落不进领域造不出的链形——同对象第二个根、回指悬空、自指、空串回指、跨对象回指、同一前版被替代
// 两次、同一入场依据两次。
func TestSupersessionChainConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	repository, transactor, pool := newSegmentRegistry(t)
	ctx := t.Context()

	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-K001", activeMember(t, "parcel-1"), activeMember(t, "parcel-2")))

	insert := func(object, basis string, supersedes *string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO transport_fulfillment.fulfillment_participation
			     (tenant_id, segment_ref, object_ref, planned_ref, entry_kind, entry_basis, entered_at,
			      end_kind, end_basis, ended_at, supersedes_entry_basis, recorded_at)
			 VALUES ('tenant-1', 'SEG-K001', $1, NULL, 'OFFSITE_PICKUP', $2, now(), NULL, NULL, NULL, $3, now())`,
			object, basis, supersedes)
		return err
	}
	text := func(value string) *string { return &value }

	// 一条正当的替代版本先落进去：后面「同一前版被替代两次」要靶它。
	if err := insert("parcel-1", "OFFSITE-PICKUP/parcel-1/v2", text("OFFSITE-PICKUP/parcel-1")); err != nil {
		t.Fatalf("正当的替代版本落不进去：%v", err)
	}

	for name, attempt := range map[string]func() error{
		"同对象第二个根": func() error { return insert("parcel-1", "OFFSITE-PICKUP/parcel-1/again", nil) },
		"回指不存在的前版": func() error {
			return insert("parcel-1", "OFFSITE-PICKUP/parcel-1/v9", text("OFFSITE-PICKUP/parcel-1/v8"))
		},
		"自指": func() error {
			return insert("parcel-1", "OFFSITE-PICKUP/parcel-1/self", text("OFFSITE-PICKUP/parcel-1/self"))
		},
		"空串回指":  func() error { return insert("parcel-1", "OFFSITE-PICKUP/parcel-1/blank", text("")) },
		"跨对象回指": func() error { return insert("parcel-2", "OFFSITE-PICKUP/parcel-2/v2", text("OFFSITE-PICKUP/parcel-1")) },
		"同一前版被替代两次": func() error {
			return insert("parcel-1", "OFFSITE-PICKUP/parcel-1/v2b", text("OFFSITE-PICKUP/parcel-1"))
		},
		"同一入场依据两次": func() error {
			return insert("parcel-1", "OFFSITE-PICKUP/parcel-1/v2", text("OFFSITE-PICKUP/parcel-1/v2b"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := attempt(); err == nil {
				t.Fatalf("%s 落进去了", name)
			}
		})
	}

	// 门只挡坏形：v2 之后接 v3 是正当的链。
	if err := insert("parcel-1", "OFFSITE-PICKUP/parcel-1/v3", text("OFFSITE-PICKUP/parcel-1/v2")); err != nil {
		t.Fatalf("正当的第三版落不进去：%v", err)
	}
	found, _, err := repository.FindByKey(ctx, segmentKeyFixture(t, "tenant-1", "SEG-K001"))
	if err != nil {
		t.Fatalf("三版链读不回：%v", err)
	}
	if len(found.Segment.ParticipationHistory(segmentRef(t, domain.NewCarriedObjectReference, "parcel-1"))) != 3 {
		t.Fatal("三版链读回走样")
	}
}

// TestSupersedeRefusesToRunOutsideATransaction 证第四个窄口与前三个一样要环境事务。
func TestSupersedeRefusesToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newSegmentRegistry(t)
	root := activeMember(t, "parcel-1")
	tail := rebuiltChainTail(t, "SEG-T002", root, supersedingMember(t, root, "v2", segmentEnteredAtFixture.Add(time.Hour)))

	if _, err := repository.Supersede(t.Context(), segmentKeyFixture(t, "tenant-1", "SEG-T002"), tail, segmentEnteredAtFixture); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("Supersede 无事务应返回 ErrTransactionRequired，实得：%v", err)
	}
}
