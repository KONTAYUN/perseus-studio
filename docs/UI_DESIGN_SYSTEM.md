# Perseus Studio UI Design System

This document is the source of truth for Perseus Studio frontend UI decisions. Read it before changing buttons, inputs, menus, panels, chat UI, file tree UI, settings UI, animation, color, spacing, or border radius.

Perseus Studio is a browser-based AI Web IDE. The interface should feel like a polished work tool: calm, precise, dense enough for repeated use, and visually consistent. Do not treat UI work as isolated one-off Tailwind tweaks.

## Goals

- Keep the product visually coherent across chat, editor, file tree, settings, tasks, dialogs, and admin views.
- Make future vibe-coding sessions safe: a new chat window should be able to read this document and continue the same UI language.
- Reduce UI debt by moving repeated styling into shared classes or shared components.
- Prefer stable, Apple-like interaction: clear feedback, no jitter, no layout shift, no decorative noise.

## Current Reality

The project has a functioning UI, but many older controls were built directly with long Tailwind class strings inside components. The design system is currently emerging from:

- CSS tokens and utilities in `src/styles/*.css`, imported by `src/index.css`
- shared settings primitives in `src/components/settings/SettingsUI.jsx`
- repeated panel/header patterns in component files

When touching UI, prefer consolidating styles instead of adding another one-off class string.

## Design Principles

1. **Consistency beats local cleverness.** If two controls do the same job, they should look and move the same way.
2. **Stable geometry.** Hover and active states must not resize the layout, shift neighboring controls, or make icons jump.
3. **Subtle motion.** Motion should confirm interaction, not call attention to itself.
4. **Operational density.** This is an IDE, not a marketing page. Avoid oversized decorative sections and card-heavy layouts.
5. **Flat depth by default.** Use light translucency, subtle borders, and inset boundaries. Avoid stacked, raised, or layered controls unless the surface is a true modal/popover.
6. **Dark mode parity.** Every new visible state needs a dark-mode equivalent.
7. **Component first.** Repeated UI should become a shared class or component before it spreads.

## Source Of Truth

Use these before inventing new styling:

- `src/index.css` plus `src/styles/*.css`: ordered style entrypoint, design tokens, radius scale, shared motion, panel surfaces, icon buttons, composer controls.
- `src/components/ui/index.jsx`: shared React primitives for buttons, icon buttons, inputs, selects, checkboxes, badges, popovers, dialog actions, settings sections, panel headers, and switches.
- `src/components/settings/SettingsUI.jsx`: existing shared form/card primitives. This should gradually become or move into a broader shared UI layer.
- `src/components/common/AppDialog.jsx`: modal/dialog direction.
- `src/components/layout/AppHeader.jsx`, `src/components/sidebar/SidebarPanel.jsx`, `src/components/chat/ChatPanel.jsx`, `src/components/editor/EditorPanel.jsx`: current panel-header patterns.

Reusable primitives now live in `src/components/ui/*`. Business components should prefer those primitives before writing raw controls.

## Shared Components

Import shared primitives from `src/components/ui` for app-wide UI, or from `src/components/settings/SettingsUI.jsx` inside settings screens when you need the existing settings aliases.

Available primitives:

- `Button`: variants `primary`, `secondary`, `danger`, `warning`, `ghost`, `subtle`, `dangerSubtle`; sizes `xs`, `sm`, `md`, `lg`; supports `fullWidth`.
- `IconButton`: sizes `compact`, `xs`, `sm`, `md`; variants `default`, `active`, `danger`, `warning`, `dangerState`; accepts legacy `danger`.
- `Input`, `Textarea`, `Select`, `Checkbox`, `Switch`, `Badge`.
- `AnchoredPopover`, `PopoverSurface`, `DialogActions`, `SettingsSection`, `PanelHeader`.

Use raw `<button>` only when a component needs special semantics such as a full-row menu item or draggable chip. It must include a standard class such as `motion-button`, `motion-row`, `motion-chip`, `ui-icon-button`, `settings-nav-card`, or `composer-icon-button`.

## Tokens

Defined in `src/styles/tokens.css` and imported by `src/index.css`:

| Token | Purpose |
|---|---|
| `--ease-apple` | Default interaction easing |
| `--ease-apple-soft` | Larger panel/modal entrance easing |
| `--radius-micro` | Tiny controls, compact icon buttons |
| `--radius-control-sm` | Small buttons and compact controls |
| `--radius-control` | Standard inputs/buttons |
| `--radius-card` | Cards and segmented controls |
| `--radius-panel` | Main panel surfaces |
| `--radius-shell` | Larger overlays/shells |
| `--radius-composer` | Chat composer shell |
| `--radius-icon` | Standard icon button radius |
| `--radius-segmented` | Segmented control outer radius |
| `--radius-segmented-option` | Segmented control inner option radius |
| `--radius-switch` | Switch track radius |
| `--surface-line` | Subtle border color |
| `--surface-highlight` | Inset surface highlight |
| `--surface-shadow-soft` | Soft depth |
| `--surface-shadow-panel` | Main panel depth |

Do not hardcode new radius scales unless a component has a proven special need. Prefer these tokens.

## Radius Rules

- Main panels: `ui-panel-surface`, usually `--radius-panel`.
- Overlays/modals: `ui-overlay-surface`, usually `--radius-shell`.
- Cards: `--radius-card`.
- Inputs and regular controls: `--radius-control` or `--radius-control-sm`.
- Icon buttons: `--radius-icon`; compact icon buttons use `--radius-micro`.
- Chat composer: `--radius-composer`.
- Segmented controls: use `SegmentedControl`, `--radius-segmented`, and `--radius-segmented-option`.
- Switches: use `Switch`; do not build track/thumb geometry inline.

Avoid mixing very round controls with square-ish neighboring controls. Controls in the same cluster must share a compatible radius family.

Nested radius rule:

- When a rounded child sits inside a rounded parent, the child radius should visually equal `outer radius - inset`.
- Example: a segmented control with `18px` outer radius and `4px` padding uses a `14px` option radius.
- Inset must be equal on all sides unless the child is intentionally full-bleed.
- Do not hand-build `inline-flex rounded-2xl ... p-0.5` segmented controls; use `SegmentedControl`.
- Do not hand-build `relative h-6 w-11 rounded-full` switches; use `Switch`.

Curvature quality rules:

- A control cluster must read as one designed object. Outer shell, inner buttons, and gaps must use the same inset math.
- If a child is inset by `4px`, its radius should be about `4px` smaller than the parent radius.
- Horizontal and vertical inset must be visually equal. Do not let the straight middle section look narrower than the rounded ends.
- Do not mix pill controls with square-ish controls in the same compact cluster.
- Do not fix a radius mismatch by moving controls closer together. Fix the parent radius, child radius, padding, and gap.

## Depth Rules

Perseus Studio now uses a flat, low-depth control language.

- Regular buttons, icon buttons, tabs, chips, inputs, cards, and panel header controls should not cast visible outer shadows.
- Hover should change color, border, or background. It should not add elevation.
- Active/focus states should use rings, borders, or subtle inset boundaries.
- Main panels may use `--surface-shadow-panel`; composer/empty states may use named low-depth classes.
- Popovers and modals may use a soft shadow, but keep it lighter than traditional `shadow-xl`/`shadow-2xl`.
- Functional indicators such as file drag insertion lines may use a small glow, but the glow must live in a named CSS class.

Never add Tailwind `shadow-sm`, `shadow-md`, `shadow-lg`, `shadow-xl`, `shadow-2xl`, `hover:shadow-*`, or arbitrary `shadow-[...]` in JSX. If depth is truly needed, create or reuse a semantic class in the matching `src/styles/*.css` section and document why.

## Color Rules

Primary action color is blue. Use it for selected states, primary actions, and important hover states.

Tone usage:

- Blue: primary action, selected, current, focus.
- Red: destructive action only.
- Amber: warning, compaction/risk states.
- Green/Emerald: success/done states.
- Gray: neutral, inactive, metadata, utility actions.

Rules:

- Do not create one-off blues/reds if an existing tone works.
- Do not use strong saturation for default inactive controls.
- Do not make the UI read as a single-color theme.
- Dark mode should use translucent surfaces and softer text, not pure black/white everywhere.

## Motion Rules

Motion must be predictable and centered.

- Default duration for small controls: about `180ms`.
- Popovers: about `260-320ms`.
- Modals/panels: about `420-900ms`, depending on size.
- Easing: use `--ease-apple` or `--ease-apple-soft`.

Hard rules:

- Icon button outer shells must not translate or scale on hover.
- Text button outer shells must not scale on hover.
- Icon buttons may scale the inner SVG slightly around its center.
- List rows and menu rows should not slide sideways on hover.
- Do not combine parent transform + child transform + action-group reveal for the same target.
- Do not use hover motion that changes perceived alignment, gaps, or rounded-corner fit.
- Avoid `hover:-translate-y-*` for buttons and cards unless there is a clear design-system exception.

Current shared classes:

- `motion-button`: standard interaction transition.
- `ui-icon-button`: stable icon-button shell.
- `ui-icon-button-danger`: destructive icon-button hover tone.
- `ui-icon-button-active`: active/selected icon-button state.
- `ui-panel-drag-handle`: panel drag handle style.
- `motion-row`: row color/shadow transition; no hover movement.
- `motion-popover`, `motion-popover-right`, `motion-modal`: overlay entrance animation.

## Button Standards

### Icon Buttons

Use for toolbar icons, close buttons, file tree actions, message actions, task actions, settings icon actions.

Required pattern:

```jsx
<IconButton title="...">
  <Icon size={15} />
</IconButton>
```

Rules:

- Use fixed square dimensions: commonly `h-7 w-7`, `h-8 w-8`, or `h-9 w-9`.
- Use `ui-icon-button-danger` for destructive actions.
- Use `ui-icon-button-active` for selected/toggled states.
- Use `ui-icon-button-compact` for very small row actions.
- Prefer lucide icons.
- Provide `title` or `aria-label`. If adding a custom tooltip, do not also show a native browser tooltip.

### Text Buttons

Use text buttons for explicit commands such as save, cancel, create, confirm, compact, download, and submit.

Rules:

- Primary action: blue background, white text.
- Secondary action: neutral background, gray text.
- Destructive action: red tone.
- Disabled states must visibly mute color and remove transform.
- Button text must fit on mobile; do not rely on viewport-scaled font sizes.

### Segmented Controls

Use for mutually exclusive modes. Keep active state visually clear with blue text/background or a raised neutral active segment.

Rules:

- Use `SegmentedControl`.
- Segment container radius uses `--radius-segmented`.
- Segment buttons use `--radius-segmented-option`.
- Container padding and child radius must follow the nested radius rule.
- Do not add individual card surfaces inside segmented controls.

## Input And Form Standards

Use existing `Input`, `Textarea`, `Select`, `Field`, `Checkbox`, `Switch`, `Badge`, `ProviderCard`, `Button`, and `IconButton` primitives where applicable.

Rules:

- Standard input height: `h-10` or `h-11`.
- Focus uses blue border/ring.
- Error states use red border/ring and short explanatory text.
- Selects must be custom-styled consistently with inputs.
- Native browser controls are allowed only when styling remains coherent.
- Avoid placing floating icon buttons where they block typed text. Add safe padding/insets when controls overlay an input.

## Surface Standards

Main surfaces:

- App panels use `ui-panel-surface`.
- Active app panels use `ui-panel-surface-active`; drag-over panels use `ui-panel-surface-drop`.
- Overlays and modals use `ui-overlay-surface`.
- Settings sections/cards use `settings-section-appear`, `settings-card`, or `settings-nav-card`.
- Small floating controls that sit above editable text/content must use a named material class in the matching `src/styles/*.css` section; avoid one-off blur utilities in JSX.

Rules:

- Do not put UI cards inside other cards unless the inner card represents a repeated item or a modal/tool frame.
- Do not make page sections look like floating cards by default.
- Panel headers should stay compact and predictable.
- Borders should be subtle; use shadows and inset highlights sparingly.
- Prefer `border`, `ring`, and named CSS surface classes over inline Tailwind depth utilities.
- Do not hand-build one-off floating glass with raw blur/shadow values in JSX. Add or reuse a semantic class in the matching `src/styles/*.css` section so the material stays consistent across similar controls.

Active glow rule:

- Large surfaces that already define `box-shadow` must not rely on Tailwind `ring-*` for their selected glow, because ring is also implemented through `box-shadow` and can be overwritten by the surface class.
- Use `ui-panel-surface-active`, `ui-panel-surface-drop`, and `composer-shell-active` for selected/drop/input glow states.
- At most one primary blue glow should be visible: the active panel, or the chat composer when the input is focused.
- The visible blue line belongs on the original border. Do not create a hard extra outline outside the border; use soft blur shadows for the outward halo.
- Active glow must animate both in and out through `border-color` and `box-shadow`, not appear as an instant class flash.
- Main panel halo lives in the fixed `panel-active-glow-fixed` overlay so it sits above panel contents, adjacent panels, and the header without changing page flow or scroll behavior.

## Panel Header Standards

Applies to file tree, editor, chat, and future panels.

Rules:

- Header height should generally be `h-12`.
- Panel drag handle is always visible and uses `ui-panel-drag-handle`.
- Header icon buttons use `ui-icon-button`.
- Header actions should align on the same grid or flex baseline.
- Avoid hover-only critical controls unless the row/list pattern requires decluttering.
- If controls appear on hover, reveal with opacity only, not slide/translate.

## File Tree Standards

File tree is dense and utility-heavy. It must remain stable while users scan names.

Rules:

- Row hover may change background, not horizontal position.
- Action buttons appear by opacity, not by sliding.
- File names should use a predictable ellipsis strategy and keep as much readable name as possible.
- Drag insertion indicators must not change row height.
- Trash/recycle entry is not a normal workspace folder.
- Destructive actions use red hover tone only.

## Chat Standards

Chat is the primary AI interaction area.

Rules:

- Streaming output must not force-scroll if the user has intentionally scrolled away from the bottom.
- The scroll-to-bottom button should be translucent, compact, and visually tied to the composer.
- The composer shell uses `--radius-composer`, soft translucency, and stable icon controls.
- The focused composer shell uses `composer-shell-active`; do not add `ring-*` to the composer shell.
- Composer text input must remain full width so the scrollbar stays at the far right edge of the composer.
- The chat composer uses an autosizing textarea plus a fixed lower-right action dock. Do not reintroduce manual `scrollHeight` resizing, dynamic `paddingRight`/`paddingBottom` logic, or a full-height right-side dock column.
- Composer action buttons belong in `composer-action-dock`; they must stay fixed to the lower right of the composer shell and must not vertically track the textarea center as it grows.
- The action dock is a translucent floating glass layer; text may pass underneath it. Do not add text-flow avoidance, contenteditable mirrors, or dynamic button-safe padding to route text around the dock.
- Composer implementation lives under `src/components/chat/composer/`; future input, action dock, attachment, reference, skill menu, and follow-up question work should stay in the matching subcomponent instead of expanding the wrapper.
- Composer CSS lives in `src/styles/chat.css`; keep shell, textarea, action dock, scroll-to-bottom, and follow-up question classes together.
- Context usage/compaction status should be visible but quiet.
- Progress items should be lightweight chips, not large cards.

## Editor Standards

Editor should prioritize reading and editing comfort.

Rules:

- Editor content needs generous but not wasteful horizontal padding.
- Editor tabs should remain compact and stable.
- Close buttons use compact icon-button rules.
- File conflict/status notices should be short and use the correct tone.
- Do not let AI/file status UI obscure editable text.

## Settings Standards

Settings is a structured operations area, not a marketing page.

Rules:

- Use shared settings primitives.
- Prefer grouped sections with clear labels and short helper text.
- Advanced options should be collapsible.
- Admin-only controls must be clearly labeled.
- Do not use large decorative illustrations or hero layouts.

## Dialog And Popover Standards

Rules:

- Dialogs use `AppDialog` where possible.
- Button-anchored menus, dropdowns, and popovers use `AnchoredPopover`.
- `AnchoredPopover` portals to `document.body`, uses fixed positioning, clamps to the viewport, flips above the anchor when needed, and closes on outside click or Escape.
- Popovers must avoid viewport overflow and must not be trapped by parent `overflow-hidden`.
- Do not build new business popovers with raw `absolute top-* left-* z-*` positioning.
- Use backdrop blur only when it improves focus.
- Destructive confirmations must clearly name the affected object.
- Menus should use row background changes, not row movement.
- Avoid double tooltips: do not combine `title` with a custom hover tooltip for the same control.

Surface categories:

- **Anchored popover/menu:** file search, recycle bin, add menus, model menu, conversation history, selects. Use `AnchoredPopover`.
- **Modal/dialog:** confirmations and forms that block the workflow. Use `AppDialog` or `ui-overlay-surface`/`motion-modal`.
- **Drawer/panel:** large task/history style surfaces. They may use fixed positioning, but should follow overlay radius/depth rules.
- **Tooltip/hover info:** short non-interactive information only. It may remain local, but it must not also use a native browser tooltip.

Allowed local popover exceptions:

- Chat composer slash-skill autocomplete stays local to the composer so typing can continue while suggestions are visible.
- Model provider submenu is a nested second-level menu inside the anchored model menu.

## Accessibility

Minimum requirements:

- Interactive elements are real `button`, `input`, `select`, or keyboard-accessible elements.
- Icon-only actions have `title` or `aria-label`.
- Focus states must be visible.
- Disabled state must be semantic (`disabled`) when possible.
- Do not rely on color alone for destructive or selected states when text/context is available.

## Responsive Rules

- Text must not overflow controls on mobile.
- Toolbars should maintain stable dimensions.
- Use fixed control sizes and responsive grid/flex constraints.
- Do not scale font size with viewport width.
- Avoid layout shifts when hover-only action buttons appear.

## Anti-Patterns

Avoid:

- Adding long one-off Tailwind class strings for a common control.
- Adding Tailwind shadow utilities in JSX.
- Creating stacked convex/concave button layers for normal controls.
- Using `hover:-translate-y-*` on buttons, rows, or cards.
- Animating both a parent and a child transform for one interaction.
- Mixing unrelated radius sizes in one control cluster.
- Adding decorative blobs, orbs, bokeh, or gradient-only hero treatments.
- Using native browser tooltip plus a custom tooltip for the same information.
- Creating separate dark-mode behavior that feels brighter/heavier than light mode.
- Adding new color palettes without checking the existing palette.

## UI Audit

`npm run test:ui` runs `scripts/check-ui-standards.mjs` and scans `src/components/**/*.{jsx,js}`.

The audit fails on:

- `hover:-translate`, `active:translate`, `translateY(-1px)`.
- `hover:scale*` and `active:scale*` Tailwind utilities.
- `transition-all`.
- Tailwind shadow utilities in `src` JS/JSX, including `shadow-sm`, `shadow-xl`, `hover:shadow-*`, and arbitrary `shadow-[...]`.
- Arbitrary component radius such as `rounded-[18px]`.
- Hand-built segmented controls such as `inline-flex rounded-2xl ... p-0.5`.
- Hand-built switches such as `relative h-6 w-11 rounded-full`.
- Raw `<button>` without a shared primitive or one of the standard motion classes.
- `className` literals over 180 characters unless the element includes a documented named UI class.
- Native `title` plus custom tooltip wiring on the same button.
- Raw absolute `motion-popover` surfaces outside the documented allowlist.

Allowed long-class exceptions must be semantic named classes, centralized in the audit script, and documented here. Current allowed semantic classes are for established structural surfaces and rows such as `ui-panel-surface`, `ui-overlay-surface`, `settings-card`, `settings-nav-card`, `composer-shell`, `message-bubble`, `chat-panel-header`, `file-tree-row`, `task-item-row`, `header-segmented`, `modal-backdrop`, and similar named panel/chat/editor/file-tree surfaces. Do not add a new allowed token just to avoid extracting a repeated style.

The composer slash-skill autocomplete is an allowed raw absolute popover in `src/components/chat/composer/SkillMenu.jsx` because it is anchored inside the typing surface and must not steal focus from the textarea while suggestions are visible.

## Change Process

Before making UI changes:

1. Read this document.
2. Inspect the existing component and nearby patterns.
3. Reuse a shared class/component if one exists.
4. If repeated styling appears twice, consider extracting it.
5. Keep the change scoped; do not silently redesign unrelated areas.
6. Test in light and dark mode when the change is visible in both.

After UI changes:

```bash
npm run test:ui
npm run test:unit
npm run build
```

If the Docker-served app is being used locally, rebuild it:

```bash
docker compose up -d --build --force-recreate
```

## New Chat Handoff Snippet

When starting a new coding chat for frontend UI or structure work, include:

> Before changing Perseus Studio frontend UI, read `docs/UI_DESIGN_SYSTEM.md` and `docs/PROJECT_HEALTH_SCORECARD.md`. Follow the standards for buttons, inputs, panels, motion, radius, color, dark mode, component reuse, and zero-behavior structure splits. Do not add one-off Tailwind styling for common controls unless there is a clear reason.

## Migration Plan

The current frontend is not fully migrated to this system. Improve it gradually:

1. Keep `src/index.css` as an ordered entrypoint and put shared CSS in the matching `src/styles/*.css` section.
2. Keep reusable React primitives in `src/components/ui/*`.
3. Replace duplicated icon buttons, text buttons, inputs, selects, popovers, and dialog actions with shared primitives.
4. Reduce large inline Tailwind strings in business components.
5. Keep component-specific exceptions documented in this file and reflected in `scripts/check-ui-standards.mjs`.
