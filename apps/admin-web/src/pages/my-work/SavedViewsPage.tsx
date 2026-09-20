import { useState, type MouseEvent } from 'react';
import { Button, ConfirmDialog, Tag } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import {
  filterSavedViews,
  listSavedViews,
  removeSavedView,
  savedViewHash,
  savedViewModuleIds,
  setDefaultSavedView,
  type SavedView,
} from './saved-views';
import { InstantCell, ModuleChips, moduleTitleOf } from './shared';

// 页面标题与出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['saved-views'];

// 保存视图页（票 admin-web-ux-alignment/02 第 3 条）：列本机存下的全部列表筛选态，行点跳 `#/<moduleId>?view=<id>`。
//
// 本页只管保管与跳转，不解释 state：它是各列表页自己的不透明 JSON，怎么读回归那张页（模板的保存视图位，票 03）。
// 也不在这里新建视图——「保存当前视图」只能在有筛选态的那张列表页上按，这里没有筛选态可存。
// 行内两个动作（设为默认 / 删除）在行点跳转之上，要拦住冒泡，否则按一下「删除」先把人带走。
// 「删除」走 ConfirmDialog 二次确认——它是唯一的删除入口（saved-views.ts 头注）；一条视图只是本机的一份筛选条件，删了不影响任何数据。

function stop(event: MouseEvent) {
  event.stopPropagation();
}

export function SavedViewsPage() {
  const [list, setList] = useState<SavedView[]>(() => listSavedViews(window.localStorage));
  const [keyword, setKeyword] = useState('');
  const [moduleId, setModuleId] = useState<string | null>(null);
  const [removing, setRemoving] = useState<SavedView | null>(null);

  const reload = () => setList(listSavedViews(window.localStorage));
  const rows = filterSavedViews(list, { keyword, moduleId });
  const open = (row: SavedView) => {
    window.location.hash = savedViewHash(row);
  };
  const makeDefault = (row: SavedView) => {
    setDefaultSavedView(window.localStorage, row.id);
    reload();
  };
  const remove = () => {
    if (removing) removeSavedView(window.localStorage, removing.id);
    setRemoving(null);
    reload();
  };

  const columns: ListColumn<SavedView>[] = [
    { id: 'name', header: '名称', render: (row) => row.name },
    { id: 'module', header: '模块', render: (row) => moduleTitleOf(row.moduleId) },
    {
      id: 'saved-at',
      header: '保存时刻',
      className: 'w-[180px]',
      render: (row) => <InstantCell value={row.savedAt} />,
    },
    {
      id: 'default',
      header: '默认',
      className: 'w-[72px]',
      render: (row) => (row.isDefault ? <Tag>默认</Tag> : null),
    },
    {
      id: 'actions',
      header: '操作',
      align: 'right',
      className: 'w-[160px]',
      render: (row) => (
        <span className="inline-flex items-center gap-1" onClick={stop} onDoubleClick={stop}>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={row.isDefault}
            onClick={() => makeDefault(row)}
          >
            设为默认
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => setRemoving(row)}>
            删除
          </Button>
        </span>
      ),
    },
  ];

  return (
    <>
      <ListPageTemplate<SavedView>
        title={info.title}
        description="各列表页存在这台浏览器里的筛选态，起了名字、可设为该模块的默认视图；只存本机，换机、换浏览器不带，不上服务端。行点回到那张列表页并带上视图。"
        moduleId="saved-views"
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜索视图名称' }}
        filters={<ModuleChips moduleIds={savedViewModuleIds(list)} value={moduleId} onChange={setModuleId} />}
        filterSummary={`本机保存 ${list.length} 个视图`}
        columns={columns}
        rows={rows}
        rowKey={(row) => row.id}
        onRowClick={open}
        onRowOpen={open}
        emptyRowsNote="当前筛选条件下没有匹配的视图"
        viewState={
          list.length === 0
            ? {
                kind: 'empty',
                title: '这台浏览器还没有保存过视图',
                description: '在列表页的过滤条上按「保存当前视图」存下筛选态后，它会列在这里。',
              }
            : { kind: 'ready' }
        }
      />
      <ConfirmDialog
        open={removing !== null}
        title="删除保存视图"
        message={
          removing
            ? `将删除视图「${removing.name}」（${moduleTitleOf(removing.moduleId)}）。删的只是这台浏览器上的一份筛选条件，不影响任何数据。`
            : ''
        }
        confirmLabel="删除"
        cancelLabel="取消"
        tone="danger"
        onConfirm={remove}
        onCancel={() => setRemoving(null)}
      />
    </>
  );
}
