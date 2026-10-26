import { Button } from '@idpxyz/ui-primitives';
import type { ListColumn } from '../../templates';

// 目录行上的「发新版本」按钮列（票 party-commercial-uiux/14）。点了把这一版带去发布表单预填；按钮不吞行点击——表格行
// 若自己能点开详情，点按钮不该同时开详情。
//
// 读面带状态的册另在同一列里给「收尾…」（票 party-commercial-uiux/16）：发了新版本之后让旧版让位。收尾与发新版本同在一列，
// 不另开第二个「操作」列。

export function withNewVersionAction<Row>(
  columns: ListColumn<Row>[],
  options: {
    /** 这一行能不能发新版本；不能就不画按钮（如随所属版本发布的两本册）。 */
    canStart?: (row: Row) => boolean;
    /** 读屏用的名字，例「信用政策 SYN-CREDIT-01 v1」。 */
    labelOf: (row: Row) => string;
    onStart: (row: Row) => void;
    /** 给了就在「发新版本」旁加「收尾…」；`canClose` 为假的行不画。 */
    closure?: {
      canClose: (row: Row) => boolean;
      onClose: (row: Row) => void;
    };
  },
): ListColumn<Row>[] {
  return [
    ...columns,
    {
      id: 'new-version',
      header: '操作',
      render: (row: Row) => {
        const canStart = !options.canStart || options.canStart(row);
        const canClose = options.closure?.canClose(row) ?? false;
        if (!canStart && !canClose) return null;
        return (
          <div className="flex items-center gap-1.5">
            {canStart ? (
              <Button
                variant="outline"
                size="sm"
                className="whitespace-nowrap"
                title="用这一版的内容预填发布表单，发布一个新版本；会替换该表单里已填的草稿"
                aria-label={`基于 ${options.labelOf(row)} 发新版本`}
                onClick={(event) => {
                  event.stopPropagation();
                  options.onStart(row);
                }}
              >
                发新版本
              </Button>
            ) : null}
            {canClose ? (
              <Button
                variant="outline"
                size="sm"
                className="whitespace-nowrap"
                title="这一版已有更新的版本接手：让它退出新的选择（截止或退役），消除重叠造成的适用冲突"
                aria-label={`收尾 ${options.labelOf(row)}`}
                onClick={(event) => {
                  event.stopPropagation();
                  options.closure?.onClose(row);
                }}
              >
                收尾…
              </Button>
            ) : null}
          </div>
        );
      },
    },
  ];
}
