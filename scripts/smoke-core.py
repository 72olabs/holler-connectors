#!/usr/bin/env python3
"""Black-box compatibility check against a supplied Holler daemon executable.
Never opens the database or requires private source, credentials, or a real harness.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--daemon', required=True, type=Path)
    parser.add_argument('--external-wakes', action='store_true', help='require core support for --connector-binary')
    parser.add_argument('--connector', type=Path, default=Path('dist/holler-connector'))
    args = parser.parse_args()
    daemon, connector = str(args.daemon.resolve()), str(args.connector.resolve())
    with tempfile.TemporaryDirectory(prefix='hc-', dir='/tmp') as tmp:
        env = {k: v for k, v in os.environ.items() if not k.startswith(('HOLLER_', 'CODEX_', 'CLAUDE_'))}
        env.update(HOME=tmp, HOLLER_SOCKET=tmp+'/h.sock', HOLLER_RUN='smoke-run')
        log = open(tmp+'/daemon.log', 'w+')
        daemon_args = [daemon, '--socket', env['HOLLER_SOCKET'], '--db', tmp+'/core.db']
        if args.external_wakes:
            daemon_args += ['--connector-binary', connector]
        process = subprocess.Popen(daemon_args, env=env, stdout=log, stderr=log)
        def run(*argv, input=None):
            result = subprocess.run([connector, *argv], env=env, input=input, capture_output=True, text=True, timeout=15)
            if result.returncode:
                raise RuntimeError(f'{argv[0]} failed: {result.stderr}')
            return result.stdout
        try:
            deadline = time.monotonic()+10
            while not Path(env['HOLLER_SOCKET']).exists():
                if process.poll() is not None or time.monotonic() > deadline:
                    log.seek(0)
                    raise RuntimeError('daemon failed to start: '+log.read())
                time.sleep(.05)
            status = json.loads(run('status'))
            sent = json.loads(run('send', '--actor', 'smoke-sender', '--run', 'smoke-run', '--to', 'smoke-recipient', '--idempotency-key', 'smoke-send', '--body', '{"text":"connector compatibility"}'))
            inbox = json.loads(run('inbox', '--actor', 'smoke-recipient'))
            assert len(inbox) == 1, inbox
            claim = json.loads(run('claim', '--actor', 'smoke-recipient'))
            run('ack', '--actor', 'smoke-recipient', '--message', claim['message']['message_id'], '--lease-token', claim['lease_token'])
            assert not json.loads(run('inbox', '--actor', 'smoke-recipient'))
            requests = [
                {'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':'2024-11-05','capabilities':{},'clientInfo':{'name':'smoke','version':'1'}}},
                {'jsonrpc':'2.0','method':'notifications/initialized'},
                {'jsonrpc':'2.0','id':2,'method':'tools/list'},
            ]
            output = run('mcp', '--actor', 'smoke-mcp', '--run', 'smoke-run', input=''.join(json.dumps(x)+'\n' for x in requests))
            responses = [json.loads(line) for line in output.splitlines()]
            tools = next(r['result']['tools'] for r in responses if r.get('id') == 2)
            assert any(t['name'] == 'bus_inbox' for t in tools)
            print(json.dumps({'ok':True,'checks':['daemon handshake','send/inbox/claim/ack','MCP initialize/tools'], 'daemon_status':status}, indent=2))
        finally:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
            log.close()

if __name__ == '__main__':
    main()
