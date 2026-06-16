export function displayPriceToMicro(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return 0;
  return Math.round(parsed * 1_000_000);
}

export function priceMicroToDisplay(value) {
  const number = Number(value || 0) / 1_000_000;
  if (!number) return '';
  return String(Number(number.toFixed(6)));
}

export function providerLabel(providers, id) {
  const item = providers.find((provider) => provider.id === id);
  return item?.name || id || '未知模型配置';
}

export function formatNumber(value) {
  return Number(value || 0).toLocaleString('zh-CN');
}

export function formatTokens(value) {
  const number = Number(value || 0);
  if (number >= 1_000_000) return `${(number / 1_000_000).toFixed(2)}M`;
  if (number >= 1_000) return `${(number / 1_000).toFixed(1)}K`;
  return formatNumber(number);
}

export function formatCost(value, currency) {
  const micro = Number(value || 0);
  if (!currency || !micro) return '未配置';
  return `${currency} ${(micro / 1_000_000).toFixed(4)}`;
}
