package main

import (
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nodocnumbers "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/documentnumbers"
	nonetwork "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/networkrouting"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
)

// errNumberingCollisionRollsBack 是送站预报、送站交接单的事务外壳在编排答「编号规则撞号」时交给事务器的回滚信号。撞号是形成了的
// 业务答案，却必须回滚：取号编排从序列取过的那些流水一个都没出号，事务一提交它们就全成了空号，而 ADR-0355 决定三只许随单落定的
// 跳号成为空号（docnumber.NumberingRuleCollision 的注释）。外壳收到这个信号就把答案原样交回，不当错误。
var errNumberingCollisionRollsBack = errors.New("parcel-api: the issuing transaction rolls back on a numbering rule collision")

// buildDeliveryScopeDocumentNumbering 装送站预报与送站交接单取单据号的两样（ADR-0355 决定二、七）：取号编排是组合根装出的那一只
// （endpointSources.documentNumbers.Assigner，与规则册各口同一份类型汇总、同一个库）；开立网点的业务时区经本上下文的防腐适配器问
// network-routing 网络目录的窄读口。
//
// assigner 收具体类型而不收接口：nil 指针塞进接口就不等于 nil，构造期的拒缺件会漏过它，第一张单据开立时才崩。
func buildDeliveryScopeDocumentNumbering(db *bentopg.DB, assigner *docnumber.Assigner) (nodeopsapp.DocumentNumberingDeps, error) {
	if assigner == nil {
		return nodeopsapp.DocumentNumberingDeps{}, errors.New("parcel-api: delivery scope document numbering needs the document number assigner")
	}
	issuer, err := nodocnumbers.NewIssuer(assigner)
	if err != nil {
		return nodeopsapp.DocumentNumberingDeps{}, fmt.Errorf("parcel-api: delivery scope document numbers: %w", err)
	}
	catalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		return nodeopsapp.DocumentNumberingDeps{}, fmt.Errorf("parcel-api: network catalog for node business time zones: %w", err)
	}
	nodes, err := nonetwork.NewNodeBusinessTimeZones(catalog)
	if err != nil {
		return nodeopsapp.DocumentNumberingDeps{}, fmt.Errorf("parcel-api: node business time zones: %w", err)
	}
	return nodeopsapp.DocumentNumberingDeps{Issuer: issuer, Nodes: nodes}, nil
}
