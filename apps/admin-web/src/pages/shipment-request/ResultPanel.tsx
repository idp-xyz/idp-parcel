import type { ReactNode } from 'react';
import { UnconfiguredState } from '../../components/states';
import type { ApiResult } from './api';
import { problemNote } from './presentation';
import { NoticeCard } from './controls';

// ApiResult 各格的统一呈现(格的划分见 api.ts)。业务答案(2xx)长什么样由页面
// 各自渲染——提交与撤回的封闭词汇表不同,合并渲染就得开「其他」格,那正是
// ADR-0022 不允许的口子;答案之外的各格在两页语义相同,集中在这里。
export function ResultPanel<Body>({
  result,
  pending,
  renderOutcome,
}: {
  result: ApiResult<Body> | null;
  pending: boolean;
  renderOutcome: (body: Body, status: number) => ReactNode;
}) {
  if (pending) {
    return (
      <div className="rounded border border-idpxyz-border p-4 text-[13px] text-idpxyz-textMuted">
        等待服务端答复…
      </div>
    );
  }
  if (!result) return null;

  switch (result.kind) {
    case 'outcome':
      return <>{renderOutcome(result.body, result.status)}</>;

    case 'unconfigured':
      // 403 + ACCESS_CHANNEL_NOT_CONFIGURED:改请求或重试都不会好,要去配置接入
      // 渠道。组件契约归 components/states,这里只按名使用。
      return <UnconfiguredState />;

    case 'callerProblem':
      // 调用方的错(4xx):答案没形成,但重发同样内容不会改变结果——交给人改,
      // 呈现为可改正的陈述,不是系统故障。
      return (
        <NoticeCard
          title="请求未被受理"
          meta={`HTTP ${result.status} · ${result.code}`}
        >
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">
            {problemNote(result.code)}
          </p>
        </NoticeCard>
      );

    case 'noAnswer':
      return (
        <NoticeCard
          title="服务端未形成答案"
          meta={`HTTP ${result.status} · ${result.code}`}
        >
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">
            {problemNote(result.code)}
            本次调用没有形成任何业务决定,重试是安全的,不会造成重复委托。
          </p>
        </NoticeCard>
      );

    case 'transport':
      return (
        <NoticeCard title="未连通服务端" meta={result.message}>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">
            请求未到达 parcel-api。请确认装配侧已注入 API 前缀
            (configureShipmentRequestApi,约定走 /api 经代理转发)且代理与服务端在运行。
          </p>
        </NoticeCard>
      );
  }
}
