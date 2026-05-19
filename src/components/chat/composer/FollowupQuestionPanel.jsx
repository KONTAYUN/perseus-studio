import React from 'react';
import { Sparkles } from 'lucide-react';
import { Button } from '../../ui';

export default function FollowupQuestionPanel({ question, disabled, onChoose }) {
  return (
    <div className="followup-question-panel motion-popover pointer-events-auto mb-3">
      <div className="flex items-start gap-3">
        <div className="followup-question-icon">
          <Sparkles size={15} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-[11px] font-bold uppercase tracking-wide text-blue-500/80 dark:text-blue-200/80">
            需要你确认
          </div>
          <div className="mt-1 whitespace-pre-wrap break-words text-sm font-bold leading-6 text-gray-800 dark:text-gray-100">
            {question.question}
          </div>
          {question.options.length > 0 && (
            <div className="mt-3 flex flex-wrap gap-2">
              {question.options.map((option) => (
                <Button
                  key={option}
                  type="button"
                  variant="subtle"
                  size="xs"
                  disabled={disabled}
                  className="followup-question-option"
                  onClick={(e) => {
                    e.stopPropagation();
                    onChoose?.(option);
                  }}
                >
                  {option}
                </Button>
              ))}
            </div>
          )}
          {question.allowFreeform && (
            <div className="mt-2 text-[11px] font-medium text-gray-400 dark:text-gray-500">
              也可以直接在下方输入自己的答案。
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
