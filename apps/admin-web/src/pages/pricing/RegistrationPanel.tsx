import { useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Textarea } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { registrationOutcomeLabels, type RegistrationResponseBody } from './api';
import { problemNote } from './presentation';

/**
 * 登记写面的表单区（ADR-0085，票 admin-write-faces/01 切片 01b）。
 *
 * **三态如实呈现**是本区存在的理由，不是顺带：
 *
 *   - 403 `ACCESS_CHANNEL_NOT_CONFIGURED` —— 接入渠道未配置。这是今天必然的答复：登记
 *     端点已在装配表里，但挂的是字面量 `UnconfiguredIntake{}`，写准入与其余命令面同等
 *     `PAR-INT-01` 证据（决定一）。它不是接线缺陷，改请求或重试都不会好。
 *   - 登记册治理答案 —— 已入册 / 已在册 / 内容冲突 / 规范化不可比 / 不受理。后两格是
 *     **答案不是失败**：原行不被顶替，续办属治理裁决。折成一句「提交失败」会让操作者
 *     以为重试有用。
 *   - 未决 —— 依赖故障，登记与否未知，可重试。
 *
 * 表单收的是登记快照 JSON 本体，与受控登记 CLI 吃的同一份形状。**不逐字段建表单**：
 * 「渠道原始载荷 → 登记快照」的翻译按决定三属渠道接入契约、随 `PAR-INT-01` 提供，
 * 现在拆成字段就是替租户拟那份契约。
 */
export interface RegistrationPanelProps {
  moduleId: string;
  title: string;
  endpoint: string;
  /** 快照形状的一句话提示，取各自受控登记 CLI 的文档口径。 */
  snapshotHint: string;
  submit: (snapshot: unknown) => Promise<ApiResult<RegistrationResponseBody>>;
}

type PanelState =
  | { kind: 'idle' }
  | { kind: 'submitting' }
  | { kind: 'malformed'; message: string }
  | { kind: 'answered'; answer: ApiResult<RegistrationResponseBody> };

export function RegistrationPanel({
  moduleId,
  title,
  endpoint,
  snapshotHint,
  submit,
}: RegistrationPanelProps) {
  const info = moduleInfoById[moduleId];
  const [draft, setDraft] = useState('');
  const [state, setState] = useState<PanelState>({ kind: 'idle' });

  const send = () => {
    let snapshot: unknown;
    try {
      snapshot = JSON.parse(draft);
    } catch (error) {
      // 本地就能判的畸形不送上去：送一份解析不了的载荷，回来的 400 会与服务端的业务
      // 拒绝挤在同一格里，而两者的续办动作不同。
      setState({
        kind: 'malformed',
        message: error instanceof Error ? error.message : String(error),
      });
      return;
    }
    setState({ kind: 'submitting' });
    void submit(snapshot).then((answer) => setState({ kind: 'answered', answer }));
  };

  return (
    <div className="flex-1 overflow-auto p-4">
      <Card>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-xs text-idpxyz-textMuted">
            {snapshotHint}
            <br />
            提交打到 <span className="font-mono">{endpoint}</span>；在线登记口与受控登记 CLI
            消费同一登记用例，答案代数一致。
          </p>
          <Textarea
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="粘贴登记快照 JSON"
            rows={12}
            className="font-mono text-xs"
          />
          <div className="flex items-center gap-3">
            <Button onClick={send} disabled={state.kind === 'submitting' || draft.trim() === ''}>
              {state.kind === 'submitting' ? '提交中…' : '提交登记'}
            </Button>
            <AnswerNote state={state} owner={info.owner} />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function AnswerNote({ state, owner }: { state: PanelState; owner: string }) {
  if (state.kind === 'idle' || state.kind === 'submitting') return null;
  if (state.kind === 'malformed') {
    return <p className="text-xs text-idpxyz-danger">不是合法 JSON，未提交：{state.message}</p>;
  }

  const answer = state.answer;
  switch (answer.kind) {
    case 'outcome': {
      const outcome = answer.body.outcome;
      const label = registrationOutcomeLabels[outcome] ?? outcome;
      // 未收录的 outcome 原样示出：服务端新增一格时，页面宁可显示英文原名，也不把它
      // 归进某个既有中文说法——那会让一种新答案冒充另一种。
      return (
        <p className="text-xs text-idpxyz-textMuted">
          登记册答复：<span className="font-mono">{outcome}</span> —— {label}
        </p>
      );
    }
    case 'unconfigured':
      return (
        <p className="text-xs text-idpxyz-textMuted">
          接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。登记端点已建立并装配，但
          {owner}的接入渠道认证方式尚未登记，服务端按 ADR-0055 如实拒绝——这是诚实答案，
          改请求或重试都不会改变结果；登记参数（PAR-INT-01，实例半边）到位后由装配侧换上
          真 Intake 即放行。此前登记仍走受控登记 CLI。
        </p>
      );
    case 'callerProblem':
      return (
        <p className="text-xs text-idpxyz-danger">
          调用方式问题（HTTP {answer.status}）：{problemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-xs text-idpxyz-danger">
          服务端未形成答案（HTTP {answer.status}）：{problemNote(answer.code)}；登记与否未知，
          可稍后重试。
        </p>
      );
    case 'transport':
      return (
        <p className="text-xs text-idpxyz-danger">请求未到达 parcel-api：{answer.message}</p>
      );
  }
}
