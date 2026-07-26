import React from 'react';
import { Button } from '../ui';
import ChatMarkdownRenderer from './markdown/ChatMarkdownRenderer';

function UserRequestMessage({ request, ide, isActive = false }) {
  const waiting = request.status === 'waiting_for_user';
  const interactive = waiting && isActive && !ide.isChatSending;
  const isChoice = request.requestKind === 'choice' || request.requestKind === 'legacy_followup';

  return (
    <div className="user-request-message min-w-0">
      <ChatMarkdownRenderer content={request.question} />
      {isChoice && interactive && request.options.length > 0 && (
        <div className="mt-3 flex flex-col gap-2">
          {request.options.map((option) => (
            <Button
              key={option}
              type="button"
              variant="subtle"
              size="sm"
              className="min-w-0 justify-start text-left font-semibold"
              onClick={(event) => {
                event.stopPropagation();
                ide.sendUserResponse?.(request, option);
              }}
            >
              <span className="min-w-0 whitespace-normal break-words">{option}</span>
            </Button>
          ))}
        </div>
      )}
      {!isChoice && waiting && isActive && (
        <div className="motion-fade-reveal mt-2 flex items-center gap-1.5 text-[11px] font-semibold text-gray-400 dark:text-gray-500">
          <span className="h-1.5 w-1.5 rounded-full bg-current" />
          <span>等待回复</span>
        </div>
      )}
    </div>
  );
}

export default React.memo(UserRequestMessage);
