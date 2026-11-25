const cleanup = document.getElementById("cleanup");
const managementKey = document.getElementById("management-key");
const loadFiles = document.getElementById("load-files");
const files = document.getElementById("files");
const confirm = document.getElementById("confirm");
const confirmLabel = document.getElementById("confirm-label");
const submit = document.getElementById("remove-history");
const result = document.getElementById("result");

function setResult(message) {
  result.textContent = message;
}

function selectedFile() {
  return files.querySelector("input[name='history-file']:checked")?.value || "";
}

function renderFiles(items) {
  files.replaceChildren();
  submit.disabled = true;
  confirmLabel.hidden = true;
  confirm.checked = false;
  for (const file of items) {
    const label = document.createElement("label");
    label.className = "file";
    const checkbox = document.createElement("input");
    checkbox.type = "radio";
    checkbox.name = "history-file";
    checkbox.value = file.name;
    checkbox.addEventListener("change", () => {
      confirmLabel.hidden = false;
      submit.disabled = false;
    });
    const text = document.createElement("span");
    text.textContent = file.name;
    const details = document.createElement("small");
    details.textContent = `${file.relative_path} · ${file.size} B · ${file.modified_at}`;
    text.append(details);
    label.append(checkbox, text);
    files.append(label);
  }
  if (!items.length) setResult("未找到时区插件生成的 concurrency-history.json。");
}

loadFiles.addEventListener("click", async () => {
  const key = managementKey.value.trim();
  if (loadFiles.disabled || !key) return;
  loadFiles.disabled = true;
  submit.disabled = true;
  confirmLabel.hidden = true;
  confirm.checked = false;
  setResult("正在读取…");
  try {
    const response = await fetch("/v0/management/plugins/codex-ny-timezone/cleanup-history", {
      headers: { Authorization: `Bearer ${key}` },
      redirect: "error", cache: "no-store", signal: AbortSignal.timeout(8000),
    });
    if (response.status === 401 || response.status === 403) throw new Error("管理密钥无效，或当前地址不允许访问管理接口。");
    if (!response.ok) throw new Error(`读取未完成（HTTP ${response.status}）。`);
    const payload = await response.json();
    if (!Array.isArray(payload.files)) throw new Error("返回结果无效，可以重试。");
    renderFiles(payload.files);
    if (payload.files.length) setResult("请选择要删除的文件。");
  } catch (error) {
    setResult(error.name === "TimeoutError" || error instanceof TypeError || error instanceof SyntaxError
      ? "未能确认读取结果，可以重试。" : error.message);
  } finally {
    loadFiles.disabled = false;
  }
});

cleanup.addEventListener("submit", async event => {
  event.preventDefault();
  const key = managementKey.value.trim();
  const file = selectedFile();
  if (submit.disabled || !key || !file || !confirm.checked) return;
  submit.disabled = true;
  confirm.checked = false;
  setResult("正在删除…");
  try {
    const response = await fetch("/v0/management/plugins/codex-ny-timezone/cleanup-history", {
      method: "POST",
      headers: { Authorization: `Bearer ${key}`, "Content-Type": "application/json" },
      body: JSON.stringify({ file, confirm: file }),
      redirect: "error", cache: "no-store", signal: AbortSignal.timeout(8000),
    });
    if (response.status === 401 || response.status === 403) throw new Error("管理密钥无效，或当前地址不允许访问管理接口。");
    if (response.status === 409) throw new Error("CPA 无法清理该文件：文件不可访问或不是预期的普通文件。");
    if (!response.ok) throw new Error(`清理未完成（HTTP ${response.status}）。`);
    const { status } = await response.json();
    if (status !== "removed" && status !== "missing") throw new Error("未能确认清理结果，可以安全重试。");
    renderFiles([]);
    setResult(status === "removed" ? "已删除选中的文件。" : "选中的文件已不存在，无需清理。");
    managementKey.value = "";
  } catch (error) {
    setResult(error.name === "TimeoutError" || error instanceof TypeError || error instanceof SyntaxError
      ? "未能确认清理结果，可以安全重试。" : error.message);
  } finally {
    submit.disabled = !selectedFile() || !confirm.checked;
  }
});
