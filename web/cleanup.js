const cleanup = document.getElementById("cleanup");
const managementKey = document.getElementById("management-key");
const confirm = document.getElementById("confirm");
const submit = document.getElementById("remove-history");
const result = document.getElementById("result");

cleanup.addEventListener("submit", async event => {
  event.preventDefault();
  const key = managementKey.value.trim();
  if (submit.disabled || !key || !confirm.checked) return;
  submit.disabled = true;
  confirm.checked = false;
  result.textContent = "正在清理…";
  try {
    const response = await fetch("/v0/management/plugins/codex-ny-timezone/cleanup-history", {
      method: "POST",
      headers: { Authorization: `Bearer ${key}`, "Content-Type": "application/json" },
      body: JSON.stringify({ confirm: "concurrency-history.json" }),
      redirect: "error", cache: "no-store", signal: AbortSignal.timeout(8000),
    });
    if (response.status === 401 || response.status === 403) throw new Error("管理密钥无效，或当前地址不允许访问管理接口。");
    if (response.status === 409) throw new Error("CPA 无法清理该文件：文件不可访问或不是预期的普通文件。");
    if (!response.ok) throw new Error(`清理未完成（HTTP ${response.status}）。`);
    const { status } = await response.json();
    if (status !== "removed" && status !== "missing") throw new Error("未能确认清理结果，可以安全重试。");
    result.textContent = status === "removed" ? "已删除旧峰值记录。" : "未找到旧峰值记录，无需清理。";
    managementKey.value = "";
  } catch (error) {
    result.textContent = error.name === "TimeoutError" || error instanceof TypeError || error instanceof SyntaxError
      ? "未能确认清理结果，可以安全重试。" : error.message;
  } finally {
    submit.disabled = false;
  }
});
