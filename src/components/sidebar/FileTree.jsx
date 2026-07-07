import FileTreeRow from './FileTreeRow';

export default function FileTree({ items, depth = 0, ide }) {
  return items.map((item) => (
    <FileTreeRow key={item.id} item={item} items={items} depth={depth} ide={ide} />
  ));
}
