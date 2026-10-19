package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// buildDepartureGatePreview 装配 `/transport-fulfillment/departure-gate-previews` 背后的问询编排（ADR-0219；票 demo-chain-gaps/69）。
// 读数编排取 buildDepartureGateReader，与出发登记同一处构造：同参出发与问询读数同一套，装配上不留第二种接法。
//
// 不包事务：本上下文这一侧什么都不写，读在池上；关务的判断在它自己那层壳的事务里记下（ADR-0206 决定六），嵌进调用方事务
// 即拒——包一层事务，每一次问询里的每一件都会读成关务读不动。
func buildDepartureGatePreview(db *bentopg.DB) (tfhttp.DepartureGatePreviewer, error) {
	reader, err := buildDepartureGateReader(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: departure gate preview: %w", err)
	}
	preview, err := tfapp.NewPreviewDepartureGateHandler(reader)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: departure gate preview: %w", err)
	}
	return preview, nil
}
