// 前端共享工具函数（大小格式化等），供 MainView / FileList 等组件复用

// formatBytes 字节转可读大小，越界或异常值兜底返回 '-'
export function formatBytes(bytes, fallback = '-') {
  const value = Number(bytes)
  if (!Number.isFinite(value) || value < 0) return fallback
  if (value === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  const num = value / Math.pow(1024, index)
  return `${parseFloat(num.toFixed(2))} ${units[index]}`
}

// formatTimestamp Unix 秒级时间戳转本地时间字符串
export function formatTimestamp(seconds, fallback = '-') {
  const value = Number(seconds)
  if (!Number.isFinite(value) || value <= 0) return fallback
  return new Date(value * 1000).toLocaleString()
}
