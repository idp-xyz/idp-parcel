import { useEffect, useState, type ReactNode } from 'react';
import { Check, Copy } from 'lucide-react';
import {
  PageHeader,
  PageHeaderContent,
  PageHeaderTitle,
  PageHeaderDescription,
  PageHeaderActions,
} from '@idpxyz/ui-patterns';
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  Timeline,
} from '@idpxyz/ui-primitives';
import { StateSlot, type TemplateViewState, type StateSlotProps } from './state-slot';
import type { DetailField, AuditEntry } from './types';

/** 业务区块：一个区块渲染成一个 Panel（Card），内容由调用方组装。 */
export interface DetailSection {
  id: string;
  title: string;
  description?: string;
  content: ReactNode;
}

/**
 * 对象头区第二行的一格元信息（如「最后更新」「提交方」「修订」）。只显调用方给的：
 * 手册把「负责人」「风险」也列在头区，本仓没有这两样的领域来源，模板不为它们留格（票面裁决 2）。
 */
export interface DetailMeta {
  label: string;
  value: ReactNode;
}

export interface DetailPageTemplateProps {
  /** 页面标题（如「托运申报单」）。 */
  title: string;
  /** 业务标识（如申报单号），展示在标题旁，等宽字体便于比对抄录。 */
  identifier?: string;
  /** 标题行右侧的状态展示，通常是 StatusBadge；语义归调用方。 */
  status?: ReactNode;
  description?: string;
  headerActions?: ReactNode;
  /**
   * 对象头区元信息行。传了就按对象工作区头区渲染（两行、主编号可复制）；
   * 不传沿用单行头，另两张详情页因此一字节不变。
   */
  meta?: DetailMeta[];
  /** 「基本信息」区字段。两列栅格排布，字段多时自动换行。 */
  basicFields: DetailField[];
  /** 基本信息区标题，默认「基本信息」。 */
  basicTitle?: string;
  /** 业务区块（如包裹明细、判定翻译结果），按序渲染为独立 Panel。 */
  sections?: DetailSection[];
  /** 审计留痕；不传则不渲染审计区（区别于「传空数组=有区但暂无记录」）。 */
  auditTrail?: AuditEntry[];
  /** 审计区标题，默认「审计留痕」。 */
  auditTitle?: string;
  viewState: TemplateViewState;
  stateOverride?: StateSlotProps['override'];
}

// 详情页模板：PageHeader + Panel 分区（基本信息 / 业务区块 / 审计区）。
// 非 ready 态替换整个内容区——详情页没有「部分可看」的中间态，
// 半份详情比没有详情更误导复核判断。
export function DetailPageTemplate({
  title,
  identifier,
  status,
  description,
  headerActions,
  meta,
  basicFields,
  basicTitle = '基本信息',
  sections,
  auditTrail,
  auditTitle = '审计留痕',
  viewState,
  stateOverride,
}: DetailPageTemplateProps) {
  // 手册「对象工作区」的 Object Header 只在调用方要了它的格时启用；旧形态那一支的 JSX
  // 原样保留而不抽成共用片段，是为了让「不传新 prop 时渲染逐字节同」能对着代码读出来。
  const objectHeader = meta !== undefined;
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      {objectHeader ? (
        <PageHeader className="items-start">
          <PageHeaderContent className="flex-col items-start gap-1">
            <div className="flex min-w-0 max-w-full items-center gap-3">
              <PageHeaderTitle>{title}</PageHeaderTitle>
              {identifier && (
                <span className="inline-flex items-center gap-1">
                  <span className="select-all font-mono text-[12px] text-idpxyz-accent">
                    {identifier}
                  </span>
                  <CopyIdentifierButton text={identifier} />
                </span>
              )}
              {status}
              {description && <PageHeaderDescription>{description}</PageHeaderDescription>}
            </div>
            {meta.length > 0 && (
              <dl className="flex flex-wrap gap-x-4 gap-y-0.5 text-[11px] leading-4">
                {meta.map((item) => (
                  <div key={item.label} className="flex gap-1">
                    <dt className="text-idpxyz-textMuted">{item.label}</dt>
                    <dd className="text-idpxyz-text">{item.value}</dd>
                  </div>
                ))}
              </dl>
            )}
          </PageHeaderContent>
          {headerActions && <PageHeaderActions>{headerActions}</PageHeaderActions>}
        </PageHeader>
      ) : (
        <PageHeader>
          <PageHeaderContent>
            <PageHeaderTitle>{title}</PageHeaderTitle>
            {identifier && (
              <span className="font-mono text-[12px] text-idpxyz-accent">{identifier}</span>
            )}
            {status}
            {description && <PageHeaderDescription>{description}</PageHeaderDescription>}
          </PageHeaderContent>
          {headerActions && <PageHeaderActions>{headerActions}</PageHeaderActions>}
        </PageHeader>
      )}

      {viewState.kind === 'ready' ? (
        <div className="flex-1 overflow-auto">
          <div className="mx-auto flex max-w-[960px] flex-col gap-4 p-4">
            <Card>
              <CardHeader>
                <CardTitle>{basicTitle}</CardTitle>
              </CardHeader>
              <CardContent>
                <dl className="grid grid-cols-1 gap-x-8 gap-y-2.5 sm:grid-cols-2">
                  {basicFields.map((field) => (
                    <div key={field.label} className="flex gap-3 text-[12px] leading-5">
                      <dt className="w-[96px] shrink-0 text-idpxyz-textMuted">{field.label}</dt>
                      <dd className="min-w-0 flex-1 break-words text-idpxyz-text">
                        {field.value}
                      </dd>
                    </div>
                  ))}
                </dl>
              </CardContent>
            </Card>

            {sections?.map((section) => (
              <Card key={section.id}>
                <CardHeader>
                  <CardTitle>{section.title}</CardTitle>
                  {section.description && (
                    <CardDescription>{section.description}</CardDescription>
                  )}
                </CardHeader>
                <CardContent>{section.content}</CardContent>
              </Card>
            ))}

            {auditTrail && (
              <Card>
                <CardHeader>
                  <CardTitle>{auditTitle}</CardTitle>
                </CardHeader>
                <CardContent>
                  {auditTrail.length === 0 ? (
                    <p className="text-[12px] text-idpxyz-textMuted">暂无审计记录。</p>
                  ) : (
                    <Timeline items={auditTrail} />
                  )}
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      ) : (
        <div className="flex-1 flex items-center justify-center overflow-auto">
          <StateSlot state={viewState} override={stateOverride} />
        </div>
      )}
    </div>
  );
}

// 主编号复制：抄录比对是操作员对主编号最常做的事。反馈只换图标一下，不走 Toast——
// 模板不该要求调用方套 ToastProvider。剪贴板 API 只在安全上下文存在，没有就只留 select-all 那条路。
function CopyIdentifierButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), 1500);
    return () => clearTimeout(timer);
  }, [copied]);
  return (
    <button
      type="button"
      aria-label="复制主编号"
      title="复制主编号"
      onClick={() => {
        if (!navigator.clipboard) return;
        void navigator.clipboard.writeText(text).then(() => setCopied(true));
      }}
      className="rounded p-0.5 text-idpxyz-textMuted hover:bg-idpxyz-hover hover:text-idpxyz-text"
    >
      {copied ? <Check size={12} /> : <Copy size={12} />}
    </button>
  );
}
