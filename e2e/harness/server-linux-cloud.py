#!/usr/bin/env python3
"""Run precompiled server tests in a deployment-owned temporary directory.

Only a disposable MySQL instance is created; no existing database is queried.
iCode uses a local deterministic model fixture, with the real native runner.
"""
import json
import os
import pathlib
import platform
import signal
import socket
import subprocess
import sys
import time


ROOT = pathlib.Path(sys.argv[1]).resolve()
if not str(ROOT).startswith('/tmp/opscopilot-linux-e2e-') or ROOT.parent == pathlib.Path('/tmp'):
    raise SystemExit('Expected a dedicated run subdirectory beneath /tmp/opscopilot-linux-e2e-*')
ENV = dict(os.environ)
ENV.update(HOME=str(ROOT / 'home'), TMPDIR=str(ROOT / 'tmp'),
           XDG_DATA_HOME=str(ROOT / 'xdg-data'),
           OPSCOPILOT_TEST_CLI_BINARY=str(ROOT / 'bin/opscopilot-cli'))
for name in ('OPSCOPILOT_TEST_JDBC_RUNTIME', 'OPSCOPILOT_TEST_JDBC_JAR',
             'OPSCOPILOT_TEST_MYSQL_PORT', 'OPSCOPILOT_TEST_MYSQL_PASSWORD'):
    ENV.pop(name, None)
for name in ('home', 'tmp', 'logs'):
    (ROOT / name).mkdir(mode=0o700, exist_ok=True)
REPORT = {'environment': {'system': platform.platform(), 'python': platform.python_version()},
          'model': 'deterministic-local-fixture', 'stages': [], 'complete': False}


def save():
    target = ROOT / 'result.json'
    temp = ROOT / 'result.json.tmp'
    temp.write_text(json.dumps(REPORT, ensure_ascii=False, indent=2), encoding='utf-8')
    os.replace(str(temp), str(target))


def stop_group(child):
    # Every child is started in its own session; never signal a service PID.
    for signum in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.killpg(child.pid, signum)
        except ProcessLookupError:
            break
        try:
            child.wait(timeout=3)
        except subprocess.TimeoutExpired:
            continue
        if signum == signal.SIGTERM:
            time.sleep(0.1)


def stage(name, command, timeout=120, env=None):
    item = {'name': name, 'status': 'running'}
    REPORT['stages'].append(item)
    save()
    started = time.monotonic()
    log = ROOT / 'logs' / (name + '.log')
    with log.open('wb') as output:
        child = subprocess.Popen(command, cwd=str(ROOT), env=env or ENV,
                                 stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
        try:
            code = child.wait(timeout=timeout)
            item.update(exitCode=code, status='passed' if code == 0 else 'failed')
        except subprocess.TimeoutExpired:
            item.update(status='timeout', exitCode=None)
        finally:
            stop_group(child)
    item.update(durationSeconds=round(time.monotonic() - started, 3), log=log.name)
    save()
    print(name + ': ' + item['status'], flush=True)
    return item['status'] == 'passed'


def unused_port():
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        return listener.getsockname()[1]


def mysql_trial():
    base = pathlib.Path('/www/server/mysql')
    server, client = base / 'bin/mysqld', base / 'bin/mysql'
    if not server.is_file() or not client.is_file():
        REPORT['stages'].append({'name': 'jdbc', 'status': 'unavailable', 'reason': 'No prepared MySQL binaries'})
        save()
        return
    directory = ROOT / 'mysql'
    directory.mkdir(mode=0o700)
    data, sock = directory / 'data', directory / 'mysql.sock'
    init = [str(server), '--no-defaults', '--initialize-insecure', '--user=root',
            '--basedir=' + str(base), '--datadir=' + str(data)]
    if not stage('mysql-initialize', init):
        return
    port = unused_port()
    logfile = (ROOT / 'logs/mysql-server.log').open('wb')
    command = [str(server), '--no-defaults', '--user=root', '--basedir=' + str(base),
               '--datadir=' + str(data), '--socket=' + str(sock), '--bind-address=127.0.0.1',
               '--port=' + str(port), '--pid-file=' + str(directory / 'pid'), '--skip-log-bin',
               '--innodb-buffer-pool-size=32M', '--innodb-log-file-size=8M',
               '--performance-schema=OFF', '--max-connections=10', '--loose-mysqlx=0']
    child = subprocess.Popen(command, cwd=str(ROOT), env=ENV, stdout=logfile,
                             stderr=subprocess.STDOUT, start_new_session=True)
    try:
        deadline = time.monotonic() + 30
        while not sock.exists() and child.poll() is None and time.monotonic() < deadline:
            time.sleep(0.2)
        if not sock.exists() or child.poll() is not None:
            raise RuntimeError('Disposable MySQL did not start')
        password = os.urandom(24).hex()
        sql = "CREATE DATABASE opscopilot_review; CREATE USER 'ops_review'@'127.0.0.1' IDENTIFIED BY '" + password + "';\n"
        sql += "GRANT SELECT ON opscopilot_review.* TO 'ops_review'@'127.0.0.1'; USE opscopilot_review;\n"
        sql += 'CREATE TABLE service_nodes(id INT PRIMARY KEY); INSERT INTO service_nodes VALUES(1);\n'
        sql += 'CREATE TABLE query_scratch(id INT); CREATE TABLE job_runs(id INT PRIMARY KEY);\n'
        sql += 'INSERT INTO job_runs VALUES' + ','.join('(%d)' % n for n in range(1, 261)) + ';\n'
        with (ROOT / 'logs/mysql-seed.log').open('wb') as output:
            seeded = subprocess.run([str(client), '--no-defaults', '--protocol=SOCKET',
                                     '--socket=' + str(sock), '--user=root'], input=sql.encode(),
                                    env=ENV, stdout=output, stderr=subprocess.STDOUT, timeout=20)
        if seeded.returncode:
            raise RuntimeError('Disposable MySQL fixture creation failed')
        test_env = dict(ENV)
        test_env.update(OPSCOPILOT_TEST_JDBC_RUNTIME=str(ROOT / 'jdbc-runtime'),
                        OPSCOPILOT_TEST_JDBC_JAR=str(ROOT / 'mysql-connector.jar'),
                        OPSCOPILOT_TEST_MYSQL_PORT=str(port), OPSCOPILOT_TEST_MYSQL_PASSWORD=password)
        stage('jdbc', [str(ROOT / 'bin/opscli.test'), '-test.run', '^TestDatabaseLiveRunJDBCWorkflow$',
                       '-test.v'], env=test_env)
    finally:
        stop_group(child)
        logfile.close()


try:
    for package in ('opscli', 'opssession', 'filetxn'):
        stage(package, [str(ROOT / ('bin/' + package + '.test')), '-test.v'])
    stage('execution', [str(ROOT / 'bin/ops.test'), '-test.run',
                        'TestPrepareOnce|TestDirectoryFailure|TestOutput', '-test.v'])
    stage('sshclient', [str(ROOT / 'bin/sshclient.test'), '-test.run', '^TestRunCommand', '-test.v'])
    mysql_trial()
    code = ROOT / 'icode/icode'
    if code.is_file():
        if stage('icode-version', [str(code), '--version'], timeout=180):
            test_env = dict(ENV, OPSCOPILOT_CHRYS_TEST_EXE=str(code))
            stage('icode-native-cli', [str(ROOT / 'bin/acp.test'), '-test.run',
                                      '^TestLiveICodeNativeServerCLIReusesSession$', '-test.v'],
                  timeout=180, env=test_env)
    else:
        REPORT['stages'].append({'name': 'icode-native-cli', 'status': 'unavailable'})
except Exception as error:
    REPORT['error'] = type(error).__name__ + ': ' + str(error)
finally:
    REPORT['complete'] = True
    REPORT['passed'] = all(item['status'] == 'passed' for item in REPORT['stages']) and 'error' not in REPORT
    save()
    print('complete: ' + str(REPORT['passed']), flush=True)
