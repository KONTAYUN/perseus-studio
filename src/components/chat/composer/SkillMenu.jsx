import React from 'react';

export default function SkillMenu({ isOpen, skills, inputValue, onChoose }) {
  if (!isOpen || skills.length === 0) return null;

  return (
    <div className="composer-popover-surface motion-popover absolute bottom-[64px] left-2 z-30 max-h-56 w-[min(360px,calc(100%-24px))] overflow-auto border border-gray-200 bg-white p-2 dark:border-white/10 dark:bg-[#252527] custom-scrollbar">
      {skills.map((skill) => (
        <button
          key={skill.id}
          onClick={(e) => {
            e.stopPropagation();
            onChoose(skill, inputValue);
          }}
          className="motion-row block w-full rounded-xl px-3 py-2 text-left hover:bg-gray-100 dark:hover:bg-white/5"
        >
          <div className="truncate text-sm font-semibold text-gray-800 dark:text-gray-100">{skill.name}</div>
          <div className="mt-0.5 line-clamp-2 text-xs text-gray-500 dark:text-gray-400">{skill.description || '无描述'}</div>
        </button>
      ))}
    </div>
  );
}
