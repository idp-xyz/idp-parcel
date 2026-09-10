package domain

// 本文件是服务产品册接进 PCC-1 的那一格（票 admin-write-faces/09「本册规范化判断」；票 admin-write-faces/25 给它
// 长出可缺的一节）。
//
// 本册的正文**可缺**：发布的是商业版本壳——它给合同、接单规则包、价格政策一个可引用的版本身份；产品的属性与渠道
// 映射走另一条登记路（register_products），不在发布载荷里。壳单独发布时 ADR-0126 Decision 一「文档只盖正文不盖壳」
// 读作「正文为空，文档只剩规范化版本与 kind 两格」：这是那一句的边界情形不是例外，因此不换号，也不把壳上的指名
// 引用读进文档——引用与范围、区间一样是壳的一格，已由 sameReleasedContent / SameSubmissionAs 逐项比对，盖进摘要
// 只会让「同引用换范围」与「同范围换引用」两种修订长成两张不同的脸。
//
// 于是不带正文的每一版摘要是同一个串。它不是退化：串带版本、可重算、可比；同键重发靠壳分重放与修订，落到登记册
// 后由同一套判据分重放与冲突，没有一处为本册另开。本册今天唯一的正文是产品层交付条件声明（ADR-0133 决定四，票
// admin-write-faces/25）：在场时文档多一节 serviceProduct.deliveryConditions，omitempty 不换号；缺席与在场是两个合法
// 状态，缺席不答「正文缺席」——所以 registerBodyOptional 对本册答真。

// ServiceProductBody 是服务产品册的正文输入面：今天只有一节可缺的产品层交付条件。它以领域值对象给出、不带拥有它
// 的（已生效）版本，理由同各册的 *Body（预览与录入发生在发布之前）。整个 Body 为零值与 nil 折出同一份两格文档。
type ServiceProductBody struct {
	DeliveryConditions *DeliveryConditionBody
}

// validate 在折成文档前把在场的一节过与发布时相同的门；缺席无事可判。
func (body ServiceProductBody) validate() error {
	if body.DeliveryConditions == nil {
		return nil
	}
	return body.DeliveryConditions.validate(ServiceProductObject)
}

// canonicalServiceProductBody 是本册在文档里的一节；键名镜像批文 declarations 下归本册的那一键。整节只在有内容时
// 在场——不带交付条件的版本文档仍是两格，字节与本节存在之前逐字节同。
type canonicalServiceProductBody struct {
	DeliveryConditions *canonicalDeliveryConditions `json:"deliveryConditions,omitempty"`
}

// canonicalServiceProductBodyOf 在正文有内容时交回一节，否则 nil——让 omitempty 把整键省掉。
func canonicalServiceProductBodyOf(body ServiceProductBody) *canonicalServiceProductBody {
	if body.DeliveryConditions == nil {
		return nil
	}
	return &canonicalServiceProductBody{DeliveryConditions: canonicalDeliveryConditionsOf(*body.DeliveryConditions)}
}

// body 把文档里的一节折回领域正文；每一格过构造门后再过与发布时相同的门。
func (document canonicalServiceProductBody) body() (ServiceProductBody, error) {
	var body ServiceProductBody
	if document.DeliveryConditions != nil {
		conditions, err := document.DeliveryConditions.body()
		if err != nil {
			return ServiceProductBody{}, err
		}
		body.DeliveryConditions = &conditions
	}
	if err := body.validate(); err != nil {
		return ServiceProductBody{}, err
	}
	return body, nil
}

// canonicalizeServiceProduct 把服务产品册折成文档并算摘要：正文缺席或为零值时两格，在场时多一节。调用方已核过壳
// 声明的类别就是本册且没有别册的正文冒名（CanonicalizePublicationContent 的 kind 不符那一格），这里不再判。
func canonicalizeServiceProduct(content PublicationContent) (CanonicalPublicationContent, error) {
	document := canonicalPublicationDocument{
		Canonicalization: publicationCanonicalizationVersion,
		Kind:             ServiceProductObject.String(),
	}
	if content.ServiceProduct != nil {
		if err := content.ServiceProduct.validate(); err != nil {
			return CanonicalPublicationContent{}, err
		}
		document.ServiceProduct = canonicalServiceProductBodyOf(*content.ServiceProduct)
	}
	return canonicalDigestOf(document)
}

// registerBodyOptional 答某一册的正文是不是可缺：这样的册折回快照时没有「正文缺席」可判，两格文档就是一份合法的
// 正文面。今天只有服务产品——其余各册壳可以单独入册，但表单路径的载体必须带正文。
func registerBodyOptional(kind CommercialObjectKind) bool {
	return kind == ServiceProductObject
}
