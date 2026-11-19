// Package registrationjson 是仓配履约登记报文的 JSON 译装：把调用方交来的一笔修订译成应用层的登记命令（ADR-0375 决定四）。
//
// 租户与登记责任方只从接入信封来（UC-WF-001「输入」），所以译装收调用方传入的两个值，报文里不许有这两格。本包只管形状：
// 国家码在不在内置目录、业务时区认不认得出、修订号接不接得上，都是应用层在判过重放与冲突之后另判的事，译装不提前把它们
// 当坏载荷拒掉，否则同一份内容在「原样重投」时会先于「已登记（重放）」被拒。
package registrationjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// ErrMalformedDocument 是报文形状不成立：改载荷才会好，原样重投不会。
var ErrMalformedDocument = errors.New("warehouse fulfillment registrationjson: malformed document")

// MalformedDocumentError 带一句只谈调用方自己这份载荷错在哪的理由。理由只点名格，不回显任何取值——地址正文与仓编码出现在
// 错误正文里，日志与界面都得再防一次（UC-WF-001「权限、隔离与审计」）。
type MalformedDocumentError struct {
	Reason string
}

func (err *MalformedDocumentError) Error() string {
	return fmt.Sprintf("%s: %s", ErrMalformedDocument, err.Reason)
}

func (err *MalformedDocumentError) Is(target error) bool {
	return target == ErrMalformedDocument
}

func malformed(format string, arguments ...any) error {
	return &MalformedDocumentError{Reason: fmt.Sprintf(format, arguments...)}
}

// warehouseDocument 是一笔仓修订的线格式。键名与登记口答复里 `field` 点名的格同一套（`address.country`、`businessTimeZone`），
// 登记方照答复里的格改报文，不必再做一次对照。
//
// TenantID 与 Registrant 两格**只为拒而存在**：用 json.RawMessage 是因为 `"tenantId": null` 也是自报，键在场就要拒，
// 指针认不出 null。只靠 DisallowUnknownFields 也会拒，但那样的理由是「未知键」，登记方读不出这是一条规则而不是拼写错误。
//
// 必填格用指针，缺失与零值才分得开：修订号 0 是应用层答修订错位的内容，不是缺失；空串是空白，不是缺失。
type warehouseDocument struct {
	TenantID         json.RawMessage  `json:"tenantId"`
	Registrant       json.RawMessage  `json:"registrant"`
	WarehouseCode    *string          `json:"warehouseCode"`
	Revision         *int             `json:"revision"`
	Name             *string          `json:"name"`
	Kind             *string          `json:"kind"`
	Address          *addressDocument `json:"address"`
	BusinessTimeZone *string          `json:"businessTimeZone"`
	ColocatedNode    *string          `json:"colocatedNode"`
	Deactivated      *bool            `json:"deactivated"`
}

type addressDocument struct {
	Country *string `json:"country"`
	Text    *string `json:"text"`
}

// WarehouseCommand 把一笔仓修订的 JSON 译成登记命令。tenant 与 registrant 由调用方给（取自认证结果），报文自带即坏载荷。
// 一份载荷只许一个对象：尾随的第二个 JSON 值也拒，放过它就是无声丢掉一段输入。
//
// 仓编码、地址正文、同址物流节点逐字交给领域：不去空白、不折大小写（CONTEXT「主数据登记」）。可缺的业务时区、同址物流节点与
// 是否停用缺失即没登记、没指名、不停用，不取任何默认。键给了却是空白的，不当作没给——那是写错了，不是没有。
func WarehouseCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.RegisterWarehouseCommand, error) {
	var none application.RegisterWarehouseCommand
	document, err := decodeWarehouseDocument(reader)
	if err != nil {
		return none, err
	}
	if len(document.TenantID) > 0 {
		return none, malformed("tenantId must not be carried in the payload; the tenant comes from the access channel, not from the request")
	}
	if len(document.Registrant) > 0 {
		return none, malformed("registrant must not be carried in the payload; the registrant comes from the operator envelope, not from the request")
	}

	if document.WarehouseCode == nil {
		return none, malformed("warehouseCode is required")
	}
	if document.Revision == nil {
		return none, malformed("revision is required")
	}
	if document.Name == nil {
		return none, malformed("name is required")
	}
	if document.Kind == nil {
		return none, malformed("kind is required")
	}
	if document.Address == nil {
		return none, malformed("address is required")
	}
	if document.Address.Country == nil {
		return none, malformed("address.country is required")
	}
	if document.Address.Text == nil {
		return none, malformed("address.text is required")
	}

	code, err := textCell("warehouseCode", *document.WarehouseCode, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	name, err := textCell("name", *document.Name, domain.NewWarehouseName)
	if err != nil {
		return none, err
	}
	kind, known := domain.WarehouseKindNamed(*document.Kind)
	if !known {
		return none, malformed("kind must be one of %s, %s", domain.WarehouseKindSelfOperated, domain.WarehouseKindAgent)
	}
	country, err := textCell("address.country", *document.Address.Country, domain.NewCountryCode)
	if err != nil {
		return none, err
	}
	addressText, err := textCell("address.text", *document.Address.Text, domain.NewAddressText)
	if err != nil {
		return none, err
	}

	spec := domain.WarehouseContentSpec{
		Name:    name,
		Kind:    kind,
		Address: domain.NewWarehouseAddress(country, addressText),
	}
	if document.BusinessTimeZone != nil {
		if spec.BusinessTimeZone, err = textCell("businessTimeZone", *document.BusinessTimeZone, domain.NewBusinessTimeZone); err != nil {
			return none, err
		}
	}
	if document.ColocatedNode != nil {
		if spec.ColocatedNode, err = textCell("colocatedNode", *document.ColocatedNode, domain.NewColocatedNode); err != nil {
			return none, err
		}
	}
	if document.Deactivated != nil {
		spec.Deactivated = *document.Deactivated
	}
	content, err := domain.NewWarehouseContent(spec)
	if err != nil {
		// 各必填格上面都已过了领域构造门，走到这里是领域与译装对「必填」的理解漂开了。
		return none, malformed("the warehouse content is incomplete")
	}
	return application.RegisterWarehouseCommand{
		Tenant:     tenant,
		Code:       code,
		Revision:   *document.Revision,
		Content:    content,
		Registrant: registrant,
	}, nil
}

// textCell 过一格文本的领域构造门。构造器的错误正文带着格名而不带取值，但这里仍自己写理由：理由只点名 JSON 里的键路径，
// 与报文的写法对得上。
func textCell[Value any](path, raw string, construct func(string) (Value, error)) (Value, error) {
	value, err := construct(raw)
	switch {
	case err == nil:
		return value, nil
	case errors.Is(err, domain.ErrUnstorableValue):
		return value, malformed("%s carries a character that cannot be stored (NUL or invalid UTF-8)", path)
	default:
		return value, malformed("%s must not be blank", path)
	}
}

func decodeWarehouseDocument(reader io.Reader) (warehouseDocument, error) {
	var document warehouseDocument
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return warehouseDocument{}, decodeFailure(err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return warehouseDocument{}, malformed("the body must carry exactly one JSON object and nothing after it")
	}
	return document, nil
}

// decodeFailure 把解码器的错译成一句不带取值的理由。解码器的错误正文里有的带着报文里的片段（类型错的取值、读错的原文），
// 所以只取它的类别与键路径，不转抄它的正文。
func decodeFailure(err error) error {
	var syntax *json.SyntaxError
	var typed *json.UnmarshalTypeError
	switch {
	case errors.Is(err, io.EOF):
		return malformed("the body is empty")
	case errors.Is(err, io.ErrUnexpectedEOF), errors.As(err, &syntax):
		return malformed("the body is not valid JSON")
	case errors.As(err, &typed):
		if typed.Field == "" {
			return malformed("the body must be a JSON object")
		}
		return malformed("%s must be %s", typed.Field, expectedKind(typed.Type))
	}
	if quoted, isUnknownKey := strings.CutPrefix(err.Error(), "json: unknown field "); isUnknownKey {
		if key, unquoteErr := strconv.Unquote(quoted); unquoteErr == nil && len(key) <= 64 {
			return malformed("the key %q is not part of a warehouse revision", key)
		}
		return malformed("the payload carries a key that is not part of a warehouse revision")
	}
	return malformed("the body could not be read as JSON")
}

func expectedKind(kind reflect.Type) string {
	switch kind.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Int, reflect.Int64:
		return "an integer"
	case reflect.Bool:
		return "a boolean"
	case reflect.Struct:
		return "an object"
	default:
		return "of the declared type"
	}
}
