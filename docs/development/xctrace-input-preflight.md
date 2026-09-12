# Fail-early declared profiling inputs

[Issue #867](https://github.com/jxsl13/perfscan/issues/867) reports an xctrace
child blocked opening a model under Desktop even though an already exposed
non-protected inode alias loaded successfully. This collector implements the
issue's **fail-early alternative only**. It never copies files, creates hard
links, stages inputs, extracts protected contents, changes TCC/permissions, or
assumes that its own access grants a profiler child access.

## Explicit inventory, actual observations

Every Capture/CLI request must provide InputPolicy (`-inputs` JSON in the CLI).
Omission, a null inputs array or an incomplete declaration fails closed. The
workload executable is resolved and included automatically; the native launch
uses its observed canonical path. The user must additionally enumerate every
direct and indirect model, tokenizer, configuration, shader, cache seed and
other required file for the selected operation and all provider/build paths.
`inventoryComplete` is an audited **user assertion**, not a source-derived proof
or a claim that arbitrary executable arguments reveal every dependency. An
explicit `inputs: []` is permitted only for a reviewed operation with no
additional required file inputs.

```json
{
  "inventoryComplete": true,
  "inputs": [
    {"path": "/private/tmp/already-approved/model.gguf", "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
  ],
  "additionalProtectedRoots": [],
  "maxInputBytes": 1073741824,
  "opened": "WORKLOAD_INPUTS_OPENED"
}
```

The digest above is illustrative, not valid evidence for any real model.
Expected SHA256 is optional; when supplied it must match an actual complete
parent read. Relative input/executable paths are resolved against `-dir` (or
the current working directory); bare executable names use the parent's observed
PATH resolution. Parent/tool PATH equivalence is not assumed: the resolved
canonical executable is passed explicitly to the recorder.

The complete declared inventory is classified before **any** input content is
read. Known protected lexical or canonical paths fail before content access.
The working directory is classified too. Passing paths must resolve to regular
files on supported local non-removable APFS mounts. Missing files, directories,
nonregular objects, dangling/cyclic symlinks, duplicate canonical paths,
unknown/non-local/removable mounts and unsupported platforms/filesystems fail.
Symlinks to already approved local inputs are resolved and both original and
canonical paths are checked; a symlink cannot bypass a known protected root.
Existing ancestor/root inode identities are compared to cover alternate volume
spellings that do not appear as symlinks. This does not discover every hard-link
alias's origin or establish permission for its contents.

For eligible inputs, preflight performs an actual bounded streaming SHA256
read with file identity, size and modification checks, under the command
deadline. The combined input byte budget includes the workload executable and
is capped at 8 GiB. It is a read bound, not a live quota or a guarantee against a
kernel-level I/O stall. `plan.json`, `input-preflight.json` and `result.json`
retain policy, observed paths, mount metadata, bytes, hashes and any rejection
before an xcrun command runs. Original inputs are never changed or removed.

## Conservative selected-platform scope

Native preflight currently supports **Darwin/local APFS only**. It observes
Darwin `statfs` filesystem/mount names and MNT_LOCAL/MNT_REMOVABLE flags; the
constants are taken from the selected macOS SDK `sys/mount.h`. A local temporary
APFS fixture was checked on the development host. No network/removable/cloud
permission experiment or protected-input content read was performed.

Default potential-protection roots bind to the actual UID's OS account home
(not the caller-controlled HOME environment); unknown account lookup fails
closed. They include that user's Desktop,
Documents and Downloads, conservative Library/Mobile Documents and
Library/CloudStorage locations, and /Volumes. Additional protected roots may
only extend that set. Common /Users/<account> spellings for these same locations
(including the APFS data-volume alias) are conservatively excluded for other
accounts too; this is not exhaustive alternate-home or TCC coverage. Explicitly
set `-dir` to an already approved non-protected directory: invoking this tool
from a Documents worktree without that setting intentionally fails preflight.
Apple documents privacy controls for Desktop, Documents,
Downloads, network and removable volumes in
[Apple Platform Security](https://support.apple.com/en-gb/guide/security/secddd1d86a6/web).
Cloud/provider roots and /Volumes are conservative exclusions, not claims that
these prefixes model every provider or distinguish all mount types. Actual
non-local/removable mount flags independently reject those observed categories.
Unknown policy/mount/platform cases are unsupported, not silently allowed.

**Passing does not mean profiler permission passed.** TCC app identity, ACLs,
privacy policy versions, per-provider controls, deferred file access, network
providers, inode aliases and later filesystem changes remain separate unknowns.
Preflight observations are snapshots, not authenticated or atomic guarantees
about later target reads. The caller must independently qualify the exact
profiler child and externally supervise its targets; extending a time limit is
not a privacy fix. A rejected path should be addressed through an explicitly
authorized workflow outside this collector, never by an automatic staging or
permission workaround.

The workload must emit exactly one start marker, then the configured `opened`
marker **only after all declared required input opens/checks succeed**, then
exactly one successful completion marker. The collector requires that order.
The post-open marker helps distinguish input stalls; it does not source-prove
inventory completeness, every deferred read, target process exit or correctness.
No marker is invented by the collector.

## Validation limits

Parallel tests use task-owned synthetic inputs and injected path/mount
classification for portable fake xcrun subprocess tests; actual input reads,
hashes and subprocess stream/status assertions remain real. A separate Darwin
test observes local temporary APFS metadata/availability only, with no profiler
launch. Existing #866 native recorder/XML smoke evidence is preserved and
explicitly predates this policy. No protected workload has been launched, no
TCC bypass has been attempted, and no new GPU or performance measurement is
claimed by #867.
