# ga-tmgr R3 independent-review evidence supplement — no source change

The R3 SOURCE HOLD is retained. No R3 runtime execution has occurred. This is
the missing primary-source context and a request for independent adjudication,
not a parent override of the review or permission to execute.

## Frozen identities

- R3 source manifest: 606b0af539dd0bd39d8022d9dcf3c1ac2fb3215669b8f7271682ac9ef8f62c63.
- R3 plan: 80ca827ce5e91fefad2860da16e7e7ee4abb5af179ef176e289d583ae253e2f6.
- R3 binary: 72d4991d1c264af7829922164efe9bb6cd82eae4572398e15bb9dbba1d325b53.
- Opus R3 review: /tmp/ga-tmgr-combined-independent-review-r3-20260910/verdict.md
  cc5d3a7e13dc20482ba310c9044f07d33809a72e753b17a8298d35cc6542e227.
- Verified completion: 1b66e6aca695b532dc050580c176e9d4be401ea2eb8a8f66b1cf422ccdc21697.
- Review payload: 041e23103dfca2101e3bc04f88903210c76ef7c906a05333ef14c2d7760488b4.

## What was missing from the reviewer payload

The payload supplied runtime/cgroup_linux.go in full but cited the lower-level
internal/runtime/cgroup/cgroup_linux.go only by hash. The review correctly noted
that the supplied first file does not itself show descriptor open flags.

The omitted installed file is readable and hash-exact:
`/home/loucmane/.local/share/go/1.26.7/src/internal/runtime/cgroup/cgroup_linux.go`
SHA256 9f192b16192c9176b5c1bc100e786f5c726359d86782aa86a32221bebb07d031.

Its OpenCPU function opens every retained CPU descriptor as follows:

```go
// cgroup v1 quota, line69
quotaFD, errno := linux.Open(&path[0], linux.O_RDONLY|linux.O_CLOEXEC, 0)
// cgroup v1 period, line79
periodFD, errno := linux.Open(&path[0], linux.O_RDONLY|linux.O_CLOEXEC, 0)
// cgroup v2 cpu.max, line96
maxFD, errno := linux.Open(&path[0], linux.O_RDONLY|linux.O_CLOEXEC, 0)
```

CPU.Close at lines31-41 closes quotaFD/periodFD for V1 and quotaFD for V2.
The already supplied runtime/cgroup_linux.go lines45-71 call OpenCPU before
main, keep the handles when containermaxprocs is enabled, and call c.Close()
when disabled. The R3 pin is literal child-only GODEBUG=containermaxprocs=0
established at exec, with frozen-plan/argv validation and unchanged FD guard.

## Correct the explanatory shortcut, do not invent a PASS

Empty child Env and no ExtraFiles alone are NOT evidence that an arbitrary
non-CLOEXEC parent descriptor cannot survive exec. That explanatory shortcut
in the earlier reassessment should not be used as a general invariant.
The actual CPU-descriptor premise must instead rest on its O_CLOEXEC open flags
and unchanged exec behavior. General unexpected descriptors are still refused
by the existing V/W guard, before the writer starts; none may be allowlisted.

The open questions for independent Astra adjudication are:

1. Do these actual open flags resolve the review's M1(1) provenance concern for
   this frozen Go runtime and exact cpu.max failure?
2. Given that evidence, is a new runner-level refusal still required for this
   narrow synthetic correction? Identify the specific residual failed operation,
   distinguishing additional hardening from the already-guarded V/W boundary.

No source, review, runroot, namespace, descriptor, environment, rig or live
component was changed by this supplement. No R3 runtime or provider acceptance
is claimed. Keep both reviews and this supplement regardless of the outcome.
