const state = {
  csrf: '',
  status: null,
  target: localStorage.getItem('gpa.target') || '',
  busy: false,
  pendingOp: '',
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
  if (token) {
    const out = await api('/api/v1/session', { method: 'POST', body: JSON.stringify({ bootstrap: token }) });
    state.csrf = out.csrf;
    history.replaceState(null, '', location.pathname);
    return;
  }
  throw new Error('需要从 GPA 启动器打开');
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
  for (const t of st.targets || []) {
    const opt = document.createElement('option');
    opt.value = t.id;
    opt.textContent = t.label;
    if (t.id === prev) opt.selected = true;
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
    const on = (a.current_on || []).some((id) => (t?.members || []).includes(id));
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
      state.target = state.status.selected_target;
      localStorage.removeItem('gpa.target');
    }
    render();
  } catch (err) {
    showBanner('服务暂时断开，列表可能过期。' + err.message, 'offline');
  }
}

async function doSwitch(accountId) {
  if (state.busy) return;
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
    if (plan.decision === 'waiting_user' && plan.reason_code === 'APP_RESTART_REQUIRED') {
      $('confirm-title').textContent = '重启并切换到 ' + findName(accountId);
      $('confirm-body').textContent = (plan.shared_note ? plan.shared_note + '。' : '') + '将更新：' + (plan.members || []).join('、');
      $('dlg-confirm').returnValue = '';
      $('dlg-confirm').showModal();
      const ok = await dialogResult('dlg-confirm', 'confirm-ok', 'confirm-cancel');
      if (!ok) {
        setResult('已取消，没有写入。');
        return;
      }
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
    $('confirm-title').textContent = '重启并切换';
    $('confirm-body').textContent = env.message || '';
    $('dlg-confirm').showModal();
    const ok = await dialogResult('dlg-confirm', 'confirm-ok', 'confirm-cancel');
    if (!ok) {
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
    const done = (v) => {
      $(okId).onclick = null;
      $(cancelId).onclick = null;
      d.close();
      resolve(v);
    };
    $(okId).onclick = () => done(true);
    $(cancelId).onclick = () => done(false);
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

async function startLogin() {
  const rec = await api('/api/v1/logins', {
    method: 'POST',
    body: JSON.stringify({ display_name: $('add-name').value, account: $('add-start').dataset.account || '' }),
  });
  $('add-status').textContent = '等待官方授权…';
  if (rec.user_code) {
    $('add-code').hidden = false;
    $('add-code').textContent = rec.user_code + (rec.verification_url ? ' · ' + rec.verification_url : '');
    $('add-open').hidden = !rec.verification_url;
    $('add-copy').hidden = !rec.user_code;
    $('add-open').onclick = () => window.open(rec.verification_url, '_blank', 'noopener');
    $('add-copy').onclick = () => navigator.clipboard.writeText(rec.user_code);
  }
  const timer = setInterval(async () => {
    const cur = await api('/api/v1/logins/' + rec.id);
    $('add-status').textContent = cur.message || cur.status;
    if (['succeeded', 'failed', 'cancelled', 'expired'].includes(cur.status)) {
      clearInterval(timer);
      if (cur.status === 'succeeded') {
        $('dlg-add').close();
        setResult(cur.updated_existing ? '已更新授权' : '已添加账号');
        await refresh();
      }
    }
  }, 1000);
  $('add-cancel').onclick = async () => {
    clearInterval(timer);
    await api('/api/v1/logins/' + rec.id + '/cancel', { method: 'POST', body: '{}' });
    $('dlg-add').close();
  };
}

function wire() {
  $('target').addEventListener('change', async () => {
    state.target = $('target').value;
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
    $('add-start').dataset.account = '';
    $('add-name').value = '';
    $('add-status').textContent = '';
    $('add-code').hidden = true;
    $('dlg-add').showModal();
  };
  $('add-start').onclick = startLogin;
  $('detail-close').onclick = () => $('dlg-detail').close();
  $('btn-ops').onclick = async () => {
    const data = await api('/api/v1/operations');
    $('ops-list').innerHTML = (data.operations || []).map((op) =>
      `<li><strong>${escapeText(op.account)}</strong> ${escapeText(op.target)} · ${escapeText(op.status)}<div class="meta">${escapeText(op.message || '')}</div></li>`
    ).join('') || '<li>还没有操作</li>';
    $('dlg-ops').showModal();
  };
  $('ops-close').onclick = () => $('dlg-ops').close();
  $('btn-settings').onclick = async () => {
    const d = await api('/api/v1/diagnostics');
    $('settings-body').textContent = '后台 ' + d.version + ' · 账号 ' + d.accounts + (d.demo ? ' · 演示模式' : '');
    $('dlg-settings').showModal();
  };
  $('settings-close').onclick = () => $('dlg-settings').close();
  $('settings-import').onclick = async () => {
    const prev = await api('/api/v1/imports/preview', { method: 'POST', body: '{}' });
    if (!(prev.new || []).length) {
      setResult('没有可导入的新账号');
      return;
    }
    const res = await api('/api/v1/imports', { method: 'POST', body: '{}' });
    setResult('新增 ' + res.added + ' 个，跳过 ' + res.skipped + ' 个，冲突 ' + res.conflicts + ' 个');
    await refresh();
  };
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
  } catch (err) {
    showBanner(err.message + '。请用 GPA 启动器打开。', 'offline');
  }
}

main();
