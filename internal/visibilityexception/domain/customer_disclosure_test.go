package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var disclosedAt = time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC)

func disclosureDecision(t *testing.T, conclusion domain.DisclosureConclusion, content string) domain.DisclosureDecision {
	t.Helper()
	var contentRef domain.DisclosureContentReference
	if content != "" {
		contentRef = mustValue(t, domain.NewDisclosureContentReference, content)
	}
	decision, err := domain.DecideDisclosure(
		mustValue(t, domain.NewEpisodeID, "episode-1"),
		mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		mustValue(t, domain.NewDisclosurePolicyReference, "disclosure-policy/v2"),
		conclusion,
		contentRef,
		disclosedAt,
	)
	if err != nil {
		t.Fatalf("decide disclosure: %v", err)
	}
	return decision
}

// Covers: VE CONTEXT 硬句 159/165「客户可见性必须根据……信息披露规则形成版本化决定」
// 「披露条件不成立或授权不足时分别形成暂不披露或待授权结果」——结论三值封闭（两个非
// 披露格分开，等条件与等授权是不同续办路）；披露必带内容快照、非披露必不带（两向拦）。
func TestDisclosureKeepsItsThreeConclusionsHonest(t *testing.T) {
	disclosed := disclosureDecision(t, domain.DiscloseToCustomer, "content/customer-1/v1")
	content, has := disclosed.Content()
	if !has || content.String() != "content/customer-1/v1" {
		t.Fatalf("content = %v has = %v", content, has)
	}

	pending := disclosureDecision(t, domain.NotYetDisclosable, "")
	if _, has := pending.Content(); has {
		t.Fatal("暂不披露交出了内容")
	}

	if _, err := domain.DecideDisclosure(
		mustValue(t, domain.NewEpisodeID, "episode-1"),
		mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		mustValue(t, domain.NewDisclosurePolicyReference, "disclosure-policy/v2"),
		domain.DiscloseToCustomer,
		domain.DisclosureContentReference{},
		disclosedAt,
	); !errors.Is(err, domain.ErrInvalidDisclosure) {
		t.Fatalf("err = %v; 没有内容快照的披露被收下了", err)
	}
	if _, err := domain.DecideDisclosure(
		mustValue(t, domain.NewEpisodeID, "episode-1"),
		mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		mustValue(t, domain.NewDisclosurePolicyReference, "disclosure-policy/v2"),
		domain.AwaitingAuthorization,
		mustValue(t, domain.NewDisclosureContentReference, "content/leak"),
		disclosedAt,
	); !errors.Is(err, domain.ErrInvalidDisclosure) {
		t.Fatalf("err = %v; 待授权带上了内容", err)
	}
}

// Covers: VE CONTEXT 硬句 162/163「客户异常通知必须保存通知对象、内容快照、披露依据、
// 目标客户、要求时限和适用渠道」「已生成、已提交消息渠道、渠道已接受、已送达、失败和
// 客户确认必须分别记录。合同要求送达或确认时，只有相应结果成立才满足通知义务」——
// 非披露结论生成不了通知；节点分别追加不覆盖（失败后送达两节点并存）；义务按要求节点
// 查（要求确认时仅送达不满足）。
func TestNotificationMilestonesAccrueAndObligationIsExplicit(t *testing.T) {
	notification, err := domain.GenerateNotification(
		mustValue(t, domain.NewNotificationID, "notification-1"),
		disclosureDecision(t, domain.DiscloseToCustomer, "content/customer-1/v1"),
		disclosedAt.Add(24*time.Hour),
		mustValue(t, domain.NewNotificationChannelReference, "EMAIL"),
		mustValue(t, domain.NewDisclosurePolicyReference, "notify-policy/v1"),
		disclosedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("generate notification: %v", err)
	}

	if _, err := domain.GenerateNotification(
		mustValue(t, domain.NewNotificationID, "notification-9"),
		disclosureDecision(t, domain.NotYetDisclosable, ""),
		disclosedAt.Add(24*time.Hour),
		mustValue(t, domain.NewNotificationChannelReference, "EMAIL"),
		mustValue(t, domain.NewDisclosurePolicyReference, "notify-policy/v1"),
		disclosedAt.Add(time.Minute),
	); !errors.Is(err, domain.ErrInvalidNotification) {
		t.Fatalf("err = %v; 暂不披露生成了通知", err)
	}

	steps := []domain.NotificationMilestone{
		domain.NotificationSubmittedToChannel,
		domain.NotificationFailed,
		domain.NotificationSubmittedToChannel,
		domain.NotificationChannelAccepted,
		domain.NotificationDelivered,
	}
	for index, step := range steps {
		if err := notification.RecordMilestone(step, disclosedAt.Add(time.Duration(index+2)*time.Minute)); err != nil {
			t.Fatalf("record %s: %v", step, err)
		}
	}
	milestones := notification.Milestones()
	if len(milestones) != 6 || milestones[2] != domain.NotificationFailed {
		t.Fatalf("milestones = %v; 失败与重试后的节点必须并存", milestones)
	}

	if !notification.ObligationMetBy(domain.NotificationDelivered) {
		t.Fatal("已送达却报义务未满足")
	}
	if notification.ObligationMetBy(domain.CustomerConfirmed) {
		t.Fatal("合同要求确认时仅送达不满足义务")
	}

	if err := notification.RecordMilestone(domain.NotificationGenerated, disclosedAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidNotification) {
		t.Fatalf("err = %v; 生成被当成了可重复动作", err)
	}
}
