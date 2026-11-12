package shipmenthttp

import (
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// financialControlBody 透出接受前财务控制采用结果的三层（ADR-0125）：接受侧结论、按哪个共同通过
// 条件推出的、逐项控制项结果。逐项是空数组而不是 null（判据同 reachability）；`明确无控制`没有条件
// 也没有项，两格照实缺席 / 为空。OccupationFormed 单独透出，因为它不能从 outcome 读出来——结论受限时
// 第一项仍可能占着钱，看队列的人要知道拒绝后有一笔要释放。
//
// 复核详情的 recordedJudgments 与委托详情（ADR-0379 决定三）共用这一份形状与 financialControlBodyOf：管理台两页照同一组键写，
// 各编一份迟早一边多出或少了一格。这里不带任何金额——金额、余额与占用归结算（PS CONTEXT），页面另向结算的读口取。
type financialControlBody struct {
	Outcome            string                     `json:"outcome"`
	ResultID           string                     `json:"resultId,omitempty"`
	Basis              string                     `json:"basis,omitempty"`
	AsOfAt             string                     `json:"asOfAt,omitempty"`
	AsOfSemantics      string                     `json:"asOfSemantics,omitempty"`
	AsOfPolicy         string                     `json:"asOfPolicyVersion,omitempty"`
	JointPassCondition string                     `json:"jointPassCondition,omitempty"`
	OccupationFormed   bool                       `json:"occupationFormed"`
	Items              []financialControlItemBody `json:"items"`
}

// financialControlItemBody 的后两格是受限项上的采用引用（ADR-0132 决定三、四）：正文登记的失败处置与责任引用，
// 只在受限项采用过时在场；成立项与采用之前记下的受限项两格缺席，照实透出不补造。
type financialControlItemBody struct {
	Kind               string `json:"kind"`
	Order              uint32 `json:"order"`
	Conclusion         string `json:"conclusion"`
	Basis              string `json:"basis,omitempty"`
	FailureDisposition string `json:"failureDisposition,omitempty"`
	Responsibility     string `json:"responsibility,omitempty"`
}

// financialControlBodyOf 对零值答 nil：零值是尚未形成（ports.RecordedJudgments 的约定），整格缺席，不造一个「空结果」冒充判断过。
func financialControlBodyOf(result domain.FinancialControlResult) *financialControlBody {
	if result.Outcome().String() == "" {
		return nil
	}
	items := result.Items()
	control := &financialControlBody{
		Outcome:            result.Outcome().String(),
		ResultID:           result.ResultID().String(),
		Basis:              result.Basis().String(),
		JointPassCondition: result.JointPassCondition().String(),
		OccupationFormed:   result.OccupationFormed(),
		Items:              make([]financialControlItemBody, 0, len(items)),
	}
	for _, item := range items {
		row := financialControlItemBody{
			Kind:       item.Kind().String(),
			Order:      item.Order(),
			Conclusion: item.Conclusion().String(),
			Basis:      item.Basis().String(),
		}
		if adopted, present := item.AdoptedDisposition(); present {
			row.FailureDisposition = adopted.FailureDisposition().String()
			row.Responsibility = adopted.Responsibility().String()
		}
		control.Items = append(control.Items, row)
	}
	if asOf := result.AsOf(); !asOf.At().IsZero() {
		control.AsOfAt = asOf.At().UTC().Format(time.RFC3339Nano)
		control.AsOfSemantics = asOf.Semantics().String()
		control.AsOfPolicy = asOf.PolicyVersion().String()
	}
	return control
}
