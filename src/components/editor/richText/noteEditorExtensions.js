import { Decoration, DecorationSet } from '@tiptap/pm/view';
import { Extension } from '@tiptap/core';
import { Plugin, PluginKey } from '@tiptap/pm/state';

export const FONT_SIZE_OPTIONS = ['14', '16', '18', '20', '24', '28'];
export const LINE_HEIGHT_OPTIONS = ['1.6', '1.8', '2', '2.2'];
export const PARAGRAPH_SPACING_OPTIONS = ['0.75', '1', '1.25', '1.5'];

const pinnedSelectionPluginKey = 'notePinnedSelection';

export const FontSize = Extension.create({
  name: 'fontSize',

  addOptions() {
    return {
      types: ['textStyle'],
    };
  },

  addGlobalAttributes() {
    return [
      {
        types: this.options.types,
        attributes: {
          fontSize: {
            default: null,
            parseHTML: (element) => element.style.fontSize?.replace('px', '') || null,
            renderHTML: (attributes) => {
              if (!attributes.fontSize) return {};
              return { style: `font-size: ${attributes.fontSize}px` };
            },
          },
        },
      },
    ];
  },

  addCommands() {
    return {
      setFontSize:
        (fontSize) =>
        ({ chain }) =>
          chain().setMark('textStyle', { fontSize }).run(),
      unsetFontSize:
        () =>
        ({ chain }) =>
          chain().setMark('textStyle', { fontSize: null }).removeEmptyTextStyle().run(),
    };
  },
});

export const BlockSpacing = Extension.create({
  name: 'blockSpacing',

  addOptions() {
    return {
      types: ['paragraph', 'heading', 'blockquote'],
    };
  },

  addGlobalAttributes() {
    return [
      {
        types: this.options.types,
        attributes: {
          lineHeight: {
            default: null,
            parseHTML: (element) => element.style.lineHeight || null,
            renderHTML: (attributes) => {
              if (!attributes.lineHeight) return {};
              return { style: `line-height: ${attributes.lineHeight}` };
            },
          },
          paragraphSpacing: {
            default: null,
            parseHTML: (element) => {
              const value = element.style.marginBottom || '';
              return value ? value.replace('em', '') : null;
            },
            renderHTML: (attributes) => {
              if (!attributes.paragraphSpacing) return {};
              return { style: `margin-bottom: ${attributes.paragraphSpacing}em` };
            },
          },
        },
      },
    ];
  },

  addCommands() {
    const updateAll = (attributes) => ({ chain }) =>
      chain()
        .updateAttributes('paragraph', attributes)
        .updateAttributes('heading', attributes)
        .updateAttributes('blockquote', attributes)
        .run();

    return {
      setLineHeight: (lineHeight) => updateAll({ lineHeight }),
      unsetLineHeight: () => updateAll({ lineHeight: null }),
      setParagraphSpacing: (paragraphSpacing) => updateAll({ paragraphSpacing }),
      unsetParagraphSpacing: () => updateAll({ paragraphSpacing: null }),
    };
  },
});

export const PinnedSelection = Extension.create({
  name: 'pinnedSelection',

  addStorage() {
    return {
      ranges: [],
    };
  },

  addCommands() {
    return {
      setPinnedSelections:
        (ranges) =>
        ({ editor, tr, dispatch }) => {
          const nextRanges = normalizeSelectionRanges(ranges);
          editor.storage.pinnedSelection.ranges = nextRanges;
          if (dispatch) {
            dispatch(tr.setMeta(pinnedSelectionPluginKey, { ranges: nextRanges }));
          }
          return true;
        },
      addPinnedSelection:
        (range) =>
        ({ editor, tr, dispatch }) => {
          const nextRanges = normalizeSelectionRanges([
            ...editor.storage.pinnedSelection.ranges,
            range,
          ]);
          editor.storage.pinnedSelection.ranges = nextRanges;
          if (dispatch) {
            dispatch(tr.setMeta(pinnedSelectionPluginKey, { ranges: nextRanges }));
          }
          return true;
        },
      removePinnedSelection:
        (range) =>
        ({ editor, tr, dispatch }) => {
          const nextRanges = removeSelectionRange(editor.storage.pinnedSelection.ranges, range);
          editor.storage.pinnedSelection.ranges = nextRanges;
          if (dispatch) {
            dispatch(tr.setMeta(pinnedSelectionPluginKey, { ranges: nextRanges }));
          }
          return true;
        },
      clearPinnedSelections:
        () =>
        ({ editor, tr, dispatch }) => {
          editor.storage.pinnedSelection.ranges = [];
          if (dispatch) {
            dispatch(tr.setMeta(pinnedSelectionPluginKey, { ranges: [] }));
          }
          return true;
        },
    };
  },

  addProseMirrorPlugins() {
    const extension = this;
    return [
      new Plugin({
        key: new PluginKey(pinnedSelectionPluginKey),
        state: {
          init: () => [],
          apply(tr, value) {
            const meta = tr.getMeta(pinnedSelectionPluginKey);
            if (meta) {
              const nextRanges = normalizeSelectionRanges(meta.ranges);
              extension.storage.ranges = nextRanges;
              return nextRanges;
            }
            if (!Array.isArray(value) || value.length === 0) return [];
            const mappedRanges = normalizeSelectionRanges(
              value
                .map((range) => ({
                  from: tr.mapping.map(range.from),
                  to: tr.mapping.map(range.to),
                }))
                .filter((range) => range.from < range.to)
            );
            extension.storage.ranges = mappedRanges;
            return mappedRanges;
          },
        },
        props: {
          decorations(state) {
            const ranges = this.getState(state);
            if (!Array.isArray(ranges) || ranges.length === 0) return DecorationSet.empty;
            return DecorationSet.create(
              state.doc,
              ranges.map((range) =>
                Decoration.inline(range.from, range.to, {
                  class: 'rich-text-ai-selection',
                })
              )
            );
          },
        },
      }),
    ];
  },
});

function normalizeSelectionRanges(ranges) {
  if (!Array.isArray(ranges)) return [];
  const normalized = ranges
    .map((range) => ({
      from: Number(range?.from) || 0,
      to: Number(range?.to) || 0,
    }))
    .filter((range) => range.from > 0 && range.to > range.from)
    .sort((left, right) => left.from - right.from);

  const merged = [];
  for (const range of normalized) {
    const previous = merged.at(-1);
    if (!previous) {
      merged.push(range);
      continue;
    }
    if (range.from <= previous.to) {
      previous.to = Math.max(previous.to, range.to);
      continue;
    }
    merged.push(range);
  }
  return merged;
}

function removeSelectionRange(ranges, target) {
  if (!target) return normalizeSelectionRanges(ranges);
  return normalizeSelectionRanges(ranges).filter(
    (range) => !(range.from === target.from && range.to === target.to)
  );
}

export function getActiveFontSize(editor) {
  const value = editor?.getAttributes('textStyle')?.fontSize;
  return value || '16';
}

export function getActiveLineHeight(editor) {
  return (
    editor?.getAttributes('paragraph')?.lineHeight ||
    editor?.getAttributes('heading')?.lineHeight ||
    editor?.getAttributes('blockquote')?.lineHeight ||
    '1.8'
  );
}

export function getActiveParagraphSpacing(editor) {
  return (
    editor?.getAttributes('paragraph')?.paragraphSpacing ||
    editor?.getAttributes('heading')?.paragraphSpacing ||
    editor?.getAttributes('blockquote')?.paragraphSpacing ||
    '1'
  );
}
