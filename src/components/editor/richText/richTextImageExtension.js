import { ReactNodeViewRenderer } from '@tiptap/react';
import Image from '@tiptap/extension-image';
import RichTextImageNodeView from './RichTextImageNodeView';

export function clampImageWidth(value) {
  return String(Math.min(100, Math.max(20, Number(value) || 100)));
}

export function normalizePixels(value, fallback = '') {
  if (value == null || value === '') return fallback;
  const numeric = Number(String(value).replace('px', ''));
  return Number.isFinite(numeric) && numeric >= 0 ? String(numeric) : fallback;
}

export function buildImageStyle(attributes = {}) {
  const align = attributes.align || 'center';
  const widthPercent = clampImageWidth(attributes.widthPercent || '100');
  const margin =
    align === 'left'
      ? 'margin-right:auto;margin-left:0;'
      : align === 'right'
        ? 'margin-left:auto;margin-right:0;'
        : 'margin-left:auto;margin-right:auto;';
  const borderStyle =
    attributes.borderStyle && attributes.borderStyle !== 'none' ? attributes.borderStyle : 'none';
  const borderWidth = normalizePixels(attributes.borderWidth, '0');
  const borderColor = attributes.borderColor || 'transparent';
  const backgroundColor = attributes.backgroundColor || 'transparent';
  const borderRadius = normalizePixels(attributes.borderRadius, '18');
  return [
    `width:${widthPercent}%`,
    'display:block',
    'max-width:100%',
    margin,
    `border-style:${borderStyle}`,
    `border-width:${borderWidth}px`,
    `border-color:${borderColor}`,
    `background-color:${backgroundColor}`,
    `border-radius:${borderRadius}px`,
  ].join(';');
}

const NoteImage = Image.extend({
  addNodeView() {
    return ReactNodeViewRenderer(RichTextImageNodeView);
  },

  addAttributes() {
    return {
      ...this.parent?.(),
      src: { default: null },
      alt: { default: null },
      title: { default: null },
      'data-note-src': { default: null },
      align: {
        default: 'center',
        parseHTML: (element) => element.getAttribute('data-align') || 'center',
        renderHTML: (attributes) => ({
          'data-align': attributes.align || 'center',
        }),
      },
      widthPercent: {
        default: '100',
        parseHTML: (element) =>
          clampImageWidth(
            element.getAttribute('data-width') || element.style.width?.replace('%', '') || '100'
          ),
        renderHTML: (attributes) => ({
          'data-width': clampImageWidth(attributes.widthPercent || '100'),
          style: buildImageStyle(attributes),
        }),
      },
      borderStyle: {
        default: 'none',
        parseHTML: (element) =>
          element.style.borderStyle || element.getAttribute('data-border-style') || 'none',
        renderHTML: (attributes) => ({
          'data-border-style': attributes.borderStyle || 'none',
        }),
      },
      borderColor: {
        default: '#cbd5e1',
        parseHTML: (element) =>
          element.style.borderColor || element.getAttribute('data-border-color') || '#cbd5e1',
        renderHTML: (attributes) => ({
          'data-border-color': attributes.borderColor || '#cbd5e1',
        }),
      },
      borderWidth: {
        default: '0',
        parseHTML: (element) =>
          normalizePixels(element.style.borderWidth || element.getAttribute('data-border-width'), '0'),
        renderHTML: (attributes) => ({
          'data-border-width': normalizePixels(attributes.borderWidth, '0'),
        }),
      },
      backgroundColor: {
        default: 'transparent',
        parseHTML: (element) =>
          element.style.backgroundColor ||
          element.getAttribute('data-background-color') ||
          'transparent',
        renderHTML: (attributes) => ({
          'data-background-color': attributes.backgroundColor || 'transparent',
        }),
      },
      borderRadius: {
        default: '18',
        parseHTML: (element) =>
          normalizePixels(
            element.style.borderRadius || element.getAttribute('data-border-radius'),
            '18'
          ),
        renderHTML: (attributes) => ({
          'data-border-radius': normalizePixels(attributes.borderRadius, '18'),
        }),
      },
    };
  },
});

export default NoteImage;
