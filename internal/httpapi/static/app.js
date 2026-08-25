const $ = selector => document.querySelector(selector);
let current = null;
let currentRisk = null;
let page = 1;
const pageSize = 20;
const jsonHeaders = {"Content-Type": "application/json"};

async function api(url, options) {
  const response = await fetch(url, options);
  const body = await response.json();
  if (!response.ok) throw Error(body.error || "请求失败");
  return body;
}

function esc(value) {
  return String(value || "").replace(/[&<>"']/g, char => ({"&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;"}[char]));
}

function post(path, body, key) {
  return api(path, {method: "POST", headers: {...jsonHeaders, ...(key ? {"Idempotency-Key": key} : {})}, body: JSON.stringify(body)});
}

function windowLabel(c) {
  const phase = c.windowPhase || (new Date(c.handoffWindowStart) > new Date() ? "未开始" : "进行中");
  return phase === "已结束" ? phase : `${phase} · 剩余 ${c.remainingMinutes ?? 0} 分钟`;
}

async function load() {
  const query = new URLSearchParams({status: $("#status").value, caseNumber: $("#keyword").value.trim(), windowPhase: $("#window-phase").value, dueWithinMinutes: $("#due-minutes").value, page: String(page), pageSize: String(pageSize)});
  const out = await api("/api/cases?" + query);
  $("#stats").innerHTML = Object.entries(out.counts).map(([key, value]) => `<span>${esc(key)} ${value}</span>`).join("") + `<span>当前 ${out.total}</span>`;
  $("#page-prev").disabled = page <= 1;
  $("#page-next").disabled = !out.hasNext;
  $("#page-state").textContent = `第 ${out.page} 页`;
  $("#cases").innerHTML = out.items.map(c => {
    const conflicts = c.windowConflicts || [];
    const warning = conflicts.length ? `<div class="warning">窗口冲突：${conflicts.map(item => esc(item.caseNumber)).join("、")}</div>` : "";
    return `<div class="case"><div><strong>${esc(c.caseNumber)}</strong><br><small>${esc(c.senderName)} → ${esc(c.receiverName)} · ${esc(c.status)} · v${c.version} · ${esc(windowLabel(c))}</small>${warning}</div><button data-id="${esc(c.id)}">查看</button></div>`;
  }).join("") || '<div class="panel">暂无案卷</div>';
  document.querySelectorAll(".case button").forEach(button => button.onclick = () => show(button.dataset.id));
}

function timeline(c) {
  return [...c.revisions].sort((a, b) => new Date(a.segmentStart) - new Date(b.segmentStart)).map(r => `<tr><td>r${r.revisionNumber}</td><td>${new Date(r.segmentStart).toLocaleString()}<br>${new Date(r.segmentEnd).toLocaleString()}</td><td>${esc(r.sealObservation)}</td><td>${r.readings.length}</td><td>${esc(r.remediationNote)}</td></tr>`).join("");
}

function ledger(c) {
  return (c.probeCoverageLedger || []).map(item => `<tr><td>${esc(item.probeId)}</td><td>${esc(item.certificateRef)}</td><td>${esc(item.status)}</td><td>${Number(item.remainingHours).toFixed(1)}</td></tr>`).join("") || '<tr><td colspan="4">尚未登记探头</td></tr>';
}

function conflictPanel(c) {
  const conflicts = c.windowConflicts || [];
  if (!conflicts.length) return '<div class="panel"><h3>交接窗口冲突</h3><p>未发现重叠案卷</p></div>';
  const rows = conflicts.map(item => `<li>${esc(item.caseNumber)} · ${new Date(item.windowStart).toLocaleString()} 至 ${new Date(item.windowEnd).toLocaleString()} · ${esc(item.status)}</li>`).join("");
  return `<div class="panel"><h3>交接窗口冲突</h3><ul>${rows}</ul><button id="review-conflicts">接受冲突复核</button><button id="clear-conflicts">解除冲突</button></div>`;
}

function coveragePanel(result) {
  const coverage = result.coverage;
  const gaps = coverage.gaps || [];
  const suggestion = coverage.nextSegment;
  return `<div class="panel"><h3>运输覆盖</h3><p>${coverage.covered ? "交接窗口已连续覆盖" : `仍有 ${gaps.length} 个缺口`}</p>${gaps.map(gap => `<div class="warning">${new Date(gap.start).toLocaleString()} 至 ${new Date(gap.end).toLocaleString()} · ${gap.durationSeconds} 秒 · 相邻修订 ${esc((gap.revisionNumbers || []).join(", "))}</div>`).join("")}${suggestion ? `<button id="use-suggestion" data-start="${esc(suggestion.start)}" data-end="${esc(suggestion.end)}">采用下一段建议</button>` : ""}</div>`;
}

function contextLine(finding) {
  const context = finding.context;
  if (!context) return "";
  const reading = item => item ? `${new Date(item.at).toLocaleString()} / ${item.temperatureC}°C` : "无";
  return `<small>上下文 ${esc(finding.contextId)} · 前值 ${esc(reading(context.previousReading))} · 触发 ${esc(reading(context.triggerReading))} · 后值 ${esc(reading(context.nextReading))} · 持续 ${context.durationSeconds} 秒 · 最大偏离 ${context.maxDeviationC}°C</small>`;
}

function closurePanel(result) {
  const closure = result.remediationClosure || {covered: [], uncovered: [], added: []};
  if (!result.baselineFingerprint) return "";
  return `<div class="panel"><h3>复审基线差异</h3><p>已覆盖 ${closure.covered.length} · 未覆盖 ${closure.uncovered.length} · 新增 ${closure.added.length}</p><small>基线 ${esc(result.baselineFingerprint)}<br>当前 ${esc(result.confirmationToken)}</small></div>`;
}

async function show(id) {
  [current, currentRisk] = await Promise.all([api("/api/cases/" + id), api(`/api/cases/${id}/risk`)]);
  const frozen = ["已批准", "已放行"].includes(current.status);
  $("#detail").hidden = false;
  $("#detail").innerHTML = `<div class="panel"><h2>${esc(current.caseNumber)} <small>v${current.version}</small></h2><p>${esc(current.status)} · ${esc(windowLabel(current))} · 容器 ${current.containers.length} · 探头 ${current.probes.length} · 证据 ${current.revisions.length}</p><div class="actions"><button id="submit">提交审核</button><button id="return">退回</button><button id="approve">批准</button><button id="release">签发</button><button id="manifest">清单预览</button></div><pre id="manifest-result"></pre></div>${conflictPanel(current)}${frozen ? "" : forms()}<div class="panel"><h3>探头校准覆盖台账</h3><table><thead><tr><th>探头</th><th>证书</th><th>状态</th><th>余量（小时）</th></tr></thead><tbody>${ledger(current)}</tbody></table></div>${coveragePanel(currentRisk)}<div class="panel"><h3>证据时间轴</h3><table><thead><tr><th>修订</th><th>时间</th><th>封签</th><th>读数</th><th>整改说明</th></tr></thead><tbody>${timeline(current)}</tbody></table></div><div class="panel"><h3>风险发现</h3>${(currentRisk.findings || []).map(f => `<div class="finding"><b>${esc(f.severity)} · ${esc(f.kind)}</b><p>${esc(f.derivedReason)} · ${esc((f.evidenceRefs || []).join(", "))}</p>${contextLine(f)}${f.decision ? `<small>${esc(f.decision)} / ${esc(f.decidedBy)}</small>` : `<button data-finding="${esc(f.id)}">裁决</button>`}</div>`).join("") || "<p>暂无风险发现</p>"}</div>${closurePanel(currentRisk)}`;
  bind(id, frozen);
}

function forms() {
  return `<div class="panel"><h3>登记基础资料</h3><div class="grid"><input id="container-code" placeholder="容器编码"><input id="seal-code" placeholder="封签标识"><input id="probe-code" placeholder="探头序列号"></div><div class="actions"><button id="basic">登记容器与探头</button><button id="replace-probe">替换探头</button></div></div><div class="panel"><h3>追加运输分段</h3><div class="grid"><input id="segment-start" type="datetime-local"><input id="segment-end" type="datetime-local"><input id="temperature" type="number" step="0.1" placeholder="温度 °C"><input id="remediation" placeholder="整改说明"></div><button id="evidence">预检并追加</button><pre id="precheck-result"></pre></div>`;
}

function localTime(value) {
  const date = new Date(value);
  const offset = date.getTimezoneOffset() * 60000;
  return new Date(date - offset).toISOString().slice(0, 16);
}

function bind(id, frozen) {
  $("#submit").onclick = () => act("submit", {});
  $("#return").onclick = () => {
    const note = prompt("整改要求") || "";
    const reviewer = prompt("审核人") || "";
    const tasks = (currentRisk.findings || []).map(finding => {
      const owner = prompt(`责任人：${finding.id}`) || "";
      if (!owner) return null;
      const due = prompt(`截止时间（RFC3339）：${finding.id}`) || "";
      return {findingId: finding.id, owner, dueAt: due, requiredEvidenceType: prompt(`必需证据类型：${finding.id}`) || "温度读数"};
    }).filter(Boolean);
    act("return", {note, reviewer, tasks});
  };
  $("#approve").onclick = () => act("approve", {reviewer: prompt("审核人") || "", fingerprint: currentRisk.confirmationToken || ""});
  $("#release").onclick = () => act("release", {reviewer: prompt("签发人") || ""});
  $("#manifest").onclick = async () => { $("#manifest-result").textContent = JSON.stringify(await api(`/api/cases/${id}/manifest`), null, 2); };
  ["review-conflicts", "clear-conflicts"].forEach(buttonId => { const button=$("#"+buttonId); if(button) button.onclick=()=>act("conflict-review", {conflictDigest: current.windowConflictDigest, decision: buttonId === "review-conflicts" ? "接受" : "解除", reviewer: prompt("复核人") || ""}); });
  document.querySelectorAll("[data-finding]").forEach(button => button.onclick = () => act("decisions", {findingID: button.dataset.finding, decision: prompt("决定：接受或整改") || "", note: prompt("裁决说明") || "", reviewer: prompt("审核人") || ""}, crypto.randomUUID()));
  if (frozen) return;
  const suggestion = $("#use-suggestion");
  if (suggestion) suggestion.onclick = () => { $("#segment-start").value = localTime(suggestion.dataset.start); $("#segment-end").value = localTime(suggestion.dataset.end); };
  $("#basic").onclick = async () => {
    try {
      const start = new Date(current.handoffWindowStart);
      const end = new Date(current.handoffWindowEnd);
      const serial = $("#probe-code").value;
      current = await post(`/api/cases/${id}/basics`, {expectedVersion: current.version, containers: [{id: crypto.randomUUID(), containerCode: $("#container-code").value, sealCode: $("#seal-code").value, sampleCategory: "冷藏样本", minTemperatureC: 2, maxTemperatureC: 8}], probes: [{id: crypto.randomUUID(), serialNumber: serial, certificateRef: `CERT-${serial}`, calibratedAt: new Date(start - 3600000).toISOString(), calibrationExpiresAt: new Date(end.getTime() + 3600000).toISOString(), accuracyC: 0.2}]});
      show(id);
    } catch (error) { alert(error.message); }
  };
  $("#replace-probe").onclick = async () => {
    const oldProbe = current.probes[0];
    if (!oldProbe) return;
    const serial = prompt("新探头序列号") || "";
    if (!serial) return;
    const start = new Date(current.handoffWindowStart);
    const end = new Date(current.handoffWindowEnd);
    const replacementAt = new Date(Math.min(Math.max(Date.now(), start.getTime()), end.getTime()));
    await act("probe-replace", {oldProbeId: oldProbe.id, replacementAt: replacementAt.toISOString(), id: crypto.randomUUID(), serialNumber: serial, certificateRef: `CERT-${serial}`, calibratedAt: start.toISOString(), calibrationExpiresAt: new Date(end.getTime() + 3600000).toISOString(), accuracyC: 0.2});
  };
  $("#evidence").onclick = async () => {
    const start = new Date($("#segment-start").value);
    const end = new Date($("#segment-end").value);
    const middle = new Date((start.getTime() + end.getTime()) / 2);
    const temperatureC = Number($("#temperature").value);
    const evidence = {probeId: current.probes.at(-1)?.id, segmentStart: start.toISOString(), segmentEnd: end.toISOString(), readings: [{at: start.toISOString(), temperatureC}, {at: middle.toISOString(), temperatureC}, {at: end.toISOString(), temperatureC}], sealObservation: current.containers[0]?.sealCode, remediationNote: $("#remediation").value};
    try {
      const precheck = await post(`/api/cases/${id}/evidence-precheck`, {expectedVersion: current.version, ...evidence});
      $("#precheck-result").textContent = JSON.stringify(precheck, null, 2);
      if (precheck.valid) await act("evidence", {...evidence, precheckFingerprint: precheck.fingerprint});
    } catch (error) { alert(error.message); }
  };
}

async function act(action, body, key) {
  try {
    if (action === "approve") {
      currentRisk = await api(`/api/cases/${current.id}/risk`);
      body = {...body, fingerprint: currentRisk.confirmationToken || ""};
    }
    current = await post(`/api/cases/${current.id}/${action}`, {expectedVersion: current.version, ...body}, key);
    await show(current.id);
  } catch (error) { alert(error.message); }
}

$("#search").onclick = () => { page = 1; load(); };
$("#page-prev").onclick = () => { if (page > 1) { page--; load(); } };
$("#page-next").onclick = () => { page++; load(); };
$("#new").onclick = async () => {
  const number = prompt("案卷编号");
  if (!number) return;
  const start = new Date();
  const end = new Date(start.getTime() + 6 * 3600000);
  await post("/api/cases", {caseNumber: number, senderName: prompt("发送方") || "发送方", receiverName: prompt("接收方") || "接收方", handoffWindowStart: start.toISOString(), handoffWindowEnd: end.toISOString()});
  page = 1;
  load();
};
$("#verify").onclick = async () => {
  try {
    const result = await api("/api/credentials?credentialNumber=" + encodeURIComponent($("#credential").value));
    document.querySelectorAll(".verification-review").forEach(button => button.remove());
    const failed = (result.items || []).filter(item => !item.valid);
    $("#verify-result").textContent = JSON.stringify(result.summary ? {summary: result.summary, items: result.items} : {valid: result.valid, checks: result.checks, credential: result.credential}, null, 2);
    failed.forEach(item => {
      const button=document.createElement("button");button.className="verification-review";button.textContent=`复核 ${item.input}`;
      button.onclick=async()=>{try{await post("/api/credentials",{input:item.input,batchId:result.batchId,checkDigest:item.checkDigest,failureCode:item.code,operator:prompt("操作人")||"",conclusion:prompt("复核结论")||"",note:prompt("复核说明")||""});}catch(error){alert(error.message);}};
      $("#verify-result").before(button);
    });
  } catch (error) { $("#verify-result").textContent = error.message; }
};

load();
