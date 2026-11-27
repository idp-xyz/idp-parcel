import { useEffect, useMemo, useState, type ElementType } from 'react';
import { ArrowUpRight, Command, Search } from 'lucide-react';
import { Button, Input } from '@idpxyz/ui-primitives';

// 命令面板是壳层自己的：上游那份把占位、分组名和空态写死成英文，也没有换成中文的口。
// 行为与那份一致（Esc 关闭、上下移动、回车执行），文案用管理台的话。

export interface CommandPaletteAction {
  id: string;
  label: string;
  group: 'navigation' | 'recent' | 'objects' | 'actions';
  keywords: string[];
  hint?: string;
  icon?: ElementType;
  run: () => void;
}

const groupLabels: Record<CommandPaletteAction['group'], string> = {
  navigation: '页面',
  recent: '最近打开',
  objects: '对象',
  actions: '操作',
};

export function CommandPalette({
  open,
  onClose,
  actions,
}: {
  open: boolean;
  onClose: () => void;
  actions: CommandPaletteAction[];
}) {
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);

  useEffect(() => {
    if (!open) {
      setQuery('');
      setActiveIndex(0);
    }
  }, [open]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return actions;
    return actions.filter(
      (action) =>
        action.label.toLowerCase().includes(needle) || action.keywords.some((keyword) => keyword.includes(needle)),
    );
  }, [actions, query]);

  useEffect(() => {
    if (activeIndex >= filtered.length) setActiveIndex(0);
  }, [filtered.length, activeIndex]);

  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        onClose();
      } else if (event.key === 'ArrowDown') {
        event.preventDefault();
        setActiveIndex((index) => (filtered.length === 0 ? 0 : (index + 1) % filtered.length));
      } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        setActiveIndex((index) => (filtered.length === 0 ? 0 : (index - 1 + filtered.length) % filtered.length));
      } else if (event.key === 'Enter') {
        const item = filtered[activeIndex];
        if (item) {
          event.preventDefault();
          item.run();
          onClose();
        }
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [open, onClose, filtered, activeIndex]);

  if (!open) return null;

  return (
    <>
      <div className="fixed inset-0 z-[130] bg-black/45" onClick={onClose} />
      <div className="fixed inset-0 z-[131] flex items-start justify-center px-4 pt-[12vh]">
        <div className="w-full max-w-[760px] overflow-hidden rounded-md border border-idpxyz-border bg-idpxyz-sidebar shadow-2xl">
          <div className="flex items-center gap-2 border-b border-idpxyz-border px-3 py-2">
            <Command size={14} className="text-idpxyz-textMuted" />
            <Search size={14} className="text-idpxyz-textMuted" />
            <Input
              autoFocus
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="输入页面名称，跳到那一页"
              className="h-8 border-0 bg-transparent px-0 type-body focus-visible:ring-0"
            />
            <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 type-caption text-idpxyz-textMuted">ESC</span>
          </div>
          <div className="max-h-[430px] overflow-y-auto p-1.5">
            {filtered.length === 0 ? (
              <div className="px-3 py-8 text-center type-body text-idpxyz-textMuted">没有匹配的页面</div>
            ) : (
              filtered.map((item, index) => {
                const Icon = item.icon || ArrowUpRight;
                const showGroup = index === 0 || filtered[index - 1].group !== item.group;
                return (
                  <div key={item.id}>
                    {showGroup && (
                      <div className="px-2 py-1 type-caption text-idpxyz-textMuted">
                        {groupLabels[item.group]}
                      </div>
                    )}
                    <Button
                      variant="ghost"
                      size="sm"
                      onMouseEnter={() => setActiveIndex(index)}
                      onClick={() => {
                        item.run();
                        onClose();
                      }}
                      className={`h-8 w-full justify-start rounded px-2 type-body ${index === activeIndex ? 'bg-idpxyz-activeItem text-idpxyz-textBright' : ''}`}
                    >
                      <Icon size={13} className="shrink-0" />
                      <span className="truncate">{item.label}</span>
                      {item.hint && <span className="ml-auto type-caption text-idpxyz-textMuted">{item.hint}</span>}
                    </Button>
                  </div>
                );
              })
            )}
          </div>
        </div>
      </div>
    </>
  );
}
