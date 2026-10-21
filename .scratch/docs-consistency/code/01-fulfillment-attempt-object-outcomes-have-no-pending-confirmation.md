# 01 TF 履约尝试的逐对象结果没有「待确认」一格

Category: bug
Status: ready-for-agent——R2（上线后尽快）：场外揽收与派送尝试已在生产命令面上，「只有车辆到场和司机扫描」「签收与地址来源冲突」这类来源第一周就会遇到；后果是执行方只能在四个已定结果里硬选一个，或者不登这次尝试，结果被说成确定，或这次尝试不留痕。可靠绕法：来源不足时先不登尝试，补齐证据后再登。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/tf.md](../audit/tf.md) M16（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象

TF CONTEXT 要求履约尝试逐对象保存「成功、失败、拒收、待确认或其他适用结果」，UC-TF-002 与 UC-TF-006 都有「待确认」的结果行与 AT。代码里两套逐对象结果都是封闭四格，没有待确认。揽收一侧也没有独立的「执行方拒收」，拒收混在失败一族里。

## 证据

- 文档：
  - TF CONTEXT Rules「揽收、派送与交付」：「每个对象必须分别保存成功、失败、拒收、待确认或其他适用结果，任务汇总只能由对象结果派生」。
  - UC-TF-002 结果契约「揽收结果待确认」：「已有来源但不足以证明接收、控制或对象范围」；`AT-TF-018`：「只有车辆到场和司机扫描 | 保存来源，结果待确认，不推导实际接收」；`AT-TF-017`：「包装不合格且执行方明确拒收 | 保存对象级拒收和证据，控制不转移」。
  - UC-TF-006 `AT-TF-070`：「本人签收和地址来源冲突 | 形成冲突或待确认，不按最后消息覆盖」。
- 代码：
  - `internal/transportfulfillment/domain` 的 `AttemptObjectOutcome`：`ObjectPickedUp`、`CustomerAbsent`、`GoodsNotReady`、`PackagingUnacceptable` 四格。
  - 同包的 `DeliveryObjectOutcome`：`ObjectDelivered`、`DeliveryRefused`、`NoOneToReceive`、`WrongAddress` 四格。
  - 命令面 `adapters/http/perform_offsite_pickup.go` 的 `attemptObjectOutcomeFromWord` 只认这四个词，生产装配在 `cmd/parcel-api/assemble_control_facts.go`。

## 三类

机制：结果集合加一格「待确认」（带已知事实与缺口），汇总派生把它算作未结束。产品策略：无。租户取值：无（哪些来源够得上确定结果仍由既有规则判，不在本票设默认）。

## 规模

S–M：两个领域枚举、汇总派生、命令面词表、库表 CHECK（若有）与读回，各加一格；补 `AT-TF-018`、`AT-TF-070` 的用例。

## 最小的诚实修法

1. `AttemptObjectOutcome` 与 `DeliveryObjectOutcome` 各加「待确认」：不转移控制、不形成有效交付或揽收到手，任务汇总把它读作未结束。
2. 待确认那一格必须带来源引用；以后同一对象的确定结果作为新尝试或更正形成，不改写这一格。
3. 揽收要不要另立「执行方拒收」，见下「要裁的」；不裁之前不加。

## 要裁的

- 揽收「执行方拒收」是独立一格，还是并在 `PackagingUnacceptable` 一族里（UC-TF-002 结果契约把「本次对象被拒收」列为单独一行）。归 TF owner；本票按「待确认」先做，拒收一格随裁定。
- 审计附注：owner 若认定「待确认」不落在尝试上、而是不登尝试，这条改归 B，改 CONTEXT 与两份 UC，本票作废。
