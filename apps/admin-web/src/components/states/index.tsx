import type { ReactNode } from 'react';
import { AlertTriangle, PlugZap, RotateCw } from 'lucide-react';
import { TrulyEmptyState } from '@idpxyz/ui-primitives';

export { SectionError, type SectionErrorProps } from './section-error';

// 页面四态展示组件。命名与 props 是跨会话契约（v2 最终版）：templates/state-slot.tsx
// 与 pages/shipment-request 按名导入，改名或收紧必填都会拆到消费方。
// 全部 props 可选、内置中文兜底文案；样式只用 idpxyz 色 token。
//
// 错误按层分（手册「Error State」）：这里的 ErrorState 是**页级**——整个内容区换掉，对应「详情半份比没有更误导」
// 那条红线；**区块级**在 section-error.tsx（一段区只红那一段）；**动作级**是登记 / 发布口答案区（RegistrationPanel
// 的 answered 态）；**后台刷新**是 useRegisterList 重取时保留旧答案。后两层已在各自的件里，这里不另出件。
//
// 四态各自回答的问题不同——加载=还没答案；空=答案是「没有记录」；错误=这次没形成
// 答案（可重试）；未配置=接入渠道未配置（实例半边未就绪），改请求或重试都不会好。
// 把「未配置」与「错误」并档会诱导人反复重试一件人不来配就永远不会好的事，
// 分档依据 ADR-0022 与 ADR-0055 的「未配置即拒」格。

/** 加载态：脉冲骨架块。不承诺进度，只表示「还没答案」。 */
export function LoadingState({ label }: { label?: string }) {
  return (
    <div className="w-full max-w-[560px] px-6" role="status" aria-label={label ?? '加载中'}>
      <div className="animate-pulse space-y-3">
        <div className="h-4 w-1/3 rounded bg-idpxyz-hover" />
        <div className="h-4 w-full rounded bg-idpxyz-hover" />
        <div className="h-4 w-5/6 rounded bg-idpxyz-hover" />
        <div className="h-4 w-2/3 rounded bg-idpxyz-hover" />
      </div>
      {label ? <p className="mt-4 text-[12px] text-idpxyz-textMuted">{label}</p> : null}
    </div>
  );
}

export interface EmptyStateProps {
  title?: string;
  description?: string;
  /** 「下一步动作」位：空态不该是死胡同，能给动作就给。 */
  action?: ReactNode;
}

/**
 * 空态：答案已形成且为「没有记录」。与「未接线/未配置」是两种不同的真话，不得混用。
 *
 * 形取 ui-primitives 的 TrulyEmptyState（票 admin-web-ux-alignment/06 第 3 条），字归本仓：标题与说明**总是**传过去，原语自己的
 * 英文缺省句（无 title / description 时才出）到不了页面。`action` 不映到它的 `onCreate`——那颗按钮的字「Create First Record」写死在
 * 原语里，映过去就是把英文按钮放到页面上；本仓的动作位仍收 ReactNode，摆在卡片下方。筛空那一行（FilteredEmptyState）不在这里，
 * 票面裁决 2：它的文案不可定制、不换。
 */
export function EmptyState({ title, description, action }: EmptyStateProps) {
  return (
    <div className="flex w-full flex-col items-center">
      <TrulyEmptyState title={title ?? '暂无记录'} description={description ?? '当前条件下没有可显示的内容。'} />
      {action ? <div className="mt-4 flex justify-center">{action}</div> : null}
    </div>
  );
}

export interface ErrorStateProps {
  title?: string;
  /** problem+json 的 error.code，原样呈现便于按码检索，不翻译。 */
  code?: string;
  description?: string;
  onRetry?: () => void;
}

/**
 * 错误态：这一次没形成答案（5xx 或传输层未通）。给重试入口，错误码原样露出。
 *
 * 不包 ui-primitives 的 UnavailableState（票 admin-web-ux-alignment/06 第 3 条判「不换」）：它的标题「Data temporarily unavailable」、
 * 说明与按钮「Retry」全写死、只收 onRetry，包一层就是把英文缺省句摆到页面上，与本仓文案规则相抵；且调用方今天把
 * 传输失败与服务端未形成答案都送到这一态、只靠 title 区分，原语也没有传这句话的口。形先保持今天的，原语开放文案再换。
 */
export function ErrorState({ title, code, description, onRetry }: ErrorStateProps) {
  return (
    <div className="text-center max-w-[420px] px-6">
      <AlertTriangle className="h-8 w-8 mx-auto mb-3 text-idpxyz-textMuted" aria-hidden />
      <p className="text-[15px] text-idpxyz-textBright mb-1">{title ?? '本次未取得结果'}</p>
      <p className="text-[12px] leading-5 text-idpxyz-textMuted">
        {description ?? '服务端没有形成答案，稍后可重试；若持续失败请携错误码求助。'}
      </p>
      {code ? (
        <p className="mt-2 text-[11px] font-mono text-idpxyz-textMuted">{code}</p>
      ) : null}
      {onRetry ? (
        <button
          type="button"
          onClick={onRetry}
          className="mt-4 inline-flex items-center gap-1.5 px-3 py-1.5 text-[12px] rounded border border-idpxyz-border text-idpxyz-text hover:bg-idpxyz-hover"
        >
          <RotateCw className="h-3.5 w-3.5" aria-hidden />
          重试
        </button>
      ) : null}
    </div>
  );
}

/**
 * 未配置态的结构化事实。三个字段都是「读的人拿去行动」的信息，不是装饰：
 * 主责上下文说明找谁、场景出处说明依据哪份文档（写文档名，不写行号）、
 * 放行条件说明什么动作会解除本态。全部可选——只给 description 字符串的
 * 既有调用方不受影响。
 */
export interface UnconfiguredFacts {
  /** 主责上下文（领域语言名 + 目录名），取 navigation 的 moduleInfoById.owner 原文。 */
  owner?: string;
  /** 场景出处：权威文档名与小节说法。 */
  source?: string;
  /** 放行条件：哪个闸门或登记动作会解除本态。 */
  unlock?: string;
}

export interface UnconfiguredStateProps {
  title?: string;
  description?: string;
  facts?: UnconfiguredFacts;
}

const unconfiguredFactLabels: ReadonlyArray<[keyof UnconfiguredFacts, string]> = [
  ['owner', '主责上下文'],
  ['source', '场景出处'],
  ['unlock', '放行条件'],
];

/**
 * 未配置态：403 + ACCESS_CHANNEL_NOT_CONFIGURED 的专属呈现。
 * 这不是故障——接入渠道未配置属实例半边未就绪（等待登记册里的渠道参数），
 * 改请求或重试都不会改变结果，出路是去完成渠道配置。
 */
export function UnconfiguredState({ title, description, facts }: UnconfiguredStateProps) {
  const factRows = unconfiguredFactLabels
    .map(([key, label]) => [label, facts?.[key]] as const)
    .filter((entry): entry is readonly [string, string] => Boolean(entry[1]));

  return (
    <div className="text-center max-w-[460px] px-6">
      <PlugZap className="h-8 w-8 mx-auto mb-3 text-idpxyz-textMuted" aria-hidden />
      <p className="text-[15px] text-idpxyz-textBright mb-1">{title ?? '接入渠道未配置'}</p>
      <p className="text-[12px] leading-5 text-idpxyz-textMuted">
        {description ??
          '该能力的接入渠道尚未配置（实例参数未就绪），这不是故障：重试不会改变结果，需要先在参数登记册完成渠道配置。'}
      </p>
      {factRows.length > 0 ? (
        <dl className="mt-3 border-t border-idpxyz-border pt-2.5 text-left space-y-1.5">
          {factRows.map(([label, value]) => (
            <div key={label} className="flex gap-2">
              <dt className="shrink-0 w-[64px] text-[11px] leading-4 text-idpxyz-textMuted">
                {label}
              </dt>
              <dd className="text-[11px] leading-4 text-idpxyz-text">{value}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      <p className="mt-2 text-[11px] font-mono text-idpxyz-textMuted">
        ACCESS_CHANNEL_NOT_CONFIGURED
      </p>
    </div>
  );
}
