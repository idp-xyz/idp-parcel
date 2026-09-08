package commercialhttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是运营操作者面的商业发布载荷（ADR-0126 Decision 四；票 admin-write-faces/08）：管理台逐字段表单组出来
// 的那份 JSON，预览口与录入口收的是**同一份形状、走同一段解码**，解出同一组领域值对象，交给同一个
// domain.PreviewPublication / SubmitPublicationDraft——摘要只在领域一处算，两口自然逐字节同答。形状由产品定义、
// 属机制半边（ADR-0101 决定一）；它不是客户渠道载荷，那一半照旧等 `PAR-INT-01`。
//
// **载荷里只有内容，没有身份，也没有摘要。** 租户与录入者 / 批准者从 ADR-0100 的 `OperatorEnvelope` 来，由
// Intake 作为入参交进来；载荷里出现 tenant / submitter / contentDigest / approval 之类的键一律按未知键拒——
// 严格解码不是挑剔，是不让自报身份与自报摘要有地方落（伞票硬句：表单不算摘要、表单不收也不送批准人）。
//
// **逐格问题。** 解码把每一格的构造门结果都收齐再答，不撞第一格就停：表单要的是「哪几格不对」。收齐的
// 问题以 PublicationPayloadProblems 交回，它 Is ErrMalformedRequest——同一份内容重发不会变好。

// ErrOperatorIdentityMissing 表示调用方没交来租户或操作者主体——那不是载荷的错（载荷本来就不该带它们），是
// Intake 没拿到操作者信封就来翻译。与 ErrMalformedRequest 分开：前者改载荷没用。
var ErrOperatorIdentityMissing = errors.New("party commercial http: operator identity (tenant, subject) is required")

// PayloadProblem 是一格立不住的记录：哪一格（JSON 路径）、为什么（领域构造门的原话）。
type PayloadProblem struct {
	Field   string
	Problem string
}

// PublicationPayloadProblems 收齐一份载荷里全部立不住的格。它是 error，且 Is ErrMalformedRequest。
type PublicationPayloadProblems struct {
	Problems []PayloadProblem
}

func (problems *PublicationPayloadProblems) Error() string {
	parts := make([]string, 0, len(problems.Problems))
	for _, problem := range problems.Problems {
		parts = append(parts, problem.Field+": "+problem.Problem)
	}
	return "party commercial http: malformed publication payload: " + strings.Join(parts, "; ")
}

// Is 让 errors.Is(err, ErrMalformedRequest) 对本类型为真：传输层的分流只认那一个哨兵。
func (problems *PublicationPayloadProblems) Is(target error) bool {
	return target == ErrMalformedRequest
}

func (problems *PublicationPayloadProblems) add(field string, err error) {
	problems.Problems = append(problems.Problems, PayloadProblem{Field: field, Problem: err.Error()})
}

func (problems *PublicationPayloadProblems) any() bool {
	return len(problems.Problems) > 0
}

// CommercialPublicationPayload 是载荷的线格式：版本壳（少了身份里的租户与摘要）加各册正文一格。字段与
// domain.PublicationDraftShell / PublicationContent 一一对应，不引入领域里没有的概念。
type CommercialPublicationPayload struct {
	Kind              string `json:"kind"`
	ObjectID          string `json:"objectId"`
	Version           string `json:"version"`
	Scope             string `json:"scope"`
	EffectiveStartsAt string `json:"effectiveStartsAt"`
	EffectiveEndsAt   string `json:"effectiveEndsAt,omitempty"`
	// References 是壳上的指名引用：被引对象类别（String() 原词）→ 对象标识。
	References map[string]string `json:"references,omitempty"`
	// CreditPolicy 是信用政策册的正文（首例）；其余各册由子票在此各加一格。
	CreditPolicy *CreditPolicyBodyPayload `json:"creditPolicy,omitempty"`
	// SupplierAgreement 是供应商协议册的正文（票 admin-write-faces/11；形状见 SupplierAgreementBodyPayload）。
	SupplierAgreement *SupplierAgreementBodyPayload `json:"supplierAgreement,omitempty"`
}

// CreditPolicyBodyPayload 镜像受控批文 creditPolicyBodyDocument 与规范化文档的键名：额度两键恰一在场（由领域
// 构造门判），区间上界可缺。
type CreditPolicyBodyPayload struct {
	LegalEntity           string `json:"legalEntity"`
	AuthorityLevel        string `json:"authorityLevel"`
	ChargeType            string `json:"chargeType"`
	LimitMinor            *int64 `json:"limitMinor,omitempty"`
	LimitRatioBasisPoints *int64 `json:"limitRatioBasisPoints,omitempty"`
	EffectiveStartsAt     string `json:"effectiveStartsAt"`
	EffectiveEndsAt       string `json:"effectiveEndsAt,omitempty"`
}

// PublicationDraftReferencePayload 是批准口与发布口的载荷：只指名哪一版载体。身份从信封来，载荷不带。
type PublicationDraftReferencePayload struct {
	Kind     string `json:"kind"`
	ObjectID string `json:"objectId"`
	Version  string `json:"version"`
}

// DecodeCommercialPublicationPayload 只做结构解码：JSON 合法、键都认识。字段值对不对留给 Publication 那一步的
// 领域构造门——这里不另造一套校验（ADR-0101 Context：导入器不需要、也不该另造校验规则）。
func DecodeCommercialPublicationPayload(body io.Reader) (CommercialPublicationPayload, error) {
	var payload CommercialPublicationPayload
	if err := decodeStrict(body, &payload); err != nil {
		return CommercialPublicationPayload{}, fmt.Errorf("%w: commercial publication payload: %v", ErrMalformedRequest, err)
	}
	return payload, nil
}

// DecodePublicationDraftReferencePayload 解码批准口与发布口的载荷，判据同上。
func DecodePublicationDraftReferencePayload(body io.Reader) (PublicationDraftReferencePayload, error) {
	var payload PublicationDraftReferencePayload
	if err := decodeStrict(body, &payload); err != nil {
		return PublicationDraftReferencePayload{}, fmt.Errorf("%w: publication draft reference payload: %v", ErrMalformedRequest, err)
	}
	return payload, nil
}

func decodeStrict(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	// 一份载荷只许一个文档：尾随的第二个 JSON 值不是本载荷的一部分，放过它就等于静默丢掉一段输入。
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing content after the payload")
	}
	return nil
}

// Publication 把载荷连同信封给的租户翻成领域的壳与正文。每一格都过领域构造器；立不住的格收齐后以
// PublicationPayloadProblems 一次交回。
func (payload CommercialPublicationPayload) Publication(tenant domain.TenantID) (domain.PublicationDraftShell, domain.PublicationContent, error) {
	if tenant.String() == "" {
		return domain.PublicationDraftShell{}, domain.PublicationContent{}, ErrOperatorIdentityMissing
	}
	problems := &PublicationPayloadProblems{}
	shell := domain.PublicationDraftShell{TenantID: tenant}

	kind, known := domain.CommercialObjectKindNamed(payload.Kind)
	if !known {
		problems.add("kind", fmt.Errorf("集合外的商业对象类别 %q", payload.Kind))
	}
	shell.Kind = kind
	shell.ObjectID = requireField(problems, "objectId", domain.NewCommercialObjectID, payload.ObjectID)
	shell.Version = requireField(problems, "version", domain.NewCommercialVersionLabel, payload.Version)
	shell.Scope = requireField(problems, "scope", domain.NewCommercialScopeReference, payload.Scope)
	shell.Effective = intervalField(problems, "effectiveStartsAt", "effectiveEndsAt", payload.EffectiveStartsAt, payload.EffectiveEndsAt)
	if len(payload.References) > 0 {
		shell.References = make(map[domain.CommercialObjectKind]domain.CommercialObjectID, len(payload.References))
		for name, objectID := range payload.References {
			referencedKind, known := domain.CommercialObjectKindNamed(name)
			if !known {
				problems.add("references."+name, fmt.Errorf("集合外的被引对象类别 %q", name))
				continue
			}
			shell.References[referencedKind] = requireField(problems, "references."+name, domain.NewCommercialObjectID, objectID)
		}
	}

	content := domain.PublicationContent{Kind: kind}
	if payload.CreditPolicy != nil {
		body := payload.CreditPolicy.body(problems)
		content.CreditPolicy = &body
	}
	if payload.SupplierAgreement != nil {
		body := payload.SupplierAgreement.body(problems)
		content.SupplierAgreement = &body
	}

	if problems.any() {
		return domain.PublicationDraftShell{}, domain.PublicationContent{}, problems
	}
	return shell, content, nil
}

// PreviewCommand 与 SubmitCommand 从同一个 Publication 出发——两口拿到的壳与正文是同一个方法的返回值，摘要在
// 领域同一处算，逐字节相同不靠约定靠结构。
func (payload CommercialPublicationPayload) PreviewCommand(tenant domain.TenantID) (application.PreviewCommercialPublicationCommand, error) {
	shell, content, err := payload.Publication(tenant)
	if err != nil {
		return application.PreviewCommercialPublicationCommand{}, err
	}
	return application.PreviewCommercialPublicationCommand{Shell: shell, Content: content}, nil
}

func (payload CommercialPublicationPayload) SubmitCommand(
	tenant domain.TenantID,
	submitter domain.OperatorSubjectReference,
) (application.SubmitPublicationDraftCommand, error) {
	if submitter.String() == "" {
		return application.SubmitPublicationDraftCommand{}, ErrOperatorIdentityMissing
	}
	shell, content, err := payload.Publication(tenant)
	if err != nil {
		return application.SubmitPublicationDraftCommand{}, err
	}
	return application.SubmitPublicationDraftCommand{Shell: shell, Content: content, Submitter: submitter}, nil
}

// ApproveCommand 把载体引用连同信封给的批准者主体翻成批准命令。
func (payload PublicationDraftReferencePayload) ApproveCommand(
	tenant domain.TenantID,
	approver domain.OperatorSubject,
) (application.ApprovePublicationDraftCommand, error) {
	if tenant.String() == "" || approver.Reference().String() == "" {
		return application.ApprovePublicationDraftCommand{}, ErrOperatorIdentityMissing
	}
	kind, objectID, version, err := payload.identity()
	if err != nil {
		return application.ApprovePublicationDraftCommand{}, err
	}
	return application.ApprovePublicationDraftCommand{
		Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version, Approver: approver,
	}, nil
}

// PublishCommand 把载体引用连同信封给的租户翻成发布命令。发布不记谁按的键（批准者已在载体上），所以只要租户。
func (payload PublicationDraftReferencePayload) PublishCommand(tenant domain.TenantID) (application.PublishPublicationDraftCommand, error) {
	if tenant.String() == "" {
		return application.PublishPublicationDraftCommand{}, ErrOperatorIdentityMissing
	}
	kind, objectID, version, err := payload.identity()
	if err != nil {
		return application.PublishPublicationDraftCommand{}, err
	}
	return application.PublishPublicationDraftCommand{Tenant: tenant, Kind: kind, ObjectID: objectID, Version: version}, nil
}

func (payload PublicationDraftReferencePayload) identity() (domain.CommercialObjectKind, domain.CommercialObjectID, domain.CommercialVersionLabel, error) {
	problems := &PublicationPayloadProblems{}
	kind, known := domain.CommercialObjectKindNamed(payload.Kind)
	if !known {
		problems.add("kind", fmt.Errorf("集合外的商业对象类别 %q", payload.Kind))
	}
	objectID := requireField(problems, "objectId", domain.NewCommercialObjectID, payload.ObjectID)
	version := requireField(problems, "version", domain.NewCommercialVersionLabel, payload.Version)
	if problems.any() {
		return domain.CommercialObjectKindInvalid, domain.CommercialObjectID{}, domain.CommercialVersionLabel{}, problems
	}
	return kind, objectID, version, nil
}

// body 把信用政策正文逐格过领域构造门。额度两键恰一在场：两空与两满都是一格问题（落在 limit 上），不由这里挑一个。
func (payload CreditPolicyBodyPayload) body(problems *PublicationPayloadProblems) domain.CreditPolicyBody {
	body := domain.CreditPolicyBody{
		LegalEntity: requireField(problems, "creditPolicy.legalEntity", domain.NewLegalEntityReference, payload.LegalEntity),
		Level:       requireField(problems, "creditPolicy.authorityLevel", domain.NewAuthorityLevel, payload.AuthorityLevel),
		ChargeType:  requireField(problems, "creditPolicy.chargeType", domain.NewChargeTypeReference, payload.ChargeType),
		Effective: intervalField(problems, "creditPolicy.effectiveStartsAt", "creditPolicy.effectiveEndsAt",
			payload.EffectiveStartsAt, payload.EffectiveEndsAt),
	}
	switch {
	case payload.LimitMinor != nil && payload.LimitRatioBasisPoints == nil:
		limit, err := domain.NewCreditAmountLimit(*payload.LimitMinor)
		if err != nil {
			problems.add("creditPolicy.limitMinor", err)
		}
		body.Limit = limit
	case payload.LimitMinor == nil && payload.LimitRatioBasisPoints != nil:
		limit, err := domain.NewCreditRatioLimit(*payload.LimitRatioBasisPoints)
		if err != nil {
			problems.add("creditPolicy.limitRatioBasisPoints", err)
		}
		body.Limit = limit
	default:
		problems.add("creditPolicy.limit", fmt.Errorf("额度须恰一格在场：limitMinor 或 limitRatioBasisPoints"))
	}
	return body
}

// requireField 过一格领域构造器，立不住就记进 problems 并交回零值——零值让后面的格照样能判，问题才收得齐。
func requireField[T any](problems *PublicationPayloadProblems, field string, construct func(string) (T, error), raw string) T {
	value, err := construct(raw)
	if err != nil {
		problems.add(field, err)
	}
	return value
}

// intervalField 把两格时刻折成领域区间：时刻按 RFC 3339，上界可缺。格式错与区间立不住各记在自己那一格。
func intervalField(problems *PublicationPayloadProblems, startsField, endsField, startsRaw, endsRaw string) domain.EffectiveInterval {
	startsAt, err := time.Parse(time.RFC3339Nano, startsRaw)
	if err != nil {
		problems.add(startsField, fmt.Errorf("须为 RFC 3339 时刻：%v", err))
		return domain.EffectiveInterval{}
	}
	var endsAt time.Time
	if endsRaw != "" {
		if endsAt, err = time.Parse(time.RFC3339Nano, endsRaw); err != nil {
			problems.add(endsField, fmt.Errorf("须为 RFC 3339 时刻：%v", err))
			return domain.EffectiveInterval{}
		}
	}
	interval, err := domain.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		problems.add(endsField, err)
	}
	return interval
}
