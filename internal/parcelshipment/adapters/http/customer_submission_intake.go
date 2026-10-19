package shipmenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// 首方客户族在本包的两格答复（ADR-0139 决定四），与 ErrAccessChannelNotConfigured、ErrOutsideAdmissionScope、
// ErrIdentityDependencyUnavailable 并列，各对一种恢复动作（ADR-0029），writeIntakeProblem 逐格映射、不合并。操作者族那两格
// 不复用：持令牌的是货主客户的系统，错误码若仍叫操作者，管授予的人会去操作者册里找一个不存在的人。
var (
	// ErrCustomerClientCredentialRejected：令牌缺失、过期、校验不过，或册上要求证书绑定而这次出示对不上（401）——持凭据
	// 的客户系统去换令牌。
	ErrCustomerClientCredentialRejected = errors.New("parcel shipment http: customer client credential rejected")
	// ErrCustomerClientNotGranted：令牌有效，但客户端不在客户渠道册，或此刻没有这一口能力的生效授予（403）——租户运营去
	// 登记或恢复授予。不在册、未授予、区间外与已撤销同答这一格，不说哪一半不对。
	ErrCustomerClientNotGranted = errors.New("parcel shipment http: customer client holds no effective grant for this capability")
)

const (
	codeCustomerClientCredentialRejected = "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"
	codeCustomerClientNotGranted         = "CUSTOMER_CLIENT_NOT_GRANTED"
)

// PresentedClientCredential 是这一次 HTTP 出示：Bearer 令牌，与这次 TLS 连接上的客户端证书（DER；没有就为空）。证书只用来
// 核对证书绑定令牌（RFC 8705）。
type PresentedClientCredential struct {
	Token       string
	Certificate []byte
}

// CustomerClientAuthenticator 认证一次首方客户出示，交回铸好的来源身份。失败只交回本包的格；从共享接入身份能力译过来的
// 那一层在 adapters/accessidentity（跨上下文翻译的位置，见 internal/architecture 的边界门禁）。
//
// accessRequestID 在令牌、册与授予都过了之后才被调用：认证没过的调用方看不到载荷校验的结果。它交回载荷里的接入请求
// 标识，由首方族的推导口铸成来源请求键（ADR-0139 决定五）；它自己的错原样交回。
type CustomerClientAuthenticator interface {
	AuthenticateSubmission(
		ctx context.Context,
		credential PresentedClientCredential,
		accessRequestID func() (string, error),
	) (domain.SourceIdentity, error)
}

// CustomerSubmissionIntake 是首方客户渠道在提交口的 Intake（ADR-0139 决定四；票 first-party-api/00）。
//
// 它把一次已认证的首方请求译成提交命令，做四件事：
//
//   - **来源信封**：租户、货主客户账户与来源整组取自认证结果——册上那一行的绑定与客户端主体，请求里写什么都不看（ADR-0003
//     的最高隔离边界）。来源请求键是载荷里的接入请求标识，由首方族的推导口铸成；它不进内容摘要（ADR-0139 决定五）。
//   - **载荷摘要与要素子段**：把载荷译成 `SubmissionPayloadSpec` 再走领域侧的 `CanonicalizeSubmission`（ADR-0014；
//     pp-seams/05 裁决 3——摘要与寄 / 收地址要素一次产出、一起进命令）。摘要只吃客户声明的内容，不吃本包现签的内部标识，
//     也不吃接入请求标识——否则同一份内容重发两次会得到两个摘要，重放就被判成`接入冲突`。
//   - **拟受理范围**：按这次提交的（租户、客户账户）立快照，形状归领域侧的 NewCustomerAccountAdmissionScope。范围到治理
//     坐标的换算归生产归属目录（PS 生产归属那一票）；在它立起之前，生产形态下归属先于读册答`权威未确定`。
//   - **期望规则修订**：向归属权威预取。`UC-PS-001` 步骤 3B 把「准入范围与期望规则修订」判给试点准入控制装配，本包据此
//     照办；编排随后在提交时点重判一次，门禁仍拦得住预取与提交之间登记册发生的变化。
//
// 提交口的生产写不在铸造侧判准入范围：它的生产归属由编排自己判（ADR-0139 决定八）。
type CustomerSubmissionIntake struct {
	authenticator CustomerClientAuthenticator
	ownership     ports.ProductionOwnershipAuthority
	clock         ports.Clock
	requests      platformidentity.Minter
	batches       platformidentity.Minter
	parcels       platformidentity.Minter
}

var _ SubmissionIntake = (*CustomerSubmissionIntake)(nil)

// 本包现签内部标识用的前缀。前缀属标识的类、不属入口：委托、批次与声明包裹从哪个接入口进来都是同一类标识，按入口另起前缀，
// 库里就有同一类标识的两种写法。
const (
	submissionRequestPrefix = "SHR"
	submissionBatchPrefix   = "SBAT"
	submissionParcelPrefix  = "DPCL"
)

// NewCustomerSubmissionIntake 由装配点构造。立不起来的依赖在这里拒：装配错误要在启动时暴露，不该等到第一个请求。
func NewCustomerSubmissionIntake(
	authenticator CustomerClientAuthenticator,
	ownership ports.ProductionOwnershipAuthority,
	clock ports.Clock,
) (*CustomerSubmissionIntake, error) {
	fail := func(err error) (*CustomerSubmissionIntake, error) {
		return nil, fmt.Errorf("parcel shipment http: customer submission intake: %w", err)
	}
	if authenticator == nil || ownership == nil || clock == nil {
		return fail(errors.New("needs an authenticator, an ownership authority and a clock"))
	}
	requests, err := platformidentity.NewMinter(submissionRequestPrefix)
	if err != nil {
		return fail(err)
	}
	batches, err := platformidentity.NewMinter(submissionBatchPrefix)
	if err != nil {
		return fail(err)
	}
	parcels, err := platformidentity.NewMinter(submissionParcelPrefix)
	if err != nil {
		return fail(err)
	}
	return &CustomerSubmissionIntake{
		authenticator: authenticator,
		ownership:     ownership,
		clock:         clock,
		requests:      requests,
		batches:       batches,
		parcels:       parcels,
	}, nil
}

// submissionDraft 是首方提交口的载荷形状：管理台提交页的草案形状（`apps/admin-web` 的 `ShipmentRequestDraft`）加一格接入
// 请求标识。它是首方 API 的产品契约（ADR-0139 决定五），字段名与版本策略的正式发布归 ADR-0140 的契约生成，不在本类型。
type submissionDraft struct {
	// AccessRequestID 是接入请求标识：客户系统逐请求给的标识，重发同一请求原样带上。它是信封元数据，由首方族的推导口铸成
	// 来源请求键，不进内容摘要（ADR-0139 决定五）。
	AccessRequestID           string  `json:"accessRequestId"`
	CustomerShipmentReference string  `json:"customerShipmentReference"`
	RequestedServiceProduct   string  `json:"requestedServiceProduct"`
	RequestEffectiveAt        *string `json:"requestEffectiveAt"`
	SenderRelation            string  `json:"senderRelation"`
	SenderAddress             string  `json:"senderAddress"`
	// SenderPostalCode / SenderCountryCode 与收件那一对是寄 / 收两范围的地址要素（PS CONTEXT「地址要素」；pp-seams/05）：
	// 译成按封闭要素名命名的范围条目，随同一次规范化既进摘要又挑成要素子段进命令。四格都可缺席——缺席即那一格没报。
	// 它们是本仓自己那张页面的草案字段，不是任何租户的渠道字段名。
	SenderPostalCode        string                `json:"senderPostalCode"`
	SenderCountryCode       string                `json:"senderCountryCode"`
	RecipientRelation       string                `json:"recipientRelation"`
	RecipientAddress        string                `json:"recipientAddress"`
	RecipientPostalCode     string                `json:"recipientPostalCode"`
	RecipientCountryCode    string                `json:"recipientCountryCode"`
	DestinationServiceScope string                `json:"destinationServiceScope"`
	Parcels                 []declaredParcelDraft `json:"parcels"`
}

type declaredParcelDraft struct {
	CustomerParcelReference string `json:"customerParcelReference"`
	DeclaredWeightValue     string `json:"declaredWeightValue"`
	DeclaredWeightUnit      string `json:"declaredWeightUnit"`
	DeclaredLength          string `json:"declaredLength"`
	DeclaredWidth           string `json:"declaredWidth"`
	DeclaredHeight          string `json:"declaredHeight"`
	DeclaredDimensionsUnit  string `json:"declaredDimensionsUnit"`
	GoodsDescription        string `json:"goodsDescription"`
	Quantity                string `json:"quantity"`
	DeclaredValue           string `json:"declaredValue"`
	Currency                string `json:"currency"`
	OriginCountry           string `json:"originCountry"`
	ServiceNotes            string `json:"serviceNotes"`
}

// IntakeSubmission 把一次首方提交译成提交命令。
//
// 先认证、后读载荷：令牌、册与授予那几格在载荷被读之前答出。请求体只供给「客户可声明」的那几组（`UC-PS-001` 输入语义
// 契约）与接入请求标识；来源信封的其余几格不看请求。形状级失败包 ErrMalformedRequest（4xx，重发同样内容不会好）；归属权威
// 调不通不包它（5xx，运维救依赖）——两格的恢复动作不同，判据同 ADR-0029。
func (intake *CustomerSubmissionIntake) IntakeSubmission(
	ctx context.Context,
	request *http.Request,
) (application.SubmitShipmentRequestCommand, error) {
	var draft submissionDraft
	identity, err := intake.authenticator.AuthenticateSubmission(ctx, presentedClientCredential(request), func() (string, error) {
		if err := decodeSubmissionDraft(request.Body, &draft); err != nil {
			return "", err
		}
		return draft.AccessRequestID, nil
	})
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}
	scope, err := domain.NewCustomerAccountAdmissionScope(identity.TenantID(), identity.CustomerAccountID())
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}

	customerReference, customerParcels, err := customerDeclaredReferences(draft)
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}
	effectiveAt, err := declaredEffectiveAt(draft)
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}
	// 只调这一次：摘要与要素子段由同一份规范化输入一次产出、一起进命令（pp-seams/05 裁决 3）。再为要素单独算一遍，
	// 内容与摘要就有了各说各话的口子——结构上不给它留位置。
	canonical, err := domain.CanonicalizeSubmission(domain.SubmissionPayloadSpec{
		RequestReference:  customerReference,
		EffectiveAt:       effectiveAt,
		DeclaredParcelIDs: customerParcels,
		Scope:             scopeEntries(draft),
		Service:           serviceEntries(draft),
	})
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}

	internalRequest, internalBatch, internalParcels, err := intake.mintInternalIdentities(len(customerParcels))
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}
	// 画像挂在现签的内部成员上，不挂客户包裹参考：版本按内部标识取成员，估价也按它取画像。
	// 挂错一侧，重量只留在摘要的服务条目里，受理前试算读到的仍是没报测量。
	profiles, err := declaredProfiles(draft, internalParcels)
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}
	customerNumbers, err := customerParcelNumbers(customerParcels, internalParcels)
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, err
	}

	// 归属权威调不通时上抛裸错：那是依赖故障，不是这次请求的形状问题。
	decision, err := intake.ownership.DecideProductionOwnership(ctx, scope)
	if err != nil {
		return application.SubmitShipmentRequestCommand{}, fmt.Errorf("customer submission intake: decide production ownership: %w", err)
	}

	now := intake.clock.Now().UTC()
	return application.SubmitShipmentRequestCommand{
		Identity:                identity,
		PayloadDigest:           canonical.Digest(),
		OccurredAt:              now,
		ReceivedAt:              now,
		BatchID:                 internalBatch,
		ShipmentRequestID:       internalRequest,
		DeclaredParcelIDs:       internalParcels,
		AdmissionScope:          scope,
		ExpectedRevision:        decision.Revision(),
		DeclaredProfiles:        profiles,
		DeclaredElements:        canonical.DeclaredElements(),
		RequestedServiceProduct: canonical.RequestedServiceProduct(),
		CustomerParcelNumbers:   customerNumbers,
	}, nil
}

// customerParcelNumbers 把逐件的客户包裹参考与现签的内部标识按次序配成客户包裹件号（ADR-0184 决定四）。
// 次序就是配对依据：内部标识按草案包裹的次序现签，与画像挂到内部成员上同一个口径。号码取
// customerDeclaredReferences 定下的声明值：两端空白已去，号码本身的字符一个不动。为什么这不算条码折算，
// 见 ADR-0184 复核记录「越权风险点 2 的事实更正」。
func customerParcelNumbers(
	customerParcels, members []domain.DeclaredParcelID,
) ([]domain.CustomerParcelNumberDeclaration, error) {
	if len(customerParcels) != len(members) {
		return nil, fmt.Errorf("customer submission intake: %d customer parcel references and %d minted members",
			len(customerParcels), len(members))
	}
	declarations := make([]domain.CustomerParcelNumberDeclaration, 0, len(members))
	for index, member := range members {
		// 拒因点名草案里那一格：号码进不了册，客户改这一格才会好（票 ps-external-mark-relations/06 裁定二）。
		number, err := domain.NewExternalIdentifierNumber(customerParcels[index].String())
		if err != nil {
			return nil, malformedField(fmt.Sprintf("parcels[%d].customerParcelReference", index), err)
		}
		declaration, err := domain.DeclareCustomerParcelNumber(member, number)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
		}
		declarations = append(declarations, declaration)
	}
	return declarations, nil
}

// declaredProfiles 把草案上已经声明的重量编成成员画像。重量两格都空就不立这一件的画像，
// 也不补一份默认测量：没报是真话，补上会让估价按一份客户没说过的重量去冻结。
// 外廓只在草案报了尺寸时带上；只报重量、不报尺寸的画像合法。尺寸报了一半则形状不完整，
// 按畸形请求拒，不把半截外廓当成没报。
func declaredProfiles(
	draft submissionDraft,
	members []domain.DeclaredParcelID,
) ([]domain.DeclaredParcelProfile, error) {
	if len(draft.Parcels) != len(members) {
		return nil, fmt.Errorf("customer submission intake: %d draft parcels and %d minted members",
			len(draft.Parcels), len(members))
	}
	profiles := make([]domain.DeclaredParcelProfile, 0, len(draft.Parcels))
	for index, parcel := range draft.Parcels {
		if !weightDeclared(parcel) {
			continue
		}
		measurement, err := declaredMeasurement(parcel)
		if err != nil {
			return nil, fmt.Errorf("%w: parcel measurement: %v", ErrMalformedRequest, err)
		}
		profile, err := domain.NewDeclaredParcelProfile(members[index], measurement)
		if err != nil {
			return nil, fmt.Errorf("%w: parcel profile: %v", ErrMalformedRequest, err)
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func weightDeclared(parcel declaredParcelDraft) bool {
	return strings.TrimSpace(parcel.DeclaredWeightValue) != "" ||
		strings.TrimSpace(parcel.DeclaredWeightUnit) != ""
}

func declaredMeasurement(parcel declaredParcelDraft) (domain.DeclaredMeasurement, error) {
	value, err := domain.NewMeasurementValue(parcel.DeclaredWeightValue)
	if err != nil {
		return domain.DeclaredMeasurement{}, err
	}
	unit, err := domain.NewMeasurementUnitReference(strings.TrimSpace(parcel.DeclaredWeightUnit))
	if err != nil {
		return domain.DeclaredMeasurement{}, err
	}
	weight, err := domain.NewDeclaredWeight(value, unit)
	if err != nil {
		return domain.DeclaredMeasurement{}, err
	}
	dimensions, err := declaredDimensions(parcel)
	if err != nil {
		return domain.DeclaredMeasurement{}, err
	}
	return domain.NewDeclaredMeasurement(weight, dimensions)
}

func declaredDimensions(parcel declaredParcelDraft) (domain.DeclaredDimensions, error) {
	length := strings.TrimSpace(parcel.DeclaredLength)
	width := strings.TrimSpace(parcel.DeclaredWidth)
	height := strings.TrimSpace(parcel.DeclaredHeight)
	unit := strings.TrimSpace(parcel.DeclaredDimensionsUnit)
	if length == "" && width == "" && height == "" && unit == "" {
		return domain.DeclaredDimensions{}, nil
	}
	lengthValue, err := domain.NewMeasurementValue(length)
	if err != nil {
		return domain.DeclaredDimensions{}, err
	}
	widthValue, err := domain.NewMeasurementValue(width)
	if err != nil {
		return domain.DeclaredDimensions{}, err
	}
	heightValue, err := domain.NewMeasurementValue(height)
	if err != nil {
		return domain.DeclaredDimensions{}, err
	}
	unitRef, err := domain.NewMeasurementUnitReference(unit)
	if err != nil {
		return domain.DeclaredDimensions{}, err
	}
	return domain.NewDeclaredDimensions(lengthValue, widthValue, heightValue, unitRef)
}

// decodeSubmissionDraft 收一份载荷：封闭解码，只许一个 JSON 对象、其后无尾随内容。载荷里出现任何不认识的格即拒——自报的
// 租户、客户账户或来源都是这一类，它们只取自认证结果。
func decodeSubmissionDraft(body io.Reader, draft *submissionDraft) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(draft); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	return nil
}

func presentedClientCredential(request *http.Request) PresentedClientCredential {
	var certificate []byte
	if request.TLS != nil && len(request.TLS.PeerCertificates) > 0 {
		certificate = request.TLS.PeerCertificates[0].Raw
	}
	return PresentedClientCredential{Token: httpapi.BearerToken(request), Certificate: certificate}
}

// declaredEffectiveAt 取客户请求生效时间。键缺席交回零值，而零值在
// `CanonicalizeSubmissionPayload` 里编成「未声明」那一格——值与缺失/显式存在状态都进
// 摘要是 CONTEXT 硬句，所以这里绝不能拿「零值」冒充「声明了但为空」，也不能拿
// occurredAt 顶替。键在场但解不出时间是形状错，如实答 4xx。
func declaredEffectiveAt(draft submissionDraft) (time.Time, error) {
	if draft.RequestEffectiveAt == nil {
		return time.Time{}, nil
	}
	declared, err := time.Parse(time.RFC3339, strings.TrimSpace(*draft.RequestEffectiveAt))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: requestEffectiveAt: %v", ErrMalformedRequest, err)
	}
	return declared, nil
}

// customerDeclaredReferences 取客户声明的委托参考与逐件包裹参考。摘要吃的是这一组，不是
// 本包现签的内部标识——理由在类型注释「载荷摘要与要素子段」那一条。
func customerDeclaredReferences(
	draft submissionDraft,
) (domain.ShipmentRequestID, []domain.DeclaredParcelID, error) {
	reference, err := domain.NewShipmentRequestID(strings.TrimSpace(draft.CustomerShipmentReference))
	if err != nil {
		return domain.ShipmentRequestID{}, nil, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if len(draft.Parcels) == 0 {
		return domain.ShipmentRequestID{}, nil, fmt.Errorf("%w: at least one parcel is required", ErrMalformedRequest)
	}
	parcels := make([]domain.DeclaredParcelID, 0, len(draft.Parcels))
	for _, parcel := range draft.Parcels {
		declared, err := domain.NewDeclaredParcelID(strings.TrimSpace(parcel.CustomerParcelReference))
		if err != nil {
			return domain.ShipmentRequestID{}, nil, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
		}
		parcels = append(parcels, declared)
	}
	return reference, parcels, nil
}

// mintInternalIdentities 现签内部标识。它们**不从客户参考派生**——`SubmissionIdentityFactory`
// 的端口注释把这条写死了：「客户参考号不得变成内部标识」。代价是同一份内容重发两次会
// 签出两组新标识，但那两组都用不上：重放在来源身份上就被认出，命令里的标识根本走不到建单。
func (intake *CustomerSubmissionIntake) mintInternalIdentities(
	parcelCount int,
) (domain.ShipmentRequestID, domain.SubmissionBatchID, []domain.DeclaredParcelID, error) {
	requestValue, err := intake.requests.Next()
	if err != nil {
		return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
	}
	requestID, err := domain.NewShipmentRequestID(requestValue)
	if err != nil {
		return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
	}
	batchValue, err := intake.batches.Next()
	if err != nil {
		return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
	}
	batchID, err := domain.NewSubmissionBatchID(batchValue)
	if err != nil {
		return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
	}
	parcels := make([]domain.DeclaredParcelID, 0, parcelCount)
	for index := 0; index < parcelCount; index++ {
		parcelValue, err := intake.parcels.Next()
		if err != nil {
			return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
		}
		parcelID, err := domain.NewDeclaredParcelID(parcelValue)
		if err != nil {
			return domain.ShipmentRequestID{}, domain.SubmissionBatchID{}, nil, err
		}
		parcels = append(parcels, parcelID)
	}
	return requestID, batchID, parcels, nil
}

// scopeEntries 与 serviceEntries 是本形状的规范化词表：哪个草案字段进摘要的哪一段。
// 缺席的字段整条不进——`CanonicalContentEntry` 把「显式清空」与「条目缺席」定为两种内容，
// 给缺席字段补一条空值条目会把前者的语义安在后者头上。
//
// 邮编与国家 / 地区码四格不用本文件自造的点号名，而取 domain.AddressElementEntryName 拼出的封闭名字：
// 那是 AddressElementsOf 唯一认的名字，本形状用别的名字它们就只进摘要、进不了要素子段。
func scopeEntries(draft submissionDraft) []domain.CanonicalContentEntry {
	sender, delivery := domain.SenderPlaceDataGroup(), domain.DeliveryPlaceDataGroup()
	return canonicalEntries(map[string]string{
		"sender.relation":          draft.SenderRelation,
		"sender.address":           draft.SenderAddress,
		"recipient.relation":       draft.RecipientRelation,
		"recipient.address":        draft.RecipientAddress,
		"destination.serviceScope": draft.DestinationServiceScope,
		domain.AddressElementEntryName(sender, domain.PostalCodeElement):    draft.SenderPostalCode,
		domain.AddressElementEntryName(sender, domain.CountryCodeElement):   draft.SenderCountryCode,
		domain.AddressElementEntryName(delivery, domain.PostalCodeElement):  draft.RecipientPostalCode,
		domain.AddressElementEntryName(delivery, domain.CountryCodeElement): draft.RecipientCountryCode,
	})
}

func serviceEntries(draft submissionDraft) []domain.CanonicalContentEntry {
	entries := map[string]string{domain.RequestedServiceProductEntryName: draft.RequestedServiceProduct}
	for _, parcel := range draft.Parcels {
		reference := strings.TrimSpace(parcel.CustomerParcelReference)
		for name, value := range map[string]string{
			"weight.value":      parcel.DeclaredWeightValue,
			"weight.unit":       parcel.DeclaredWeightUnit,
			"dimensions.length": parcel.DeclaredLength,
			"dimensions.width":  parcel.DeclaredWidth,
			"dimensions.height": parcel.DeclaredHeight,
			"dimensions.unit":   parcel.DeclaredDimensionsUnit,
			"goods.description": parcel.GoodsDescription,
			"goods.quantity":    parcel.Quantity,
			"goods.value":       parcel.DeclaredValue,
			"goods.currency":    parcel.Currency,
			"goods.origin":      parcel.OriginCountry,
			"service.notes":     parcel.ServiceNotes,
		} {
			entries["parcel["+reference+"]."+name] = value
		}
	}
	return canonicalEntries(entries)
}

func canonicalEntries(values map[string]string) []domain.CanonicalContentEntry {
	entries := make([]domain.CanonicalContentEntry, 0, len(values))
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		entry, err := domain.NewCanonicalContentEntry(name, value)
		if err != nil {
			// 名字是本文件的字面量常量，构造不出来只可能是这里写错了。
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}
