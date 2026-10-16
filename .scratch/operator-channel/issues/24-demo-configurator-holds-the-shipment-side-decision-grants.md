# 24 演示配置员补委托侧五种运营决定的授予

Category: enhancement
Status: resolved——2026-10-16 进 main（`e8673ac1`，分支 `claude-oc21` 快进，远端 main 含 `efc84d87`）；见「进 main 记录」。此前 in-progress——2026-10-16 完工，待进 main；见「完成记录」。此前 in-progress——2026-10-16 Claude Code 会话立并认领（用户「针对上面的问题，你能自决，全部解决吗」），分支 `claude-oc21`。
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施（收口后的清扫）
地盘：`scripts/demo-seeds/data/access/02-grants.json`、`seed.sh` 与 `README.md` 里授予那几句。
出处：[15](./15-operation-decision-faces-take-operator-intake.md) 完成记录「发现，不做」第三条；[ADR-0150](../../../docs/adr/0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定二。

## 为什么

委托侧五口（复核完成、主动拒绝、授权处置、受控关闭、受控重开）已换操作者渠道，演示租户的准入对照与本产品区间都在册，唯独配置员没有这五种决定的授予：管理台在这五口上答 `OPERATOR_NOT_GRANTED`。这一格答的是「你没被授权」，而真实的停点在后面——各口越过 Intake 之后如实停在自己的授权缺口上（票 15 完成判据第三条）。演示要展示的是后者：产品在哪一步、为什么停下。

## 裁定

给演示配置员补这五种授予。理由：演示租户的配置员在合成环境里扮的是运营主管，持有全部运营决定是运营企业里常见的岗位配置；授予只开门，不替任何人形成决定——决定方、授权角色与授权依据快照仍全由 party-commercial 的授权答复给出（ADR-0151 决定二），授权缺口照旧如实停。查阅员不动，仍只有查阅读，继续证「授予不覆盖即拒」。证据层级只记 `S`。

## 完成判据

- 配置员的授予覆盖委托侧五种决定；一次性库上 `seed.sh` 整跑退 0，五种授予各一行查得到。
- README 授予表与 `seed.sh` 注释同步。

## 完成记录（2026-10-16，Claude Code 会话，分支 `claude-oc21`，提交 `e8673ac1`）

- **落点**：`data/access/02-grants.json` 加五条（依据 `SYN-GRANT-BASIS-DEX-10`–`14`）；`seed.sh` 授予与对照两段注释、README 授予表同步。
- **判据**：✅ 一次性库 `oc24_seed_check` 上 `seed.sh` 整跑退 0，五种授予各一行查得到，库已删；✅ README 与注释同步。
- **评审**：纯种子与文档，作者自审。证据层级 `S`。

### 进 main 记录（2026-10-16）

- 分支 `claude-oc21` 五笔 `7ec601ab` / `9c551dc4` / `e8673ac1` / `c7a462cd` / `efc84d87` 叠在远端 main `f33d849d` 之上，快进进 main、SHA 未改写；`efc84d87` 是远端 main 的祖先（其后另一会话叠了 `43309fa5`，只动 `.scratch/group-legal-entities-uiux/`，与本票无交集）。
- 推送方全量一次：钉 `efc84d87` 的全新 detached 检出，带 DSN，`go test -p 1 -count=1 -v ./...` 退 0——有测试的包 141 个 ok、0 FAIL，`--- SKIP` 1（`TestHelperTemplateOwnerProcess`，照设计跳过）；同一检出上全仓 `gofmt -l .` 无输出，`go build ./...`、`go vet ./...` 退 0。
- 评审：只有作者自审，没有非作者评审（本机无别的会话可派；仓规 2026-09-16 不用子代理）。
