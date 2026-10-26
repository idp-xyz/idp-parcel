package domain

// 首方客户命令四口的规范化形状版本（ADR-0139 决定八「未定形状的口不开首方渠道」；ADR-0310；票 first-party-api/06）。
// 写法照 payloadCanonicalizationVersion（PSC-1，ADR-0014）：版本进摘要输入，形状变更走新版本、不改旧形状的产出。
//
// 本票只定名与版本，不定字段：形状的字段、编码与摘要函数归各口换上首方 Intake 的那一票填，填之前这四口的摘要没有产出者，
// 对应的首方口也就不开。名彼此不同、也不同于 PSC-1，是为了让已存的摘要串凭前缀就认得出它是哪一口、哪一版的形状。
const (
	// CustomerWithdrawalShapeVersion 是委托撤回的形状，字段由票 first-party-api/07 填。
	CustomerWithdrawalShapeVersion = "PWD-1"
	// CustomerParcelCancellationShapeVersion 是取消包裹的形状，字段由票 first-party-api/11 填。
	CustomerParcelCancellationShapeVersion = "PCN-1"
	// CustomerSourceDataAmendmentShapeVersion 是客户资料修订的形状，字段由票 first-party-api/10 填；要素内容复用 PSC-1 的要素构造，
	// 不重算（pp-seams/05）。
	CustomerSourceDataAmendmentShapeVersion = "PAM-1"
	// CustomerSupplementShapeVersion 是委托补充的形状，字段由票 first-party-api/08 填；它另立名而不沿用 PSC-1，是因为补充载荷
	// 多带基准版本一格，且重放与冲突要在补充自己的判重空间里判。
	CustomerSupplementShapeVersion = "PSU-1"
)

// CanonicalizeCustomerSupplement 是首方补充口的规范化入口（形状 PSU-1，票 first-party-api/08）。内容段与 PSC-1 同构——补充重报
// 整份内容、不从旧版本继承（FormNewSubmissionVersionCommand 的同名纪律），规格里的 BasisVersion 必须带上客户带回的基准：基准
// 不同即内容不同，同一补充请求标识换基准重发是来源冲突、不是重放。版本名 PSU-1 进摘要前缀，补充的判重空间因此不与提交的
// PSC-1 摘要相混。
func CanonicalizeCustomerSupplement(spec SubmissionPayloadSpec) (CanonicalSubmission, error) {
	if !spec.BasisVersion.valid() {
		return CanonicalSubmission{}, ErrInvalidSubmissionPayload
	}
	digest, err := canonicalizeSubmissionPayloadUnder(CustomerSupplementShapeVersion, spec)
	if err != nil {
		return CanonicalSubmission{}, err
	}
	return canonicalSubmissionOf(digest, spec), nil
}
