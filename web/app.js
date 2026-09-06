const state = {
  csrf: '',
  status: null,
  target: localStorage.getItem('gpa.target') || '',
  busy: false,
  pendingOp: '',
  activeLogin: sessionStorage.getItem('gpa.login') || '',
  mustChooseTarget: false,
};

const $ = (id) => document.getElementById(id);

function say(msg) {
  $('live').textContent = msg;
}

function showBanner(msg, kind) {
  const el = $('banner');
  el.hidden = !msg;
  el.textContent = msg || '';
  el.dataset.kind = kind || '';
}

async function api(path, opts = {}) {
  const headers = { Accept: 'application/json', ...(opts.headers || {}) };
  if (opts.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
  if (opts.method && opts.method !== 'GET' && state.csrf) headers['X-CSRF-Token'] = state.csrf;
  const res = await fetch(path, { credentials: 'same-origin', ...opts, headers });
  const text = await res.text();
  let data = {};
  if (text) {
    try { data = JSON.parse(text); } catch { data = { error: text }; }
  }
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

async function bootstrap() {
  const hash = new URLSearchParams(location.hash.replace(/^#/, ''));
  const token = hash.get('bootstrap');
  state.pendingOp = hash.get('operation') || '';
  if (token) {
    const out = await api('/api/v1/session', { method: 'POST', body: JSON.stringify({ bootstrap: token }) });
    state.csrf = out.csrf;
    history.replaceState(null, '', location.pathname);
    return;
  }
  const out = await api('/api/v1/session');
  state.csrf = out.csrf;
}

function currentOn(account) {
  return (account.current_on || []).join(', ');
}

function render() {
  const st = state.status;
  if (!st) return;
  showBanner(st.demo ? '演示数据，不会改真实登录。' : '', 'demo');
  $('conn').textContent = st.connected ? '服务已连接' : '服务断开';
  const sel = $('target');
  const prev = state.target || st.selected_target;
  sel.innerHTML = '';
  if (state.mustChooseTarget) { const o = document.createElement('option'); o.value = ''; o.textContent = '请选择切换范围'; sel.appendChild(o); }
  for (const t of st.targets || []) {
    const opt = document.createElement('option');
    opt.value = t.id;
    opt.textContent = t.label;
    if (!state.mustChooseTarget && t.id === prev) opt.selected = true;
    sel.appendChild(opt);
  }
  state.target = sel.value;
  const t = (st.targets || []).find((x) => x.id === state.target);
  $('target-note').textContent = t?.shared_note || (st.mixed_current ? '各客户端使用不同账号' : '');
  const list = $('accounts');
  list.innerHTML = '';
  const accounts = st.accounts || [];
  $('empty').hidden = accounts.length > 0;
  for (const a of accounts) {
    const li = document.createElement('li');
    li.className = 'row';
    const on = (t?.members || []).length > 0 && t.members.every((id) => (a.current_on || []).includes(id));
    li.innerHTML = `
      <div>
        <div class="name">${escapeText(a.display_name)}</div>
        <div class="meta">${escapeText(a.plan)} · ${escapeText(a.email_hint)}</div>
      </div>
      <div class="act">${on ? '<span class="badge">当前凭据</span>' : `<button class="primary switch" data-id="${escapeAttr(a.id)}">切换</button>`}</div>
      <div class="menu">
        <button type="button" class="more" data-id="${escapeAttr(a.id)}" aria-haspopup="true">⋯</button>
      </div>`;
    list.appendChild(li);
    const switchButton = li.querySelector('.switch');
    if (switchButton) switchButton.disabled = state.busy || !st.connected || !state.target;
  }
}

function escapeText(s) {
  return String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
function escapeAttr(s) { return escapeText(s); }

async function refresh() {
  try {
    state.status = await api('/api/v1/status?target=' + encodeURIComponent(state.target || ''));
    if (state.target && !(state.status.targets || []).some((t) => t.id === state.target)) {
      state.target = ''; state.mustChooseTarget = true;
      localStorage.removeItem('gpa.target');
    }
    if (!document.querySelector('.menu-list')) render();
  } catch (err) {
    if (err.message.includes('unknown target')) { state.target = ''; state.mustChooseTarget = true; localStorage.removeItem('gpa.target'); await refresh(); return; }
    if (state.status) state.status.connected = false;
    $('conn').textContent = '服务断开';
    document.querySelectorAll('.switch').forEach((b) => { b.disabled = true; });
    showBanner('服务暂时断开，列表可能过期。' + err.message, 'offline');
  }
}

async function doSwitch(accountId) {
  if (state.busy || !state.target) return;
  state.busy = true;
  document.querySelectorAll('.switch').forEach((b) => { b.disabled = true; });
  try {
    const plan = await api('/api/v1/switch-plans', {
      method: 'POST',
      body: JSON.stringify({ account: accountId, target: state.target }),
    });
    if (plan.decision === 'blocked') {
      setResult(plan.message || '无法切换');
      return;
    }
    if (plan.decision === 'noop') {
      setResult(plan.message || '无需切换');
      return;
    }
    const env = await api('/api/v1/operations', {
      method: 'POST',
      body: JSON.stringify({ plan_id: plan.id, request_id: 'req_' + Date.now(), idempotency_key: 'ui-' + plan.id }),
    });
    await handleEnv(env);
  } catch (err) {
    setResult(err.message);
  } finally {
    state.busy = false;
    await refresh();
  }
}

async function handleEnv(env) {
  state.pendingOp = env.operation_id || '';
  if (env.status === 'waiting_user' && env.reason_code === 'APP_RESTART_REQUIRED') {
    const op = await api('/api/v1/operations/' + env.operation_id);
    $('confirm-title').textContent = '重启并切换到 ' + findName(op.account_id);
    const target = state.status?.targets?.find((t) => t.id === op.target);
    $('confirm-body').textContent = (env.message || '') + ' · ' + (target?.label || op.target) + (target?.shared_note ? ' · ' + target.shared_note : '');
    $('dlg-confirm').showModal();
    const ok = await dialogResult('dlg-confirm', 'confirm-ok', 'confirm-cancel');
    if (!ok) {
      await api('/api/v1/operations/' + env.operation_id + '/cancel', { method: 'POST', body: '{}' });
      setResult('已取消，没有写入。');
      return;
    }
    const next = await api('/api/v1/operations/' + env.operation_id + '/confirm', { method: 'POST', body: '{}' });
    await handleEnv(next);
    return;
  }
  if (env.status === 'waiting_user') {
    $('wait-body').textContent = env.message || '请先关闭正在使用的客户端';
    $('dlg-wait').showModal();
    const retry = await dialogResult('dlg-wait', 'wait-retry', 'wait-later');
    if (retry) {
      const next = await api('/api/v1/operations/' + env.operation_id + '/retry', { method: 'POST', body: '{}' });
      await handleEnv(next);
    } else {
      setResult('已暂缓。操作编号 ' + env.operation_id);
    }
    return;
  }
  setResult((env.message || env.status) + (env.operation_id ? ' · ' + env.operation_id : ''));
}

function findName(id) {
  return (state.status?.accounts || []).find((a) => a.id === id)?.display_name || id;
}

function setResult(msg) {
  $('result').hidden = !msg;
  $('result').textContent = msg;
  say(msg);
}

function dialogResult(dlg, okId, cancelId) {
  return new Promise((resolve) => {
    const d = $(dlg);
    let finished = false;
    const done = (v) => {
      if (finished) return;
      finished = true;
      $(okId).onclick = null; $(cancelId).onclick = null;
      d.removeEventListener('cancel', cancelled); d.removeEventListener('close', closed);
      if (d.open) d.close();
      resolve(v);
    };
    const cancelled = (e) => { e.preventDefault(); done(false); };
    const closed = () => done(false);
    d.addEventListener('cancel', cancelled); d.addEventListener('close', closed);
    $(okId).onclick = () => done(true); $(cancelId).onclick = () => done(false);
  });
}

function openMenu(btn) {
  document.querySelectorAll('.menu-list').forEach((n) => n.remove());
  const id = btn.dataset.id;
  const box = document.createElement('div');
  box.className = 'menu-list';
  box.innerHTML = `
    <button type="button" data-act="detail">查看详情</button>
    <button type="button" data-act="rename">修改备注</button>
    <button type="button" data-act="reauth">更新授权</button>
    <button type="button" data-act="archive">归档</button>`;
  btn.parentElement.appendChild(box);
  box.addEventListener('click', async (ev) => {
    const act = ev.target.dataset.act;
    box.remove();
    if (act === 'detail') {
      const a = await api('/api/v1/accounts/' + encodeURIComponent(id));
      $('detail-title').textContent = a.display_name;
      $('detail-body').textContent = (a.email || a.email_hint) + ' · ' + a.plan + (currentOn(a) ? ' · 使用位置 ' + currentOn(a) : '');
      $('dlg-detail').showModal();
    }
    if (act === 'rename') {
      const name = prompt('新的备注名');
      if (name) {
        await api('/api/v1/accounts/' + encodeURIComponent(id), { method: 'PATCH', body: JSON.stringify({ display_name: name }) });
        await refresh();
      }
    }
    if (act === 'archive') {
      await api('/api/v1/accounts/' + encodeURIComponent(id), { method: 'PATCH', body: JSON.stringify({ archived: true }) });
      await refresh();
    }
    if (act === 'reauth') {
      $('add-name').value = findName(id);
      $('dlg-add').showModal();
      $('add-start').dataset.account = id;
    }
  });
}

let loginTimer;
async function pollLogin() {
  if (!state.activeLogin) return;
  try {
    const cur = await api('/api/v1/logins/' + state.activeLogin);
    $('add-status').textContent = cur.message || ({starting: '正在启动官方登录…', waiting_authorization: '请在官方页面完成授权'}[cur.status] || cur.status);
    $('add-code').hidden = !cur.user_code;
    $('add-code').textContent = cur.user_code || '';
    $('add-open').hidden = !cur.verification_url;
    $('add-copy').hidden = !cur.user_code;
    $('add-open').onclick = () => window.open(cur.verification_url, '_blank', 'noopener');
    $('add-copy').onclick = () => navigator.clipboard.writeText(cur.user_code);
    if (['succeeded', 'failed', 'cancelled', 'expired'].includes(cur.status)) {
      state.activeLogin = ''; sessionStorage.removeItem('gpa.login');
      $('add-start').disabled = false;
      if (cur.status === 'succeeded') { $('dlg-add').close(); setResult(cur.updated_existing ? '已更新授权' : '已添加账号'); await refresh(); }
      return;
    }
  } catch (err) { $('add-status').textContent = err.message; }
  loginTimer = setTimeout(pollLogin, 1000);
}
async function startLogin() {
  if (state.activeLogin) return;
  $('add-start').disabled = true;
  try {
    const rec = await api('/api/v1/logins', { method: 'POST', body: JSON.stringify({display_name: $('add-name').value, account: $('add-start').dataset.account || ''}) });
    state.activeLogin = rec.id; sessionStorage.setItem('gpa.login', rec.id);
    await pollLogin();
  } catch (err) { $('add-status').textContent = err.message; $('add-start').disabled = false; }
}
async function cancelLogin() {
  clearTimeout(loginTimer);
  try {
    if (state.activeLogin) await api('/api/v1/logins/' + state.activeLogin + '/cancel', {method:'POST',body:'{}'});
    state.activeLogin = ''; sessionStorage.removeItem('gpa.login'); $('add-start').disabled = false; $('dlg-add').close();
  } catch (err) { $('add-status').textContent = err.message; }
}
async function continueOperation(id) {
  if (state.busy) return;
  state.busy = true;
  try {
    const op = await api('/api/v1/operations/' + id);
    if (op.status === 'waiting_user') { await handleEnv({...op, operation_id: op.id}); }
    else if (op.status === 'blocked' || op.status === 'failed') { await handleEnv(await api('/api/v1/operations/' + id + '/retry', {method:'POST',body:JSON.stringify({request_id:crypto.randomUUID()})})); }
    else { setResult(op.message || op.status); }
  } catch (err) { setResult(err.message); }
  finally { state.busy = false; await refresh(); }
}
async function showOperations() {
  try {
    const data = await api('/api/v1/operations');
    $('ops-list').innerHTML = (data.operations || []).map((op) => `<li><strong>${escapeText(findName(op.account_id) || op.account)}</strong> ${escapeText(op.target)} · ${escapeText(op.status)}<div class="meta">${escapeText(op.message || '')}</div>${['waiting_user','blocked','failed'].includes(op.status) ? `<button data-operation="${escapeAttr(op.id)}">继续处理</button>` : ''}</li>`).join('') || '<li>还没有操作</li>';
    $('dlg-ops').showModal();
  } catch (err) { setResult(err.message); }
}
async function showArchived() {
  const data = await api('/api/v1/accounts?archived=true');
  $('detail-title').textContent = '已归档账号';
  $('detail-body').innerHTML = (data.accounts || []).map((a) => `<p>${escapeText(a.display_name)} <button data-restore="${escapeAttr(a.id)}">恢复</button></p>`).join('') || '没有归档账号';
  $('dlg-detail').showModal();
}

async function importAccounts() {
  try {
    const prev = await api('/api/v1/imports/preview', { method: 'POST', body: '{}' });
    if (!(prev.new || []).length) {
      setResult('没有可导入的新账号');
      return;
    }
    const res = await api('/api/v1/imports', { method: 'POST', body: '{}' });
    setResult('新增 ' + res.added + ' 个，跳过 ' + res.skipped + ' 个，冲突 ' + res.conflicts + ' 个');
    await refresh();
  } catch (err) {
    setResult(err.message);
  }
}

function wire() {
  $('target').addEventListener('change', async () => {
    state.target = $('target').value; state.mustChooseTarget = !state.target;
    localStorage.setItem('gpa.target', state.target);
    await refresh();
  });
  $('accounts').addEventListener('click', (ev) => {
    const sw = ev.target.closest('.switch');
    if (sw) doSwitch(sw.dataset.id);
    const more = ev.target.closest('.more');
    if (more) openMenu(more);
  });
  $('btn-add').onclick = () => {
    if (state.activeLogin) { $('dlg-add').showModal(); return; }
    $('add-start').disabled = false; $('add-open').hidden = true; $('add-copy').hidden = true;
    $('add-start').dataset.account = '';
    $('add-name').value = '';
    $('add-status').textContent = '';
    $('add-code').hidden = true;
    $('dlg-add').showModal();
  };
  $('add-start').onclick = startLogin;
  $('add-cancel').onclick = cancelLogin;
  $('dlg-add').addEventListener('cancel', (e) => { e.preventDefault(); cancelLogin(); });
  $('ops-list').onclick = (e) => { const b = e.target.closest('[data-operation]'); if (b) { $('dlg-ops').close(); continueOperation(b.dataset.operation); } };
  $('detail-body').onclick = async (e) => { const b = e.target.closest('[data-restore]'); if (b) { try { await api('/api/v1/accounts/' + b.dataset.restore, {method:'PATCH',body:JSON.stringify({archived:false})}); $('dlg-detail').close(); await refresh(); } catch (err) { setResult(err.message); } } };
  $('settings-archived').onclick = () => showArchived().catch((err) => setResult(err.message));
  $('detail-close').onclick = () => $('dlg-detail').close();
  $('btn-ops').onclick = showOperations;
  $('ops-close').onclick = () => $('dlg-ops').close();
  $('btn-settings').onclick = async () => {
    const d = await api('/api/v1/diagnostics');
    $('settings-body').textContent = '后台 ' + d.version + ' · 账号 ' + d.accounts + (d.demo ? ' · 演示模式' : '');
    $('dlg-settings').showModal();
  };
  $('settings-close').onclick = () => $('dlg-settings').close();
  $('settings-import').onclick = importAccounts;
  $('btn-import').onclick = importAccounts;
  $('settings-diag').onclick = async () => {
    const d = await api('/api/v1/diagnostics');
    $('detail-title').textContent = '诊断';
    $('detail-body').textContent = JSON.stringify(d, null, 2);
    $('dlg-detail').showModal();
  };
}

async function main() {
  wire();
  try {
    await bootstrap();
    $('conn').textContent = '服务已连接';
    await refresh();
    if (state.pendingOp) await continueOperation(state.pendingOp);
    if (state.activeLogin) { $('dlg-add').showModal(); $('add-start').disabled = true; await pollLogin(); }
    setInterval(() => { if (!state.busy && !document.hidden) refresh(); }, 5000);
  } catch (err) {
    showBanner(err.message + '。请用 GPA 启动器打开。', 'offline');
  }
}

main();
