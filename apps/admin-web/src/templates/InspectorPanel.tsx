import {
  Button,
  InspectorActions,
  InspectorBody,
  InspectorHeader,
  InspectorIdentity,
  InspectorRow,
  InspectorSection,
  InspectorShell,
  Tooltip,
} from '@idpxyz/ui-primitives';
import { SectionError } from '../components/states';
import { StatusBadgeFor, domainStatusTones, type DomainStatus } from '../domain/status';
import {
  INSPECTOR_CONTRACT_ERROR_TITLE,
  INSPECTOR_EMPTY_NOTE,
  inspectorActionDisabled,
  resolveInspectorForPanel,
  type InspectorAction,
  type InspectorContent,
  type InspectorField,
  type InspectorSection as InspectorSectionContent,
} from './inspector';

// 右侧检查器的渲染件（票 admin-web-workspace-form/02 第 2 条）。内容契约与节序归 inspector.ts，这里只按归并结果摆：
// 用 @idpxyz/ui-primitives 的 Inspector* 一族（设计系统自带的检查器形），不另画一套节与行。
// 宽度由父级给，自己不定宽（蓝图 13 节「宽度稳定」是壳层的事）。null 渲染一句空态，不放假内容。
//
// 状态簇按 domain/status 的词表着色与定层：词表词走 StatusBadgeFor（层定形、词定色），词表外的原样示文不猜色调——
// 与各列表页对同一格的处置一致（如 ShipmentRequestListPage 的 requestStateBadge）。
// 禁用动作用 aria-disabled + Tooltip 说明而不是 disabled：Button 的 disabled 带 pointer-events-none，悬停说明就出不来、
// 键盘也聚焦不到它——说明本身是给人看的，位不能自己把说明藏起来（与 ListPageTemplate 的 DisabledSlot 同一理由）。

export interface InspectorPanelProps {
  content: InspectorContent | null;
  /** 栏顶的关闭 / 折叠动作；不传则不出 × 。 */
  onClose?: () => void;
}

const PANEL_TITLE = '检查器';

function StatusWord({ word }: { word: string }) {
  return word in domainStatusTones ? (
    <StatusBadgeFor status={word as DomainStatus} />
  ) : (
    <span className="font-mono text-[11px] text-idpxyz-textMuted">{word}</span>
  );
}

function FieldRows({ fields }: { fields: InspectorField[] }) {
  return (
    <>
      {fields.map((field) => (
        <InspectorRow key={field.label} label={field.label} value={field.value} mono={field.mono} />
      ))}
    </>
  );
}

function ActionButton({ action }: { action: InspectorAction }) {
  if (inspectorActionDisabled(action)) {
    return (
      <Tooltip content={action.disabledReason ?? ''}>
        <Button
          variant="ghost"
          size="sm"
          aria-disabled="true"
          className="w-full justify-start cursor-not-allowed opacity-50 hover:bg-transparent hover:text-idpxyz-textMuted"
          onClick={(event) => event.preventDefault()}
        >
          {action.label}
        </Button>
      </Tooltip>
    );
  }
  return (
    <Button variant="ghost" size="sm" className="w-full justify-start" onClick={action.onRun}>
      {action.label}
    </Button>
  );
}

function SectionBody({ section }: { section: InspectorSectionContent }) {
  switch (section.kind) {
    case 'summary':
    case 'audit':
      return <FieldRows fields={section.fields} />;
    case 'status':
      return (
        <>
          {section.items.map((item) => (
            <InspectorRow key={item.label} label={item.label} value={<StatusWord word={item.word} />} />
          ))}
        </>
      );
    case 'actions':
      // 纵排：动作名是整句中文，横排在 240px 的栏里会折成两行半。
      return (
        <InspectorActions className="flex-col items-stretch gap-1 mt-0">
          {section.actions.map((action) => (
            <ActionButton key={action.label} action={action} />
          ))}
        </InspectorActions>
      );
    case 'related':
      // 原生锚点带 href：hash 地址本来就是可收藏的链接，中键 / 右键「新标签打开」都照常，不用按钮伪装成链接。
      return (
        <ul className="space-y-1">
          {section.links.map((link) => (
            <li key={link.hash}>
              <a href={link.hash} className="text-[11px] text-idpxyz-accent hover:underline">
                {link.label}
              </a>
            </li>
          ))}
        </ul>
      );
  }
}

function ResolvedSections({ content }: { content: InspectorContent }) {
  const resolution = resolveInspectorForPanel(content);
  if (resolution.kind === 'contractError') {
    // 不给重试：内容是页面从行上算出来的，再算一遍还是同一份。
    return <SectionError title={INSPECTOR_CONTRACT_ERROR_TITLE} description={resolution.message} />;
  }
  return (
    <>
      {/* key 只按节：翻行时节的展开态保留——处理队列的姿势是折掉不看的节、一行行往下翻，换一行就把折好的节全弹开等于每行重折一次。 */}
      {resolution.sections.map((resolved) => (
        <InspectorSection key={resolved.kind} title={resolved.label} defaultOpen={resolved.defaultOpen}>
          <SectionBody section={resolved.section} />
        </InspectorSection>
      ))}
    </>
  );
}

export function InspectorPanel({ content, onClose }: InspectorPanelProps) {
  return (
    <InspectorShell className="bg-idpxyz-sidebar" data-inspector-panel>
      <InspectorHeader title={PANEL_TITLE} onClose={onClose} />
      {content === null ? (
        <InspectorBody>
          <p className="text-[11px] text-idpxyz-textMuted" data-inspector-empty>
            {INSPECTOR_EMPTY_NOTE}
          </p>
        </InspectorBody>
      ) : (
        <InspectorBody>
          <InspectorIdentity title={content.title} subtitle={content.subtitle} />
          <ResolvedSections content={content} />
        </InspectorBody>
      )}
    </InspectorShell>
  );
}
