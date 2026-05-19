import React from 'react';
import { File, MessageSquare, X } from 'lucide-react';
import { IconButton } from '../../ui';

export default function ComposerReferences({
  fileReferences,
  skillReferences,
  onRemoveFileReference,
  onRemoveSkillReference,
}) {
  if (fileReferences.length === 0 && skillReferences.length === 0) return null;

  return (
    <div className="mb-2 flex flex-wrap gap-2 px-1.5 pt-0.5">
      {fileReferences.map((item) => (
        <ReferenceChip
          key={item.path}
          icon={<File size={13} />}
          title={item.name}
          subtitle={item.path}
          onRemove={() => onRemoveFileReference(item.path)}
        />
      ))}
      {skillReferences.map((item) => (
        <ReferenceChip
          key={item.skill_id}
          icon={<MessageSquare size={13} />}
          title={item.name}
          subtitle="Skill"
          onRemove={() => onRemoveSkillReference(item.skill_id)}
        />
      ))}
    </div>
  );
}

function ReferenceChip({ icon, title, subtitle, onRemove }) {
  return (
    <div className="motion-chip flex max-w-full items-center gap-2 rounded-2xl border border-blue-200/70 bg-blue-50/80 px-3 py-1.5 text-xs text-blue-700 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-200">
      <span className="shrink-0 text-blue-500">{icon}</span>
      <span className="min-w-0">
        <span className="block max-w-[180px] truncate font-semibold">{title}</span>
        <span className="block max-w-[180px] truncate text-[10px] opacity-70">{subtitle}</span>
      </span>
      <IconButton
        size="compact"
        variant="danger"
        onClick={(e) => {
          e.stopPropagation();
          onRemove();
        }}
        title="移除引用"
      >
        <X size={12} />
      </IconButton>
    </div>
  );
}
