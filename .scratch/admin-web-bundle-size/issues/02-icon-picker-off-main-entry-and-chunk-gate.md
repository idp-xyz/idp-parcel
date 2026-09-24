# 02 上游把 IconPicker 移出 ui-primitives 主入口，本仓换装新包并把超线改为构建失败

Category: enhancement
Status: ready-for-human——上游 `idp-ui` 半边归用户（或另开 `idp-ui` 会话）；本仓半边等上游发版后转 ready-for-agent
Blocked by: 01（门禁那半要求所有块都在线内）；另阻于本仓外：带本票上游改动的 `@idpxyz/ui-primitives` 新版本发布
地盘：本仓半边为 `apps/admin-web/vendor/idpxyz-ui/`、`package.json` 的 `pnpm.overrides`、`pnpm-lock.yaml`、`vite.config.ts`、`README.md`「已知跟进」。
出处：[spec](../spec.md)「现状」的 `vendor` 病根与「原型实测」的 A 格。

## 上游半边（`idpxyz/idp-ui` 仓，不在本仓做）

把 `IconPicker` 连同只为它存在的 `renderIconValue`、`isFlag`、`getFlagCode`、`FLAG_PREFIX` 挪到子路径入口（如 `@idpxyz/ui-primitives/icon-picker`），主入口不再 `import * as` 引 `lucide-react` 与 `country-flag-icons`。

选子路径入口，不选「把顶层计算改成用到时才算」：后者要求往后每个改主入口的人都不在顶层碰那两个命名空间，前者由入口边界保证，谁也漏不进来。spec 实测的 A 格模拟的正是子路径落地后 admin-web 看到的主入口。代价：其它消费方若在用 `IconPicker`，导入路径要跟着改。

## 本仓半边（上游发版后）

1. 按 `apps/admin-web/README.md` 的换版三件：替换 tarball、改 overrides 那几行、不带 `--frozen-lockfile` 重装一次让 lockfile 跟上。
2. 超线改为构建失败：`vite build` 出现超过告警线的块即退非 0，本地与 CI 同一口径，限值只在 `vite.config.ts` 一处定义。与第 1 条同一笔落——门禁只能在它第一次能通过的那一刻打开。
3. `README.md`「已知跟进」删去产物体积一条；`vite.config.ts` 注释写明这道门。

## 不做

- 不在本仓用 `pnpm patch` 给 0.1.25 打补丁过渡，理由见 spec「不做」。

## 完成判据

- `vite build` 无超线告警；`vendor` 块里不再有 `country-flag-icons`，`lucide-react` 只剩具名导入的图标。
- 门禁反证：临时把限值调到某块之下，`vite build` 退非 0；恢复后退 0。
- 三道门退 0；CI `admin-web` job 绿（它跑的就是 `vite build`，门禁随之进 CI）。

## Comments
