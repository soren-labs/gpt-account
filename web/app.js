const I18N = {
  en: {
    'title': 'GPA Manager',
    'skip': 'Skip to main content',
    'nav.ops': 'Recent activity',
    'nav.settings': 'Settings',
    'lang.switch': '中文',
    'target.label': 'Switch where',
    'target.choose': 'Choose a scope',
    'conn.connecting': 'Connecting…',
    'conn.ok': 'Connected',
    'conn.down': 'Disconnected',
    'accounts.title': 'Your accounts',
    'accounts.add': '＋ Add account',
    'accounts.switch': 'Switch',
    'accounts.current': 'Current credentials',
    'empty.text': 'No saved accounts yet. Import existing logins or add a new account.',
    'empty.import': 'Import existing accounts',
    'confirm.title': 'Restart and switch',
    'confirm.to': 'Restart and switch to {name}',
    'confirm.ok': 'Restart and switch',
    'common.cancel': 'Cancel',
    'common.close': 'Close',
    'wait.title': 'Action needed first',
    'wait.retry': 'I closed it, retry',
    'wait.later': 'Later',
    'wait.default': 'Close the client that is in use first',
    'add.title': 'Add account',
    'add.name': 'Label',
    'add.placeholder': 'e.g. Work account',
    'add.start': 'Start sign-in',
    'add.open': 'Open sign-in page',
    'add.copy': 'Copy device code',
    'add.starting': 'Starting the official sign-in…',
    'add.waiting': 'Finish authorization on the official page',
    'add.updated': 'Authorization updated',
    'add.added': 'Account added',
    'detail.title': 'Account details',
    'detail.usedOn': 'used on',
    'menu.detail': 'Details',
    'menu.rename': 'Rename',
    'menu.reauth': 'Refresh authorization',
    'menu.archive': 'Archive',
    'rename.prompt': 'New label',
    'settings.archived': 'Archived accounts',
    'settings.diag': 'Diagnostics',
    'settings.body': 'Backend {version} · {count} accounts',
    'settings.demo': ' · demo mode',
    'archived.title': 'Archived accounts',
    'archived.restore': 'Restore',
    'archived.none': 'No archived accounts',
    'diag.title': 'Diagnostics',
    'ops.none': 'No activity yet',
    'ops.continue': 'Continue',
    'result.blocked': 'Cannot switch',
    'result.noop': 'Nothing to switch',
    'result.cancelled': 'Cancelled. Nothing was written.',
    'result.deferred': 'Deferred. Operation {id}',
    'import.none': 'No new accounts to import',
    'import.done': 'Added {added}, skipped {skipped}, conflicts {conflicts}',
    'banner.demo': 'Demo data. Real logins are not touched.',
    'banner.offline': 'Service temporarily unavailable; the list may be stale. ',
    'banner.bootstrap': '. Open this page from the GPA launcher.',
    'mixed': 'Clients use different accounts',
  },
  'zh-CN': {
    'title': 'GPA 账号管理',
    'skip': '跳到主要内容',
    'nav.ops': '最近操作',
    'nav.settings': '设置',
    'lang.switch': 'English',
    'target.label': '切换到哪里',
    'target.choose': '请选择切换范围',
    'conn.connecting': '正在连接',
    'conn.ok': '服务已连接',
    'conn.down': '服务断开',
    'accounts.title': '你的账号',
    'accounts.add': '＋添加账号',
    'accounts.switch': '切换',
    'accounts.current': '当前凭据',
    'empty.text': '还没有保存的账号。可以导入已有账号，或添加一个新账号。',
    'empty.import': '导入已有账号',
    'confirm.title': '重启并切换',
    'confirm.to': '重启并切换到 {name}',
    'confirm.ok': '重启并切换',
    'common.cancel': '取消',
    'common.close': '关闭',
    'wait.title': '需要先处理',
    'wait.retry': '我已关闭，重试',
    'wait.later': '稍后',
    'wait.default': '请先关闭正在使用的客户端',
    'add.title': '添加账号',
    'add.name': '备注名',
    'add.placeholder': '例如 工作账号',
    'add.start': '开始登录',
    'add.open': '打开登录页面',
    'add.copy': '复制设备码',
    'add.starting': '正在启动官方登录…',
    'add.waiting': '请在官方页面完成授权',
    'add.updated': '已更新授权',
    'add.added': '已添加账号',
    'detail.title': '账号详情',
    'detail.usedOn': '使用位置',
    'menu.detail': '查看详情',
    'menu.rename': '修改备注',
    'menu.reauth': '更新授权',
    'menu.archive': '归档',
    'rename.prompt': '新的备注名',
    'settings.archived': '已归档账号',
    'settings.diag': '查看诊断',
    'settings.body': '后台 {version} · 账号 {count}',
    'settings.demo': ' · 演示模式',
    'archived.title': '已归档账号',
    'archived.restore': '恢复',
    'archived.none': '没有归档账号',
    'diag.title': '诊断',
    'ops.none': '还没有操作',
    'ops.continue': '继续处理',
    'result.blocked': '无法切换',
    'result.noop': '无需切换',
    'result.cancelled': '已取消，没有写入。',
    'result.deferred': '已暂缓。操作编号 {id}',
    'import.none': '没有可导入的新账号',
    'import.done': '新增 {added} 个，跳过 {skipped} 个，冲突 {conflicts} 个',
    'banner.demo': '演示数据，不会改真实登录。',
    'banner.offline': '服务暂时断开，列表可能过期。',
    'banner.bootstrap': '。请用 GPA 启动器打开。',
    'mixed': '各客户端使用不同账号',
  },
};

// Backend messages are produced in Chinese. When the page runs in English they
// are mapped here; anything unknown is shown as-is so nothing is lost.
const SERVER_EN = [
  [/^桌面端$/, 'Desktop'],
  [/^本机全部$/, 'Everything on this machine'],
  [/^Windows App 和 Windows CLI 共用登录，会一起更新$/, 'The Windows App and Windows CLI share one login and are updated together'],
  [/^各客户端使用不同账号$/, 'Clients use different accounts'],
  [/^无法确认运行状态，不能写入凭据$/, 'Cannot confirm the running state, so credentials will not be written'],
  [/^(.+) 正在使用，关闭后再重试$/, '$1 is in use. Close it and retry'],
  [/^桌面应用正在运行，需要确认重启$/, 'The desktop app is running and must be restarted'],
  [/^凭据版本冲突: (.+)$/, 'Credential revision conflict: $1'],
  [/^所选范围已经是这个账号$/, 'The selected scope already uses this account'],
  [/^(.+) 与账号库凭据版本冲突$/, '$1 conflicts with the stored credential revision'],
  [/^预览已过期，请重新检查$/, 'The preview expired. Check again'],
  [/^账号或范围已变化，请重新预览$/, 'The account or scope changed. Preview again'],
  [/^账号或影响范围已变化，请重新确认。(.*)$/, 'The account or affected scope changed. Confirm again. $1'],
  [/^已切换本地凭据$/, 'Local credentials switched'],
  [/^操作结果未能保存，请检查客户端状态: (.*)$/, 'The result could not be saved. Check the client state: $1'],
  [/^请先检查并处理恢复状态$/, 'Review and resolve the recovery state first'],
  [/^已取消$/, 'Cancelled'],
  [/^后台中断，请检查恢复结果后重试$/, 'The backend was interrupted. Check the recovery result and retry'],
  [/^后台已重启，请重新开始授权$/, 'The backend restarted. Start the authorization again'],
  [/^已有登录正在进行，请完成或取消后再添加$/, 'A sign-in is already in progress. Finish or cancel it first'],
  [/^已取消本次登录$/, 'Sign-in cancelled'],
  [/^登录未产生可用的订阅凭据$/, 'Sign-in did not produce usable subscription credentials'],
  [/^登录身份与原账号不同，原账号未修改；请通过添加账号保存其他身份$/, 'The signed-in identity differs from this account; it was left unchanged. Use Add account to save another identity'],
  [/^保存登录失败: (.*)$/, 'Saving the login failed: $1'],
  [/^授权超时，请重新开始$/, 'Authorization timed out. Start again'],
  [/^官方登录未完成，请检查设备码权限或重试（(.*)）$/, 'The official sign-in did not finish. Check the device-code permission or retry ($1)'],
  [/^未取得隔离认证文件: (.*)$/, 'Isolated auth file was not produced: $1'],
];

const STATUS_EN = {
  succeeded: 'succeeded', failed: 'failed', blocked: 'blocked', waiting_user: 'waiting for you',
  running: 'running', queued: 'queued', cancelled: 'cancelled',
};
const STATUS_ZH = {
  succeeded: '已完成', failed: '失败', blocked: '已阻断', waiting_user: '等待处理',
  running: '进行中', queued: '排队中', cancelled: '已取消',
};

function detectLang() {
  const saved = localStorage.getItem('gpa.lang');
  if (saved && I18N[saved]) return saved;
  return /^zh/i.test(navigator.language || '') ? 'zh-CN' : 'en';
}

const state = {
  csrf: '',
  status: null,
  target: localStorage.getItem('gpa.target') || '',
  busy: false,
  pendingOp: '',
  activeLogin: sessionStorage.getItem('gpa.login') || '',
  mustChooseTarget: false,
  lang: detectLang(),
};

function t(key, vars) {
  let s = (I18N[state.lang] && I18N[state.lang][key]) ?? I18N.en[key] ?? key;
  if (vars) for (const [k, v] of Object.entries(vars)) s = s.replaceAll('{' + k + '}', String(v));
  return s;
}

function ts(msg) {
  if (!msg || state.lang === 'zh-CN') return msg || '';
  for (const [re, out] of SERVER_EN) {
    if (re.test(msg)) return msg.replace(re, out);
  }
  return msg;
}

function opStatus(s) {
  return (state.lang === 'zh-CN' ? STATUS_ZH : STATUS_EN)[s] || s;
}

function applyLang() {
  document.documentElement.lang = state.lang;
  document.querySelectorAll('[data-i18n]').forEach((el) => { el.textContent = t(el.dataset.i18n); });
  document.querySelectorAll('[data-i18n-placeholder]').forEach((el) => { el.placeholder = t(el.dataset.i18nPlaceholder); });
  document.title = t('title');
  $('btn-lang').textContent = t('lang.switch');
  $('btn-lang').setAttribute('aria-label', state.lang === 'zh-CN' ? 'Switch to English' : '切换到中文');
}

function setLang(lang) {
  state.lang = lang;
  localStorage.setItem('gpa.lang', lang);
  applyLang();
  render();
}

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
  showBanner(st.demo ? t('banner.demo') : '', 'demo');
  $('conn').textContent = st.connected ? t('conn.ok') : t('conn.down');
  const sel = $('target');
  const prev = state.target || st.selected_target;
  sel.innerHTML = '';
  if (state.mustChooseTarget) { const o = document.createElement('option'); o.value = ''; o.textContent = t('target.choose'); sel.appendChild(o); }
  for (const tg of st.targets || []) {
    const opt = document.createElement('option');
    opt.value = tg.id;
    opt.textContent = ts(tg.label);
    if (!state.mustChooseTarget && tg.id === prev) opt.selected = true;
    sel.appendChild(opt);
  }
  state.target = sel.value;
  const tg = (st.targets || []).find((x) => x.id === state.target);
  $('target-note').textContent = ts(tg?.shared_note) || (st.mixed_current ? t('mixed') : '');
  const list = $('accounts');
  list.innerHTML = '';
  const accounts = st.accounts || [];
  $('empty').hidden = accounts.length > 0;
  for (const a of accounts) {
    const li = document.createElement('li');
    li.className = 'row';
    const on = (tg?.members || []).length > 0 && tg.members.every((id) => (a.current_on || []).includes(id));
    li.innerHTML = `
      <div>
        <div class="name">${escapeText(a.display_name)}</div>
        <div class="meta">${escapeText(a.plan)} · ${escapeText(a.email_hint)}</div>
      </div>
      <div class="act">${on ? `<span class="badge">${escapeText(t('accounts.current'))}</span>` : `<button class="primary switch" data-id="${escapeAttr(a.id)}">${escapeText(t('accounts.switch'))}</button>`}</div>
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
    if (state.target && !(state.status.targets || []).some((x) => x.id === state.target)) {
      state.target = ''; state.mustChooseTarget = true;
      localStorage.removeItem('gpa.target');
    }
    if (!document.querySelector('.menu-list')) render();
  } catch (err) {
    if (err.message.includes('unknown target')) { state.target = ''; state.mustChooseTarget = true; localStorage.removeItem('gpa.target'); await refresh(); return; }
    if (state.status) state.status.connected = false;
    $('conn').textContent = t('conn.down');
    document.querySelectorAll('.switch').forEach((b) => { b.disabled = true; });
    showBanner(t('banner.offline') + err.message, 'offline');
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
      setResult(ts(plan.message) || t('result.blocked'));
      return;
    }
    if (plan.decision === 'noop') {
      setResult(ts(plan.message) || t('result.noop'));
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
    $('confirm-title').textContent = t('confirm.to', { name: findName(op.account_id) });
    const target = state.status?.targets?.find((x) => x.id === op.target);
    $('confirm-body').textContent = ts(env.message) + ' · ' + (ts(target?.label) || op.target) + (target?.shared_note ? ' · ' + ts(target.shared_note) : '');
    $('dlg-confirm').showModal();
    const ok = await dialogResult('dlg-confirm', 'confirm-ok', 'confirm-cancel');
    if (!ok) {
      await api('/api/v1/operations/' + env.operation_id + '/cancel', { method: 'POST', body: '{}' });
      setResult(t('result.cancelled'));
      return;
    }
    const next = await api('/api/v1/operations/' + env.operation_id + '/confirm', { method: 'POST', body: '{}' });
    await handleEnv(next);
    return;
  }
  if (env.status === 'waiting_user') {
    $('wait-body').textContent = ts(env.message) || t('wait.default');
    $('dlg-wait').showModal();
    const retry = await dialogResult('dlg-wait', 'wait-retry', 'wait-later');
    if (retry) {
      const next = await api('/api/v1/operations/' + env.operation_id + '/retry', { method: 'POST', body: '{}' });
      await handleEnv(next);
    } else {
      setResult(t('result.deferred', { id: env.operation_id }));
    }
    return;
  }
  setResult((ts(env.message) || opStatus(env.status)) + (env.operation_id ? ' · ' + env.operation_id : ''));
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
    <button type="button" data-act="detail">${escapeText(t('menu.detail'))}</button>
    <button type="button" data-act="rename">${escapeText(t('menu.rename'))}</button>
    <button type="button" data-act="reauth">${escapeText(t('menu.reauth'))}</button>
    <button type="button" data-act="archive">${escapeText(t('menu.archive'))}</button>`;
  btn.parentElement.appendChild(box);
  box.addEventListener('click', async (ev) => {
    const act = ev.target.dataset.act;
    box.remove();
    if (act === 'detail') {
      const a = await api('/api/v1/accounts/' + encodeURIComponent(id));
      $('detail-title').textContent = a.display_name;
      $('detail-body').textContent = (a.email || a.email_hint) + ' · ' + a.plan + (currentOn(a) ? ' · ' + t('detail.usedOn') + ' ' + currentOn(a) : '');
      $('dlg-detail').showModal();
    }
    if (act === 'rename') {
      const name = prompt(t('rename.prompt'));
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
    $('add-status').textContent = ts(cur.message) || ({ starting: t('add.starting'), waiting_authorization: t('add.waiting') }[cur.status] || cur.status);
    $('add-code').hidden = !cur.user_code;
    $('add-code').textContent = cur.user_code || '';
    $('add-open').hidden = !cur.verification_url;
    $('add-copy').hidden = !cur.user_code;
    $('add-open').onclick = () => window.open(cur.verification_url, '_blank', 'noopener');
    $('add-copy').onclick = () => navigator.clipboard.writeText(cur.user_code);
    if (['succeeded', 'failed', 'cancelled', 'expired'].includes(cur.status)) {
      state.activeLogin = ''; sessionStorage.removeItem('gpa.login');
      $('add-start').disabled = false;
      if (cur.status === 'succeeded') { $('dlg-add').close(); setResult(cur.updated_existing ? t('add.updated') : t('add.added')); await refresh(); }
      return;
    }
  } catch (err) { $('add-status').textContent = err.message; }
  loginTimer = setTimeout(pollLogin, 1000);
}
async function startLogin() {
  if (state.activeLogin) return;
  $('add-start').disabled = true;
  try {
    const rec = await api('/api/v1/logins', { method: 'POST', body: JSON.stringify({ display_name: $('add-name').value, account: $('add-start').dataset.account || '' }) });
    state.activeLogin = rec.id; sessionStorage.setItem('gpa.login', rec.id);
    await pollLogin();
  } catch (err) { $('add-status').textContent = ts(err.message); $('add-start').disabled = false; }
}
async function cancelLogin() {
  clearTimeout(loginTimer);
  try {
    if (state.activeLogin) await api('/api/v1/logins/' + state.activeLogin + '/cancel', { method: 'POST', body: '{}' });
    state.activeLogin = ''; sessionStorage.removeItem('gpa.login'); $('add-start').disabled = false; $('dlg-add').close();
  } catch (err) { $('add-status').textContent = err.message; }
}
async function continueOperation(id) {
  if (state.busy) return;
  state.busy = true;
  try {
    const op = await api('/api/v1/operations/' + id);
    if (op.status === 'waiting_user') { await handleEnv({ ...op, operation_id: op.id }); }
    else if (op.status === 'blocked' || op.status === 'failed') { await handleEnv(await api('/api/v1/operations/' + id + '/retry', { method: 'POST', body: JSON.stringify({ request_id: crypto.randomUUID() }) })); }
    else { setResult(ts(op.message) || opStatus(op.status)); }
  } catch (err) { setResult(err.message); }
  finally { state.busy = false; await refresh(); }
}
async function showOperations() {
  try {
    const data = await api('/api/v1/operations');
    $('ops-list').innerHTML = (data.operations || []).map((op) => `<li><strong>${escapeText(findName(op.account_id) || op.account)}</strong> ${escapeText(op.target)} · ${escapeText(opStatus(op.status))}<div class="meta">${escapeText(ts(op.message))}</div>${['waiting_user', 'blocked', 'failed'].includes(op.status) ? `<button data-operation="${escapeAttr(op.id)}">${escapeText(t('ops.continue'))}</button>` : ''}</li>`).join('') || `<li>${escapeText(t('ops.none'))}</li>`;
    $('dlg-ops').showModal();
  } catch (err) { setResult(err.message); }
}
async function showArchived() {
  const data = await api('/api/v1/accounts?archived=true');
  $('detail-title').textContent = t('archived.title');
  $('detail-body').innerHTML = (data.accounts || []).map((a) => `<p>${escapeText(a.display_name)} <button data-restore="${escapeAttr(a.id)}">${escapeText(t('archived.restore'))}</button></p>`).join('') || escapeText(t('archived.none'));
  $('dlg-detail').showModal();
}

async function importAccounts() {
  try {
    const prev = await api('/api/v1/imports/preview', { method: 'POST', body: '{}' });
    if (!(prev.new || []).length) {
      setResult(t('import.none'));
      return;
    }
    const res = await api('/api/v1/imports', { method: 'POST', body: '{}' });
    setResult(t('import.done', { added: res.added, skipped: res.skipped, conflicts: res.conflicts }));
    await refresh();
  } catch (err) {
    setResult(err.message);
  }
}

function wire() {
  $('btn-lang').onclick = () => setLang(state.lang === 'zh-CN' ? 'en' : 'zh-CN');
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
  $('detail-body').onclick = async (e) => { const b = e.target.closest('[data-restore]'); if (b) { try { await api('/api/v1/accounts/' + b.dataset.restore, { method: 'PATCH', body: JSON.stringify({ archived: false }) }); $('dlg-detail').close(); await refresh(); } catch (err) { setResult(err.message); } } };
  $('settings-archived').onclick = () => showArchived().catch((err) => setResult(err.message));
  $('detail-close').onclick = () => $('dlg-detail').close();
  $('btn-ops').onclick = showOperations;
  $('ops-close').onclick = () => $('dlg-ops').close();
  $('btn-settings').onclick = async () => {
    const d = await api('/api/v1/diagnostics');
    $('settings-body').textContent = t('settings.body', { version: d.version, count: d.accounts }) + (d.demo ? t('settings.demo') : '');
    $('dlg-settings').showModal();
  };
  $('settings-close').onclick = () => $('dlg-settings').close();
  $('settings-import').onclick = importAccounts;
  $('btn-import').onclick = importAccounts;
  $('settings-diag').onclick = async () => {
    const d = await api('/api/v1/diagnostics');
    $('detail-title').textContent = t('diag.title');
    $('detail-body').textContent = JSON.stringify(d, null, 2);
    $('dlg-detail').showModal();
  };
}

async function main() {
  applyLang();
  wire();
  try {
    await bootstrap();
    $('conn').textContent = t('conn.ok');
    await refresh();
    if (state.pendingOp) await continueOperation(state.pendingOp);
    if (state.activeLogin) { $('dlg-add').showModal(); $('add-start').disabled = true; await pollLogin(); }
    setInterval(() => { if (!state.busy && !document.hidden) refresh(); }, 5000);
  } catch (err) {
    showBanner(err.message + t('banner.bootstrap'), 'offline');
  }
}

main();
