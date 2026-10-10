package application

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 本文件是各口在 SAC-1 之前的摘要算法，即「无版本」那一版。改动前入库的记录存的是它们算出的
// 不带前缀的摘要，存量不回写（ADR-0014），所以它们只用来跟这类已存摘要比（domain.CompareStoredDigest），
// 不再写进新记录。函数体一个字节都不能改：改了，同内容的旧记录就会被答成冲突。
//
// 重分摊、重派生与供应商预计成本三口没有这一版：前两口换版本走 Replace、从不比已存摘要，
// 后一口不存摘要，两侧都从领域对象现算。

func unversionedPortionsDigestPart(portions []PortionDirective) []string {
	parts := make([]string, 0, len(portions))
	for _, portion := range portions {
		parts = append(parts, fmt.Sprintf("%s|%d", portion.Target, portion.AmountMinor))
	}
	sort.Strings(parts)
	return parts
}

func unversionedComponentsDigestPart(components []ComponentDirective) []string {
	parts := make([]string, 0, len(components))
	for _, component := range components {
		parts = append(parts, fmt.Sprintf("%s|%d|%d", component.Source, component.Effect, component.AmountMinor))
	}
	sort.Strings(parts)
	return parts
}

func unversionedAllocateDigest(command AllocateCostCommand) string {
	parts := append([]string{
		command.Source,
		fmt.Sprintf("%d", command.SourceMinor),
		command.Currency,
		command.Version,
		command.AllocatedAt.UTC().Format(time.RFC3339Nano),
	}, unversionedPortionsDigestPart(command.Portions)...)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedDeriveDigest(command DeriveResultCommand) string {
	parts := append([]string{
		command.Currency,
		command.Version,
		command.AsOf.UTC().Format(time.RFC3339Nano),
	}, unversionedComponentsDigestPart(command.Components)...)
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedAssessDigest(command AssessAdvanceCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.Verdict),
		command.Obligation,
		command.FundsFact,
		command.Payer,
		command.Responsibility,
		command.Basis,
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.Version,
		command.JudgedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedRecoveryDigest(command FormRecoveryCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Assessment,
		command.Customer,
		command.Account,
		fmt.Sprintf("%d", command.AmountMinor),
		command.FormedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedAdjustDigest(command AdjustRecoveryCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Recovery,
		fmt.Sprintf("%d", command.Reason),
		command.NewBasis,
		fmt.Sprintf("%d", command.Direction),
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.Period,
		command.FormedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedAuditDigest 是同一应付身份的内容比对锚：主张、版本与行任一不同即是另一份内容。
func unversionedAuditDigest(command AuditBillLineCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Claim.String(),
		command.Version.String(),
		command.Line.String(),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedCreditNoteDigest 是同一贷项身份与版本的内容比对锚：原应付、金额、原因与出具时点任一不同
// 即是另一份内容。
func unversionedCreditNoteDigest(command FormSupplierCreditNoteCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Payable.String(),
		fmt.Sprintf("%d", command.AmountMinor),
		command.Reason.String(),
		command.IssuedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedPublishDigest(command PublishStatementCommand) string {
	charges := append([]string(nil), command.ChargeIDs...)
	sort.Strings(charges)
	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		command.Account,
		command.Period,
		command.Version,
		command.Currency,
		fmt.Sprintf("%d", command.DeclaredTotalMinor),
		command.CutOffAt.UTC().Format(time.RFC3339Nano),
	}, charges...), "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedInclusionDigest(command IncludeLateChargeCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Number,
		command.ChargeID,
		command.SubsequentPeriod,
		command.IncludedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedAdjustmentInclusionDigest 与 unversionedInclusionDigest 分开算：两种纳入指向的金额对象不同类
// （费用 / 调整），同一个纳入标识先纳费用再纳调整是冲突，不能因为两段字符串恰好相同而
// 被读成重放。
func unversionedAdjustmentInclusionDigest(command IncludeAdjustmentCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"adjustment",
		command.Number,
		command.AdjustmentID,
		command.SubsequentPeriod,
		command.IncludedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedDisputeDigest(command OpenDisputeCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Number,
		command.ChargeID,
		fmt.Sprintf("%d", command.DisputedMinor),
		command.Reason,
		command.OpenedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedAdoptDigest 把付款人算进内容：同引用换付款人是另一份内容（冲突），不是重放。付款人自 0018 起
// 进这一版（票 sa-cc/03），0018 之前的采用存量为零，所以没有比这一版更早的行。
func unversionedAdoptDigest(command AdoptFundsFactCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Source,
		strings.TrimSpace(command.Payer),
		fmt.Sprintf("%d", command.Kind),
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.Version,
		command.OccurredAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedCorrectDigest 只算命令自己带的四样：回指、新版本、金额、更正时刻。与 unversionedAdoptDigest
// 元素不同是有意的：同一（事实、版本）若先经 AdoptFact 作首版落下、再有人拿它当更正版本来提，两份摘要
// 必不相等，答`冲突`而不是把首版当成更正的重放。
func unversionedCorrectDigest(command CorrectFundsFactCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Corrects,
		command.Version,
		fmt.Sprintf("%d", command.AmountMinor),
		command.CorrectedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedMapDigest(command MapFundsCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Fact,
		fmt.Sprintf("%d", command.TargetKind),
		command.Target,
		command.Basis,
		command.MappedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedApplyDigest(command ApplySettlementCommand) string {
	mappings := append([]string(nil), command.MappingRefs...)
	sort.Strings(mappings)
	parts := []string{
		command.Fact,
		command.TargetCurrency,
		command.Basis,
		command.AppliedAt.UTC().Format(time.RFC3339Nano),
	}
	parts = append(parts, mappings...)
	for _, allocation := range command.Allocations {
		parts = append(parts, fmt.Sprintf("%s|%d|%s|%d|%d",
			allocation.Mapping, allocation.TargetKind, allocation.Target,
			allocation.Direction, allocation.AmountMinor))
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

// unversionedBillContentDigest 是同一主张身份的内容比对锚：供应商、账期、币种、行金额与逐行裁决
// 任一不同即是另一份内容。行先排序——提交顺序不构成不同的内容。
func unversionedBillContentDigest(command ReceiveSupplierBillCommand) string {
	lines := make([]string, 0, len(command.Claim.Lines))
	for _, line := range command.Claim.Lines {
		lines = append(lines, strings.Join([]string{
			line.Line.String(),
			line.FeeItem.String(),
			fmt.Sprintf("%d", line.ClaimedMinor),
		}, "\x1f"))
	}
	sort.Strings(lines)
	directives := make([]string, 0, len(command.Directives))
	for _, directive := range command.Directives {
		directives = append(directives, strings.Join([]string{
			directive.Line.String(),
			fmt.Sprintf("%d", directive.Classification),
			directive.ExpectedVersion.String(),
			directive.Basis.String(),
		}, "\x1f"))
	}
	sort.Strings(directives)
	digest := sha256.Sum256([]byte(strings.Join(append(append([]string{
		command.Claim.Supplier.String(),
		command.Claim.Period.String(),
		command.Claim.Currency.String(),
		command.Claim.ReceivedAt.UTC().Format(time.RFC3339Nano),
	}, lines...), directives...), "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedClaimAmountDigest(command FormClaimAmountCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.Kind),
		command.ClaimItem,
		command.Responsibility,
		command.LegalEntity,
		command.OriginalCharge,
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.Period,
		command.FormedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedReceivableDigest(command FormReceivableCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Matter,
		command.Responsibility,
		command.Counterparty,
		command.LegalEntity,
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.FormedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedAcknowledgeDigest(command AcknowledgeCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Receivable,
		command.Response,
		fmt.Sprintf("%d", command.Standing),
		fmt.Sprintf("%d", command.AcknowledgedMinor),
		command.AcknowledgedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func unversionedClaimAdjustDigest(command AdjustClaimAmountCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.TargetKind),
		command.Target,
		fmt.Sprintf("%d", command.Reason),
		command.Basis,
		fmt.Sprintf("%d", command.Direction),
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.Period,
		command.FormedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
