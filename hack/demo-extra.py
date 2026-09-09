import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
ROOT = pathlib.Path(__file__).resolve().parents[1]
NAME = sys.argv[1]
META = json.loads((ROOT / 'extras' / NAME / 'demo.json').read_text())
TOOL = META['core']
OLD = 'package auth\nimport "strings"\nfunc ValidToken(header string) bool { return strings.Contains(header, "Bearer ") }\n'
NEW = 'package auth\nimport "strings"\nfunc ValidToken(header string) bool {\n if !strings.HasPrefix(header, "Bearer ") { return false }\n token := strings.TrimPrefix(header, "Bearer ")\n return token != "" && !strings.ContainsAny(token, " \\t\\n")\n}\n'
TEST = 'package auth\nimport "testing"\nfunc TestTokenBoundary(t *testing.T) {\n for _, tc := range []struct{header string; valid bool}{\n {"Bearer signed-token",true},{"",false},{"Bearer ",false},{"prefix Bearer token",false},{"Bearer two tokens",false},\n } { if got:=ValidToken(tc.header); got!=tc.valid {t.Errorf("%q: got %v want %v",tc.header,got,tc.valid)} }\n}\n'

def execute(argv, cwd, env, stdin=None, show=True, check=True):
    if show:
        display = [pathlib.Path(str(argv[0])).name, *[str(value).replace(str(cwd), '.') for value in argv[1:]]]
        print('$ ' + ' '.join(display), flush=True)
    result = subprocess.run([str(value) for value in argv], cwd=cwd, env=env, input=stdin, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=90)
    if show or result.returncode:
        print(result.stdout.rstrip().replace(str(cwd), '.'), flush=True)
    if check and result.returncode:
        raise RuntimeError('command failed: ' + str(result.returncode))
    return result

def build(package, binary, bins):
    module = ROOT
    source = ROOT / package.removeprefix('./')
    if (source / 'go.mod').exists():
        module, package = (source, '.')
    target = bins / binary
    execute(['go', 'build', '-o', target, package], module, os.environ.copy(), show=False)
    return str(target)

def shim(bins, name, body):
    target = bins / name
    target.write_text('#!/usr/bin/env python3\n' + body)
    target.chmod(493)

def demo(work, env, bins):
    binary = build('./extras/' + NAME, META['binary'], bins)
    if NAME in {'ast-grep', 'calldiff'}:
        request = {'version': 'changes.provider/v1', 'action': 'changes.symbols' if NAME == 'ast-grep' else 'changes.calls', 'directory': str(work), 'files': ['internal/auth/token.go']}
        result = execute([binary], work, env, json.dumps(request), show=False)
        response = json.loads(result.stdout)
        for file, symbols in response.get('symbols', {}).items():
            for symbol in symbols:
                print(file + ':' + str(symbol['From']) + '  ' + symbol['Name'], flush=True)
        for file, edges in response.get('edges', {}).items():
            for edge in edges:
                print(('+' if edge['Added'] else '-') + ' ' + file + ':' + str(edge['Line']) + '  changed call', flush=True)
    else:
        core = build('./cmd/changes', 'changes', bins)
        providers = pathlib.Path(env['XDG_CONFIG_HOME']) / 'changes/providers' / NAME
        providers.mkdir(parents=True)
        shutil.copy(ROOT / 'extras' / NAME / 'provider.yaml', providers / 'provider.yaml')
        if NAME == 'codex-review':
            print('Offline model response fixture.', flush=True)
            (bins / 'codex').write_text((ROOT / 'hack/demo-codex.sh').read_text().replace('main.go', 'internal/auth/token.go').replace('line\":8', 'line\":4').replace('Keep the fallback explicit', 'Reject malformed authorization headers').replace('Callers rely on this empty-input behavior.', 'Validate the prefix before session lookup.'))
            (bins / 'codex').chmod(493)
            store = build('./extras/local-notes', 'changes-provider-local-notes', bins)
            target = providers.parent / 'local-notes'
            target.mkdir()
            shutil.copy(ROOT / 'extras/local-notes/provider.yaml', target / 'provider.yaml')
            execute([core, 'note', 'generate', '--provider', NAME, '--store', 'local-notes'], work, env)
        elif NAME == 'github-pr':
            print('Offline pull request thread fixture.', flush=True)
            shutil.copy(ROOT / 'hack/demo-gh.sh', bins / 'gh')
            (bins / 'gh').chmod(493)
            execute(['git', 'remote', 'add', 'origin', 'https://github.com/example/checkout-service.git'], work, env, show=False)
            execute([core, 'note', 'list', '--provider', NAME], work, env)
        else:
            args = [core, 'note', 'add', '--provider', NAME, '--file', 'internal/auth/token.go', '--line', '4', '--message', 'Keep prefix validation before session lookup']
            if NAME == 'git-notes':
                execute(['git', 'add', 'internal/auth/token.go'], work, env, show=False)
                execute(['git', 'commit', '-qm', 'Reject malformed authorization headers'], work, env, show=False)
                args.extend(['--commit', 'HEAD'])
            execute(args, work, env)
            execute([core, 'note', 'list', '--provider', NAME], work, env)
        execute(['git', 'diff', '--', 'internal/auth/token.go'], work, env)

def main():
    with tempfile.TemporaryDirectory(prefix='token-review-') as temporary:
        root = pathlib.Path(temporary).resolve()
        work = root / 'checkout-service'
        bins = root / 'bin'
        bins.mkdir()
        (work / 'internal/auth').mkdir(parents=True)
        (work / 'go.mod').write_text('module checkout-service\n\ngo 1.26\n')
        path = work / 'internal/auth/token.go'
        path.write_text(OLD)
        (work / 'internal/auth/token_test.go').write_text(TEST)
        env = os.environ.copy()
        for name, dirname in [('HOME', 'home'), ('XDG_CONFIG_HOME', 'config'), ('XDG_DATA_HOME', 'data'), ('XDG_STATE_HOME', 'state'), ('XDG_CACHE_HOME', 'cache'), ('XDG_RUNTIME_DIR', 'runtime')]:
            (root / dirname).mkdir()
            env[name] = str(root / dirname)
        env['PATH'] = str(bins) + os.pathsep + env['PATH']
        env['XDG_DATA_DIRS'] = str(root / 'data')
        for name in ['ORC_SESSION_ID', 'ORC_SCOPE', 'WEZTERM_PANE', 'WEZTERM_UNIX_SOCKET', 'GATE_STATE_DIR']:
            env.pop(name, None)
        execute(['git', 'init', '-b', 'main'], work, env, show=False)
        execute(['git', 'config', 'user.name', 'Review fixture'], work, env, show=False)
        execute(['git', 'config', 'user.email', 'review@example.invalid'], work, env, show=False)
        execute(['git', 'add', '.'], work, env, show=False)
        execute(['git', 'commit', '-m', 'add token parser'], work, env, show=False)
        path.write_text(NEW)
        print(META['summary'] + '\n', flush=True)
        demo(work, env, bins)
        print('\nDemo complete', flush=True)
if __name__ == '__main__':
    main()
