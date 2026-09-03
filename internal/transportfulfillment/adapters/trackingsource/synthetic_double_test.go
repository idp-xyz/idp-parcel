// 本包只有测试文件，这是刻意的，形照 `parcelshipment/adapters/channel`：替身不进生产装配——
// 没有非测试源文件，生产代码就**导入不了**它，这条红线由编译器守着。将来某家真源的适配器落进
// 本包时，替身仍然进不了它的 import。
//
// 证据层级是隔离合成 `S`。它不证明任何真实轨迹源已接入——账号与地址属 `PAR-INT-02`，租户尚未
// 到位。它证的只有一件：票 `15` 定的端口形状**装得下**票面点名的几种情形，且装法不会把两种
// 要人做相反事情的状态压成一格。
package trackingsource

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/outbound"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var _ ports.TrackingSource = syntheticTrackingSource{}

// syntheticTrackingSource 按预置答复作答，并记下收到的游标——游标往返是拉取形态唯一要证的
// 「幂等与顺序」那半，而替身若不记下它，往返就只能靠读代码相信。
type syntheticTrackingSource struct {
	answer       ports.TrackingPullOutcome
	cursorsSeen  *[]ports.PullCursor
	subjectsSeen *int
}

func (source syntheticTrackingSource) Pull(
	_ context.Context, request ports.TrackingPullRequest,
) (ports.TrackingPullOutcome, error) {
	if source.cursorsSeen != nil {
		*source.cursorsSeen = append(*source.cursorsSeen, request.Cursor)
	}
	if source.subjectsSeen != nil {
		*source.subjectsSeen += len(request.Subjects)
	}
	return source.answer, nil
}

var receivedAt = time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

// 情形一：源没给发生时间，素材如实交出「没有」，而不是一个长得像时间的零值。
//
// 要证的是 `SourceTime` 这道分法在端口上真的走得通：一条有 OccurredAt、一条没有，两条在同一批
// 里并存，读的人不必去猜零值 time.Time 是「没给」还是「1 年 1 月 1 日」。补一个时间是压平
// 这一格最省事的写法，而它在类型上完全合法——ADR-0023 禁的正是它。
func TestAMaterialWithoutASourceOccurredAtSaysSoInsteadOfCarryingAZeroTime(t *testing.T) {
	t.Parallel()

	source := syntheticTrackingSource{answer: ports.TrackingPullOutcome{
		Support: ports.TrackingPullOffered,
		Outcome: outbound.Accept(),
		Materials: []ports.TrackingMaterial{
			{
				Source:          "synthetic-source",
				SourceEventID:   "evt-1",
				OccurredAt:      ports.SourceTime{Given: true, At: receivedAt.Add(-time.Hour)},
				ReceivedAt:      receivedAt,
				StatusReference: "source-status-a",
			},
			{
				Source:          "synthetic-source",
				SourceEventID:   "evt-2",
				OccurredAt:      ports.SourceTime{},
				ReceivedAt:      receivedAt,
				StatusReference: "source-status-b",
			},
		},
	}}

	answered, err := source.Pull(t.Context(), ports.TrackingPullRequest{})
	if err != nil {
		t.Fatalf("替身作答：%v", err)
	}
	if len(answered.Materials) != 2 {
		t.Fatalf("素材 = %d 条，想要 2", len(answered.Materials))
	}
	given, missing := answered.Materials[0], answered.Materials[1]
	if !given.OccurredAt.Given || given.OccurredAt.At.IsZero() {
		t.Error("源给了发生时间的那条应 Given 且带时间")
	}
	if missing.OccurredAt.Given {
		t.Error("源没给发生时间的那条不得 Given——补一个时间就是 ADR-0023 禁的代铸")
	}
	if missing.ReceivedAt.IsZero() {
		t.Error("ReceivedAt 由本仓铸，任何一条素材都不得缺——那是端口这一层唯一该铸的时间")
	}
	if missing.OccurredAt.At.Equal(missing.ReceivedAt) {
		t.Error("没给的发生时间不得被 ReceivedAt 顶替，否则迟到轨迹与实时轨迹在类型上分不开")
	}
}

// 情形二：只推送、不提供拉取的源，答的是「不提供拉取」这一格，不是一个失败处置。
//
// 两者要人做的事不同：拉取失败可以再拉一次，无拉取口只能等推送或交人对账。压成失败处置最
// 自然的写法是交回 `答案未确定`，而那一格的续办恰恰是「去查询」——对一个没有查询口的源，
// 那是一条走不通的续办。
func TestAPushOnlySourceAnswersNotOfferedInsteadOfAFailureDisposition(t *testing.T) {
	t.Parallel()

	source := syntheticTrackingSource{answer: ports.TrackingPullOutcome{
		Support: ports.TrackingPullNotOfferedBySource,
	}}

	answered, err := source.Pull(t.Context(), ports.TrackingPullRequest{})
	if err != nil {
		t.Fatalf("替身作答：%v", err)
	}
	if got := answered.Support; got != ports.TrackingPullNotOfferedBySource {
		t.Fatalf("Support = %q，想要 NOT_OFFERED_BY_SOURCE", got)
	}
	if answered.Outcome.RequiresQueryToSettle() {
		t.Error("无拉取口的源不得被读成「去查询」——它没有可查的口")
	}
	if len(answered.Materials) != 0 || answered.NextCursor != "" {
		t.Error("不提供拉取的源不该交出素材或游标")
	}
}

// 情形三：游标往返，且`答案未确定`不准在同一拍里重拉。
//
// 「上次拉到哪」由源的适配器解释、调用方原样带回——替身记下每次收到的游标，钉住调用方确实
// 把上一次交回的 NextCursor 原样送了回去，而不是自己造一个。第二半钉的是 ADR-0090 的纪律在
// 拉取侧照样成立：拉取是幂等读，重拉不花钱，但代数不为它改形。
func TestTheCursorRoundTripsAndAnUndeterminedPullIsNotResentInTheSameBeat(t *testing.T) {
	t.Parallel()

	var cursors []ports.PullCursor
	first := syntheticTrackingSource{
		cursorsSeen: &cursors,
		answer: ports.TrackingPullOutcome{
			Support:    ports.TrackingPullOffered,
			Outcome:    outbound.Accept(),
			NextCursor: "synthetic-cursor-after-page-1",
		},
	}
	page1, err := first.Pull(t.Context(), ports.TrackingPullRequest{Cursor: ""})
	if err != nil {
		t.Fatalf("第一拉：%v", err)
	}

	second := syntheticTrackingSource{
		cursorsSeen: &cursors,
		answer: ports.TrackingPullOutcome{
			Support: ports.TrackingPullOffered,
			Outcome: outbound.Undetermined(),
		},
	}
	page2, err := second.Pull(t.Context(), ports.TrackingPullRequest{Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("第二拉：%v", err)
	}

	if len(cursors) != 2 || cursors[0] != "" || cursors[1] != "synthetic-cursor-after-page-1" {
		t.Errorf("游标往返 = %v，想要 [\"\" synthetic-cursor-after-page-1]", cursors)
	}
	if page2.Outcome.AdmitsResend() {
		t.Error("答案未确定不准重发，拉取侧也不开例外——多久再拉是实例参数，不是代数的一格")
	}
	if !page2.Outcome.RequiresQueryToSettle() {
		t.Error("答案未确定的续办是查询；对拉取而言就是下一拍再拉")
	}
}

// 情形四：一次拉取问多个对象，且未配置时调用不得发起。
//
// 聚合平台按批答，直连一次答一个——端口收 []TrackingSubject 两者都装得下。第二半钉的是
// ADR-0090 决定五：Configuration 零值即未配置，`outbound.AdmitCall` 答不准，调用方拿到的是
// 「未配置」而不是一次真的发出去的请求。
func TestABatchOfSubjectsFitsAndAnUnconfiguredSourceIsNeverCalled(t *testing.T) {
	t.Parallel()

	var seen int
	source := syntheticTrackingSource{
		subjectsSeen: &seen,
		answer:       ports.TrackingPullOutcome{Support: ports.TrackingPullOffered, Outcome: outbound.Accept()},
	}
	request := ports.TrackingPullRequest{
		Subjects: []ports.TrackingSubject{
			{CredentialReference: "credential-1", CarrierReference: "carrier-a"},
			{CredentialReference: "credential-2"},
		},
	}

	outcome, admitted := outbound.AdmitCall(request.Configuration)
	if admitted {
		t.Fatal("零值配置不得放行调用")
	}
	if got := outcome.Disposition(); got != outbound.NotConfigured {
		t.Fatalf("未配置的处置格 = %q，想要 NOT_CONFIGURED", got)
	}
	if seen != 0 {
		t.Fatal("未配置时替身不该被调到")
	}

	if _, err := source.Pull(t.Context(), request); err != nil {
		t.Fatalf("替身作答：%v", err)
	}
	if seen != 2 {
		t.Errorf("一次拉取应把两个对象一起交给源，实得 %d", seen)
	}
}
