// fzt-web.js — Higher-level fzt terminal component with sensible defaults.
// Consumers import this instead of fzt-terminal.js directly.
// Override any option to customize; defaults match fzt-terminal.css.

import { createFztTerminal } from './fzt-terminal.js';

// Catppuccin Mocha 16-color palette
const DEFAULT_PALETTE = [
  "#1e1e2e", "#f38ba8", "#a6e3a1", "#f9e2af",
  "#89b4fa", "#cba6f7", "#94e2d5", "#bac2de",
  "#585b70", "#f38ba8", "#a6e3a1", "#f9e2af",
  "#89b4fa", "#cba6f7", "#94e2d5", "#cdd6f4",
];

const DEFAULT_FONT = '"Perfect DOS VGA 437","Cascadia Code","Fira Code","JetBrains Mono","Consolas","Symbols Nerd Font Mono",monospace';
const DEFAULT_NERD_FONT = "'Symbols Nerd Font Mono','Perfect DOS VGA 437',monospace";

export function createFztWeb(container, options = {}) {
  return createFztTerminal(container, {
    palette: options.palette || DEFAULT_PALETTE,
    fontFamily: options.fontFamily || DEFAULT_FONT,
    nerdFontFamily: options.nerdFontFamily || DEFAULT_NERD_FONT,
    cursorClass: options.cursorClass || "fzt-cursor",
    containerPadding: options.containerPadding ?? 0,
    defaultCursorPos: options.defaultCursorPos !== undefined ? options.defaultCursorPos : { x: 3, y: 1 },
    wasmUrl: options.wasmUrl || "fzt.wasm",
    onAction: options.onAction,
    onRender: options.onRender,
    shouldForwardKey: options.shouldForwardKey,
  });
}
