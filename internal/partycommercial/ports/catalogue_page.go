package ports

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// CataloguePage 是统一结果类型 cataloguepage.Result 的别名（ADR-0144 决定七「一处定义」：类型只在共用件里定义一次）。调用点
// 暂不改写：旧名有几处在 cmd/parcel-api/unwired_orchestration.go 里，紧挨着同批别的迁移票改过的占位方法，现在改写会让分批进 main
// 撞出合并冲突。删除条件：catalogue-read-pagination-rollout 各票都进 main 之后，一笔改写调用点、删掉别名。
type CataloguePage[Row any] = cataloguepage.Result[Row]
