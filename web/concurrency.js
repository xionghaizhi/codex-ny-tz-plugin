const query = document.getElementById("concurrency-query");
const managementKey = document.getElementById("management-key");
const refresh = document.getElementById("refresh");
const statusText = document.getElementById("status");
const previousPage = document.getElementById("previous-page");
const nextPage = document.getElementById("next-page");
let page = 1;
let pages = 0;
let hasSnapshot = false;

async function readConcurrency(key, requestedPage) {
  const response = await fetch(`/v0/management/plugins/cpa-account-config-manager/accounts?page=${requestedPage}&page_size=1000`, {
    headers: { Authorization: `Bearer ${key}` },
    cache: "no-store",
    redirect: "error",
    signal: AbortSignal.timeout(30_000),
  });
  if (response.status === 401 || response.status === 403) throw new Error("管理密钥无效，或当前地址不允许访问管理接口。");
  if (response.status === 404) throw new Error("未找到账号管理插件接口，请确认插件已启用。");
  if (!response.ok) throw new Error(`查询失败（HTTP ${response.status}）。`);
  const snapshot = await response.json();
  if (snapshot.account_concurrency?.supported === false) throw new Error("当前 CPA 不支持账号并发统计。");
  if (snapshot.account_concurrency?.supported !== true || !Array.isArray(snapshot.accounts) ||
      snapshot.page !== requestedPage || !Number.isInteger(snapshot.pages) || snapshot.pages < 0 ||
      snapshot.accounts.some(({ concurrency }) => !concurrency || ![concurrency.active, concurrency.waiting, concurrency.limit]
        .every(count => Number.isSafeInteger(count) && count >= 0))) {
    throw new Error("账号管理插件未返回有效的并发数据。");
  }
  if (requestedPage > 1 && requestedPage > snapshot.pages) throw new Error("账号页数已变化，请点「刷新并发」回到首页。");
  return snapshot;
}

function renderConcurrency(snapshot) {
  const rows = document.createDocumentFragment();
  let active = 0;
  let waiting = 0;
  for (const { name, id, concurrency } of snapshot.accounts) {
    active += concurrency.active;
    waiting += concurrency.waiting;
    if (concurrency.active === 0 && concurrency.waiting === 0) continue;
    const row = document.createElement("tr");
    for (const value of [name || id, concurrency.active, concurrency.waiting, concurrency.limit || "不限"]) {
      const cell = document.createElement("td");
      cell.textContent = value;
      row.append(cell);
    }
    rows.append(row);
  }
  const hasBusyAccounts = rows.childElementCount > 0;
  document.getElementById("busy-accounts").replaceChildren(rows);
  document.getElementById("empty").hidden = hasBusyAccounts;
  document.getElementById("account-table").hidden = !hasBusyAccounts;
  document.getElementById("active").textContent = active;
  document.getElementById("waiting").textContent = waiting;
  const scope = pages > 1 ? `本页合计 · 第 ${page} / ${pages} 页` : "账号列表合计";
  document.getElementById("sample-scope").textContent = `${scope} · ${new Date().toLocaleTimeString()} 更新`;
  document.getElementById("pages").hidden = pages <= 1;
  document.getElementById("account-results").hidden = false;
}

async function refreshConcurrency(requestedPage) {
  if (refresh.disabled || !query.reportValidity()) return;
  const key = managementKey.value.trim();
  if (key === "") {
    statusText.textContent = "请输入管理密钥。";
    return;
  }
  refresh.disabled = previousPage.disabled = nextPage.disabled = managementKey.disabled = true;
  statusText.textContent = "正在读取账号并发…";
  try {
    const snapshot = await readConcurrency(key, requestedPage);
    page = snapshot.page;
    pages = snapshot.pages;
    renderConcurrency(snapshot);
    hasSnapshot = true;
    refresh.textContent = "刷新并发";
    statusText.textContent = "已更新。";
  } catch (error) {
    const message = error instanceof TypeError || error instanceof SyntaxError ? "无法读取并发接口，请检查 CPA 连接。" :
      error.name === "TimeoutError" ? "查询超时，请重试。" : error.message;
    statusText.textContent = hasSnapshot ? `${message} 当前显示上次查询结果。` : message;
  } finally {
    refresh.disabled = managementKey.disabled = false;
    previousPage.disabled = page <= 1;
    nextPage.disabled = page >= pages;
  }
}

query.addEventListener("submit", event => {
  event.preventDefault();
  void refreshConcurrency(1);
});
previousPage.addEventListener("click", () => void refreshConcurrency(page - 1));
nextPage.addEventListener("click", () => void refreshConcurrency(page + 1));
