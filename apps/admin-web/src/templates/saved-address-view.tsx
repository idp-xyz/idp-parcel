import { useEffect, useState, type ReactNode } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { addressQueryWithoutView, hashForSavedView } from './address-query';
import { useOwnAddressChange } from './use-own-address-change';
import {
  defaultSavedViewFor,
  listSavedViews,
  saveView,
  savedViewIdFromHash,
  type SavedView,
} from '../pages/my-work/saved-views';

// 列表上的「保存视图」：存的是这张列表地址上的筛选（检索词、页签，以及页面已经写进地址的条件），
// 记在本机。选中一份就把地址换回去，页面按地址读回。只活在内存里、没写进地址的筛选不在这一份里。
// 「保存的视图」「最近打开」自己不存自己。

const skippedModules = new Set(['saved-views', 'recent-objects', 'workbench']);

export interface SavedAddressQuery {
  query: string;
}

export function savedAddressQuery(value: unknown): SavedAddressQuery | null {
  if (typeof value !== 'object' || value === null) return null;
  const query = (value as { query?: unknown }).query;
  return typeof query === 'string' ? { query } : null;
}

function replaceHash(next: string): void {
  if (window.location.hash === next) return;
  window.history.replaceState(window.history.state, '', next);
  window.dispatchEvent(new HashChangeEvent('hashchange'));
}

function viewFor(moduleId: string, id: string): SavedView | null {
  return listSavedViews(window.localStorage).find((view) => view.id === id && view.moduleId === moduleId) ?? null;
}

function applyStored(moduleId: string, view: SavedView): void {
  const stored = savedAddressQuery(view.state);
  if (!stored) return;
  if (addressQueryWithoutView(window.location.hash) === stored.query) return;
  replaceHash(hashForSavedView(moduleId, stored.query, view.id));
}

export function useAddressSavedViews(
  moduleId: string | undefined,
  enabled: boolean,
): {
  props:
    | { current: string | null; list: { id: string; label: string }[]; onSave: () => void; onSelect: (id: string) => void }
    | undefined;
  dialog: ReactNode;
} {
  const active = enabled && moduleId !== undefined && !skippedModules.has(moduleId);
  const [hash, setHash] = useState(() => (typeof window === 'undefined' ? '' : window.location.hash));
  const [views, setViews] = useState<SavedView[]>([]);
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState('');

  useOwnAddressChange(() => setHash(window.location.hash));

  useEffect(() => {
    if (!active || !moduleId) return;
    setViews(listSavedViews(window.localStorage).filter((view) => view.moduleId === moduleId));
    const id = savedViewIdFromHash(window.location.hash);
    if (id) {
      const named = viewFor(moduleId, id);
      if (named) applyStored(moduleId, named);
      return;
    }
    if (addressQueryWithoutView(window.location.hash) !== '') return;
    const fallback = defaultSavedViewFor(window.localStorage, moduleId);
    if (fallback) applyStored(moduleId, fallback);
  }, [active, moduleId]);

  if (!active || !moduleId) return { props: undefined, dialog: null };

  const current = savedViewIdFromHash(hash);
  const reload = () => setViews(listSavedViews(window.localStorage).filter((view) => view.moduleId === moduleId));

  const dialog = naming ? (
    <div className="fixed inset-0 z-[120] flex items-center justify-center bg-black/45 px-4" role="presentation">
      <form
        role="dialog"
        aria-labelledby="save-view-title"
        className="w-full max-w-sm rounded-md border border-idpxyz-border bg-idpxyz-sidebar p-4 shadow-xl"
        onSubmit={(event) => {
          event.preventDefault();
          const saved = saveView(window.localStorage, {
            moduleId,
            name,
            state: { query: addressQueryWithoutView(window.location.hash) } satisfies SavedAddressQuery,
          });
          setNaming(false);
          setName('');
          if (!saved) return;
          reload();
          replaceHash(hashForSavedView(moduleId, addressQueryWithoutView(window.location.hash), saved.id));
        }}
      >
        <h2 id="save-view-title" className="type-body font-medium text-idpxyz-textBright">
          保存当前筛选
        </h2>
        <p className="mt-1 type-caption text-idpxyz-textMuted">存在这台电脑上，换一台就没有了。起个认得出的名字。</p>
        <Input
          autoFocus
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="例如：待受理的运单"
          className="mt-3"
        />
        <div className="mt-3 flex justify-end gap-2">
          <Button type="button" variant="ghost" size="sm" onClick={() => setNaming(false)}>
            取消
          </Button>
          <Button type="submit" size="sm" disabled={name.trim() === ''}>
            保存
          </Button>
        </div>
      </form>
    </div>
  ) : null;

  return {
    dialog,
    props: {
      current: views.some((view) => view.id === current) ? current : null,
      list: views.map((view) => ({ id: view.id, label: view.isDefault ? `${view.name}（默认）` : view.name })),
      onSave: () => {
        setName('');
        setNaming(true);
      },
      onSelect: (id) => {
        const picked = viewFor(moduleId, id);
        if (picked) applyStored(moduleId, picked);
      },
    },
  };
}
