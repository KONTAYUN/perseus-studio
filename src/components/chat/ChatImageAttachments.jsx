import React from 'react';
import { createPortal } from 'react-dom';
import { ChevronLeft, ChevronRight, Download, ImageOff, Maximize2, Minimize2, X } from 'lucide-react';
import useMotionPresence from '../../hooks/useMotionPresence';
import { Button, IconButton } from '../ui';
import { formatFileSize } from './chatUtils';

export default function ChatImageAttachments({ images, messageId }) {
  const [previewIndex, setPreviewIndex] = React.useState(-1);
  const columns = images.length === 1 ? 'chat-image-grid-single' : 'chat-image-grid-multiple';
  return (
    <>
      <div className={`chat-image-grid ${columns}`}>
        {images.map((item, index) => (
          <ChatImageThumbnail
            item={item}
            key={item.id || `${messageId}-image-${index}`}
            onOpen={() => item.available && setPreviewIndex(index)}
          />
        ))}
      </div>
      <MediaPreviewDialog
        images={images}
        index={previewIndex}
        onIndexChange={setPreviewIndex}
        onClose={() => setPreviewIndex(-1)}
      />
    </>
  );
}

function ChatImageThumbnail({ item, onOpen }) {
  const [failed, setFailed] = React.useState(false);
  const available = item.available === true && Boolean(item.content_url) && !failed;
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className={`chat-image-thumbnail ${available ? 'chat-image-thumbnail-available' : 'chat-image-thumbnail-missing'}`}
      onClick={(event) => {
        event.stopPropagation();
        if (available) onOpen();
      }}
      disabled={!available}
      title={available ? item.name : '旧图片未保存，无法预览'}
    >
      {available ? (
        <img
          src={item.content_url}
          alt={item.name || '聊天图片'}
          loading="lazy"
          width={item.width || undefined}
          height={item.height || undefined}
          onError={() => setFailed(true)}
        />
      ) : (
        <span className="chat-image-missing-state">
          <ImageOff size={18} />
          <span>{item.name || '图片'}</span>
          <small>旧图片未保存，无法预览</small>
        </span>
      )}
    </Button>
  );
}

function MediaPreviewDialog({ images, index, onIndexChange, onClose }) {
  const open = index >= 0 && index < images.length;
  const { closeWithPresence, exiting, present } = useMotionPresence(open, { exitToken: '--motion-modal' });
  const latestRef = React.useRef(null);
  const [originalSize, setOriginalSize] = React.useState(false);
  const [failed, setFailed] = React.useState(false);

  if (open) latestRef.current = { item: images[index], index };
  const current = open ? { item: images[index], index } : latestRef.current;
  const requestClose = React.useCallback(() => closeWithPresence(onClose), [closeWithPresence, onClose]);

  React.useEffect(() => {
    if (!present) return undefined;
    const onKeyDown = (event) => {
      if (event.key === 'Escape') requestClose();
      if (event.key === 'ArrowLeft' && images.length > 1) onIndexChange((current?.index - 1 + images.length) % images.length);
      if (event.key === 'ArrowRight' && images.length > 1) onIndexChange((current?.index + 1) % images.length);
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [current?.index, images.length, onIndexChange, present, requestClose]);

  React.useEffect(() => {
    setOriginalSize(false);
    setFailed(false);
  }, [current?.item?.id]);

  if (!present || !current?.item) return null;
  const item = current.item;
  const content = (
    <div
      className={`media-preview-backdrop modal-backdrop motion-fade-reveal fixed inset-0 z-[520] flex items-center justify-center bg-black/70 p-4 backdrop-blur-xl ${exiting ? 'opacity-0' : ''}`}
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) requestClose();
      }}
    >
      <div className={`media-preview-dialog ui-overlay-surface motion-modal flex h-[min(90vh,960px)] w-[min(94vw,1440px)] flex-col overflow-hidden border border-white/15 bg-black/80 ${exiting ? 'motion-presence-exit motion-presence-exit-modal' : ''}`}>
        <div className="media-preview-toolbar">
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-semibold text-white">{item.name || '图片'}</div>
            <div className="text-[11px] text-white/55">
              {item.width && item.height ? `${item.width} x ${item.height} · ` : ''}{formatFileSize(item.size)}
            </div>
          </div>
          <IconButton
            onClick={() => setOriginalSize((value) => !value)}
            title={originalSize ? '适应窗口' : '查看原始尺寸'}
            className="text-white"
          >
            {originalSize ? <Minimize2 size={17} /> : <Maximize2 size={17} />}
          </IconButton>
          <a href={item.download_url} download={item.name} className="ui-icon-button ui-icon-button-default ui-icon-button-md text-white" title="下载原图">
            <Download size={17} />
          </a>
          <IconButton onClick={requestClose} title="关闭" className="text-white">
            <X size={18} />
          </IconButton>
        </div>
        <div className="media-preview-stage custom-scrollbar">
          {failed ? (
            <div className="media-preview-error"><ImageOff size={28} /><span>图片加载失败</span></div>
          ) : (
            <img
              src={item.content_url}
              alt={item.name || '聊天图片'}
              key={item.id || item.content_url}
              className={`${originalSize ? 'media-preview-image-original' : 'media-preview-image-fit'} motion-content-switch`}
              onError={() => setFailed(true)}
            />
          )}
        </div>
        {images.length > 1 && (
          <>
            <IconButton
              className="media-preview-nav media-preview-nav-left text-white"
              onClick={() => onIndexChange((current.index - 1 + images.length) % images.length)}
              title="上一张"
            >
              <ChevronLeft size={22} />
            </IconButton>
            <IconButton
              className="media-preview-nav media-preview-nav-right text-white"
              onClick={() => onIndexChange((current.index + 1) % images.length)}
              title="下一张"
            >
              <ChevronRight size={22} />
            </IconButton>
          </>
        )}
      </div>
    </div>
  );
  return createPortal(content, document.body);
}
