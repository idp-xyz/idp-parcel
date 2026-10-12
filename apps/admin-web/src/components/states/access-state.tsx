import type { ReactNode } from 'react';
import { KeyRound, PlugZap, ShieldX } from 'lucide-react';
import { ReloginButton } from '../../auth/ReloginButton';
import {
  accessRefusalCode,
  accessRefusalTitle,
  operatorRefusalRecovery,
  operatorRefusalSentence,
  type OperatorRefusalKind,
} from '../../auth/access-refusal';

// 接入那一格的呈现（票 operator-channel/07）。原先只有一个「未配置」态，操作者渠道接上之后分成三态：渠道未配置、需要登录、
// 没有授予（判据与续办在 auth/access-refusal.ts）。三态共用一张卡的形——图标、标题、说明、结构化事实、错误码原名——
// 差的是图标、缺省字与出路：只有「需要登录」带一个按钮，另两态重试与重新登录都不会好，不给假出口。
//
// 自成一个文件而不并进 index.tsx，理由同 section-error.tsx：index.tsx 引 @idpxyz/ui-primitives，CJS 测试产物 require
// 不到；放在这里，node:test 才能用 renderToStaticMarkup 钉三态各自的字与按钮。index.tsx 照旧按名导出。

/**
 * 接入态的结构化事实。三个字段都是「读的人拿去行动」的信息，不是装饰：
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

const factLabels: ReadonlyArray<[keyof UnconfiguredFacts, string]> = [
  ['owner', '主责上下文'],
  ['source', '场景出处'],
  ['unlock', '放行条件'],
];

function AccessStateCard({
  icon,
  title,
  description,
  facts,
  code,
  action,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  facts?: UnconfiguredFacts;
  code: string;
  action?: ReactNode;
}) {
  const factRows = factLabels
    .map(([key, label]) => [label, facts?.[key]] as const)
    .filter((entry): entry is readonly [string, string] => Boolean(entry[1]));

  return (
    <div className="text-center max-w-[460px] px-6">
      {icon}
      <p className="text-[15px] text-idpxyz-textBright mb-1">{title}</p>
      <p className="text-[12px] leading-5 text-idpxyz-textMuted">{description}</p>
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
      {action ? <div className="mt-4 flex justify-center">{action}</div> : null}
      <p className="mt-2 text-[11px] font-mono text-idpxyz-textMuted">{code}</p>
    </div>
  );
}

const iconClass = 'h-8 w-8 mx-auto mb-3 text-idpxyz-textMuted';

/**
 * 未配置态：403 + ACCESS_CHANNEL_NOT_CONFIGURED 的专属呈现。
 * 这不是故障——这一口没接上渠道或部署没设发行方参数，改请求或重试都不会改变结果，
 * 出路是去完成渠道配置。
 */
export function UnconfiguredState({ title, description, facts }: UnconfiguredStateProps) {
  return (
    <AccessStateCard
      icon={<PlugZap className={iconClass} aria-hidden />}
      title={title ?? accessRefusalTitle('unconfigured')}
      description={
        description ??
        '该能力的接入渠道尚未配置（实例参数未就绪），这不是故障：重试不会改变结果，需要先在参数登记册完成渠道配置。'
      }
      facts={facts}
      code={accessRefusalCode('unconfigured')}
    />
  );
}

export interface OperatorRefusalStateProps extends UnconfiguredStateProps {
  kind: OperatorRefusalKind;
}

/**
 * 需要登录（401）与没有授予（403）的页级呈现。缺省字取 operatorRefusalRecovery，与行内那一句同源；页面只需给主责上下文与
 * 出处——找谁登记授予、哪一口拒的，随页而异；怎么续办不随页面变。
 */
export function OperatorRefusalState({ kind, title, description, facts }: OperatorRefusalStateProps) {
  return (
    <AccessStateCard
      icon={
        kind === 'loginRequired' ? (
          <KeyRound className={iconClass} aria-hidden />
        ) : (
          <ShieldX className={iconClass} aria-hidden />
        )
      }
      title={title ?? accessRefusalTitle(kind)}
      description={description ?? operatorRefusalRecovery[kind]}
      facts={facts}
      code={accessRefusalCode(kind)}
      action={kind === 'loginRequired' ? <ReloginButton /> : undefined}
    />
  );
}

/**
 * 行内一句（动作答案区、表单里一段候选、抽屉里一段区）：「{what}：需要登录（401 …）。续办。」，需要登录时句尾带重新登录。
 * `className` 跟着调用处原来那一段的字色走——各页的未配置句今天各用各的色，这两格与它们并排摆。
 */
export function OperatorRefusalNote({
  kind,
  what,
  className,
}: {
  kind: OperatorRefusalKind;
  /** 哪一个动作或哪一段被拒，如「预览」「存草稿」；缺省就不带前缀。 */
  what?: string;
  className?: string;
}) {
  return (
    <p className={className ?? 'text-xs text-idpxyz-textMuted'}>
      {operatorRefusalSentence(kind, what)}
      {kind === 'loginRequired' ? <ReloginButton size="inline" /> : null}
    </p>
  );
}
