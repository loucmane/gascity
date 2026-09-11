# Final preparation disposition: HOLD

The frozen draft is NOT executable-ready. This append-forward finding supersedes
any interpretation of the earlier preparation report as a completed runnable
package. Nothing was executed; production source/image and fresh runroot remain
unchanged. The prior frozen controller/package/result bytes are preserved.

One precise blocker: controller.py launch appends the new Popen child to direct,
then calls enroll. If pidfd acquisition or initial identity enrollment fails
before handles receives that child, finally signals only handles. It merely
waits two seconds for an unenrolled direct child and may leave that owned process
alive. This does not meet the required guaranteed exact-owned failure teardown.
The fast successful finalizer can also become terminal before live enrollment;
that outcome must be distinguished from missing ownership evidence.

Smallest next change is confined to an append-forward controller revision:
install an immediate exact-Popen-owned cleanup obligation after successful
Popen, preserve a pidfd whenever available, and handle already-terminal owned
children explicitly. No production contract, custody guard, namespace mechanism
or new runtime authority is needed. Review the corrected controller before any
execution. Do not run the currently frozen command.

Frozen draft controller SHA256:
aa0c5dec7932ff394528c0ba7045073acc52659141e56c0161082176a1dee32a
Frozen package.json SHA256:
89f908b462ba7bf649fc07c509710f53498aa7b05afd1be11319d2d437448b0e
Historical preparation result SHA256:
80ad8229cfd77d44db012e4d274534b24b735106bbffea830a3c9a1863a77f1a
