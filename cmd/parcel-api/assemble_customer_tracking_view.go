package main

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	veaccess "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

// buildCustomerTrackingViewIntake 装配客户追踪视图查阅口的首方 Intake（ADR-0139 决定四；UC-VE-008；票 demo-chain-gaps/96）。铸造器与
// 提交口同一只（buildCustomerClientMinter）：首方族一个进程一只铸造器。查阅口不判准入范围（ADR-0139 决定八），所以铸造器里那只准入
// 范围在这一口不被问到。
func buildCustomerTrackingViewIntake(minter *accessidentity.CustomerClientMinter) (*visibilityhttp.CustomerTrackingViewIntake, error) {
	if minter == nil {
		return nil, fmt.Errorf("customer tracking view intake needs a minter")
	}
	authenticator, err := veaccess.NewCustomerTrackingViewAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	intake, err := visibilityhttp.NewCustomerTrackingViewIntake(authenticator)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: customer tracking view intake: %w", err)
	}
	return intake, nil
}

// customerTrackingViewQueryIntake 交回端点表那一行用的 Intake：首方 Intake 缺席时是字面量未配置即拒。在这里判 nil，是为了不把一个
// 带类型的空指针塞进接口——那样的 Intake 不等于 nil，第一次请求才会崩。两个隔离开关都换不了这一口（ADR-0150 决定三）。
func customerTrackingViewQueryIntake(intake *visibilityhttp.CustomerTrackingViewIntake) visibilityhttp.QueryIntake {
	if intake == nil {
		return visibilityhttp.UnconfiguredIntake{}
	}
	return intake
}
