const query = document.getElementById("concurrency-query");
const managementKey = document.getElementById("management-key");
const refresh = document.getElementById("refresh");
const statusText = document.getElementById("status");
const series = document.getElementById("series");
const clock = new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
let timer;
let hasSnapshot = false;

async function readConcurrency(key) {
  const response = await fetch("/v0/management/plugins/codex-ny-timezone/concurrency", {
    headers: { Authorization: `Bearer ${key}` },
    cache: "no-store",
    redirect: "error",
    signal: AbortSignal.timeout(8000),
  });
  if (response.status === 401 || response.status === 403) throw new DOMException("管理密钥无效，或当前地址不允许管理接口访问。", "AccessError");
  if (response.status === 503) throw new DOMException("当前 CPA 不支持请求生命周期统计。", "NotSupportedError");
  if (!response.ok) throw new Error(`查询失败（HTTP ${response.status}）。`);
  return response.json();
}

function renderConcurrency({ active, peak, peak_at, tick, points }) {
  document.getElementById("active").textContent = active;
  document.getElementById("peak").textContent = peak;
  document.getElementById("peak-at").textContent = peak_at ? `发生于 ${new Date(peak_at * 1000).toLocaleString()}` : "尚未记录到请求";
  const scale = Math.max(1, ...points.map(point => point.peak));
  document.getElementById("chart-max").textContent = scale;
  document.getElementById("chart-start").textContent = clock.format((tick - 119) * 5000);
  document.getElementById("chart-end").textContent = clock.format(tick * 5000);
  const segments = points.map(({ tick: time, peak: count }) => {
    const y = 166 - count * 150 / scale;
    const x = 30 + (time - tick + 119) * 550 / 120;
    return [x, y];
  });
  series.setAttribute("d", segments.map(([x, y], i) => `${i ? "V" : "M"}${i ? y : `${x} ${y}`} H${x + 550 / 120}`).join(" "));
  document.getElementById("sample-scope").textContent = `${clock.format(Date.now())} 更新 · 近 10 分钟`;
  document.getElementById("chart").hidden = false;
}

async function refreshConcurrency() {
  clearTimeout(timer);
  const key = managementKey.value.trim();
  if (!key) return;
  refresh.disabled = true;
  managementKey.disabled = true;
  try {
    renderConcurrency(await readConcurrency(key));
    hasSnapshot = true;
    statusText.textContent = "实时更新中，每 2 秒刷新。";
    timer = setTimeout(refreshConcurrency, 2000);
  } catch (error) {
    const stopped = error.name === "AccessError" || error.name === "NotSupportedError";
    const message = error.name === "TimeoutError" ? "查询超时。" : error instanceof TypeError ? "CPA 连接中断。" : error.message;
    statusText.textContent = `${message}${hasSnapshot ? " 显示的是上次数据。" : ""}${stopped ? " 监控已暂停。" : " 2 秒后重试。"}`;
    if (!stopped) timer = setTimeout(refreshConcurrency, 2000);
  } finally {
    refresh.disabled = false;
    managementKey.disabled = false;
  }
}

managementKey.addEventListener("input", () => {
  clearTimeout(timer);
  hasSnapshot = false;
  document.getElementById("active").textContent = "—";
  document.getElementById("peak").textContent = "—";
  document.getElementById("peak-at").textContent = "";
  document.getElementById("sample-scope").textContent = "尚未读取数据";
  document.getElementById("chart").hidden = true;
  statusText.textContent = "输入管理密钥后开始监控。";
});
query.addEventListener("submit", event => {
  event.preventDefault();
  if (!refresh.disabled && query.reportValidity()) void refreshConcurrency();
});
