package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	visibilitydomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Covers: AT-VE-229、AT-VE-230 生产装配半边、CONTEXT「客户索赔项」（document-numbering/06，证据 S）——`/v1/claims` 的受理事务即开立事务：同一（批次+项）
// 并发受理只落一项、只占一个号，输掉建行竞争的那一方答赢家与赢家的号，本方取过的号随受理事务回滚退回（DiscardIssuing 由
// transactionalClaims 兑现），下一项紧接着数；没登单据编号时区的租户照常受理、不带号。
func TestTheWiredClaimsIssueOneNumberPerRecordedClaimAndRollBackTheLosersNumber(t *testing.T) {
	db := documentNumberingDB(t)
	numbers := documentNumberingComponents(t, db, docnumber.SecretTable{})
	if registered, err := numbers.Registrations.RegisterTimeZone(t.Context(), docnumber.RegisterTimeZoneCommand{
		Tenant: "SYN-TENANT-1", TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
	}); err != nil || registered.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登单据编号时区：%+v %v", registered, err)
	}
	claims, err := buildClaimsOrchestration(db, numbers)
	if err != nil {
		t.Fatalf("装配索赔编排：%v", err)
	}
	if _, err := buildClaimsOrchestration(db, docnumbering.Components{}); err == nil {
		t.Fatal("没有编号组件也装起来了")
	}

	command := func(tenant, item string) visibilityapp.ReceiveClaimCommand {
		return visibilityapp.ReceiveClaimCommand{
			TenantID:    mustValueNoT(visibilitydomain.NewTenantID, tenant),
			Batch:       mustValueNoT(visibilitydomain.NewClaimBatchReference, "SYN-CLAIM-BATCH-1"),
			Item:        mustValueNoT(visibilitydomain.NewClaimItemID, item),
			Customer:    mustValueNoT(visibilitydomain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
			Applicant:   mustValueNoT(visibilitydomain.NewApplicantReference, "SYN-APPLICANT-1"),
			Contract:    mustValueNoT(visibilitydomain.NewContractScopeReference, "SYN-CONTRACT-1"),
			Target:      mustValueNoT(visibilitydomain.NewRequestScopeReference, "SYN-PARCEL-"+item),
			Kind:        mustValueNoT(visibilitydomain.NewClaimKindReference, "SYN-KIND-LOSS"),
			SubmittedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
		}
	}
	numberOf := func(result visibilityapp.HandleClaimResult) string {
		claim, present := result.Claim()
		if !present {
			return ""
		}
		number, _ := claim.DocumentNumber().Number()
		return number
	}

	const racers = 6
	results := make([]visibilityapp.HandleClaimResult, racers)
	errs := make([]error, racers)
	var wait sync.WaitGroup
	for index := range racers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index], errs[index] = claims.ReceiveClaim(context.Background(), command("SYN-TENANT-1", "SYN-CLAIM-ITEM-1"))
		}()
	}
	wait.Wait()
	received := 0
	first := ""
	for index, result := range results {
		if errs[index] != nil {
			t.Fatalf("第 %d 份受理：%v", index, errs[index])
		}
		if result.Outcome() == visibilityapp.ClaimReceived {
			received++
		}
		if first == "" {
			first = numberOf(result)
		}
		if numberOf(result) == "" || numberOf(result) != first {
			t.Fatalf("第 %d 份受理答 %q 号 %q，want 全都是赢家那一个", index, result.Outcome(), numberOf(result))
		}
	}
	if received != 1 {
		t.Fatalf("同一项落了 %d 次", received)
	}
	if !strings.HasPrefix(first, "CLM-") || !strings.HasSuffix(first, "-00001") {
		t.Fatalf("首号 = %q，want 产品默认模板出的第一个", first)
	}
	next, err := claims.ReceiveClaim(t.Context(), command("SYN-TENANT-1", "SYN-CLAIM-ITEM-2"))
	if want := strings.TrimSuffix(first, "00001") + "00002"; err != nil || next.Outcome() != visibilityapp.ClaimReceived || numberOf(next) != want {
		t.Fatalf("竞争之后下一项 = %q（%v），want %q——输家取过的号没退回", numberOf(next), err, want)
	}

	unconfigured, err := claims.ReceiveClaim(t.Context(), command("SYN-TENANT-2", "SYN-CLAIM-ITEM-1"))
	if err != nil || unconfigured.Outcome() != visibilityapp.ClaimReceived || numberOf(unconfigured) != "" {
		t.Fatalf("没登时区的租户：%v %q %q", err, unconfigured.Outcome(), numberOf(unconfigured))
	}
	claim, _ := unconfigured.Claim()
	if claim.DocumentNumber().NumberlessReason() != visibilitydomain.NumberingTimeZoneNotConfigured {
		t.Fatalf("无号原因 = %q", claim.DocumentNumber().NumberlessReason())
	}
}
