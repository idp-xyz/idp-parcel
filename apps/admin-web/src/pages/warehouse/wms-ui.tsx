import type { ReactNode } from 'react';
import { Check, X } from 'lucide-react';
import {
  Button,
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
} from '@idpxyz/ui-primitives';
import { chipClass } from '../../components/registration';

export type WmsTone = 'success' | 'info' | 'warning' | 'danger' | 'neutral';

const toneClasses: Record<WmsTone, { pill: string; dot: string }> = {
  success: {
    pill: 'bg-emerald-500/12 text-emerald-700 dark:text-emerald-300 border-emerald-500/25',
    dot: 'bg-emerald-500',
  },
  info: {
    pill: 'bg-blue-500/12 text-blue-700 dark:text-blue-300 border-blue-500/25',
    dot: 'bg-blue-500',
  },
  warning: {
    pill: 'bg-amber-500/15 text-amber-700 dark:text-amber-300 border-amber-500/30',
    dot: 'bg-amber-500',
  },
  danger: {
    pill: 'bg-rose-500/12 text-rose-700 dark:text-rose-300 border-rose-500/25',
    dot: 'bg-rose-500',
  },
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
  OPEN: { label: '待执行', tone: 'warning' },
  SHORT: { label: '短拣', tone: 'danger' },
  LATE: { label: '迟到取消', tone: 'neutral' },
  // 退货与代理声明
  COUNTED_RETURN: { label: '已点收入库', tone: 'success' },
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
  failedLabel,
}: {
  steps: readonly StepItem[];
  currentId: string;
  failedLabel?: string;
}) {
  const currentIdx = steps.findIndex((s) => s.id === currentId);
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-idpxyz-border bg-idpxyz-editor/60 px-4 py-2.5">
      {steps.map((step, index) => {
        const done = !failedLabel && currentIdx >= 0 && index < currentIdx;
        const active = !failedLabel && index === currentIdx;
        return (
          <div key={step.id} className="flex items-center gap-2">
            <div className="flex items-center gap-1.5">
              <span
                className={`inline-flex h-5 w-5 items-center justify-center rounded-full type-caption font-semibold ${
                  done
                    ? 'bg-emerald-500 text-white'
                    : active
                      ? 'bg-idpxyz-accent text-idpxyz-buttonFg ring-2 ring-idpxyz-accent/25'
                      : 'border border-idpxyz-border bg-idpxyz-panel text-idpxyz-textMuted'
                }`}
              >
                {done ? <Check className="h-3 w-3" /> : index + 1}
              </span>
              <span
                className={`type-caption whitespace-nowrap ${
                  active
                    ? 'font-semibold text-idpxyz-textBright'
                    : done
                      ? 'text-idpxyz-text'
                      : 'text-idpxyz-textMuted'
                }`}
              >
                {step.label}
              </span>
            </div>
            {index < steps.length - 1 ? <span className="h-px w-5 bg-idpxyz-border" aria-hidden="true" /> : null}
          </div>
        );
      })}
      {failedLabel ? (
        <span className="ml-auto inline-flex items-center gap-1 rounded-full border border-rose-500/30 bg-rose-500/12 px-2.5 py-0.5 type-caption font-medium text-rose-700 dark:text-rose-300">
          <X className="h-3 w-3" />
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
            <div className="rounded border border-rose-500/30 bg-rose-500/10 p-4 type-body text-rose-700 dark:text-rose-300">
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
    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3 rounded-lg border border-idpxyz-border bg-idpxyz-panel p-4">
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
      <div className="rounded-lg border border-idpxyz-border bg-idpxyz-panel overflow-x-auto">
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
                <tr key={rowKey(row, idx)} className="hover:bg-idpxyz-hover/40 transition-colors">
                  {columns.map((col) => (
                    <td
                      key={col.id}
                      className={`px-3 py-2 type-body text-idpxyz-text border-b border-idpxyz-border/60 align-top ${
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

/** 列表筛选：第一项用 `ALL` 表示不按下推状态。 */
export function WmsStatusChips({
  value,
  options,
  onChange,
}: {
  value: string;
  options: readonly string[];
  onChange: (next: string) => void;
}) {
  return (
    <>
      {options.map((item) => (
        <button
          key={item}
          type="button"
          className={chipClass(item === value)}
          aria-pressed={item === value}
          onClick={() => onChange(item)}
        >
          {item === 'ALL' ? '全部' : labelOf(item)}
        </button>
      ))}
    </>
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
        <div key={item.label} className="rounded-lg border border-idpxyz-border bg-idpxyz-panel px-4 py-3">
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
