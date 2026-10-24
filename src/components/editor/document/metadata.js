import { parseDocument as parseYamlDocument, isMap, isScalar, stringify as stringifyYaml } from 'yaml';
import { parseForESLint } from 'toml-eslint-parser';
import * as TOML from 'smol-toml';

function payload(raw) {
  const from = raw.indexOf('\n') + 1, to = raw.lastIndexOf('\n');
  return { body: raw.slice(from, raw[to - 1] === '\r' ? to - 1 : to), from };
}
export function readMetadata(raw, format) {
  const { body, from } = payload(raw);
  try {
    let rows;
    if (format === 'yaml') {
      const document = parseYamlDocument(body, { uniqueKeys: true });
      if (document.errors.length || !isMap(document.contents)) return { rows: [], editable: false };
      rows = document.contents.items.map(pair => ({ key: String(pair.key?.value), value: pair.value?.value,
        type: typeof pair.value?.value, range: pair.value?.range?.slice(0, 2),
        simple: isScalar(pair.key) && typeof pair.key.value === 'string' && isScalar(pair.value) }));
    } else {
      const tree = parseForESLint(body).ast.body[0];
      rows = tree.body.map(pair => ({ key: pair.key?.keys?.[0]?.name ?? pair.key?.keys?.[0]?.value,
        value: pair.value?.value, type: typeof pair.value?.value, range: pair.value?.range,
        simple: pair.type === 'TOMLKeyValue' && pair.key.keys.length === 1 && pair.value.type === 'TOMLValue'
          && ['string', 'integer', 'float', 'boolean'].includes(pair.value.kind) }));
    }
    for (const row of rows) {
      if (row.type === 'number' && (!Number.isFinite(row.value) || (Number.isInteger(row.value) && !Number.isSafeInteger(row.value)))) row.simple = false;
    }
    return { rows, editable: rows.every(row => row.simple && row.range), body, from };
  } catch { return { rows: [], editable: false }; }
}

export function updateMetadata(raw, format, values) {
  const original = readMetadata(raw, format);
  if (!original.editable) throw new Error('复杂属性已完整保留，可以让助手调整。');
  const edits = [];
  for (const row of original.rows) {
    if (!Object.hasOwn(values, row.key) || String(values[row.key]) === String(row.value ?? '')) continue;
    let value = values[row.key];
    if (row.type === 'number') {
      if (!String(value).trim() || !Number.isFinite(Number(value))) throw new Error(`${row.key} 需要填写有效数字`);
      value = Number(value);
      if (Number.isInteger(value) && !Number.isSafeInteger(value)) throw new Error(`${row.key} 的数字超出精确编辑范围`);
    }
    if (row.type === 'boolean') {
      if (!['true', 'false'].includes(String(value))) throw new Error(`${row.key} 需要选择 true 或 false`);
      value = String(value) === 'true';
    }
    let replacement;
    if (format === 'yaml') replacement = stringifyYaml(value).trimEnd();
    else {
      const text = TOML.stringify({ value }), node = parseForESLint(text).ast.body[0].body[0].value;
      replacement = text.slice(...node.range);
    }
    edits.push({ from: original.from + row.range[0], to: original.from + row.range[1], replacement });
  }
  let result = raw;
  for (const edit of edits.sort((a, b) => b.from - a.from)) result = result.slice(0, edit.from) + edit.replacement + result.slice(edit.to);
  if (!readMetadata(result, format).editable) throw new Error('属性转换无法完整验证，原稿未修改。');
  return result;
}
