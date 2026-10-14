# 12 签名 webhook 入向：只能推送的外部源经签名校验后以集成客户端身份进入

Category: enhancement
Status: in-progress——2026-10-14 12:2x 通道 4 认领（派单 task-8669890e ← 通道 1），隔离 worktree `idp-parcel-mcp4-oc12`、分支 `mcp4-oc12`，基 `210eafac`。派单写的基是 `b3c8bd46`：10-13 22:06 自它建树后会话中断、一笔未落；续做时 `ls-remote` 远端 main 已至 `210eafac`（其间进 main 的有票 18、19，19 改过本票要接的铸造器），照派单「变了就自新 tip 起」快进。此前 ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）
Blocked by: 11
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：入向连接器的签名校验件（平台件或 `internal/accessidentity`，本票定落点）；各上下文入向连接器接它的那一处归 psb/09、10、12 的连接器项。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定三第三、四条。

## 做什么

1. 签名校验：HMAC 共享密钥与对方公钥两种形态；时间戳与重放窗口；密钥只存引用（租户取值）。
2. 校验通过后以该推送源绑定的集成客户端身份铸信封；签名不过、时间戳越窗、重放各答其格。

## 完成判据

- 两种签名形态各有用例；同一推送重放被识别；密钥轮换期新旧并存可配置。
