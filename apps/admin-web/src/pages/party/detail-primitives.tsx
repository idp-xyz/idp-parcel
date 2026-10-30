import type { ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { domainStatusTones, type StatusTone as DomainStatusTone } from '../../domain/status';
import { statusToneSurface } from '../../components/status-tone';
import { CodeRef } from '../../components/CodeRef';
import { StatusPill } from '../../components/StatusPill';
import { toneFor, toneForLabel, type StatusTone } from '../../vocabulary/status';
import { formatInstant } from '../catalogue-view';
import type { ListColumn } from '../../templates';
import { labelOf, partyNameUnknownNote } from './presentation';

// 参与方模块册页共用的呈现件（票 admin-web-group-legal-entities/09 第 2、5 条）。它们先在票 01 的法人页里长出来，
// 参与方页两签要同一份形状——两页各写一份会在改时刻写法或复制反馈时分叉，所以抬到这里，两页引同一份。
// 只放呈现层的小件：不放取数、不放业务判读。

/**
 * 状态格：词仍取本模块词表（身份与关系两册状态代数不同，各传各的），色调按码查用语词典（vocabulary/status.ts）——
 * 改版第一步先把色调一处定义，词由本模块接词典的那张票换掉；这里是过渡桥，不是第二套词表。
 */
export function statusBadge(table: Record<string, string>, code: string): ReactNode {
  return <StatusPill label={labelOf(table, code)} tone={toneFor(code)} />;
}

// domain/status 的五档色调落到用语词典的五档：行模型只交词、词还是 CONTEXT 原词时，先按旧表取色，不让这些格在接词典之前
// 全变成灰。词典里有的词以词典为准。
const legacyToneByDomainTone: Record<DomainStatusTone, StatusTone> = {
  neutral: 'muted',
  info: 'info',
  warning: 'warn',
  critical: 'bad',
  positive: 'good',
};

/**
 * 同上，给已经译成词的格用：行模型（policy-rows、supplier-agreement-rows）为了能在 node:test 里跑，只交字符串、不碰组件库，
 * 徽章由页面在渲染那一层套上。
 */
export function statusWordBadge(word: string): ReactNode {
  const fromDictionary = toneForLabel(word);
  const legacy = (domainStatusTones as Record<string, DomainStatusTone>)[word];
  const tone = fromDictionary !== 'muted' || legacy === undefined ? fromDictionary : legacyToneByDomainTone[legacy];
  return <StatusPill label={word} tone={tone} />;
}

/** 行模型列表里 `status` 那一列换成徽章，其余列原样。 */
export function withStatusBadge<Row extends { values: Record<string, string> }>(columns: ListColumn<Row>[]): ListColumn<Row>[] {
  return columns.map((column) =>
    column.id === 'status' ? { ...column, render: (row: Row) => statusWordBadge(row.values.status ?? '—') } : column,
  );
}

/** 名称转写不到（悬空引用）时那一格的话，弱化显示；句子只在 presentation.ts 一处。 */
export function UnknownPartyName() {
  return <span className="text-idpxyz-breadcrumb">{partyNameUnknownNote}</span>;
}

/**
 * 时刻格：显本地墙钟带显式偏移（moment.ts 按装配点配置的区算），原 ISO 串放进 dateTime 与 title——
 * 跨 CN/SG 两地对同一事件时悬停给的是线格式那一份，两边比对不必各自换算（票 01 裁决 3）。
 */
export function Instant({ value }: { value: string }) {
  return (
    <time dateTime={value} title={value}>
      {formatInstant(value)}
    </time>
  );
}

/**
 * 有效区间格：两端各是一个 Instant，悬停各给原串；无终点时说「持续有效」——那是 CONTEXT 的口径（有效期间未
 * 声明终点即持续有效），不是「未知」。与 moment.ts 的 formatRange 同一种写法，只是把两端换成可悬停的格。
 */
export function InstantRange({ from, to }: { from: string; to?: string }) {
  return (
    <span>
      <Instant value={from} /> → {to ? <Instant value={to} /> : '持续有效'}
    </span>
  );
}

// 过滤条里的下拉比表单里的格矮一号（py-1 / 12px），与同栏的搜索框齐高；表单那份 selectClass 不借来用。
export const filterSelectClass =
  'rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1 text-[12px] ' +
  'text-idpxyz-text focus:outline-none focus:border-idpxyz-accent';

/** 复制到剪贴板，成败都以 toast 反馈；不支持 clipboard 的上下文（非 https）如实说，不静默。 */
export function useCopyToClipboard() {
  const { addToast } = useToast();
  return (label: string, value: string) => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) {
      addToast({ type: 'warning', title: '无法复制', message: '当前页面上下文不支持剪贴板（需 https 或 localhost）。' });
      return;
    }
    void navigator.clipboard.writeText(value).then(
      () => addToast({ type: 'success', title: '已复制', message: `${label}：${value}` }),
      (error: unknown) =>
        addToast({
          type: 'error',
          title: '复制失败',
          message: error instanceof Error ? error.message : String(error),
        }),
    );
  };
}

/** 详情栏头部的引用签：点一下复制，与参照页同一手势。没有值就不摆。件本体是全站共用的 CodeRef，这里只是带 toast 反馈的包装。 */
export function RefChip({
  label,
  value,
  title,
  onCopy,
}: {
  label: string;
  value?: string;
  title: string;
  onCopy: (label: string, value: string) => void;
}) {
  if (!value) return null;
  return <CodeRef label={label} value={value} title={title} onCopy={onCopy} />;
}

/** 详情栏事实条里的一格：标签在上、等宽值在下。 */
export function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2">
      <p className="text-[12px] text-idpxyz-breadcrumb">{label}</p>
      <p className="mt-0.5 break-words font-mono text-[13px] font-medium leading-5 text-idpxyz-text [&_time]:text-[12px] [&_time]:font-normal">{children}</p>
    </div>
  );
}

/**
 * 事实条按栏宽在四列与两列之间二选一（票 group-legal-entities-uiux/10）：固定四列在窄栏里把「08:00:00」这样一个词拆成
 * 两半；按最小格宽自动分列又会在中间宽度排出三格加一格。minmax 的下限用 clamp 夹：栏宽过 32rem 时中间项为负、取下界
 * 25%，排四列；不到时中间项极大、取上界 50%，排两列——四格放得下每个词时才排四列。
 */
export function FactStrip({ children }: { children: ReactNode }) {
  return (
    <div className="grid grid-cols-[repeat(auto-fit,minmax(clamp(calc(25%_-_0.625rem),calc((32rem_-_100%)_*_999),calc(50%_-_0.625rem)),1fr))] gap-2.5 border-b border-idpxyz-border bg-idpxyz-editor px-5 py-3">
      {children}
    </div>
  );
}

/**
 * 「在用中已生效」的占比条（身份册工作台的指标位）。不用上游 Progress：它不转发 aria-*，进度条起不了名字
 * （axe aria-progressbar-name），轨道的 `bg-idpxyz-border/50` 在本仓也生不出来（主题色是裸 CSS 变量，见
 * components/primary-action.ts）。名字由各册给，说清分子分母是哪一册的。
 */
export function EffectiveShareBar({ share, label }: { share: number; label: string }) {
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuenow={share}
      aria-valuemin={0}
      aria-valuemax={100}
      className="h-1 w-24 overflow-hidden rounded-full bg-idpxyz-border"
    >
      <div
        className={`h-full rounded-full ${share >= 100 ? statusToneSurface.success : 'bg-idpxyz-accent'}`}
        style={{ width: `${share}%` }}
      />
    </div>
  );
}

/** 抽屉里的一格：标签 + 值，可选复制按钮。用在 `<dl>` 里。 */
export function DetailRow({
  label,
  children,
  mono = false,
  onCopy,
}: {
  label: string;
  children: ReactNode;
  mono?: boolean;
  onCopy?: () => void;
}) {
  return (
    <div className="grid grid-cols-[7rem_minmax(0,1fr)] items-start gap-x-4 border-b border-idpxyz-border py-2.5 last:border-b-0">
      <dt className="pt-0.5 text-[12px] leading-5 text-idpxyz-breadcrumb">{label}</dt>
      <dd className={`flex min-w-0 items-start justify-between gap-2 text-[13px] leading-5 text-idpxyz-text ${mono ? 'font-mono' : ''}`}>
        <span className="min-w-0 break-words">{children}</span>
        {onCopy ? (
          <Button variant="ghost" size="sm" className="shrink-0 px-1.5 text-[12px] text-idpxyz-breadcrumb" onClick={onCopy}>
            复制
          </Button>
        ) : null}
      </dd>
    </div>
  );
}
