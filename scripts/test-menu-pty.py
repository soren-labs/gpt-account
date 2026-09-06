#!/usr/bin/env python3
"""Exercise the built GPA in a real PTY with synthetic accounts and stub processes."""
import base64
import json
import os
from pathlib import Path
import pty
import select
import subprocess
import tempfile
import time

BINARY = Path(__file__).resolve().parents[1] / 'dist/gpa'

def auth(name):
    claims = {'email': name+'@example.invalid', 'sub': 'user-'+name,
              'https://api.openai.com/auth': {'chatgpt_user_id': 'user-'+name, 'chatgpt_plan_type': 'plus'}}
    token = 'e30.'+base64.urlsafe_b64encode(json.dumps(claims).encode()).decode().rstrip('=')+'.fake'
    return {'auth_mode':'chatgpt','last_refresh':'2026-01-01T00:00:00Z',
            'tokens':{'account_id':'ws-'+name,'id_token':token,'access_token':'fake','refresh_token':'fake-'+name}}

class Terminal:
    def __init__(self, env):
        self.fd, slave = pty.openpty()
        self.process = subprocess.Popen([str(BINARY)], stdin=slave, stdout=slave, stderr=slave, env=env)
        os.close(slave)
        self.output = ''
        self.read_until('编号/名称/命令：')
    def read_until(self, text):
        end = time.monotonic()+8
        chunk = b''
        while time.monotonic()<end:
            if select.select([self.fd],[],[],0.1)[0]:
                try: data=os.read(self.fd,65536)
                except OSError: break
                if not data: break
                chunk += data
                if text in chunk.decode(errors='replace'):
                    while select.select([self.fd],[],[],0.2)[0]:
                        try: chunk += os.read(self.fd,65536)
                        except OSError: break
                    self.output += chunk.decode(errors='replace')
                    return
        raise AssertionError('missing '+text+' in '+chunk.decode(errors='replace')[-1600:])
    def send(self, keys, expect='编号/名称/命令：'):
        os.write(self.fd, keys.encode())
        self.read_until(expect)
    def close(self):
        os.write(self.fd,b'/q\r')
        self.process.wait(timeout=5)
        os.close(self.fd)
        assert self.process.returncode==0

with tempfile.TemporaryDirectory(prefix='gpa-pty-') as tmp:
    root=Path(tmp)
    store=root/'store'
    bin_dir=root/'bin'; bin_dir.mkdir()
    env=dict(os.environ, GPA_STORE=str(store), GPA_CODEX_HOME=str(root/'linux'),
             GPA_WINDOWS_CODEX=str(root/'windows'), GPA_CHATGPT='off', GPA_IGNORE_CLI='1',
             GPA_CALLER='human', GPA_FAKE_CLI='', GPA_FAKE_APP='', GPA_PROC_QUERY='',
             PATH=str(bin_dir)+':/usr/bin:/bin', HOME=str(root), TERM='xterm-256color',
             GPA_TEST_ROOT=str(root))
    # All external app/process commands are local stubs, even when app mode is on.
    stub='''#!/usr/bin/python3
import os,json
from pathlib import Path
root=Path(os.environ['GPA_TEST_ROOT'])
name=Path(__file__).name
if name=='tasklist.exe':
 if (root/'running').exists(): print('ChatGPT.exe')
elif name=='taskkill.exe': (root/'running').unlink(missing_ok=True)
elif name=='powershell.exe': (root/'running').touch()
elif name=='codex':
 Path(os.environ['CODEX_HOME'],'auth.json').write_text((root/'login.json').read_text())
 print('SIMULATED LOGIN COMPLETE')
'''
    for name in ['tasklist.exe','taskkill.exe','powershell.exe','codex','wsl.exe']:
        p=bin_dir/name; p.write_text(stub); p.chmod(0o755)
    for name in ['biz1','biz2','plus']:
        slot=store/'accounts'/name; slot.mkdir(parents=True)
        (slot/'auth.json').write_text(json.dumps(auth(name)))
        (slot/'meta.json').write_text(json.dumps({'name':name}))
    for folder in ['linux','windows']:
        (root/folder).mkdir()
        (root/folder/'auth.json').write_text(json.dumps(auth('plus')))
    (root/'login.json').write_text(json.dumps(auth('added')))
    t=Terminal(env)
    t.send('plus\r', '上次结果：completed')
    assert json.loads((store/'state.json').read_text())['current']=='plus'
    # Fragmented escape sequence, followed by Enter; then batched text/backspace.
    t.send('\x1b[B')
    t.send('\r','上次结果：completed')
    assert json.loads((store/'state.json').read_text())['current']=='biz2'
    t.send('biz9\x7f1\r','上次结果：completed')
    assert json.loads((store/'state.json').read_text())['current']=='biz1'
    t.send('/d\r','编号/名称/命令：')
    t.send('/t\r','切换范围：ChatGPT App')
    t.send('/a\r','新账号别名')
    t.send('added\r','已添加账号：added')
    assert (store/'accounts/added/auth.json').exists()
    t.close()
    print('PASS PTY: name, arrows, backspace, diagnostics, scope, simulated login, quit')

    env['GPA_FAKE_CLI']='running'
    t=Terminal(env)
    t.send('biz2\r','请先关闭对应 Codex')
    t.send('y\r','请先关闭对应 Codex')
    t.send('r\r','请先关闭对应 Codex')
    assert len(list((store/'operations').glob('*.json')))==1
    t.send('n\r','已暂缓')
    t.close()
    print('PASS PTY: busy CLI hint, retry retains ID, defer')

    env['GPA_FAKE_CLI']=''; env['GPA_CHATGPT']='auto'
    (root/'running').touch()
    t=Terminal(env)
    t.send('plus\r','Y 确认重启 App')
    ops=list((store/'operations').glob('*.json'))
    pending=[p for p in ops if json.loads(p.read_text())['account']=='plus'][0]
    t.send('y\r','上次结果：completed')
    assert json.loads(pending.read_text())['status']=='applied'
    assert len(list((store/'operations').glob('*.json')))==len(ops)
    t.close()
    print('PASS PTY: simulated App stop/start and original operation marked applied')

    # First-run account addition returns to the menu instead of exiting.
    env['GPA_STORE']=str(root/'empty-store'); env['GPA_CHATGPT']='off'
    t=Terminal(env)
    t.send('2\r','新账号别名')
    t.send('added\r','已添加账号：added')
    t.close()
    print('PASS PTY: first-run add returns to menu')
