package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// podCorrection 指名前版的一份 POD 更正（票 operator-channel/41 裁定 (a)）。
func podCorrection(t *testing.T, predecessor, proof string, correctedAt time.Time) application.CorrectDeliveryProofCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.CorrectDeliveryProofCommand{
		TenantID:           tenant,
		Attempt:            "attempt-1",
		Object:             "parcel-1",
		PredecessorVersion: predecessor,
		NewProof:           proof,
		CorrectedAt:        correctedAt,
	}
}

func registeredDeliveryFixture(t *testing.T) *deliveryFixture {
	t.Helper()
	fixture := newDeliveryFixture(t)
	result, err := fixture.handler.Register(context.Background(), registerCommand(t))
	wantOutcome(t, result.Outcome(), err, application.DeliveryRegistered)
	return fixture
}

func currentDelivery(t *testing.T, fixture *deliveryFixture) ports.EffectiveDeliveryRecord {
	t.Helper()
	if len(fixture.store.records) != 1 {
		t.Fatalf("登记库里有 %d 个（对象+尝试），want 1", len(fixture.store.records))
	}
	for _, record := range fixture.store.records {
		return record
	}
	return ports.EffectiveDeliveryRecord{}
}

// Covers: 票 operator-channel/41 裁定 (a)——POD 更正指名前版，链线性、一版至多被更正一次。原样重发答已有版本、
// 交回原版本并重发同一版本的意图，不签号、不落新版（ADR-0023「同一身份重复到达返回原结果，不追加第二条事实」）；
// 同一前版换内容答冲突，当前版不动。
func TestAPODCorrectionReplayIsAnExistingVersionAndADifferentOneIsAConflict(t *testing.T) {
	fixture := registeredDeliveryFixture(t)
	at := deliveryRecordedAt.Add(24 * time.Hour)
	command := podCorrection(t, "delivery-result/v1", "pod-2", at)

	first, err := fixture.handler.Correct(context.Background(), command)
	wantOutcome(t, first.Outcome(), err, application.DeliveryCorrected)

	replay, err := fixture.handler.Correct(context.Background(), command)
	wantOutcome(t, replay.Outcome(), err, application.DeliveryExistingVersion)
	record, has := replay.Record()
	if !has || record.Delivery.Version().String() != "delivery-result/v2" {
		t.Fatalf("重放交回 %q has=%v，want 原版本 delivery-result/v2", record.Delivery.Version(), has)
	}
	if fixture.versions.minted != 2 {
		t.Fatalf("签发 %d 次，want 2（重放不耗版本号）", fixture.versions.minted)
	}
	intents := fixture.handoff.intents
	if len(intents) != 3 || intents[2].Record.Delivery.Version().String() != "delivery-result/v2" {
		t.Fatalf("意图 %d 份，want 3 且最后一份仍是 v2（重放重发同一版本，事件标识不变）", len(intents))
	}

	t.Run("the same predecessor with another POD is a conflict", func(t *testing.T) {
		result, err := fixture.handler.Correct(context.Background(), podCorrection(t, "delivery-result/v1", "pod-3", at))
		wantOutcome(t, result.Outcome(), err, application.DeliveryRegistrationConflict)
		if got := currentDelivery(t, fixture).Delivery.Proof().String(); got != "pod-2" {
			t.Fatalf("当前 POD = %q，want pod-2（冲突不顶替）", got)
		}
	})

	t.Run("the same predecessor at another correction time is a conflict", func(t *testing.T) {
		result, err := fixture.handler.Correct(context.Background(), podCorrection(t, "delivery-result/v1", "pod-2", at.Add(time.Minute)))
		wantOutcome(t, result.Outcome(), err, application.DeliveryRegistrationConflict)
	})

	t.Run("a sub-microsecond tail is still the same correction", func(t *testing.T) {
		result, err := fixture.handler.Correct(context.Background(), podCorrection(t, "delivery-result/v1", "pod-2", at.Add(300*time.Nanosecond)))
		wantOutcome(t, result.Outcome(), err, application.DeliveryExistingVersion)
	})

	if fixture.versions.minted != 2 {
		t.Fatalf("签发 %d 次，want 2（冲突也不耗版本号）", fixture.versions.minted)
	}
}

// Covers: 票 operator-channel/41 附录 F-6「迟到的旧更正把当前 POD 改回旧值」——链往前走了一步之后，指名旧前版的那份
// 更正迟到，与当前版比：内容对不上答冲突，当前版不被改回（不按接收顺序定序，ADR-0023）。
func TestALateStaleCorrectionDoesNotTurnTheCurrentPODBack(t *testing.T) {
	fixture := registeredDeliveryFixture(t)
	at := deliveryRecordedAt.Add(24 * time.Hour)

	result, err := fixture.handler.Correct(context.Background(), podCorrection(t, "delivery-result/v1", "pod-2", at))
	wantOutcome(t, result.Outcome(), err, application.DeliveryCorrected)
	result, err = fixture.handler.Correct(context.Background(), podCorrection(t, "delivery-result/v2", "pod-3", at.Add(time.Hour)))
	wantOutcome(t, result.Outcome(), err, application.DeliveryCorrected)

	late, err := fixture.handler.Correct(context.Background(), podCorrection(t, "delivery-result/v1", "pod-1", at.Add(-time.Hour)))
	wantOutcome(t, late.Outcome(), err, application.DeliveryRegistrationConflict)
	current := currentDelivery(t, fixture).Delivery
	if current.Version().String() != "delivery-result/v3" || current.Proof().String() != "pod-3" {
		t.Fatalf("当前版 = %q/%q，want delivery-result/v3/pod-3", current.Version(), current.Proof())
	}
	if fixture.versions.minted != 3 {
		t.Fatalf("签发 %d 次，want 3", fixture.versions.minted)
	}
}

// Covers: 票 operator-channel/41 做什么「POD 更正在签版本号之前先过完备性检查」——POD 更正先过完备性再签号，
// 照 pickupCorrectionFrom 的「一份形成不了的更正不该烧掉一个版本号」。
func TestARefusedPODCorrectionMintsNoVersion(t *testing.T) {
	refused := map[string]func(*application.CorrectDeliveryProofCommand){
		"no correction time": func(command *application.CorrectDeliveryProofCommand) { command.CorrectedAt = time.Time{} },
		"corrected before delivery": func(command *application.CorrectDeliveryProofCommand) {
			command.CorrectedAt = deliveryArrivedAt.Add(-time.Hour)
		},
		"no predecessor named":          func(command *application.CorrectDeliveryProofCommand) { command.PredecessorVersion = " " },
		"no new proof":                  func(command *application.CorrectDeliveryProofCommand) { command.NewProof = " " },
		"nothing registered to correct": func(command *application.CorrectDeliveryProofCommand) { command.Object = "parcel-9" },
	}
	for name, breakCommand := range refused {
		t.Run(name, func(t *testing.T) {
			fixture := registeredDeliveryFixture(t)
			command := podCorrection(t, "delivery-result/v1", "pod-2", deliveryRecordedAt.Add(24*time.Hour))
			breakCommand(&command)
			result, err := fixture.handler.Correct(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.DeliveryNotAccepted)
			if fixture.versions.minted != 1 {
				t.Fatalf("签发 %d 次，want 1（被拒的更正不签号）", fixture.versions.minted)
			}
		})
	}
}

// Covers: 票 operator-channel/41 附录 F-6「读旧当前版时版本链分叉」——两份更正都读到 v1 为当前版，另一方先提交；
// 落后的这一份翻不动前版（顶替带「前版仍是当前版」的条件），读回赢家后按同一套比对作答，不形成第二个后继。
func TestACorrectionThatLostTheRaceIsComparedWithTheWinner(t *testing.T) {
	at := deliveryRecordedAt.Add(24 * time.Hour)
	cases := []struct {
		name   string
		winner string
		want   application.DeliveryRegistrationOutcome
	}{
		{"the winner carries the same content", "pod-2", application.DeliveryExistingVersion},
		{"the winner carries another content", "pod-other", application.DeliveryRegistrationConflict},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := registeredDeliveryFixture(t)
			fixture.store.beforeSupersede = func() {
				winner := currentDelivery(t, fixture)
				corrected, err := winner.Delivery.CorrectProof(
					mustDelivery(t, domain.NewDeliveryProofReference, testCase.winner),
					mustDelivery(t, domain.NewDeliveryResultVersion, "delivery-result/winner"),
					at,
				)
				if err != nil {
					t.Fatalf("赢家更正：%v", err)
				}
				winner.Delivery = corrected
				fixture.store.records[deliveryKey(winner.Key)] = winner
			}

			result, err := fixture.handler.Correct(context.Background(), podCorrection(t, "delivery-result/v1", "pod-2", at))
			wantOutcome(t, result.Outcome(), err, testCase.want)
			if got := currentDelivery(t, fixture).Delivery.Version().String(); got != "delivery-result/winner" {
				t.Fatalf("当前版 = %q，want 赢家那一版", got)
			}
		})
	}
}

func mustDelivery[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}
