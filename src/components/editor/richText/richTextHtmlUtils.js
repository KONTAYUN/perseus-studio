import { workspaceFileContentUrl } from '../../../api/client';
import { resolveNoteAssetPath } from '../../../hooks/noteTreeUtils.js';

export function sanitizeTableMarkup(root) {
  root.querySelectorAll('table').forEach((table) => {
    const rows = Array.from(table.querySelectorAll('tr'));
    if (rows.length > 0) {
      const carry = [];
      let totalColumns = 0;

      rows.forEach((row) => {
        const cells = Array.from(row.children).filter((node) =>
          ['TD', 'TH'].includes(node.tagName)
        );
        let columnIndex = 0;

        cells.forEach((cell) => {
          while (carry[columnIndex] > 0) columnIndex += 1;
          const colspan = Math.max(1, Number(cell.getAttribute('colspan')) || 1);
          const rowspan = Math.max(1, Number(cell.getAttribute('rowspan')) || 1);
          for (let offset = 0; offset < colspan; offset += 1) {
            carry[columnIndex + offset] = Math.max(carry[columnIndex + offset] || 0, rowspan);
          }
          columnIndex += colspan;
        });

        const activeColumns = carry.reduce(
          (last, span, index) => (span > 0 ? index + 1 : last),
          columnIndex
        );
        totalColumns = Math.max(totalColumns, activeColumns);

        for (let index = 0; index < carry.length; index += 1) {
          if (carry[index] > 0) carry[index] -= 1;
        }
      });

      const fillCarry = [];
      rows.forEach((row) => {
        const cells = Array.from(row.children).filter((node) =>
          ['TD', 'TH'].includes(node.tagName)
        );
        let columnIndex = 0;

        cells.forEach((cell) => {
          while (fillCarry[columnIndex] > 0) columnIndex += 1;
          const colspan = Math.max(1, Number(cell.getAttribute('colspan')) || 1);
          const rowspan = Math.max(1, Number(cell.getAttribute('rowspan')) || 1);
          for (let offset = 0; offset < colspan; offset += 1) {
            fillCarry[columnIndex + offset] = Math.max(
              fillCarry[columnIndex + offset] || 0,
              rowspan
            );
          }
          columnIndex += colspan;
        });

        while (columnIndex < totalColumns) {
          while (fillCarry[columnIndex] > 0) columnIndex += 1;
          if (columnIndex >= totalColumns) break;
          const filler = row.ownerDocument.createElement('td');
          filler.appendChild(row.ownerDocument.createElement('p'));
          row.appendChild(filler);
          fillCarry[columnIndex] = Math.max(fillCarry[columnIndex] || 0, 1);
          columnIndex += 1;
        }

        for (let index = 0; index < fillCarry.length; index += 1) {
          if (fillCarry[index] > 0) fillCarry[index] -= 1;
        }
      });
    }

    table.querySelectorAll('colgroup').forEach((node) => node.remove());
    table.style.removeProperty('border');
    table.style.removeProperty('border-color');
    table.style.removeProperty('border-style');
    table.style.removeProperty('border-width');
    table.removeAttribute('data-table-width');
    table.removeAttribute('data-table-align');
    table.removeAttribute('data-border-style');
    table.removeAttribute('data-border-color');
    table.removeAttribute('data-border-width');
    table.removeAttribute('data-background-color');
    table.querySelectorAll('th, td').forEach((cell) => {
      cell.removeAttribute('colwidth');
      cell.removeAttribute('data-cell-width');
      cell.removeAttribute('data-border-style');
      cell.removeAttribute('data-border-color');
      cell.removeAttribute('data-border-width');
      cell.removeAttribute('data-background-color');
      cell.removeAttribute('data-cell-padding');
      cell.removeAttribute('data-cell-text-align');
      cell.removeAttribute('data-cell-vertical-align');
      cell.style.removeProperty('width');
      cell.style.removeProperty('min-width');
      cell.style.removeProperty('max-width');
      cell.style.removeProperty('border');
      cell.style.removeProperty('border-color');
      cell.style.removeProperty('border-style');
      cell.style.removeProperty('border-width');
    });
    table.querySelectorAll('tr').forEach((row) => {
      row.removeAttribute('data-row-height');
      row.style.removeProperty('height');
    });
  });
}

export function decorateHtmlWithAssetUrls(notePath, html) {
  if (typeof window === 'undefined') return html;
  const template = window.document.createElement('template');
  template.innerHTML = html || '';
  sanitizeTableMarkup(template.content);
  template.content.querySelectorAll('img').forEach((image) => {
    const original = image.getAttribute('data-note-src') || image.getAttribute('src') || '';
    const resolvedPath = resolveNoteAssetPath(notePath, original);
    image.setAttribute('data-note-src', original);
    if (resolvedPath) {
      image.setAttribute('src', workspaceFileContentUrl(resolvedPath));
    }
  });
  return template.innerHTML;
}

export function getSanitizedEditorHtml(editor) {
  if (typeof window === 'undefined') return editor.getHTML();
  const template = window.document.createElement('template');
  template.innerHTML = editor.getHTML();
  sanitizeTableMarkup(template.content);
  return template.innerHTML;
}
