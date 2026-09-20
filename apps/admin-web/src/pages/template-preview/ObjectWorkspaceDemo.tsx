import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@idpxyz/ui-primitives';
import { StatusBadge } from '@idpxyz/ui-patterns';
import {
  DetailPageTemplate,
  type DetailWorkspaceTab,
  type TemplateViewState,
} from '../../templates';
// 与预览页同一条理由按路径引 demo.ts：合成假数据只从演示入口注入。
import {
  demoDetailAuditTrail,
  demoDetailFields,
  demoDetailSections,
  demoWorkspaceAsideFacts,
  demoWorkspaceIdentifier,
  demoWorkspaceMeta,
  demoWorkspaceRelated,
  demoWorkspaceSummary,
} from '../../templates/demo';

// 对象工作区演示（票 admin-web-ux-alignment/04 第 6 条）：同一份合成申报单换成手册「对象工作区」的排布——
// 头区两行 + 指标带 + 稳定命名的签 + 右侧上下文。抽成独立组件而不写进预览页，是让一次性的
// renderToStaticMarkup 探针能拿到与页面完全相同的那棵树，而不是另拼一份「像页面」的 props。
// 不在这里取 useToast：快速动作的反馈交给调用方，探针渲染时就不必套 ToastProvider。
export function ObjectWorkspaceDemo({
  viewState,
  onDemoAction,
}: {
  viewState: TemplateViewState;
  /** 头区快速动作被点时的反馈；预览页接成 toast。演示动作没有实际行为。 */
  onDemoAction: (title: string, message: string) => void;
}) {
  // 签故意按 审计 → 关联 → 概要 倒着传：渲染出来仍是 概要 · 关联 · 审计，顺序由 workspace-tabs 的固定表钉住，
  // 调用方传进来的顺序不算数。概要与审计两签的模板内容（基本信息 / 区块、审计留痕）在前，这里给的卡接在其后，
  // 顺带演示同 id 归并；时间线 / 异常 / 文档没人给内容，不出签（票面裁决 1）。
  const tabs: DetailWorkspaceTab[] = [
    {
      id: 'audit',
      content: (
        <Card>
          <CardHeader>
            <CardTitle>留痕口径（演示）</CardTitle>
            <CardDescription>调用方接在模板审计留痕之后的同 id 内容</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-[12px] leading-6 text-idpxyz-text">
              上方留痕是模板从 auditTrail 渲染的；真实页面由审计读口供数，本卡只演示调用方可以在其后补充说明。
            </p>
          </CardContent>
        </Card>
      ),
    },
    {
      id: 'related',
      content: (
        <Card>
          <CardHeader>
            <CardTitle>关联对象</CardTitle>
            <CardDescription>与本申报单相关的批次、声明包裹与接受判断任务（合成）</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>对象</TableHead>
                  <TableHead>标识</TableHead>
                  <TableHead>说明</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {demoWorkspaceRelated.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell>{item.kind}</TableCell>
                    <TableCell>
                      <span className="font-mono text-[12px]">{item.id}</span>
                    </TableCell>
                    <TableCell className="text-idpxyz-textMuted">{item.note}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      ),
      count: demoWorkspaceRelated.length,
    },
    {
      id: 'summary',
      content: (
        <Card>
          <CardHeader>
            <CardTitle>演示说明</CardTitle>
            <CardDescription>调用方接在模板概要内容之后的同 id 内容</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-[12px] leading-6 text-idpxyz-text">
              本签前面的基本信息与两个区块是模板按 basicFields / sections 放进「概要」的；这张卡是调用方另给的同 id 内容，
              模板把它接在自己的内容之后。
            </p>
          </CardContent>
        </Card>
      ),
    },
  ];

  return (
    <DetailPageTemplate
      title="托运申报单（对象工作区演示）"
      identifier={demoWorkspaceIdentifier}
      status={<StatusBadge status="warning">待人工复核</StatusBadge>}
      headerActions={
        <>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => onDemoAction('演示动作', '头区次要快速动作占位，无实际行为。')}
          >
            次要动作（演示）
          </Button>
          <Button size="sm" onClick={() => onDemoAction('演示动作', '头区主快速动作占位，无实际行为。')}>
            主动作（演示）
          </Button>
        </>
      }
      meta={demoWorkspaceMeta.map((item) => ({
        label: item.label,
        value: <span className="font-mono">{item.value}</span>,
      }))}
      summary={demoWorkspaceSummary}
      basicFields={demoDetailFields}
      sections={demoDetailSections.map((section) => ({
        id: section.id,
        title: section.title,
        description: section.description,
        content: <p className="text-[12px] leading-6 text-idpxyz-text">{section.body}</p>,
      }))}
      auditTrail={demoDetailAuditTrail}
      tabs={tabs}
      aside={
        <Card>
          <CardHeader>
            <CardTitle>上下文（演示）</CardTitle>
            <CardDescription>窄屏上整栏隐藏，只放看不见也不缺的辅助信息</CardDescription>
          </CardHeader>
          <CardContent>
            <dl className="flex flex-col gap-1.5 text-[12px] leading-5">
              {demoWorkspaceAsideFacts.map((fact) => (
                <div key={fact.label} className="flex flex-col">
                  <dt className="text-idpxyz-textMuted">{fact.label}</dt>
                  <dd className="text-idpxyz-text">{fact.value}</dd>
                </div>
              ))}
            </dl>
          </CardContent>
        </Card>
      }
      viewState={viewState}
    />
  );
}
