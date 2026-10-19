package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证以送站交接单为依据推范围时的成员判法（票 ps-external-mark-relations/12 评审阻断的代决；ADR-0212 文末复核记录）：交接单
// 成立的范围只落到它名下的作业实物，不是成员的实物借单号拿不到范围。夹具全为合成（S 级）。

// Covers: NO CONTEXT「可验证交付范围」的「落到它名下的作业实物」与 ADR-0212 决定三「客户确认的是一份冻结的成员清单」——范围已成立、
// 已关闭的交接单：成员照旧答已证、范围是声称的整组；不是成员答依据不适用·不是交接单成员、不给任何客户账户；交接单未关闭时不是成员
// 同答（收寄带它的标识时编排先落收寄成员，走到这里的非成员只有没落成员的旧行）；成员判在绑定之前：不是成员时补绑定也补不出范围。
func TestAHandoverNoteScopeReachesOnlyItsMembers(t *testing.T) {
	note := handoverNote(t, "SYN-ACCOUNT-01", "SYN-ACCOUNT-02")
	confirmed := []domain.DeliveryHandoverBinding{
		binding(t, note, "SYN-ACCOUNT-01", domain.BindingConfirmed),
		binding(t, note, "SYN-ACCOUNT-02", domain.BindingConfirmed),
	}
	closure, err := domain.FormDeliveryHandoverClosure(domain.DeliveryHandoverClosureSpec{
		TenantID: note.TenantID(), Note: note.ID(), FactID: "SYN-FACT-CLOSE-PSXM12", ClosedAt: handoverOpenedAt.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	member := handoverDerivation(t, confirmed...)
	member.HandoverNote.Closure = closure
	if scope := domain.DeriveVerifiableDeliveryScope(member); scope.Check().State() != domain.DeliveryScopeVerified || len(scope.Accounts()) != 2 {
		t.Fatalf("已关闭交接单的成员：check = %s，账户 %v，want 已证、声称的整组", scope.Check().State(), scope.Accounts())
	}

	stranger := member
	stranger.HandoverMember = false
	assertUnverified(t, "已关闭交接单的非成员", domain.DeriveVerifiableDeliveryScope(stranger),
		domain.ScopeBasisNotApplicable, domain.ScopeNotAHandoverMember)

	openStranger := handoverDerivation(t, confirmed...)
	openStranger.HandoverMember = false
	assertUnverified(t, "开放交接单的非成员", domain.DeriveVerifiableDeliveryScope(openStranger),
		domain.ScopeBasisNotApplicable, domain.ScopeNotAHandoverMember)

	unconfirmedStranger := handoverDerivation(t, confirmed[0])
	unconfirmedStranger.HandoverMember = false
	assertUnverified(t, "绑定没全部确认的非成员", domain.DeriveVerifiableDeliveryScope(unconfirmedStranger),
		domain.ScopeBasisNotApplicable, domain.ScopeNotAHandoverMember)

	if parsed, err := domain.ParseDeliveryScopeInapplicability("NOT_A_HANDOVER_MEMBER"); err != nil || parsed != domain.ScopeNotAHandoverMember {
		t.Fatalf("线上名往返：%v %v", parsed, err)
	}
	if _, err := domain.NewDeliveryScopeCheck(domain.DeliveryScopeUnverified, domain.ScopeBasisNotApplicable, domain.ScopeNotAHandoverMember); err != nil {
		t.Fatalf("还原核对态：%v", err)
	}
	if _, err := domain.NewDeliveryScopeCheck(domain.DeliveryScopeUnverified, domain.ScopeBindingsNotAllConfirmed, domain.ScopeNotAHandoverMember); err == nil {
		t.Fatal("细分挂在别的原因上也立起来了")
	}
}
