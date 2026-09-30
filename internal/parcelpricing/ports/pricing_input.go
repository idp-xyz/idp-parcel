package ports

import (
	"context"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// EligibleSourceReferences 是 SA 评价请求带来的三件合格来源引用（UC-SA-002 步 2）：TF 运输收费发生项（身份 +
// 有效性版本）、费用项目、供应商协议版本。按字面转述，本上下文不解析它们的内容——它们是造计价输入快照时
// 要去别的上下文读事实的**钥匙**，不是事实本身。
type EligibleSourceReferences struct {
	Occurrence        string
	OccurrenceVersion string
	FeeItem           string
	SupplierAgreement string
}

// OccurrenceReferenced 报出发生项引用是否成形：身份与版本都在、不带首尾空白。入口的受理门只查这一件——发生项是
// 造快照时唯一能拿去键事实的引用（成员对象、业务时点都在它上面），另两件是 SA 那一侧的钥匙，本上下文不读内容。
func (sources EligibleSourceReferences) OccurrenceReferenced() bool {
	return trimmedNonEmpty(sources.Occurrence) && trimmedNonEmpty(sources.OccurrenceVersion)
}

func trimmedNonEmpty(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}

// PricingInputQuery 是「为这份评价请求造一份计价输入快照」所需的全部钥匙：租户、范围、目的、计价基准时点
// （发生项的业务时间）与三件来源引用。
type PricingInputQuery struct {
	Tenant  domain.TenantID
	Scope   domain.PricingScopeID
	Purpose domain.PricingPurpose
	BasisAt time.Time
	Sources EligibleSourceReferences
}

// PricingInputOutcome 是造快照的封闭结果。
type PricingInputOutcome uint8

const (
	PricingInputOutcomeInvalid PricingInputOutcome = iota
	// PricingInputResolved：四样（主体、分区或邮编路线、计费重量、业务时点）都指到了，快照随答案交回。
	PricingInputResolved
	// PricingInputUnavailable：至少一样指不到。缺哪几只读口随答案点名——不猜、不填、不拿默认重量顶
	// （票 sa-cc/11 裁决 4）。
	PricingInputUnavailable
)

func (outcome PricingInputOutcome) String() string {
	switch outcome {
	case PricingInputResolved:
		return "RESOLVED"
	case PricingInputUnavailable:
		return "UNAVAILABLE"
	default:
		return ""
	}
}

// PricingInputResolution 是造快照的答案。Input 只在 PricingInputResolved 时有值；Missing 只在
// PricingInputUnavailable 时有值，一条一只缺的读口或缺的事实，文字给人读。
type PricingInputResolution struct {
	Outcome PricingInputOutcome
	Input   domain.PricingInputSnapshot
	Missing []string
}

// PricingInputResolver 按合格来源引用造计价输入快照（UC-SA-002 步 2 后半「`parcel-pricing` 采用测量、运输收费
// 发生项、其他履约、面单及已解析商业依据形成不可变计价输入快照」）。
//
// 这是本上下文的消费侧端口。实现在 adapters/transportfulfillment（ADR-0171）：成员走运输履约，申报测量与
// 地址要素走小包托运。节点实测登记册仍缺（pp-pricing-input-seams/04）；没接实测读口时用申报，并在事实引用里
// 标明来源。调用方没装实现时，入口仍答「输入不可得」并点名那三只读口，不用一个永远答「不在」的替身顶上。
type PricingInputResolver interface {
	ResolvePricingInput(ctx context.Context, query PricingInputQuery) (PricingInputResolution, error)
}
