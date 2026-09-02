// 本包只有测试文件，这是刻意的。票 `08` 的红线要求「替身不进生产装配——它是取证件，不是
// 先用着的实现」：没有非测试源文件，生产代码就**导入不了**它，这条红线因此由编译器守着，
// 而不是靠一句注释提醒下一个人。将来某家真渠道适配器落进本包时，替身仍然进不了它的 import。
//
// 证据层级是隔离合成 `S`。它不使任何切片的达标结论前进一格，也不构成任何真实渠道已接入的
// 证明——真渠道账号与授权属 `PAR-INT-02`，租户尚未到位。
package channel

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

var _ ports.LabelChannelGateway = syntheticLabelChannel{}

// syntheticLabelChannel 按预置答复作答。它不发起任何调用、不认识 HTTP，存在的唯一理由是
// 证明票 `07` 定的端口形状**装得下**盘点点名的三种情形。
type syntheticLabelChannel struct {
	submission ports.LabelSubmissionOutcome
	fetch      ports.LabelDocumentFetchOutcome
	query      ports.LabelSubmissionQueryOutcome
}

func (channel syntheticLabelChannel) SubmitLabelRequest(
	context.Context, ports.LabelChannelRequest,
) (ports.LabelSubmissionOutcome, error) {
	return channel.submission, nil
}

func (channel syntheticLabelChannel) FetchLabelDocuments(
	context.Context, ports.LabelChannelRequest,
) (ports.LabelDocumentFetchOutcome, error) {
	return channel.fetch, nil
}

func (channel syntheticLabelChannel) QuerySubmission(
	context.Context, ports.LabelChannelRequest,
) (ports.LabelSubmissionQueryOutcome, error) {
	return channel.query, nil
}

// 情形一：结果不确定。
//
// 要证的不是「端口有这一格」——那看类型就知道；要证的是这一格**走到领域侧仍然不是失败**。
// 聚合把`结果不确定`与`失败`分成两格正是为了守住 CONTEXT 那句「该结果不得直接按失败处理」，
// 而压平这一格最省事的写法（把它并进失败）在类型上完全合法。
func TestAnUncertainAnswerStaysItsOwnGradeInsteadOfCollapsingToFailure(t *testing.T) {
	t.Parallel()

	channel := syntheticLabelChannel{
		submission: ports.LabelSubmissionOutcome{
			Outcome:              outbound.Undetermined(),
			DocumentAvailability: ports.LabelDocumentsAwaitSeparateFetch,
		},
	}

	answered, err := channel.SubmitLabelRequest(t.Context(), ports.LabelChannelRequest{})
	if err != nil {
		t.Fatalf("替身作答：%v", err)
	}
	if got := answered.Outcome.Disposition(); got != outbound.AnswerUndetermined {
		t.Fatalf("处置格 = %q，想要 ANSWER_UNDETERMINED", got)
	}
	if answered.Outcome.AdmitsResend() {
		t.Error("答案未确定不得重发——重发一次产生的是真实的供应商成本与第二个运输标识")
	}
	if !answered.Outcome.RequiresQueryToSettle() {
		t.Error("答案未确定的续办只能是查询或对账")
	}

	submitted := submittedTransaction(t, "parcel-1")
	uncertain, err := submitted.MarkResultUncertain()
	if err != nil {
		t.Fatalf("记结果不确定：%v", err)
	}
	if got := uncertain.State(); got != domain.LabelTransactionResultUncertain {
		t.Errorf("交易状态 = %q，想要结果不确定", got)
	}
	if uncertain.Finalized() {
		t.Error("结果不确定不是定案：定案会让重试与替代对着一笔还没有答案的交易发起")
	}
}

// 情形二：部分成功。
//
// 要证的是两层结果**互不推导**：交易级停在`部分成功`，而逐包裹结果各自成立——受理那件带渠道
// 签发的标识，未受理那件带原因且不带标识。拿交易级去推包裹级是 CONTEXT 明禁的一步，而它在
// 类型上同样合法。
func TestPartialSuccessKeepsEachParcelResultStandingOnItsOwn(t *testing.T) {
	t.Parallel()

	accepted := mustValue(t, domain.NewDeclaredParcelID, "parcel-1")
	refused := mustValue(t, domain.NewDeclaredParcelID, "parcel-2")

	channel := syntheticLabelChannel{
		submission: ports.LabelSubmissionOutcome{
			Outcome: outbound.Accept(),
			ParcelOutcomes: []ports.LabelChannelParcelOutcome{
				{Parcel: accepted, HasResult: true, Accepted: true, Identifier: "channel-parcel-1"},
				{Parcel: refused, HasResult: true, Accepted: false, ReasonReference: "reason-oversize"},
			},
			DocumentAvailability: ports.LabelDocumentsReturnedWithSubmission,
		},
	}

	answered, err := channel.SubmitLabelRequest(t.Context(), ports.LabelChannelRequest{})
	if err != nil {
		t.Fatalf("替身作答：%v", err)
	}

	submitted := submittedTransaction(t, "parcel-1", "parcel-2")
	recorded, err := submitted.RecordChannelResult(domain.RecordChannelResultSpec{
		Outcome:       domain.LabelTransactionPartiallySucceeded,
		ParcelResults: translateParcelOutcomes(t, answered.ParcelOutcomes),
		ObservedAt:    labelSubmittedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("记渠道结果：%v", err)
	}
	if got := recorded.State(); got != domain.LabelTransactionPartiallySucceeded {
		t.Fatalf("交易状态 = %q，想要部分成功", got)
	}

	first, found := recorded.ParcelResult(accepted)
	if !found {
		t.Fatal("受理那件的结果读不回来")
	}
	if !first.Accepted() || first.Identifier().String() != "channel-parcel-1" {
		t.Errorf("受理那件应带渠道标识，实得 accepted=%v identifier=%q",
			first.Accepted(), first.Identifier().String())
	}

	second, found := recorded.ParcelResult(refused)
	if !found {
		t.Fatal("未受理那件的结果读不回来")
	}
	if second.Accepted() {
		t.Error("未受理那件不该因为交易级是部分成功就变成受理——那正是拿交易级推包裹级")
	}
	if second.Reason().String() != "reason-oversize" || second.Identifier().String() != "" {
		t.Errorf("未受理那件应带原因且不带标识，实得 reason=%q identifier=%q",
			second.Reason().String(), second.Identifier().String())
	}
}

// 情形三：批粒度面单。
//
// 要证的是端口**不逼**适配器把一份批面单拆成 N 份假的包裹级图件。拆开是压平这一格最自然的
// 写法，而拆完之后没有任何东西说得出「这 N 份其实是同一张纸」。
//
// 只证到端口为止：件往领域与库里落的形状归票 `09`，那一票落地前，批面单在领域侧没有落点。
func TestABatchLabelStaysOneDocumentCoveringManyParcels(t *testing.T) {
	t.Parallel()

	first := mustValue(t, domain.NewDeclaredParcelID, "parcel-1")
	second := mustValue(t, domain.NewDeclaredParcelID, "parcel-2")

	channel := syntheticLabelChannel{
		fetch: ports.LabelDocumentFetchOutcome{
			Outcome:      outbound.Accept(),
			Availability: ports.LabelDocumentsReturnedWithSubmission,
			Documents: []ports.LabelDocument{{
				Role:           "shipping-label",
				Format:         "channel-declared-format",
				Granularity:    domain.LabelDocumentPerBatch,
				CoveredParcels: []domain.DeclaredParcelID{first, second},
				Digest:         "synthetic-digest-1",
				Content:        []byte("synthetic batch label"),
			}},
		},
	}

	answered, err := channel.FetchLabelDocuments(t.Context(), ports.LabelChannelRequest{})
	if err != nil {
		t.Fatalf("替身作答：%v", err)
	}
	if len(answered.Documents) != 1 {
		t.Fatalf("批面单应是一份件，实得 %d 份——拆成逐件就说不出它们是同一张纸", len(answered.Documents))
	}

	document := answered.Documents[0]
	if got := document.Granularity; got != domain.LabelDocumentPerBatch {
		t.Errorf("粒度 = %q，想要 PER_BATCH", got)
	}
	if len(document.CoveredParcels) != 2 ||
		document.CoveredParcels[0] != first ||
		document.CoveredParcels[1] != second {
		t.Errorf("批面单应逐件列出它覆盖的包裹，实得 %v", document.CoveredParcels)
	}
}

// 顺带钉住「受理了但按约定不回图件」与「等另一次取件」是两格。两者的 Documents 都为空，
// 一个布尔字段区分不了它们，而它们要人做的事相反：前者不必再取，后者必须再取一次。
func TestAcceptedWithoutDocumentsIsNotTheSameAsAwaitingASeparateFetch(t *testing.T) {
	t.Parallel()

	notProduced := ports.LabelSubmissionOutcome{
		Outcome:              outbound.Accept(),
		DocumentAvailability: ports.LabelDocumentsNotProducedByChannel,
	}
	awaiting := ports.LabelSubmissionOutcome{
		Outcome:              outbound.Accept(),
		DocumentAvailability: ports.LabelDocumentsAwaitSeparateFetch,
	}

	if len(notProduced.Documents) != 0 || len(awaiting.Documents) != 0 {
		t.Fatal("两格的件都应为空，否则这条断言测的不是它们的区别")
	}
	if notProduced.DocumentAvailability == awaiting.DocumentAvailability {
		t.Error("两格必须分得开：一个不必再取，一个必须再取一次")
	}
}

var labelEstablishedAt = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

var labelSubmittedAt = labelEstablishedAt.Add(time.Minute)

func submittedTransaction(t *testing.T, parcels ...string) domain.LabelTransaction {
	t.Helper()

	covered := make([]domain.DeclaredParcelID, 0, len(parcels))
	for _, parcel := range parcels {
		covered = append(covered, mustValue(t, domain.NewDeclaredParcelID, parcel))
	}

	established, err := domain.EstablishLabelTransaction(domain.EstablishLabelTransactionSpec{
		Tenant:                 mustValue(t, domain.NewTenantID, "tenant-1"),
		ID:                     mustValue(t, domain.NewLabelTransactionID, "label-txn-1"),
		CoveredParcels:         covered,
		ChannelAccount:         mustValue(t, domain.NewChannelAccountReference, "channel-account-1"),
		AccountHolder:          mustValue(t, domain.NewChannelAccountHolderReference, "party-holder-1"),
		ServiceProvider:        mustValue(t, domain.NewChannelServiceProviderReference, "party-channel-1"),
		SettlementCounterparty: mustValue(t, domain.NewSettlementCounterpartyReference, "party-settlement-1"),
		Contract:               mustValue(t, domain.NewChannelContractReference, "contract-1"),
		Rate:                   mustValue(t, domain.NewChannelRateReference, "rate-1"),
		ResponsibilityBasis:    mustValue(t, domain.NewResponsibilityBasisSnapshotReference, "basis-snapshot-1"),
		EstablishedAt:          labelEstablishedAt,
	})
	if err != nil {
		t.Fatalf("建立面单交易：%v", err)
	}
	submitted, err := established.SubmitToChannel(labelSubmittedAt)
	if err != nil {
		t.Fatalf("提交渠道：%v", err)
	}
	return submitted
}

// translateParcelOutcomes 是**测试本地**的译法，只为把端口那一侧的答复喂进聚合以取证。
// 生产侧的这一步归后续票的编排或适配器，本包不给它一个可被生产导入的实现。
func translateParcelOutcomes(
	t *testing.T,
	outcomes []ports.LabelChannelParcelOutcome,
) []domain.LabelTransactionParcelResultSpec {
	t.Helper()

	specs := make([]domain.LabelTransactionParcelResultSpec, 0, len(outcomes))
	for _, outcome := range outcomes {
		if !outcome.HasResult {
			continue
		}
		spec := domain.LabelTransactionParcelResultSpec{
			Parcel:   outcome.Parcel,
			Accepted: outcome.Accepted,
		}
		if outcome.Accepted {
			spec.Identifier = mustValue(t, domain.NewChannelParcelIdentifier, outcome.Identifier)
		} else {
			spec.Reason = mustValue(t, domain.NewChannelResultReasonReference, outcome.ReasonReference)
		}
		specs = append(specs, spec)
	}
	return specs
}

func mustValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()

	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}
