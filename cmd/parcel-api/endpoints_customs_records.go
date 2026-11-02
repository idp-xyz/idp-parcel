package main

import (
	"net/http"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 关务记录各口（案件、提交版本、外部监管结果、动作准入判断册与其中的卡关对象）：customs-cases 页对象族签与门禁判断册的查阅面，
// 卡关对象另供首页「今日待办」数件数。
// 与 endpoints_customs_catalogue.go 同族——操作者查阅读、Intake 同变量；但那边是登记册（配置），这边是已发生的
// 事实记录，所以另立一个特性，不往目录特性里塞。

type customsRecordsWiring struct {
	cases       customshttp.CustomsCaseRecordReader
	submissions customshttp.SubmissionRecordReader
	results     customshttp.ExternalResultViewReader
	judgements  customshttp.ActionAdmissionJudgementReader
	unadmitted  customshttp.UnadmittedObjectReader
}

func init() {
	registerEndpoints(endpointFeature[customsRecordsWiring]{
		lines: map[string]endpointLine{
			"/customs-case-records":                {method: http.MethodGet, face: operationsReadFace()},
			"/customs-submission-records":          {method: http.MethodGet, face: operationsReadFace()},
			"/customs-external-result-records":     {method: http.MethodGet, face: operationsReadFace()},
			"/customs-action-admission-judgements": {method: http.MethodGet, face: operationsReadFace()},
			"/customs-unadmitted-objects":          {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (customsRecordsWiring, error) {
			views, err := ccpostgres.NewOperationsRecordViews(sources.db)
			if err != nil {
				return customsRecordsWiring{}, err
			}
			// 各口共用一个读适配器（同一上下文的几张事实表），各按自己的接口消费；卡关对象读的是判断册那张表。
			return customsRecordsWiring{cases: views, submissions: views, results: views, judgements: views, unadmitted: views}, nil
		},
		mount: func(channels endpointChannels, wiring customsRecordsWiring) []httpapi.BusinessEndpoint {
			intake := channels.complianceRules()
			return []httpapi.BusinessEndpoint{
				{Pattern: "/customs-case-records", Handler: customshttp.NewQueryCustomsCasesEndpoint(intake, wiring.cases)},
				{Pattern: "/customs-submission-records", Handler: customshttp.NewQuerySubmissionVersionsEndpoint(intake, wiring.submissions)},
				{Pattern: "/customs-external-result-records", Handler: customshttp.NewQueryExternalResultsEndpoint(intake, wiring.results)},
				{Pattern: "/customs-action-admission-judgements", Handler: customshttp.NewQueryActionAdmissionJudgementsEndpoint(intake, wiring.judgements)},
				{Pattern: "/customs-unadmitted-objects", Handler: customshttp.NewQueryUnadmittedObjectsEndpoint(intake, wiring.unadmitted)},
			}
		},
	})
}
