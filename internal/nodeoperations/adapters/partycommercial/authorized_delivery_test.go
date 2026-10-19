package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nopc "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/partycommercial"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证 NO 消费 PC 授权交付判断的防腐适配器（ADR-0212 决定三；CONTEXT-MAP `party-commercial → node-operations`）：
// 问法逐格译过去，PC 的每种答复各译成 NO 端口的一格，读不动原样交回、不折成任何一种答复。夹具全为合成引用（S 级）。

var openedAt = time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)

func must[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("%q：%v", raw, err)
	}
	return value
}

type judgmentsFake struct {
	answer pcdomain.AuthorizedDeliveryAnswer
	err    error
	asked  []pcdomain.AuthorizedDeliveryQuery
}

func (fake *judgmentsFake) JudgeAuthorizedDelivery(_ context.Context, query pcdomain.AuthorizedDeliveryQuery) (pcdomain.AuthorizedDeliveryAnswer, error) {
	fake.asked = append(fake.asked, query)
	return fake.answer, fake.err
}

func noQuery(t *testing.T) noports.AuthorizedDeliveryQuery {
	t.Helper()
	return noports.AuthorizedDeliveryQuery{
		Tenant:          must(t, nodomain.NewTenantID, "SYN-TENANT-01"),
		DeliveringParty: must(t, nodomain.NewDeliveringPartyReference, "SYN-PARTY-DELIVERER-01"),
		CustomerAccount: "SYN-ACCOUNT-01",
		Node:            must(t, nodomain.NewNodeReference, "SYN-NODE-SHA-HUB"),
		OpenedAt:        openedAt,
	}
}

// Covers: 完成判据「NO 防腐适配器用例：每种答复（含读不动）各译一格」。
func TestAuthorizedDeliveryViewTranslatesEveryAnswer(t *testing.T) {
	cases := map[string]struct {
		answer pcdomain.AuthorizedDeliveryAnswer
		want   noports.AuthorizedDeliveryAnswer
	}{
		"是":   {answer: pcdomain.DeliveryAuthorized, want: noports.DeliveryAuthorized},
		"否":   {answer: pcdomain.DeliveryNotAuthorized, want: noports.DeliveryNotAuthorized},
		"未配置": {answer: pcdomain.DeliveryAuthorizationNotConfigured, want: noports.DeliveryAuthorizationNotConfigured},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := &judgmentsFake{answer: tc.answer}
			view, err := nopc.NewAuthorizedDeliveryView(fake)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			got, err := view.JudgeAuthorizedDelivery(context.Background(), noQuery(t))
			if err != nil {
				t.Fatalf("判断：%v", err)
			}
			if got != tc.want {
				t.Fatalf("答复 = %s, want %s", got, tc.want)
			}
			if len(fake.asked) != 1 {
				t.Fatalf("问了 %d 次，want 1", len(fake.asked))
			}
			asked := fake.asked[0]
			if asked.Tenant.String() != "SYN-TENANT-01" ||
				asked.DeliveringParty.String() != "SYN-PARTY-DELIVERER-01" ||
				asked.CustomerAccount.String() != "SYN-ACCOUNT-01" ||
				asked.Node.String() != "SYN-NODE-SHA-HUB" ||
				!asked.At.Equal(openedAt) {
				t.Fatalf("问法没逐格译过去：%+v", asked)
			}
		})
	}
}

// Covers: 读不动原样交回错误，答复留空——「没问到」不能译成否，更不能译成是。
func TestAuthorizedDeliveryViewPassesAReadFailureThrough(t *testing.T) {
	unreachable := errors.New("party commercial unreachable")
	view, err := nopc.NewAuthorizedDeliveryView(&judgmentsFake{err: unreachable})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	got, err := view.JudgeAuthorizedDelivery(context.Background(), noQuery(t))
	if !errors.Is(err, unreachable) {
		t.Fatalf("err = %v, want 原样包住读不动", err)
	}
	if got != noports.AuthorizedDeliveryAnswerInvalid {
		t.Fatalf("读不动时答复 = %s, want 无", got)
	}
}

// Covers: PC 交回三格以外的答复、或问法在 PC 那边立不起来，都是译不动，交回错误而不是猜一格。
func TestAuthorizedDeliveryViewRefusesWhatItCannotTranslate(t *testing.T) {
	view, err := nopc.NewAuthorizedDeliveryView(&judgmentsFake{answer: pcdomain.AuthorizedDeliveryAnswerInvalid})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	got, err := view.JudgeAuthorizedDelivery(context.Background(), noQuery(t))
	if !errors.Is(err, nopc.ErrUntranslatableAnswer) || got != noports.AuthorizedDeliveryAnswerInvalid {
		t.Fatalf("集合外答复 = (%s, %v), want ErrUntranslatableAnswer", got, err)
	}

	fake := &judgmentsFake{answer: pcdomain.DeliveryAuthorized}
	view, err = nopc.NewAuthorizedDeliveryView(fake)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	blankAccount := noQuery(t)
	blankAccount.CustomerAccount = " "
	got, err = view.JudgeAuthorizedDelivery(context.Background(), blankAccount)
	if !errors.Is(err, nopc.ErrUntranslatableAnswer) || got != noports.AuthorizedDeliveryAnswerInvalid {
		t.Fatalf("空客户账户 = (%s, %v), want ErrUntranslatableAnswer", got, err)
	}
	if len(fake.asked) != 0 {
		t.Fatalf("译不动的问法仍问了 PC %d 次", len(fake.asked))
	}

	if _, err := nopc.NewAuthorizedDeliveryView(nil); err == nil {
		t.Fatal("缺 PC 读口仍构造成功，漏装要在构造期暴露")
	}
}
