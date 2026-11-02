package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	ccdocnumbering "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/docnumbering"
	ccidentity "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/identity"
	ccparcelshipment "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/parcelshipment"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
)

// customsDecisionTransaction 把三口的编排各包进一笔事务：案件、授权与提交各册的写口按框架合同无事务即拒，事务边界归装配点；
// 案件与它的单据号、发布意图，提交版本与首次尝试和发布意图各同笔落地。
//
// 编排答未决时整笔回滚、再交回那一份答复：未决可能是读口在事务里报了 SQL 错误，PostgreSQL 已把整笔置为 aborted，照常提交只
// 换回提交错误；提交执行在单元行落下之后才停的那几格，回滚也只是让重试从头再来。建案另有两格要回滚：编号规则撞号，与取过号
// 才发现同一范围已被并发建案抢先——取过的流水没随案件落下，只有回滚才释放（ADR-0355 决定三，见 EstablishCaseResult.RollsBack）。
// 其余各格照常提交——它们要么本无写入，要么是编排已经说清的业务答案。编排交回 error 时整笔回滚、原样上抛。
type customsDecisionTransaction[C, R any] struct {
	transactor bentoapp.Transactor
	handle     func(context.Context, C) (R, error)
	rollBack   func(R) bool
}

// errCustomsDecisionRolledBack 只在本壳里流转：答复要求回滚时回调交它，逼事务回滚；回滚之后认出它，交回编排那一份答复。
var errCustomsDecisionRolledBack = errors.New("parcel-api: customs decision answered without committing; transaction rolled back")

func (wrapper customsDecisionTransaction[C, R]) Handle(ctx context.Context, command C) (R, error) {
	var result R
	err := wrapper.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := wrapper.handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if wrapper.rollBack(handled) {
			return errCustomsDecisionRolledBack
		}
		return nil
	})
	if errors.Is(err, errCustomsDecisionRolledBack) {
		return result, nil
	}
	if err != nil {
		var none R
		return none, err
	}
	return result, nil
}

type customsDecisionFaces struct {
	caseEstablishment       customsDecisionTransaction[customsapp.EstablishCaseForParcelsCommand, customsapp.EstablishCaseResult]
	submissionAuthorization customsDecisionTransaction[customsapp.ConfirmSubmissionAuthorizationCommand, customsapp.AuthorizationConfirmationResult]
	declarationSubmission   customsDecisionTransaction[customsapp.SubmitDeclarationCommand, customsapp.SubmitDeclarationResult]
}

// buildCustomsDecisionFaces 装配 `/customs-cases`、`/customs-submission-authorizations` 与 `/customs-declaration-submissions`
// 背后的真编排（票 product-strategy-boundary/18，ADR-0227）。缝全接真：
//
//   - 建案：parcel-shipment 委托仓储上那只按包裹反查当前已接受委托的口（经 CC 的消费侧适配器译出客户归属与来源引用）、建案要求
//     规则册、案件册、案件标识签发、案件单据号（进程共用的那一套编号组件的取号编排，判撞查案件册；ADR-0355）与案件建立意图
//     （VE 案件视图消费）。
//   - 授权确认：就绪判断读口与提交授权册的读写两半。授权确认不入队：今天没有它的消费者。
//   - 提交执行：提交册、案件册（反查核存在）、申报单元册、就绪与授权两个读口、版本标识签发与提交版本形成意图（VE 投影消费；
//     发送通道归票 product-strategy-boundary/10「申报发送通道」一项，今天没有任何东西把它交给海关或报关服务商）。
//
// 规则、就绪与授权三本册的内容是租户取值：没登时各口如实答未决并指名未配置，不为此种任何行。
//
// numbers 必须是在同一个 db 上装出的编号组件：取号要在建案事务里，框架按 DB 认事务归属。
func buildCustomsDecisionFaces(db *bentopg.DB, numbers docnumbering.Components) (customsDecisionFaces, error) {
	none := customsDecisionFaces{}
	if numbers.Assigner == nil {
		return none, errors.New("parcel-api: customs decisions need the document numbering components")
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}

	acceptedRequests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	acceptedParcels, err := ccparcelshipment.NewAcceptedParcelSources(acceptedRequests)
	if err != nil {
		return none, fmt.Errorf("parcel-api: accepted parcel sources: %w", err)
	}
	requirements, err := ccpostgres.NewCaseRequirementView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: case requirement view: %w", err)
	}
	cases, err := ccpostgres.NewCustomsCases(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs cases: %w", err)
	}
	caseIdentities, err := ccidentity.NewCaseIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: case identities: %w", err)
	}
	caseNumbers, err := ccdocnumbering.NewCaseNumbers(numbers.Assigner, cases)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs case numbers: %w", err)
	}
	caseHandoff, err := ccpostgres.NewOutboxCustomsCaseHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs case handoff: %w", err)
	}
	establishment := customsapp.NewEstablishCaseForParcelsHandler(customsapp.EstablishCaseForParcelsDeps{
		Parcels: acceptedParcels,
		Cases: customsapp.NewEstablishCaseHandler(customsapp.EstablishCaseDeps{
			Requirement: requirements,
			Store:       cases,
			Identity:    caseIdentities,
			Numbers:     caseNumbers,
			Downstream:  caseHandoff,
			Clock:       clock,
		}),
	})

	readiness, err := ccpostgres.NewReadinessView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: readiness view: %w", err)
	}
	authorities, err := ccpostgres.NewSubmissionAuthorityRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: submission authority registrations: %w", err)
	}
	authorityView, err := ccpostgres.NewSubmissionAuthorityView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: submission authority view: %w", err)
	}
	confirmation := customsapp.NewConfirmSubmissionAuthorizationHandler(customsapp.ConfirmSubmissionAuthorizationDeps{
		Readiness:     readiness,
		Authorities:   authorities,
		AuthorityView: authorityView,
		Clock:         clock,
	})

	submissions, err := ccpostgres.NewDeclarationSubmissions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: declaration submissions: %w", err)
	}
	units, err := ccpostgres.NewDeclarationUnits(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: declaration units: %w", err)
	}
	versions, err := ccidentity.NewDeclarationVersions()
	if err != nil {
		return none, fmt.Errorf("parcel-api: declaration versions: %w", err)
	}
	submissionHandoff, err := ccpostgres.NewOutboxDeclarationSubmissionHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: declaration submission handoff: %w", err)
	}
	submission := customsapp.NewSubmitDeclarationHandler(customsapp.SubmitDeclarationDeps{
		Submissions: submissions,
		Cases:       cases,
		Units:       units,
		Readiness:   readiness,
		Authority:   authorityView,
		Versions:    versions,
		Downstream:  submissionHandoff,
		Clock:       clock,
	})

	transactor := db.Transactor()
	return customsDecisionFaces{
		caseEstablishment: customsDecisionTransaction[customsapp.EstablishCaseForParcelsCommand, customsapp.EstablishCaseResult]{
			transactor: transactor, handle: establishment.Handle,
			rollBack: customsapp.EstablishCaseResult.RollsBack,
		},
		submissionAuthorization: customsDecisionTransaction[customsapp.ConfirmSubmissionAuthorizationCommand, customsapp.AuthorizationConfirmationResult]{
			transactor: transactor, handle: confirmation.Handle,
			rollBack: func(result customsapp.AuthorizationConfirmationResult) bool {
				return result.Outcome() == customsapp.AuthorizationConfirmationUndecided
			},
		},
		declarationSubmission: customsDecisionTransaction[customsapp.SubmitDeclarationCommand, customsapp.SubmitDeclarationResult]{
			transactor: transactor, handle: submission.Handle,
			rollBack: func(result customsapp.SubmitDeclarationResult) bool {
				return result.Outcome() == customsapp.DeclarationUndecided
			},
		},
	}, nil
}
