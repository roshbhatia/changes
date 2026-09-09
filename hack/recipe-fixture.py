#!/usr/bin/env python3
"""Create a tested authorization-header change for recipe recordings."""
from pathlib import Path
import subprocess
import sys
root = Path(sys.argv[1]).resolve()
mode = sys.argv[2] if len(sys.argv) > 2 else 'change'
def run(*args):
    subprocess.run(args, cwd=root, check=True, stdout=subprocess.DEVNULL)
def write(name, text):
    path = root / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)
def commit(message):
    run('gofmt', '-w', '.')
    run('git', 'add', '.')
    run('git', 'commit', '-qm', message)
OLD = 'package auth\nimport "strings"\nfunc ValidToken(header string) bool { return strings.Contains(header, "Bearer ") }\n'
NEW = 'package auth\nimport "strings"\nfunc ValidToken(header string) bool {\n if !strings.HasPrefix(header, "Bearer ") { return false }\n token := strings.TrimPrefix(header, "Bearer ")\n return token != "" && !strings.ContainsAny(token, " \\t\\n")\n}\n'
TEST = 'package auth\nimport "testing"\nfunc TestTokenBoundary(t *testing.T) {\n for _, tc := range []struct{header string; valid bool}{\n {"Bearer signed-token",true},{"",false},{"Bearer ",false},{"prefix Bearer token",false},{"Bearer two tokens",false},\n } { if got:=ValidToken(tc.header); got!=tc.valid {t.Errorf("%q: got %v want %v",tc.header,got,tc.valid)} }\n}\n'
if mode == 'history':
    prefix_only = 'package auth\nimport "strings"\nfunc ValidToken(header string) bool { return strings.HasPrefix(header, "Bearer ") }\n'
    write('internal/auth/token.go', prefix_only)
    commit('require the Bearer prefix')
    write('internal/auth/token.go', prefix_only.replace('return strings.HasPrefix(header, "Bearer ")', 'return strings.HasPrefix(header, "Bearer ") && len(header) > len("Bearer ")'))
    commit('reject an empty bearer token')
    write('internal/auth/token.go', NEW)
else:
    root.mkdir(parents=True, exist_ok=True)
    run('git', 'init', '-q')
    run('git', 'config', 'user.email', 'review@example.invalid')
    run('git', 'config', 'user.name', 'Review fixture')
    write('go.mod', 'module checkout-service\n\ngo 1.26\n')
    write('internal/auth/token.go', OLD)
    commit('add authorization header parser')
    write('internal/auth/token.go', NEW)
    write('internal/auth/token_test.go', TEST)
    if mode == 'groups':
        commit('validate bearer tokens')
        write('handler.go', 'package checkout\nfunc Authorize(header string) bool { return header != "" }\n')
        write('store.go', 'package checkout\nfunc SessionKey(header string) string { return header }\n')
        commit('add session lookup boundary')
        write('handler.go', 'package checkout\nimport "checkout-service/internal/auth"\nfunc Authorize(header string) bool { return auth.ValidToken(header) }\n')
        write('store.go', 'package checkout\nimport "strings"\nfunc SessionKey(header string) string { return strings.TrimPrefix(header, "Bearer ") }\n')
        write('session_test.go', 'package checkout\nimport "testing"\nfunc TestSessionBoundary(t *testing.T) { if Authorize("prefix Bearer token") || SessionKey("Bearer signed-token") != "signed-token" { t.Fatal("invalid session boundary") } }\n')
run('gofmt', '-w', '.')
run('go', 'test', './...')
