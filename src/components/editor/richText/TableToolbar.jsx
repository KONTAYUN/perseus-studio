import React, { useRef, useState } from 'react';
import { ChevronDown, CornerDownRight, Table2, Trash2 } from 'lucide-react';
import { AnchoredPopover, Button, IconButton } from '../../ui';
import { alignTableColumn, paragraphOutsideTable, runTableAction, tableActionGroups } from './tableActions.js';

export default function TableToolbar({ editor, table, ownerId }) {
  const rowAnchor = useRef(null), columnAnchor = useRef(null);
  const [menu, setMenu] = useState('');
  const actions = tableActionGroups(table);
  const anchorRef = menu === 'rows' ? rowAnchor : columnAnchor;
  return <div className="rich-table-toolbar" role="toolbar" aria-label="表格编辑" data-testid="table-toolbar"
    data-rich-controls={ownerId} onMouseDown={event => event.preventDefault()}>
    <span className="rich-table-dimensions"><Table2 size={15} />{table.childCount} 行 × {table.firstChild.childCount} 列</span>
    <Button ref={rowAnchor} variant="ghost" size="xs" aria-label="调整表格行" aria-haspopup="menu" aria-expanded={menu === 'rows'}
      onClick={() => setMenu(value => value === 'rows' ? '' : 'rows')}>行<ChevronDown size={12} /></Button>
    <Button ref={columnAnchor} variant="ghost" size="xs" aria-label="调整表格列" aria-haspopup="menu" aria-expanded={menu === 'columns'}
      onClick={() => setMenu(value => value === 'columns' ? '' : 'columns')}>列<ChevronDown size={12} /></Button>
    <Button variant="ghost" size="xs" title="在表格后插入段落并继续写作（Ctrl/⌘+Enter）"
      onClick={() => paragraphOutsideTable(editor)}><CornerDownRight size={14} />继续写</Button>
    <span className="rich-table-hint">Enter 换行 · Tab 下一格</span>
    {['left', 'center', 'right'].map((alignment, index) => <Button key={alignment} variant="ghost" size="xs" title={`当前列${['左对齐', '居中', '右对齐'][index]}`} aria-label={`当前列${['左对齐', '居中', '右对齐'][index]}`}
      onClick={() => alignTableColumn(editor, alignment)}>{['左', '中', '右'][index]}</Button>)}
    <IconButton size="xs" title="删除表格（可撤销）" danger onClick={() => runTableAction(editor, 'deleteTable')}><Trash2 size={14} /></IconButton>
    <AnchoredPopover anchorRef={anchorRef} open={Boolean(menu)} onClose={() => setMenu('')} align="start"
      width={200} estimatedHeight={132} testId="table-action-menu" className="rich-block-menu">
      <div role="menu" aria-label={menu === 'rows' ? '表格行操作' : '表格列操作'} data-rich-controls={ownerId}
        onMouseDown={event => event.preventDefault()}>
        {(actions[menu] || []).map(([label, command, disabled]) => <button key={command} type="button" role="menuitem"
          className="rich-menu-item motion-row" disabled={Boolean(disabled)}
          title={disabled ? '至少保留一行和一列；如需移除，请删除表格' : undefined}
          onClick={() => { runTableAction(editor, command); setMenu(''); }}>{label}</button>)}
      </div>
    </AnchoredPopover>
  </div>;
}
