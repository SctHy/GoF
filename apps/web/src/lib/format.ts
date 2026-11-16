
export function formatBadgeCount(count: number | string | null | undefined) {
  const value = Number(count || 0);
  if (!Number.isFinite(value) || value <= 0) return "";
  return value > 99 ? "99+" : String(value);
}

export function formatMetric(value: number | string | null | undefined) {
  const number = Number(value || 0);
  if (number >= 100000000) return `${trimMetric(number / 100000000, number >= 1000000000 ? 0 : 1)}亿`;
  if (number >= 10000) return `${trimMetric(number / 10000, number >= 100000 ? 0 : 1)}万`;
  return String(number);
}

export function formatOptionalMetric(value: number | string | null | undefined) {
  if (value === undefined || value === null) return "...";
  return formatMetric(value);
}

export function trimMetric(value: number, digits: number) {
  return value.toFixed(digits).replace(/\.0$/, "");
}

export function formatRelativeTime(value: string) {
  const time = new Date(value).getTime();
  if (!Number.isFinite(time)) return "";
  const seconds = Math.max(0, Math.floor((Date.now() - time) / 1000));
  if (seconds < 60) return "刚刚";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days} 天前`;
  return new Date(value).toLocaleDateString("zh-CN");
}
