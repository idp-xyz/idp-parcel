import type { ReactNode } from 'react';
import { Check, ChevronLeft, ChevronRight, X } from 'lucide-react';
import {
  Button,
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
} from '@idpxyz/ui-primitives';
import { chipClass } from '../../components/registration';
import { statusToneBanner, statusToneSurface, statusToneText } from '../../components/status-tone';
import { useCloseOnHashChange } from './use-close-on-hash';

export type WmsTone = 'success' | 'info' | 'warning' | 'danger' | 'neutral';

// 徽章的颜色取主题运行时按当前主题写好的语义状态色，写法同 StatusPill：淡底、语气描边、正文色的字，语气色只落在圆点上。
// 不用 Tailwind 色板加 `dark:`——`dark:` 跟的是系统偏好而不是应用主题（见 components/status-tone.ts 文件头），应用浅色而
// 系统深色时字会变成 300 色阶落在浅底上，读不出来。
const toneClasses: Record<WmsTone, { pill: string; dot: string }> = {
  success: { pill: statusToneBanner.success, dot: statusToneSurface.success },
  info: { pill: statusToneBanner.info, dot: statusToneSurface.info },
  warning: { pill: statusToneBanner.warning, dot: statusToneSurface.warning },
  danger: { pill: statusToneBanner.danger, dot: statusToneSurface.danger },
  neutral: {
    pill: 'bg-idpxyz-hover text-idpxyz-textMuted border-idpxyz-border',
    dot: 'bg-idpxyz-textMuted',
  },
};

export interface StatusMeta {
  label: string;
  tone: WmsTone;
}

/** 仓配全链路业务状态与枚举词的统一中文映射表，页面不裸露后端英文状态字面量。 */
export const wmsStatusMap: Record<string, StatusMeta> = {
  // 通用与主数据
  ACTIVE: { label: '在用', tone: 'success' },
  DEACTIVATED: { label: '已停用', tone: 'neutral' },
  CONFIGURED: { label: '已配置', tone: 'success' },
  UNCONFIGURED: { label: '未配置', tone: 'warning' },
  // 库存状态
  GOOD: { label: '良品', tone: 'success' },
  PENDING_INSPECTION: { label: '待检', tone: 'warning' },
  QUARANTINE: { label: '待检', tone: 'warning' },
  DEFECTIVE: { label: '次品', tone: 'danger' },
  // 入库预报
  ADVISED: { label: '已预报', tone: 'info' },
  RECEIVING: { label: '收货中', tone: 'warning' },
  CLOSED: { label: '已完结', tone: 'success' },
  CANCELLED: { label: '已取消', tone: 'neutral' },
  // 待认领
  UNCLAIMED: { label: '待认领', tone: 'warning' },
  CLAIMED: { label: '已认领', tone: 'info' },
  DISPOSED: { label: '已处置', tone: 'success' },
  // 建账、盘点、调整、差异、指令
  PENDING: { label: '待处理', tone: 'warning' },
  COUNTING: { label: '盘点中', tone: 'info' },
  COUNTED: { label: '已盘点', tone: 'success' },
  POSTED: { label: '已入账', tone: 'success' },
  APPROVED: { label: '已批准', tone: 'success' },
  REJECTED: { label: '已驳回', tone: 'danger' },
  DONE: { label: '已完成', tone: 'success' },
  VOID: { label: '已作废', tone: 'neutral' },
  FROZEN: { label: '冻结中', tone: 'danger' },
  UNFROZEN: { label: '已解冻', tone: 'neutral' },
  // 出库单与波次
  RESERVED: { label: '已预占', tone: 'info' },
  PARTIALLY_RESERVED: { label: '部分预占', tone: 'warning' },
  PENDING_POLICY: { label: '待定缺货策略', tone: 'warning' },
  ALLOCATED: { label: '已放波次', tone: 'info' },
  PICKED: { label: '已拣货', tone: 'info' },
  CHECKED: { label: '已复核', tone: 'info' },
  PACKED: { label: '已打包', tone: 'info' },
  SHIPPED: { label: '已出库', tone: 'success' },
  SPLIT: { label: '已拆单', tone: 'neutral' },
  AWAITING_RETURN: { label: '待回库', tone: 'warning' },
  PARTIALLY_CANCELLED: { label: '部分取消', tone: 'warning' },
  HANDED_OFF: { label: '待转出代理仓', tone: 'warning' },
  FORWARDED: { label: '已转出代理仓', tone: 'info' },
  HANDED_OVER: { label: '代理仓已交接', tone: 'success' },
  // 波次自己的状态：已完成与已取消用通用的那两个词
  RELEASED: { label: '已放出', tone: 'warning' },
  PICKING: { label: '拣货中', tone: 'info' },
  OPEN: { label: '待执行', tone: 'warning' },
  SHORT: { label: '短拣', tone: 'danger' },
  LATE: { label: '迟到取消', tone: 'neutral' },
  // 退货：预报没有状态字段，待到仓与已到仓是按到仓记录对出来的；到仓记录的待处置、转待认领是按到仓结果与处置判的
  AWAITING_ARRIVAL: { label: '待到仓', tone: 'warning' },
  ARRIVED: { label: '已到仓', tone: 'info' },
  AWAITING_DISPOSITION: { label: '待处置', tone: 'warning' },
  DIVERTED: { label: '转待认领', tone: 'neutral' },
  // 退货与代理声明
  PUTAWAY: { label: '良品上架', tone: 'success' },
  GOOD_PUTAWAY: { label: '良品上架', tone: 'success' },
  DESTROY: { label: '销毁', tone: 'danger' },
  RETURN_TO_OWNER: { label: '退还货主', tone: 'info' },
  RETURN_TO_SENDER: { label: '退回寄件人', tone: 'info' },
  DONATE: { label: '捐出', tone: 'warning' },
  VALID: { label: '有效', tone: 'success' },
  UNKNOWN: { label: '已过期', tone: 'warning' },
  SUPERSEDED: { label: '已被取代', tone: 'neutral' },
  // 确认与计费
  CONFIRMED: { label: '已确认', tone: 'success' },
  RECORDED: { label: '离线已记', tone: 'warning' },
  ORIGINAL: { label: '原始', tone: 'info' },
  RESTATE: { label: '重述更正', tone: 'warning' },
  COMPLETED: { label: '已完成', tone: 'success' },
};

export const wmsWordLabels: Record<string, string> = {
  // 到仓与入库形态
  FULL_CONTAINER: '整柜',
  PALLET: '托盘',
  LOOSE_CARTON: '散箱',
  BY_GOODS: '按商品',
  BY_CARTON: '按整箱',
  // 发货途径
  PRODUCT_NETWORK: '本产品网络',
  OWNER_LABEL: '货主面单',
  PICKUP: '到仓自提',
  // 调整种类
  INCREASE: '盘盈调增',
  DECREASE: '盘亏调减',
  DESTRUCTION: '库存销毁',
  // 加工与换标
  PROCESS: '组合加工',
  DISASSEMBLE: '组合拆解',
  RELABEL: '商品换标',
  // 库内作业
  INTERNAL_MOVE: '上架 / 移库',
  FORM_CONVERSION: '包装形态转换',
  CONDITION_CONVERSION: '库存状态转换',
  SHORTAGE: '账实短少上报',
  // 收费类别
  INBOUND_HANDLING: '入库操作费',
  OUTBOUND_HANDLING: '出库操作费',
  STORAGE: '仓租日结',
  RETURN_HANDLING: '退货操作费',
  VALUE_ADDED: '增值服务费',
  PROCESSING: '加工费',
  // 来源
  OPERATOR: '运营代录',
  CUSTOMER: '货主自助',
  ON_BEHALF: '运营代录',
  BY_OWNER: '货主自助',
  OWNER: '货主自助',
  // 收货结果
  DIVERTED_TO_UNCLAIMED: '转成待认领',
  WORK_FACT: '一线扫码登记',
  DIVERTED_RECEIPT: '收货异常转入',
  INBOUND: '入库到货',
  RETURN: '退件',
  GOODS: '按商品格',
  CARTON: '按整箱',
  PARTIAL: '允许部分预占',
  REJECT: '库存不足整单拒单',
  ACTUAL_BATCH: '按实批库龄',
  THEORETICAL_FIFO: '按理论先进先出',
  OUTBOUND_CONFIRMATION: '正常出库确认',
  PICKUP_HANDOVER: '自提现场交付',
  BARCODE: '商品条码',
  PLATFORM_CODE: '平台商品码',
  PARCEL: '包裹',
  LOOSE: '散货',
  PUTAWAY: '良品上架',
  SELF_OPERATED: '自营仓',
  AGENT: '代理仓',
  // 组合商品的出库方式
  BY_COMPONENT: '按组件出库',
  AS_FINISHED_GOODS: '按成品出库',
};

export function labelOf(code: string | undefined, fallback = '—'): string {
  if (!code) return fallback;
  return wmsWordLabels[code] ?? wmsStatusMap[code]?.label ?? code;
}

/** 语义状态徽章（圆点 + 中文词）。 */
export function WmsBadge({
  code,
  label,
  tone,
}: {
  code?: string;
  label?: string;
  tone?: WmsTone;
}) {
  const meta = code ? wmsStatusMap[code] : undefined;
  const resolvedTone = tone ?? meta?.tone ?? 'neutral';
  const resolvedLabel = label ?? meta?.label ?? code ?? '—';
  const style = toneClasses[resolvedTone];
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 type-caption font-medium whitespace-nowrap ${style.pill}`}
    >
      <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${style.dot}`} />
      {resolvedLabel}
    </span>
  );
}

export interface StepItem {
  id: string;
  label: string;
}

/** 单据生命周期步进条：清晰指示当前单据走到哪一步、是否已取消或异常中止。 */
export function WmsStepper({
  steps,
  currentId,
  currentLabel,
  failedLabel,
  failedTone = 'danger',
}: {
  steps: readonly StepItem[];
  currentId: string;
  /** 卡在当前这一步、还没做完时的称呼；给了就按待办色标出。 */
  currentLabel?: string;
  failedLabel?: string;
  failedTone?: 'danger' | 'neutral';
}) {
  const currentIdx = steps.findIndex((s) => s.id === currentId);
  return (
    // 不带外框也不带底：它摆在单据页头区下面那一条里，上下已经各有一道线，再套一圈框就是线叠着线。
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1.5">
      {steps.map((step, index) => {
        const done = !failedLabel && currentIdx >= 0 && index < currentIdx;
        const active = !failedLabel && index === currentIdx;
        const stuck = active && currentLabel !== undefined;
        return (
          <div key={step.id} className="flex items-center gap-2">
            <div className="flex items-center gap-1.5">
              <span
                className={`inline-flex h-5 w-5 items-center justify-center rounded-full type-caption font-semibold ${
                  done
                    ? 'bg-emerald-500 text-white'
                    : stuck
                      ? 'bg-amber-500 text-white ring-2 ring-amber-500/25'
                      : active
                        ? // 光圈用 color-mix 写：主题色是裸 CSS 变量，`/25` 这类透明度写法生不出样式。
                          'bg-idpxyz-accent text-white ring-2 ring-[color:color-mix(in_srgb,var(--idpxyz-accent)_30%,transparent)]'
                        : 'border border-idpxyz-border bg-idpxyz-sidebar text-idpxyz-textMuted'
                }`}
              >
                {done ? <Check className="h-3 w-3" /> : index + 1}
              </span>
              <span
                className={`type-caption whitespace-nowrap ${
                  stuck
                    ? 'font-semibold text-[color:var(--ds-status-warning-fg)]'
                    : active
                      ? 'font-semibold text-idpxyz-textBright'
                      : done
                        ? 'text-idpxyz-text'
                        : 'text-idpxyz-textMuted'
                }`}
              >
                {stuck ? currentLabel : step.label}
              </span>
            </div>
            {index < steps.length - 1 ? <span className="h-px w-5 bg-idpxyz-border" aria-hidden="true" /> : null}
          </div>
        );
      })}
      {failedLabel ? (
        <span
          className={`ml-auto inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 type-caption font-medium ${
            failedTone === 'neutral'
              ? 'border-idpxyz-border bg-idpxyz-hover text-idpxyz-textMuted'
              : 'border-[color:var(--ds-status-danger-border)] bg-[color:var(--ds-status-danger-bg-subtle)] text-idpxyz-text'
          }`}
        >
          {failedTone === 'neutral' ? null : <X className="h-3 w-3" />}
          {failedLabel}
        </span>
      ) : null}
    </div>
  );
}

/** 宽幅 360° 单据详情工作抽屉：固定顶栏（单号/状态/主动作按钮）、可选步进条、可滚动明细区。 */
export function WmsDetailModal({
  open,
  title,
  subtitle,
  badge,
  actions,
  stepper,
  loading,
  error,
  onClose,
  children,
}: {
  open: boolean;
  title: string;
  subtitle?: string;
  badge?: ReactNode;
  actions?: ReactNode;
  stepper?: ReactNode;
  loading?: boolean;
  error?: string | null;
  onClose: () => void;
  children: ReactNode;
}) {
  // Layout 标签保活：换 hash 不卸载本页，门户会盖到下一页——开着就听 hashchange 关。
  useCloseOnHashChange(open, onClose);
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <DialogContent size="xl" className="w-[min(96vw,1040px)] max-w-[1040px] max-h-[88vh] flex flex-col">
        <DialogHeader className="flex flex-col gap-3 border-b border-idpxyz-border pb-3">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="type-section font-bold text-idpxyz-textBright font-mono">{title}</h2>
                {badge}
              </div>
              {subtitle ? <p className="mt-0.5 type-caption text-idpxyz-textMuted">{subtitle}</p> : null}
            </div>
            {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
          </div>
          {stepper}
        </DialogHeader>

        <DialogBody className="flex-1 overflow-y-auto py-4 space-y-5">
          {loading ? (
            <p className="py-8 text-center type-body text-idpxyz-textMuted">正在加载单据完整明细…</p>
          ) : error ? (
            <div className="rounded border border-[color:var(--ds-status-danger-border)] bg-[color:var(--ds-status-danger-bg-subtle)] p-4 type-body text-idpxyz-text">
              {error}
            </div>
          ) : (
            children
          )}
        </DialogBody>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** 详情页顶部的关键属性摘要网格。 */
export function WmsInfoGrid({
  items,
}: {
  items: ReadonlyArray<{ label: string; value: ReactNode; mono?: boolean }>;
}) {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3 rounded-lg border border-idpxyz-border bg-idpxyz-sidebar p-4">
      {items.map((item) => (
        <div key={item.label} className="min-w-0">
          <div className="type-caption text-idpxyz-textMuted">{item.label}</div>
          <div className={`mt-0.5 type-body text-idpxyz-textBright break-words ${item.mono ? 'font-mono' : ''}`}>
            {item.value || '—'}
          </div>
        </div>
      ))}
    </div>
  );
}

export interface WmsSubColumn<T> {
  id: string;
  header: string;
  mono?: boolean;
  align?: 'left' | 'right';
  render: (row: T, index: number) => ReactNode;
}

/** 详情抽屉与台账内嵌的统一结构化子表。 */
export function WmsSubTable<T>({
  title,
  count,
  extra,
  columns,
  rows,
  rowKey,
  emptyText,
}: {
  title?: string;
  count?: number;
  extra?: ReactNode;
  columns: ReadonlyArray<WmsSubColumn<T>>;
  rows: readonly T[];
  rowKey: (row: T, index: number) => string;
  emptyText: string;
}) {
  return (
    <section className="space-y-2">
      {title || extra ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            {title ? <h3 className="type-body font-semibold text-idpxyz-textBright">{title}</h3> : null}
            {count !== undefined ? (
              <span className="rounded-full bg-idpxyz-hover px-2 py-0.5 font-mono type-caption text-idpxyz-textMuted">
                {count}
              </span>
            ) : null}
          </div>
          {extra}
        </div>
      ) : null}
      <div className="rounded-lg border border-idpxyz-border bg-idpxyz-sidebar overflow-x-auto">
        {rows.length === 0 ? (
          <p className="px-4 py-5 type-body text-idpxyz-textMuted">{emptyText}</p>
        ) : (
          <table className="w-full border-collapse">
            <thead>
              <tr className="bg-idpxyz-editor/70">
                {columns.map((col) => (
                  <th
                    key={col.id}
                    className={`px-3 py-2 type-caption font-medium text-idpxyz-textMuted border-b border-idpxyz-border whitespace-nowrap ${
                      col.align === 'right' ? 'text-right' : 'text-left'
                    }`}
                  >
                    {col.header}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, idx) => (
                <tr key={rowKey(row, idx)}>
                  {columns.map((col) => (
                    <td
                      key={col.id}
                      // 行线不带透明度：主题色是裸 CSS 变量，`/60` 生不出样式，线会落回缺省的浅灰，深色主题下是白线。
                      className={`px-3 py-2 type-body text-idpxyz-text border-b border-idpxyz-border align-top ${
                        col.mono ? 'font-mono whitespace-nowrap' : ''
                      } ${col.align === 'right' ? 'text-right' : 'text-left'}`}
                    >
                      {col.render(row, idx)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </section>
  );
}

/**
 * 状态签：一队一个签，签上是这一队此刻有几张。数量没答出来的签不写数，也不写成 0；不数的签（已出库这类只增不减的）
 * 不给 count。
 */
export function WmsStatusTabs({
  value,
  tabs,
  onChange,
}: {
  value: string;
  tabs: ReadonlyArray<{ value: string; label: string; count?: number | null }>;
  onChange: (next: string) => void;
}) {
  return (
    <div role="tablist" aria-label="按状态看" className="flex flex-wrap items-center gap-1.5">
      {tabs.map((tab) => {
        const active = tab.value === value;
        return (
          <button
            key={tab.value}
            type="button"
            role="tab"
            aria-selected={active}
            className={`${chipClass(active)} whitespace-nowrap`}
            onClick={() => onChange(tab.value)}
          >
            {tab.label}
            {typeof tab.count === 'number' ? (
              <span className={`ml-1.5 font-mono ${active ? '' : tab.count > 0 ? 'font-semibold text-idpxyz-textBright' : ''}`}>{tab.count}</span>
            ) : null}
          </button>
        );
      })}
    </div>
  );
}

export interface WmsQueueStat {
  id: string;
  label: string;
  /** 这一队有几张；没答出来是 null，写「—」，不写成 0。 */
  count: number | null;
  /** 正按这一队筛着。 */
  active: boolean;
  /** 这一队有单就是等人拿主意的：数字用警示色。 */
  attention?: boolean;
  onClick: () => void;
}

/**
 * 工作台标题行上的各队数量：一队一格，上面是队名、下面是数字，点一下看这一队。样子照工作台模板的指标格，
 * 只是数字在队名下面居中：队名长短不一（两个字到四个字），数字靠左时各格的数字离自己的队名忽远忽近，一排读下来不齐。
 *
 * 不走模板的 kpis：那一组在窗口不到 2xl 宽时固定另起一行，而这里只有几格、标题行摆得下——另起一行的话，
 * 把它从工具条里拿出来省下的那一行高度就又还回去了。摆在标题旁的插槽里，放不下时才跟着标题行折行。
 * 一张也没有的队数字压淡。选中的那一格上了底色，语气色在那层底上对比不够，数字改用正文亮色。
 */
export function WmsQueueStats({ items }: { items: readonly WmsQueueStat[] }) {
  return (
    <div role="group" aria-label="各队还有几张" className="flex items-center gap-x-2">
      <span aria-hidden="true" className="mr-1 h-7 w-px shrink-0 bg-idpxyz-border" />
      {items.map((item) => {
        const tone = item.active
          ? 'text-idpxyz-textBright'
          : item.count === null || item.count === 0
            ? 'text-idpxyz-textMuted'
            : item.attention
              ? statusToneText.warning
              : 'text-idpxyz-textBright';
        return (
          <button
            key={item.id}
            type="button"
            aria-pressed={item.active}
            title={item.active ? `正在看「${item.label}」，再点一下看全部` : `只看「${item.label}」`}
            className={`flex shrink-0 flex-col items-center gap-0.5 rounded px-2 py-0.5 transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-idpxyz-accent ${
              item.active ? 'bg-idpxyz-activeItem ring-1 ring-idpxyz-accent' : 'hover:bg-idpxyz-hover'
            }`}
            onClick={item.onClick}
          >
            <span className="whitespace-nowrap type-caption leading-none text-idpxyz-breadcrumb">{item.label}</span>
            <span className={`type-title font-semibold leading-none tabular-nums ${tone}`}>{item.count === null ? '—' : item.count}</span>
          </button>
        );
      })}
    </div>
  );
}

/**
 * 列表里一格两行：上面是人认的那个值（名称、单号），下面是辅助的小字（编码、时刻、地址）。没有辅助值就只有一行。
 * 两行各自截断，全文在悬停里；`code` 为真时小字用等宽体，编码才对得齐。按内容定宽的列里它最宽到 18rem。
 *
 * `fill` 为真是放在吃宽度的列里：跟着列宽走、不设上限，最窄留 11rem。里面那层 `w-0 min-w-full` 是让长文字不把列撑开——
 * 表格按内容算列宽，一行不折的长地址会要走它的全长；宽度记成零，它就不参与算列宽，再用最小宽度铺满算出来的那一列。
 */
export function WmsTwoLine({
  primary,
  secondary,
  code = false,
  fill = false,
  title,
}: {
  primary: ReactNode;
  secondary?: ReactNode;
  code?: boolean;
  fill?: boolean;
  title?: string;
}) {
  const lines = (
    <>
      <div className="truncate type-body text-idpxyz-text">{primary}</div>
      {secondary ? (
        <div className={`mt-0.5 truncate type-caption text-idpxyz-textMuted ${code ? 'font-mono' : ''}`}>{secondary}</div>
      ) : null}
    </>
  );
  if (fill) {
    return (
      <div className="min-w-[11rem] leading-tight" title={title}>
        <div className="w-0 min-w-full">{lines}</div>
      </div>
    );
  }
  return (
    <div className="max-w-[18rem] leading-tight" title={title}>
      {lines}
    </div>
  );
}

/** 列表筛选区里的一个下拉：第一项是「全部」，即不按这一维筛。 */
export function WmsFilterSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: ReadonlyArray<{ value: string; label: string }>;
  onChange: (next: string) => void;
}) {
  return (
    <label className="inline-flex items-center gap-1.5 type-body text-idpxyz-textMuted">
      <span className="whitespace-nowrap">{label}</span>
      <select
        className="h-7 max-w-[14rem] rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text focus:border-idpxyz-accent focus:outline-none"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        <option value="">全部</option>
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

/**
 * 不走列表模板的页（仓库监控、实时库存）自己摆的翻页条。它是页面纵向排布里最后一格、不随内容滚动，
 * 所以行多行少它都在页底同一处；样子与列表模板的翻页条一致。
 */
export function WmsPagerBar({
  summary,
  canPrevious,
  canNext,
  onPrevious,
  onNext,
  pageSize,
}: {
  summary: string;
  canPrevious: boolean;
  canNext: boolean;
  onPrevious: () => void;
  onNext: () => void;
  /** 给了就能改每页多少条。不给时翻页条与原来逐字节相同。 */
  pageSize?: { value: number; options: readonly number[]; onChange: (next: number) => void };
}) {
  return (
    <div className="shrink-0 border-t border-idpxyz-border px-4 py-1.5">
      <nav aria-label="翻页" className="flex items-center justify-between gap-4 type-caption text-idpxyz-textMuted">
        <span className="tabular-nums">{summary}</span>
        <div className="flex items-center gap-1">
          {pageSize ? (
            <label className="mr-2 inline-flex items-center gap-1.5">
              <span className="whitespace-nowrap">每页</span>
              <select
                className="h-7 rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 type-body text-idpxyz-text focus:border-idpxyz-accent focus:outline-none"
                value={String(pageSize.value)}
                onChange={(event) => pageSize.onChange(Number(event.target.value))}
              >
                {pageSize.options.map((size) => (
                  <option key={size} value={size}>
                    {size}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
          <Button type="button" variant="ghost" size="sm" disabled={!canPrevious} onClick={onPrevious}>
            <ChevronLeft className="h-3 w-3" aria-hidden="true" />
            上一页
          </Button>
          <Button type="button" variant="ghost" size="sm" disabled={!canNext} onClick={onNext}>
            下一页
            <ChevronRight className="h-3 w-3" aria-hidden="true" />
          </Button>
        </div>
      </nav>
    </div>
  );
}

/**
 * 一条细的进度条加「做完 / 总数」。全做完是成功色，没做完是强调色；`flagged` 是其中出了岔子的数量（比如少拣），
 * 大于零时数字后面用危险色点出来。总数是 0 时不画条，只写一个横杠——没有可做的不等于做完了。
 */
export function WmsProgress({ done, total, flagged = 0, flaggedLabel }: { done: number; total: number; flagged?: number; flaggedLabel?: string }) {
  if (total <= 0) return <span className="text-idpxyz-textMuted">—</span>;
  const percent = Math.min(100, Math.floor((done / total) * 100));
  const complete = done >= total;
  return (
    <span className="inline-flex items-center gap-2 whitespace-nowrap">
      <span
        className="h-1.5 w-20 overflow-hidden rounded-full bg-idpxyz-hover"
        role="progressbar"
        aria-label={`${done} / ${total}`}
        aria-valuemin={0}
        aria-valuemax={total}
        aria-valuenow={done}
      >
        <span
          className={`block h-full rounded-full ${complete ? 'bg-[color:var(--ds-status-success-fg)]' : 'bg-idpxyz-accent'}`}
          style={{ width: `${percent}%` }}
        />
      </span>
      <span className="font-mono type-body leading-4">
        {done} / {total}
      </span>
      {flagged > 0 ? (
        <span className={`type-caption ${statusToneText.danger}`}>
          {flaggedLabel ?? ''}
          {flagged}
        </span>
      ) : null}
    </span>
  );
}

/** 列表/工作页顶部的紧凑 KPI 摘要条。 */
export function WmsKpiStrip({
  items,
}: {
  items: ReadonlyArray<{ label: string; value: string | number; note?: string; tone?: WmsTone }>;
}) {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
      {items.map((item) => (
        <div key={item.label} className="rounded-lg border border-idpxyz-border bg-idpxyz-sidebar px-4 py-3">
          <div className="flex items-center justify-between gap-2">
            <span className="type-caption text-idpxyz-textMuted">{item.label}</span>
            {item.tone ? <span className={`h-2 w-2 rounded-full ${toneClasses[item.tone].dot}`} /> : null}
          </div>
          <div className="mt-1 font-mono type-section font-bold text-idpxyz-textBright">{item.value}</div>
          {item.note ? <div className="mt-0.5 type-caption text-idpxyz-textMuted">{item.note}</div> : null}
        </div>
      ))}
    </div>
  );
}
