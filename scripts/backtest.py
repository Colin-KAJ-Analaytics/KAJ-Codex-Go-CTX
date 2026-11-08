#!/usr/bin/env python3
"""Matched executable backtest. No third-party Python modules or git required.

Run from the repository root after building bin/goctx:
  python3 scripts/backtest.py --baseline /usr/local/bin/goctx --candidate ./bin/goctx
The baseline is an existing executable, not a reconstructed pre-change source tree.
"""
import argparse
import csv
import hashlib
import json
import os
from pathlib import Path
import random
import shlex
import statistics
import subprocess
import tempfile
import time


def invoke(exe, args, cwd, env, allow_failure=False):
    start = time.perf_counter_ns()
    result = subprocess.run([str(exe), *args], cwd=cwd, env=env, capture_output=True, timeout=90)
    elapsed = (time.perf_counter_ns() - start) / 1e6
    if result.returncode and not allow_failure:
        raise RuntimeError(f'{exe.name} {args}: {result.stderr.decode(errors="replace")[:1000]}')
    return result, elapsed


def fixture(root, files_per_package):
    root.mkdir(parents=True)
    (root / 'go.mod').write_text('module example.com/backtest\n\ngo 1.23.0\n')
    for package in ('auth', 'billing', 'catalog', 'delivery'):
        folder = root / package
        folder.mkdir()
        for i in range(files_per_package):
            lines = [f'package {package}\n']
            for j in range(8):
                lines.append(f'// Validate{package.title()}{i}_{j} validates the current {package} contract.\n')
                lines.append(f'func Validate{package.title()}{i}_{j}(value string) bool {{ return value != "" }}\n')
            (folder / f'{package}_{i:03}.go').write_text(''.join(lines))
        (folder / 'verify.go').write_text(f'package {package}\nfunc VerifyToken(value string) bool {{ return value != "" }}\n')
        (folder / 'verify_test.go').write_text(f'package {package}\nimport "testing"\nfunc TestVerifyToken(t *testing.T) {{ if !VerifyToken("x") {{ t.Fatal("expected valid token") }} }}\n')
    (root / 'routes.go').write_text('''package backtest
import "net/http"
func Health(w http.ResponseWriter, r *http.Request) {}
func Login(w http.ResponseWriter, r *http.Request) {}
func RegisterRoutes() {
 http.HandleFunc("/health", Health)
 http.HandleFunc("/login", Login)
}
''')


def percent(before, after):
    return round((before-after)*100/before, 2) if before else 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--baseline', required=True, type=Path)
    parser.add_argument('--candidate', default=Path('bin/goctx'), type=Path)
    parser.add_argument('--output', default=Path('benchmarks/latest'), type=Path)
    parser.add_argument('--repeats', type=int, default=7)
    parser.add_argument('--require-identical-output', action='store_true')
    parser.add_argument('--baseline-description', default='Existing October 22 installed binary; not an exact reconstruction of the source immediately before the reviewed changes.')
    args = parser.parse_args()
    repo = Path.cwd().resolve()
    baseline, candidate, output = args.baseline.resolve(), args.candidate.resolve(), args.output.resolve()
    if args.repeats < 3:
        parser.error('--repeats must be at least 3')
    output.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, GOCACHE='/tmp/goctx-review-cache', GOPROXY='off', GOWORK='off', GOFLAGS='-buildvcs=false')
    rng = random.Random(20261107)
    checks, measurements = [], []
    def check(name, condition, detail=''):
        checks.append({'name':name, 'passed':bool(condition), 'detail':detail})
        print(('PASS ' if condition else 'FAIL ') + name, flush=True)
    with tempfile.TemporaryDirectory(prefix='goctx-backtest-') as workdir:
        work = Path(workdir)
        # Snapshots make the comparison independent of later installation changes.
        for name, exe in (('baseline',baseline),('candidate',candidate)):
            snapshot = work / name
            snapshot.write_bytes(exe.read_bytes()); snapshot.chmod(0o755)
            if name=='baseline': old=snapshot
            else: new=snapshot
        fixtures = []
        for name, size in (('small',1), ('medium',10), ('large',50)):
            root=work/name
            fixture(root,size)
            fixtures.append((name,root))
        small = fixtures[0][1]
        behavior = work/'behavior'; behavior.mkdir()
        (behavior/'go.mod').write_text('module example.com/behavior\n\ngo 1.23.0\n')
        sources = {
            'passing': 'package p\nimport "testing"\nfunc TestHandlesFAILure(t *testing.T) { t.Log("expected literal FAIL") }\n',
            'subtests': 'package p\nimport "testing"\nfunc TestX(t *testing.T) { t.Run("case_two/punctuation#01",func(t *testing.T){t.Fatal("expected valid input")}) }\n',
            'compile': 'package p\nvar _ = MissingSymbol\n',
            'noisy': 'package p\nimport "testing"\nfunc TestNoisy(t *testing.T) { for i:=0;i<1200;i++ {t.Logf("file%d.go:%d: expected result %s",i,i+1,"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")}; t.Fail() }\n',
        }
        diagnostics={}
        for name,src in sources.items():
            folder=behavior/name; folder.mkdir()
            (folder/'probe_test.go').write_text(src)
            command=['test-summary','--','go','test','-v','-count=1','.']
            results={}
            for label, exe in (('baseline',old),('candidate',new)):
                result, ms=invoke(exe,command,folder,env,True)
                results[label]={'exit':result.returncode,'bytes':len(result.stdout),'estimated_tokens':len(result.stdout)//4,'output':result.stdout.decode(errors='replace'),'ms':ms}
            diagnostics[name]=results
        check('D1 nested subtest names retained','TestX/case_two/punctuation#01' in diagnostics['subtests']['candidate']['output'])
        p=diagnostics['passing']['candidate']
        check('D2 passing FAIL-name test has passing status',p['exit']==0 and p['output'].startswith('Tests passed.'))
        p=diagnostics['compile']['candidate']
        check('D2 compiler exit and actual diagnostic retained',p['exit']!=0 and p['output'].startswith('FAIL') and 'MissingSymbol' in p['output'])
        p=diagnostics['noisy']['candidate']
        check('D3 noisy failure bounded and omissions visible',p['exit']!=0 and p['bytes']<=12000 and 'Omitted:' in p['output'],f"{p['bytes']} bytes")
        for budget in (250,500,1000,2000):
            for mode in ([],['--compact-json'],['--quiet','--compact-json']):
                command=['codex-pack','fix auth token','--root',str(small),'--format','json','--budget',str(budget),*mode]
                result,_=invoke(new,command,repo,env)
                decoded=json.loads(result.stdout)
                actual=len(result.stdout)//4
                metadata_only = not any(decoded[k] for k in ('files','symbols','routes','tests','read_plan'))
                explicit_overflow = metadata_only and any('exceeds requested budget' in w for w in decoded['warnings'])
                check(f'D4 JSON accounting budget={budget} mode={mode}',decoded['actual_estimated_tokens']==actual and (actual<=budget or explicit_overflow),f'emitted={actual}; metadata-only overflow={explicit_overflow}')
        tiny,_=invoke(new,['codex-pack','fix auth','--root',str(small),'--format','json','--budget','1'],repo,env)
        td=json.loads(tiny.stdout)
        check('D4 impossible budget reported honestly',td['actual_estimated_tokens']==len(tiny.stdout)//4 and any('exceeds' in x for x in td['warnings']))
        route,_=invoke(new,['codex-pack','fix login','--root',str(small),'--format','json','--max-routes','1','--budget','10000'],repo,env)
        rd=json.loads(route.stdout)
        check('D5 relevant route selected before limit',len(rd['routes'])==1 and rd['routes'][0]['path']=='/login')
        plan,_=invoke(new,['task','VerifyToken','--root',str(small),'--format','json','--budget','20000'],repo,env)
        pd=json.loads(plan.stdout)
        locations={s['file'] for s in pd['symbols'] if s['name']=='VerifyToken'}
        valid=True; count=0
        for command in pd['read_plan']:
            argv=shlex.split(command)
            if len(argv)>1 and argv[1]=='slice':
                r,_=invoke(new,argv[1:],small,env,True);valid &= r.returncode==0;count+=1
        check('D6 same-named symbols keep distinct files',len(locations)==4)
        check('D6 emitted slice commands actually execute',valid and count>0,f'{count} slices')
        text,_=invoke(new,['codex-pack','fix auth token','--root',str(small),'--budget','250'],repo,env)
        textbody=text.stdout.decode()
        check('D7 small text budget retains actions and tests',len(text.stdout)//4<=250 and 'goctx symbols' in textbody and 'go test ' in textbody)
        full,_=invoke(new,['codex-pack','fix auth token','--root',str(small),'--format','json','--quiet','--budget','20000'],repo,env)
        compact,_=invoke(new,['codex-pack','fix auth token','--root',str(small),'--format','json','--quiet','--compact-json','--budget','20000'],repo,env)
        a,b=json.loads(full.stdout),json.loads(compact.stdout)
        a.pop('actual_estimated_tokens');b.pop('actual_estimated_tokens')
        check('E5 compact JSON preserves semantic content',a==b and len(compact.stdout)<len(full.stdout))
        check('G6 quiet keeps warnings/omissions fields and removes advice','warnings' in b and 'omitted' in b and 'savings' not in b['data'] and not any('reasons' in f for f in b['files']))
        repeated=[invoke(new,['codex-pack','fix login','--root',str(small),'--format','json','--budget','2000'],repo,env)[0].stdout for _ in range(4)]
        check('E10 byte-stable repeated pack output',len(set(repeated))==1)
        claude=work/'claude';claude.mkdir()
        shared='Existing shared instructions.\nDo not use git commands.\n'
        custom='Existing Claude instructions.\n'
        (claude/'AGENTS.md').write_text(shared);(claude/'CLAUDE.md').write_text(custom)
        first,_=invoke(new,['init-claude','--root',str(claude)],repo,env)
        second,_=invoke(new,['init-claude','--root',str(claude)],repo,env)
        body=(claude/'CLAUDE.md').read_text()
        check('H4 Claude bridge preserves policies and is idempotent',(claude/'AGENTS.md').read_text()==shared and body.startswith(custom) and body.count('@AGENTS.md')==1 and b'updated:' not in second.stdout)
        # Each variant is warmed once. Timed trials are paired and order-shuffled.
        roots=[('real_repo',repo),*fixtures]
        for root_name,root in roots:
            workloads=[
                ('task_text',['task','fix auth token','--budget','2000']),
                ('pack_text',['codex-pack','fix auth token','--budget','2000']),
                ('pack_json',['codex-pack','fix auth token','--budget','2000','--format','json']),
                ('map_json',['map','--budget','2000','--format','json']),
            ]
            for case, command in workloads:
                variants=[('baseline',old,command),('candidate',new,command)]
                if case in ('task_text','pack_text'):
                    variants.append(('candidate_quiet',new,[*command,'--quiet']))
                if case=='pack_json':
                    variants.append(('candidate_compact',new,[*command,'--quiet','--compact-json']))
                samples={name:[] for name,_,_ in variants}; outputs={}
                for name,exe,cmd in variants:
                    outputs[name]=invoke(exe,cmd,root,env)[0]
                if args.require_identical_output:
                    check(f'unchanged output: {root_name}/{case}',outputs['baseline'].stdout==outputs['candidate'].stdout)
                for _ in range(args.repeats):
                    shuffled=variants[:];rng.shuffle(shuffled)
                    for name,exe,cmd in shuffled:
                        result,ms=invoke(exe,cmd,root,env)
                        samples[name].append(ms)
                        if result.stdout!=outputs[name].stdout:
                            raise RuntimeError(f'unstable output: {root_name}/{case}/{name}')
                for name,exe,cmd in variants:
                    raw=outputs[name].stdout
                    row={'repository':root_name,'case':case,'variant':name,'bytes':len(raw),'estimated_tokens':len(raw)//4,'median_ms':round(statistics.median(samples[name]),3),'min_ms':round(min(samples[name]),3),'max_ms':round(max(samples[name]),3),'samples_ms':[round(x,3) for x in samples[name]]}
                    if 'json' in case:
                        data=json.loads(raw)
                        row.update(files=len(data.get('files',[])),symbols=len(data.get('symbols',[])),tests=len(data.get('tests',[])),read_plan=len(data.get('read_plan',[])),reported_tokens=data.get('actual_estimated_tokens'))
                    measurements.append(row)
                print(f'BENCH {root_name}/{case} complete',flush=True)
        metadata={'baseline_path':str(baseline),'baseline_sha256':hashlib.sha256(baseline.read_bytes()).hexdigest(),'candidate_path':str(candidate),'candidate_sha256':hashlib.sha256(candidate.read_bytes()).hexdigest(),'repeats':args.repeats,'seed':20261107,'token_metric':'UTF-8 bytes // 4; rough proxy, not model token counts','runtime_metric':'warm-cache wall-clock median; shuffled paired subprocess trials','fixture_sizes':{'small':13,'medium':49,'large':209},'baseline_limit':args.baseline_description}
        result={'metadata':metadata,'checks':checks,'diagnostic_comparison':diagnostics,'measurements':measurements}
        (output/'results.json').write_text(json.dumps(result,indent=2)+'\n')
        fields=['repository','case','variant','bytes','estimated_tokens','median_ms','min_ms','max_ms']
        with (output/'measurements.csv').open('w',newline='') as f:
            writer=csv.DictWriter(f,fieldnames=fields,extrasaction='ignore');writer.writeheader();writer.writerows(measurements)
        print(f'Saved {output}; {sum(x["passed"] for x in checks)}/{len(checks)} behavioral checks passed',flush=True)
        return 0 if all(x['passed'] for x in checks) else 1

if __name__=='__main__':
    raise SystemExit(main())
