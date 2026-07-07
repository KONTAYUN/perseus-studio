export default function DropInsertionLine({ depth }) {
  return (
    <div
      data-testid="file-drop-indicator"
      className="pointer-events-none relative z-20 h-0"
      style={{ marginLeft: `${depth * 12 + 28}px`, marginRight: '8px' }}
    >
      <div className="file-drop-indicator-line -translate-y-1/2 rounded-full border-t-2 border-blue-500 dark:border-blue-300" />
    </div>
  );
}
