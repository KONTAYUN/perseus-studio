import katex from 'katex';
export function renderFormula(formula, displayMode = false) {
  try { return { html: katex.renderToString(formula, { displayMode, throwOnError: true, trust: false, strict: 'error', maxExpand: 500, maxSize: 20, output: 'htmlAndMathml' }), error: '' }; }
  catch { return { html: '', error: '公式暂时无法排版，请检查公式内容。' }; }
}
