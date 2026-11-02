package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbertypes"
	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberhttp"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
)

// documentNumbering 是编号组件在本进程的那一套：规则册的登记与读，以及取号编排。
type documentNumbering struct {
	registrar *docnumber.Registrar
	reader    *docnumber.Reader
	// assigner 是取号编排。各类单据的开立（票 document-numbering/04–09）经各自上下文的消费侧端口接它；本票只装，
	// 装不起来（声明不合规、序列存储构造失败）要在启动时带原因退出，而不是等第一张单据开立。
	assigner   *docnumber.Assigner
	transactor bentoapp.Transactor
}

// buildDocumentNumbering 装编号组件。规则册、序列与调用方的开立事务接在同一个 DB 上：框架按 DB 认事务归属。
func buildDocumentNumbering(db *bentopg.DB, secrets docnumber.ScrambleSecrets) (documentNumbering, error) {
	if db == nil || secrets == nil {
		return documentNumbering{}, errors.New("document numbering needs a db and scramble secrets")
	}
	catalogue, err := docnumbertypes.Catalogue()
	if err != nil {
		return documentNumbering{}, fmt.Errorf("parcel-api: document number catalogue: %w", err)
	}
	store, err := docnumberpg.NewStore(db)
	if err != nil {
		return documentNumbering{}, err
	}
	sequences, err := docnumberpg.NewSequences(db)
	if err != nil {
		return documentNumbering{}, err
	}
	registrar, err := docnumber.NewRegistrar(docnumber.RegistrarDeps{Catalogue: catalogue, Book: store, Secrets: secrets, Clock: systemClock{}})
	if err != nil {
		return documentNumbering{}, err
	}
	reader, err := docnumber.NewReader(docnumber.ReaderDeps{Catalogue: catalogue, Book: store, Secrets: secrets, Clock: systemClock{}})
	if err != nil {
		return documentNumbering{}, err
	}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: store, Sequences: sequences, Transactions: store, Secrets: secrets, Clock: systemClock{},
	})
	if err != nil {
		return documentNumbering{}, err
	}
	return documentNumbering{registrar: registrar, reader: reader, assigner: assigner, transactor: db.Transactor()}, nil
}

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
// accessidentity。
func buildDocumentNumberingHandlers(db *bentopg.DB, minter *accessidentity.OperatorMinter, secrets docnumber.ScrambleSecrets) (documentNumberingHandlers, error) {
	if minter == nil {
		return documentNumberingHandlers{}, errors.New("document numbering needs a minter")
	}
	numbering, err := buildDocumentNumbering(db, secrets)
	if err != nil {
		return documentNumberingHandlers{}, err
	}
	authenticator := documentNumberingAuthenticator{minter: minter}
	registrar := transactionalDocumentNumberRegistrar{transactor: numbering.transactor, inner: numbering.registrar}
	read, err := docnumberhttp.NewReadEndpoint(authenticator, numbering.reader)
	if err != nil {
		return documentNumberingHandlers{}, err
	}
	preview, err := docnumberhttp.NewPreviewEndpoint(authenticator, numbering.reader)
	if err != nil {
		return documentNumberingHandlers{}, err
	}
	ruleRegistration, err := docnumberhttp.NewRuleRegistrationEndpoint(authenticator, registrar)
	if err != nil {
		return documentNumberingHandlers{}, err
	}
	timeZoneRegistration, err := docnumberhttp.NewTimeZoneRegistrationEndpoint(authenticator, registrar)
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

// transactionalDocumentNumberRegistrar 把一次登记包进一笔事务：规则册的写口无事务即拒，事务边界归装配点；编排返回
// error 时整笔回滚。
type transactionalDocumentNumberRegistrar struct {
	transactor bentoapp.Transactor
	inner      *docnumber.Registrar
}

var (
	_ docnumberhttp.RuleRegistrar     = transactionalDocumentNumberRegistrar{}
	_ docnumberhttp.TimeZoneRegistrar = transactionalDocumentNumberRegistrar{}
)

func (registrar transactionalDocumentNumberRegistrar) RegisterRule(ctx context.Context, command docnumber.RegisterRuleCommand) (docnumber.RuleRegistration, error) {
	var result docnumber.RuleRegistration
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, err := registrar.inner.RegisterRule(txCtx, command)
		result = handled
		return err
	})
	if err != nil {
		return docnumber.RuleRegistration{}, err
	}
	return result, nil
}

func (registrar transactionalDocumentNumberRegistrar) RegisterTimeZone(ctx context.Context, command docnumber.RegisterTimeZoneCommand) (docnumber.TimeZoneRegistration, error) {
	var result docnumber.TimeZoneRegistration
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, err := registrar.inner.RegisterTimeZone(txCtx, command)
		result = handled
		return err
	})
	if err != nil {
		return docnumber.TimeZoneRegistration{}, err
	}
	return result, nil
}
