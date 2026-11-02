package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberhttp"
)

// documentNumberingHandlers 是规则册四口（ADR-0355 决定六）装好的处理器。
type documentNumberingHandlers struct {
	read                 http.Handler
	preview              http.Handler
	ruleRegistration     http.Handler
	timeZoneRegistration http.Handler
}

// buildDocumentNumberingHandlers 装规则册四口。规则册不属任何限界上下文，认证件由本特性从生产共享的操作者铸造器自建
// （同基础数据目录）：读口与预览口挂主数据与运营查阅读，两个登记口挂登记册配置写；都不判准入范围——登记一行不形成
// 生产事实（ADR-0100 决定四）。共享接入身份的答复格译成 docnumberhttp 的格这一步放在组合根：platform 不得导入
// accessidentity。编号组件本身由 assembleProductionEndpoints 装一次、经 endpointSources 交来，开立单据的特性取同一套。
func buildDocumentNumberingHandlers(numbers docnumbering.Components, minter *accessidentity.OperatorMinter) (documentNumberingHandlers, error) {
	if minter == nil {
		return documentNumberingHandlers{}, errors.New("document numbering needs a minter")
	}
	if numbers.Reader == nil || numbers.Assigner == nil {
		return documentNumberingHandlers{}, errors.New("document numbering components are not built")
	}
	authenticator := documentNumberingAuthenticator{minter: minter}
	read, err := docnumberhttp.NewReadEndpoint(authenticator, numbers.Reader)
	if err != nil {
		return documentNumberingHandlers{}, err
	}
	preview, err := docnumberhttp.NewPreviewEndpoint(authenticator, numbers.Reader)
	if err != nil {
		return documentNumberingHandlers{}, err
	}
	ruleRegistration, err := docnumberhttp.NewRuleRegistrationEndpoint(authenticator, numbers.Registrations)
	if err != nil {
		return documentNumberingHandlers{}, err
	}
	timeZoneRegistration, err := docnumberhttp.NewTimeZoneRegistrationEndpoint(authenticator, numbers.Registrations)
	if err != nil {
		return documentNumberingHandlers{}, err
	}
	return documentNumberingHandlers{read: read, preview: preview, ruleRegistration: ruleRegistration, timeZoneRegistration: timeZoneRegistration}, nil
}

// documentNumberingAuthenticator 实现 docnumberhttp 的两种认证件：按各口的能力面铸操作者信封，租户只取自信封。
type documentNumberingAuthenticator struct {
	minter *accessidentity.OperatorMinter
}

var (
	_ docnumberhttp.OperationsReadAuthenticator = documentNumberingAuthenticator{}
	_ docnumberhttp.RegistryWriteAuthenticator  = documentNumberingAuthenticator{}
)

func (authenticator documentNumberingAuthenticator) AuthenticateOperationsRead(ctx context.Context, bearerToken string) (string, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, accessidentity.NewOperatorCredential(bearerToken),
		accessidentity.OperatorRequest{Face: accessidentity.CapabilityMasterDataAndOperationsRead})
	if err != nil {
		return "", answerDocumentNumberingAuthentication(err)
	}
	return envelope.TenantID(), nil
}

// AuthenticateRegistryWrite 交回租户与登记人。登记人写成「OPERATOR/发行方#sub」：sub 只在它的发行方之内唯一
// （ADR-0100 决定二），与关务登记口记操作证据的写法同形。
func (authenticator documentNumberingAuthenticator) AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (docnumberhttp.Registrant, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, accessidentity.NewOperatorCredential(bearerToken),
		accessidentity.OperatorRequest{Face: accessidentity.CapabilityRegistryConfigurationWrite})
	if err != nil {
		return docnumberhttp.Registrant{}, answerDocumentNumberingAuthentication(err)
	}
	subject := envelope.Subject()
	return docnumberhttp.Registrant{
		Tenant:   envelope.TenantID(),
		Operator: "OPERATOR/" + subject.Issuer() + "#" + subject.Subject(),
	}, nil
}

func answerDocumentNumberingAuthentication(err error) error {
	switch {
	case errors.Is(err, accessidentity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", docnumberhttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, accessidentity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", docnumberhttp.ErrOperatorCredentialRejected, err)
	case errors.Is(err, accessidentity.ErrOperatorNotGranted):
		return fmt.Errorf("%w: %w", docnumberhttp.ErrOperatorNotGranted, err)
	case errors.Is(err, accessidentity.ErrCredentialVerifierUnavailable),
		errors.Is(err, accessidentity.ErrOperatorRegistryUnavailable):
		return fmt.Errorf("%w: %w", docnumberhttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
