import React from 'react';

export default function PanelExitGhost({ ghost }) {
  const { rect, type } = ghost;
  const lineCount = type === 'fileTree' ? 4 : type === 'chat' ? 3 : 5;

  return (
    <div
      aria-hidden="true"
      className="panel-exit-ghost ui-panel-surface motion-panel-exit"
      style={{
        height: `${rect.height}px`,
        left: `${rect.left}px`,
        top: `${rect.top}px`,
        width: `${rect.width}px`,
      }}
    >
      <div className="panel-exit-ghost-header">
        <span className="panel-exit-ghost-dot" />
        <span className="panel-exit-ghost-tab" />
      </div>
      <div className="panel-exit-ghost-body">
        {Array.from({ length: lineCount }).map((_, index) => (
          <span
            key={index}
            className="panel-exit-ghost-line"
            style={{ width: `${42 + ((index * 17) % 34)}%` }}
          />
        ))}
      </div>
    </div>
  );
}
