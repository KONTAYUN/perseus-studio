import { getNoteTitle, isNotePath } from '../../hooks/noteTreeUtils.js';

export default function FileTreeLabel({ item }) {
  const label = item.type === 'file' && isNotePath(item.path) ? getNoteTitle(item.path) : item.name;
  return (
    <span className="file-tree-label min-w-0 select-none" title={item.name}>
      <span className="file-tree-label-text">{label}</span>
    </span>
  );
}
