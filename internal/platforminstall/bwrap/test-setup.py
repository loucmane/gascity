#!/usr/bin/python3
"""Offline parser/identity/Landlock-rule wiring tests; no namespace enforcement."""
import argparse
import json
from pathlib import Path
import subprocess

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name in ('source','build','sysroot','out'):
        parser.add_argument('--'+name,required=True,type=Path)
    args=parser.parse_args()
    here=Path(__file__).resolve().parent
    for path in (args.source,args.build,args.sysroot):
        if path.resolve()!=path or not path.is_dir():
            raise SystemExit('existing canonical build path required')
    if args.out.exists() or args.out.resolve()!=args.out or args.out.parent!=args.build.parent:
        raise SystemExit('fresh build-owned test directory required')
    args.out.mkdir(mode=0o700)
    binary=args.out/'test-setup'
    command=['/usr/bin/gcc','-D_GNU_SOURCE','-O2','-fPIE','-pie','-Wl,-z,relro,-z,now',
             '-I'+str(args.source),'-I'+str(args.build),'-I'+str(args.sysroot/'usr/include'),
             str(here/'test-setup.c'),str(args.source/'bind-mount.c'),str(args.source/'network.c'),
             str(args.source/'utils.c'),'-L'+str(args.sysroot/'usr/lib/x86_64-linux-gnu'),
             '-lcap','-lselinux','-o',str(binary)]
    subprocess.run(command,check=True,timeout=90)
    output=args.out/'output'; output.mkdir()
    alias=args.out/'alias'; alias.symlink_to(output)
    cases=[(['--ro-bind-null','--','/fixture-command'],0),(['--ro-bind-urandom','--','/fixture-command'],0),
           (['--ro-bind-urandom','/dev/random','--','/fixture-command'],1),
           (['--ro-bind-urandom','--ro-bind-urandom','--','/fixture-command'],1),
           (['--perms','0600','--ro-bind-urandom','--','/fixture-command'],1)]
    cases += [(['policy',kind,str(output)],0 if kind=='none' else 1) for kind in
              ('none','nnp','abi','create','add','restrict','conditional','unexpected')]
    cases += [(['policy','none',path],1) for path in ('/','/dev','/dev/urandom',str(alias))]
    cases += [(['identity'],0)]
    records=[]
    for argv,want in cases:
        result=subprocess.run([str(binary),*argv],env={'PATH':'/usr/bin:/bin'},capture_output=True,text=True,timeout=5)
        records.append({'argv':argv,'expected':want,'actual':result.returncode,'stderr':result.stderr})
    record={'pass':all(r['expected']==r['actual'] for r in records),'records':records,
            'namespace_executed':False,'syscalls_mocked':True,'kernel_enforcement_proved':False}
    with (args.out/'result.json').open('x') as stream:
        json.dump(record,stream,indent=2,sort_keys=True)
    if not record['pass']:
        raise SystemExit('offline setup regression failed; evidence preserved')
    print(json.dumps({'offline_cases':len(records),'pass':True,'kernel_enforcement_proved':False}))

if __name__=='__main__': main()
