package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: PS CONTEXT「委托」草稿 → 已提交时在同一提交事务内取得单据号，同一提交重放答原号（ADR-0355 决定三、十一）——
// 建成的答复带号，重放读回同一个号、不再取号。
//
// Covers: `AT-PS-177`。
func TestASubmittedRequestCarriesItsDocumentNumberAndAReplayAnswersTheSameOne(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	number, numbered := first.DocumentNumbering().Number()
	if first.Outcome() != application.OutcomeSubmitted || !numbered || number.String() != "P20260807100001" {
		t.Fatalf("first = %s, number %q (%v)", first.Outcome(), number, numbered)
	}
	stored, _ := fixture.requests.stored(t, fixture.identity(t))
	if storedNumber, _ := stored.DocumentNumbering().Number(); storedNumber != number {
		t.Fatalf("stored number = %q, want %q", storedNumber, number)
	}

	fixture.calls = nil
	replay, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	replayed, present := replay.DocumentNumbering().Number()
	if replay.Outcome() != application.OutcomeExistingResult || !present || replayed != number {
		t.Fatalf("replay = %s, number %q (%v)", replay.Outcome(), replayed, present)
	}
	if slices.Contains(fixture.calls, "issue-document-number") || fixture.numbers.issued != 1 {
		t.Fatalf("the replay issued another number: calls %v, issued %d", fixture.calls, fixture.numbers.issued)
	}
}

// Covers: ADR-0355 决定七——开立方没有业务时区、租户也没登单据编号时区时委托照常建成、不带号，记下无号原因，答复如实带这一格。
//
// Covers: `AT-PS-178`。
func TestASubmissionWithoutANumberingTimeZoneIsSubmittedWithoutANumber(t *testing.T) {
	fixture := newFixture(t)
	fixture.numbers.outcome = ports.ShipmentDocumentNumberingTimeZoneNotConfigured

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("outcome = %s, want SUBMITTED", result.Outcome())
	}
	if _, numbered := result.DocumentNumbering().Number(); numbered {
		t.Fatal("a request without a numbering time zone got a number")
	}
	if reason, absent := result.DocumentNumbering().Absence(); !absent || reason != domain.DocumentNumberingTimeZoneNotConfigured {
		t.Fatalf("absence = %v (%v)", reason, absent)
	}
	stored, _ := fixture.requests.stored(t, fixture.identity(t))
	if reason, _ := stored.DocumentNumbering().Absence(); reason != domain.DocumentNumberingTimeZoneNotConfigured {
		t.Fatalf("stored absence = %v", reason)
	}
}

// Covers: ADR-0355 决定八——编号规则撞号是一个业务答案：这一次不建单，不记号；答复是那一格，不是故障。
//
// Covers: `AT-PS-179`。
func TestANumberingRuleCollisionAnswersWithoutRecordingANumber(t *testing.T) {
	fixture := newFixture(t)
	fixture.numbers.outcome = ports.ShipmentDocumentNumberRuleCollision

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.OutcomeNumberingRuleCollision || result.Outcome().String() != "NUMBERING_RULE_COLLISION" {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if _, named := result.ShipmentRequestID(); named {
		t.Fatal("a collision answer names a shipment request that the rolled-back transaction never kept")
	}
	if slices.Contains(fixture.calls, "record-document-number") {
		t.Fatalf("a collision recorded document numbering: %v", fixture.calls)
	}
}

// Covers: 取号失败不得被吞成一个业务答案，也不得让委托以「没有号」的样子答已提交——错误上抛，生产装配里守卫整笔回滚。
func TestAFailedIssueSurfacesInsteadOfSubmittingWithoutANumber(t *testing.T) {
	fixture := newFixture(t)
	failure := errors.New("numbering unavailable")
	fixture.numbers.failNext = failure

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the issue failure", err)
	}
	if slices.Contains(fixture.calls, "record-document-number") {
		t.Fatalf("a failed issue still recorded numbering: %v", fixture.calls)
	}
}

// Covers: ADR-0355 决定三「取号只发生在确定要落单的那一步」——建单答「已存在」时这一笔不落单、事务照常提交，不取号。
//
// Covers: `AT-PS-177`。
func TestAnInsertThatFindsTheRequestAlreadyThereIssuesNoNumber(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	fixture.sources.missPreservedOnce = true
	fixture.requests.missesOnFind = 2
	fixture.calls = nil

	result, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("concurrent handle: %v", err)
	}
	if result.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("outcome = %s, want EXISTING_RESULT", result.Outcome())
	}
	if !slices.Contains(fixture.calls, "insert-request") || slices.Contains(fixture.calls, "issue-document-number") {
		t.Fatalf("calls = %v: the already-exists branch must insert-attempt without issuing", fixture.calls)
	}
	if fixture.numbers.issued != 1 {
		t.Fatalf("issued = %d, want only the first submission's number", fixture.numbers.issued)
	}
}
