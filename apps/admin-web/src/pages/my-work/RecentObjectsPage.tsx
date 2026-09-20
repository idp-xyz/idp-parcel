import { useState } from 'react';
import { Button, ConfirmDialog } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import {
  clearRecentObjects,
  filterRecentObjects,
  listRecentObjects,
  recentModuleIds,
  recentObjectHash,
  type RecentObject,
} from './recent-objects';
import { InstantCell, ModuleChips, moduleTitleOf } from './shared';

// 页面标题与出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['recent-objects'];

// 最近对象页（票 admin-web-ux-alignment/02 第 2 条）：列本机浏览器里打开过的对象地址，行点即跳回去。
//
// 走 ListPageTemplate 而不另画一张表：黄金标准 Rule 2 要所有列表页长同一个母版形，「我的工作」两页也是列表页；
// 母版上排序 / 保存视图 / 更多筛选三个位照旧禁用 + 说明——本页的数据是本机历史，那三样没有可接的动作。
// 页头没有图标位（模板 title 收 string，改模板属票 03 地盘且票面明写不改），图标在左导航条目上，这里不重复。
//
// 数据一次读进 state：本页自己是这份存储唯一的写方（清空），别的地方写（外壳记一条）时人不在这页上，回来时重挂载重读。
// 「清空历史」走 ConfirmDialog 二次确认——它是唯一的删除入口，删的是整份历史，没有逐条删（理由见 recent-objects.ts 头注）。

const columns: ListColumn<RecentObject>[] = [
  {
    id: 'object',
    header: '对象',
    render: (row) => <span className="font-mono text-[12px]">{row.objectId}</span>,
  },
  {
    id: 'module',
    header: '模块',
    render: (row) => moduleTitleOf(row.moduleId),
  },
  {
    id: 'at',
    header: '上次打开',
    className: 'w-[180px]',
    render: (row) => <InstantCell value={row.at} />,
  },
];

export function RecentObjectsPage() {
  const [list, setList] = useState<RecentObject[]>(() => listRecentObjects(window.localStorage));
  const [keyword, setKeyword] = useState('');
  const [moduleId, setModuleId] = useState<string | null>(null);
  const [confirmingClear, setConfirmingClear] = useState(false);

  const rows = filterRecentObjects(list, { keyword, moduleId });
  const open = (row: RecentObject) => {
    window.location.hash = recentObjectHash(row);
  };
  const clear = () => {
    clearRecentObjects(window.localStorage);
    setList([]);
    setModuleId(null);
    setConfirmingClear(false);
  };

  return (
    <>
      <ListPageTemplate<RecentObject>
        title={info.title}
        description="你在这台浏览器里打开过的对象地址，最近的在前；只存本机，换机、换浏览器不带，不上服务端。标题从模块名与对象标识拼出，不是对象名称。"
        moduleId="recent-objects"
        headerActions={
          <Button
            variant="outline"
            size="sm"
            disabled={list.length === 0}
            onClick={() => setConfirmingClear(true)}
          >
            清空历史
          </Button>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜索对象标识或标题' }}
        filters={<ModuleChips moduleIds={recentModuleIds(list)} value={moduleId} onChange={setModuleId} />}
        filterSummary={`本机记录 ${list.length} 条`}
        columns={columns}
        rows={rows}
        rowKey={(row) => `${row.moduleId}/${row.objectId}`}
        onRowClick={open}
        onRowOpen={open}
        emptyRowsNote="当前筛选条件下没有匹配的历史"
        viewState={
          list.length === 0
            ? {
                kind: 'empty',
                title: '这台浏览器还没有打开过对象',
                description: '从列表页进到某个对象的详情后，它的地址会记在这里。',
              }
            : { kind: 'ready' }
        }
      />
      <ConfirmDialog
        open={confirmingClear}
        title="清空最近对象"
        message={`将删除这台浏览器上的 ${list.length} 条打开历史。删的只是历史记录，对象本身不受影响。`}
        confirmLabel="清空"
        cancelLabel="取消"
        tone="warning"
        onConfirm={clear}
        onCancel={() => setConfirmingClear(false)}
      />
    </>
  );
}
