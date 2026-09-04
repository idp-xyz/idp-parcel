package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRehydratedActualCarrierJudgment 是重建入口因库里那几行本身而拒绝时的理由。与
// ErrInvalidActualCarrierJudgment 分开：后者说「此刻要形成的这一版不合规则」，前者说「这份已经登记过
// 的历史不可能是本上下文形成的」——处置是去查库里那几行或写它们的适配器。
var ErrInvalidRehydratedActualCarrierJudgment = errors.New("transport fulfillment: invalid rehydrated actual carrier judgment")

// RehydrateActualCarrierJudgmentVersionSpec 是判断的一版在库面的样子。Subject 与 Pending 恰有一个在场
// ——判断值没有第三态；两者的配合由重建门核，不由适配器拼。
type RehydrateActualCarrierJudgmentVersionSpec struct {
	Sequence     int
	Subject      CarrierSubject
	Pending      PendingCarrierReason
	BusinessTime time.Time
	FormedAt     time.Time
	Bases        []CarrierEvidence
}

// RehydrateActualCarrierJudgmentSpec 是一份判断连同它全部版本在库面的样子，版本按序号升序。
type RehydrateActualCarrierJudgmentSpec struct {
	TenantID      TenantID
	Segment       FulfillmentSegmentReference
	EstablishedAt time.Time
	Versions      []RehydrateActualCarrierJudgmentVersionSpec
}

// RehydrateActualCarrierJudgment 从库面重建一份判断（ADR-0028）。字段一律当数据收下、**不重走派生
// 算法**：某一版当时为什么答已识别或冲突，依据是当时在场的依据，重放 deriveCarrierVerdict 等于拿今天的
// 算法追认昨天的判断。这里只挡库里一行自己就看得出的坏：
//
//   - 序号必须从 1 起连续——版本只追加，中间少一版说明有人删过；
//   - 判断值恰居其一；业务时间不早于段成立；
//   - 无合格证据 ⇔ 没有依据（那一格由算法定义就是「一条依据都没有」，反过来已识别与冲突必有依据）；
//   - 同一版里同一引用不出现两次；每条依据自身立得住且不早于段成立。
func RehydrateActualCarrierJudgment(spec RehydrateActualCarrierJudgmentSpec) (ActualCarrierJudgment, error) {
	if !spec.TenantID.valid() || !spec.Segment.valid() || spec.EstablishedAt.IsZero() {
		return ActualCarrierJudgment{}, rehydratedJudgmentRefusal("租户、段或段成立时刻缺失")
	}
	if len(spec.Versions) == 0 {
		return ActualCarrierJudgment{}, rehydratedJudgmentRefusal("没有任何版本——段成立即有首版")
	}
	judgment := ActualCarrierJudgment{
		tenantID:      spec.TenantID,
		segment:       spec.Segment,
		establishedAt: spec.EstablishedAt.UTC(),
		versions:      make([]ActualCarrierJudgmentVersion, 0, len(spec.Versions)),
	}
	for index, row := range spec.Versions {
		if row.Sequence != index+1 {
			return ActualCarrierJudgment{}, rehydratedJudgmentRefusal(fmt.Sprintf("第 %d 个版本的序号是 %d，序号必须从 1 起连续", index+1, row.Sequence))
		}
		version, err := rehydrateJudgmentVersion(row, judgment.establishedAt)
		if err != nil {
			return ActualCarrierJudgment{}, err
		}
		judgment.versions = append(judgment.versions, version)
	}
	return judgment, nil
}

func rehydrateJudgmentVersion(row RehydrateActualCarrierJudgmentVersionSpec, establishedAt time.Time) (ActualCarrierJudgmentVersion, error) {
	verdict := CarrierVerdict{subject: row.Subject, pending: row.Pending}
	if !verdict.valid() {
		return ActualCarrierJudgmentVersion{}, rehydratedJudgmentRefusal(fmt.Sprintf("版本 %d 的判断值不是恰居已识别或待确认之一", row.Sequence))
	}
	if row.BusinessTime.IsZero() || row.FormedAt.IsZero() {
		return ActualCarrierJudgmentVersion{}, rehydratedJudgmentRefusal(fmt.Sprintf("版本 %d 缺业务时间或形成时间", row.Sequence))
	}
	if row.BusinessTime.Before(establishedAt) {
		return ActualCarrierJudgmentVersion{}, rehydratedJudgmentRefusal(fmt.Sprintf("版本 %d 的业务时间早于段成立", row.Sequence))
	}
	if (row.Pending == NoQualifiedCarrierEvidence) != (len(row.Bases) == 0) {
		return ActualCarrierJudgmentVersion{}, rehydratedJudgmentRefusal(fmt.Sprintf("版本 %d：无合格证据当且仅当没有依据", row.Sequence))
	}
	seen := make(map[CarrierEvidenceReference]struct{}, len(row.Bases))
	for _, basis := range row.Bases {
		if !basis.valid() {
			return ActualCarrierJudgmentVersion{}, rehydratedJudgmentRefusal(fmt.Sprintf("版本 %d 有一条依据立不住", row.Sequence))
		}
		if basis.occurredAt.Before(establishedAt) {
			return ActualCarrierJudgmentVersion{}, rehydratedJudgmentRefusal(fmt.Sprintf("版本 %d 有一条依据早于段成立", row.Sequence))
		}
		if _, duplicate := seen[basis.reference]; duplicate {
			return ActualCarrierJudgmentVersion{}, rehydratedJudgmentRefusal(fmt.Sprintf("版本 %d 里同一引用出现两次", row.Sequence))
		}
		seen[basis.reference] = struct{}{}
	}
	return ActualCarrierJudgmentVersion{
		sequence:     row.Sequence,
		verdict:      verdict,
		businessTime: row.BusinessTime.UTC(),
		formedAt:     row.FormedAt.UTC(),
		bases:        append([]CarrierEvidence(nil), row.Bases...),
	}, nil
}

func rehydratedJudgmentRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedActualCarrierJudgment, reason)
}
