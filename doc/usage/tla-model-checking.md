# TLA+ Model Checking

## Overview

The `doc/tla/` directory contains TLA+ formal specifications that model agent behavior. These specifications can be checked with the TLC model checker to verify safety and liveness properties of the agent ticket-processing protocol.

Currently one specification exists:

- **`AgentRun.tla`** — models the `agent run` lifecycle: fetch an assigned ticket, post acknowledgement, process it, and post a completion comment. Properties verify that a run cannot complete without acking first (AckBeforeComplete), that an acked ticket is either still being processed or done (NoAckCrashGap), and that the ticket eventually completes (EventuallyDone).

The Go implementation (`src/cmd/agent/run.go`) and the spec document (`doc/spec/agent.md`) are the source of truth. The TLA+ spec is a verification aid, not the canonical definition.

## Prerequisites

TLC (the TLA+ model checker) requires Java and `tla2tools.jar`. The TLA+ Toolbox application bundles both.

Download the TLA+ Toolbox from the [TLA+ GitHub releases](https://github.com/tlaplus/tlaplus/releases) if not already installed.

### macOS

The TLA+ Toolbox at `/Applications/TLA+ Toolbox.app/` bundles everything needed — no separate Java installation is required:

- **`tla2tools.jar`**: `Contents/Eclipse/tla2tools.jar`
- **OpenJDK 14**: `Contents/Eclipse/plugins/org.lamport.openjdk.macosx.x86_64_14.0.1.7/Contents/Home/bin/java`

### Other platforms

Instructions for Linux and Windows will be added when needed. The general approach is the same: ensure `java` (11+) is on `PATH` and obtain `tla2tools.jar` from the TLA+ Toolbox installation or GitHub releases.

## Running TLC from the Command Line

This requires `doc/tla/AgentRun.cfg` to exist alongside the `.tla` file. The `.cfg` file defines the specification entry point, model constants, and which properties to check.

Current `AgentRun.cfg`:

```
SPECIFICATION Spec

\* done=TRUE is the intended terminal state; no further actions are enabled.
CHECK_DEADLOCK FALSE

CONSTANTS
  Ticket = 1

INVARIANT TypeOK
INVARIANT AckBeforeComplete
INVARIANT NoAckCrashGap

PROPERTY EventuallyDone
```

### macOS

Define a shell alias for convenience (or add to your shell profile):

```bash
alias tlc='"/Applications/TLA+ Toolbox.app/Contents/Eclipse/plugins/org.lamport.openjdk.macosx.x86_64_14.0.1.7/Contents/Home/bin/java" -XX:+UseParallelGC -cp "/Applications/TLA+ Toolbox.app/Contents/Eclipse/tla2tools.jar" tlc2.TLC'
```

Then from the project root (TLC auto-discovers the matching `.cfg`):

```bash
tlc doc/tla/AgentRun
```

Or without the alias:

```bash
"/Applications/TLA+ Toolbox.app/Contents/Eclipse/plugins/org.lamport.openjdk.macosx.x86_64_14.0.1.7/Contents/Home/bin/java" \
  -XX:+UseParallelGC \
  -cp "/Applications/TLA+ Toolbox.app/Contents/Eclipse/tla2tools.jar" \
  tlc2.TLC doc/tla/AgentRun
```

### Other platforms

```bash
java -cp /path/to/tla2tools.jar tlc2.TLC doc/tla/AgentRun
```

## Running from the TLA+ Toolbox GUI

These steps are platform-independent:

1. Open the TLA+ Toolbox application.
2. File > Open Spec > Add New Spec, select `doc/tla/AgentRun.tla`.
3. TLC Model Checker > New Model.
4. Under "What is the model?", set constants:
   - `Ticket` = `1` (ordinary assignment)
5. Under "What to check?":
   - Invariants: `TypeOK`, `AckBeforeComplete`, `NoAckCrashGap`
   - Properties: `EventuallyDone`
6. Run the model checker.

## Expected output

```
TLC2 Version 2.19 of 08 August 2024 (rev: 5a47802)
...
Model checking completed. No error has been found.
  ...
6 states generated, 4 distinct states found, 0 states left on queue.
The depth of the complete state graph search is 4.
Finished in 01s at (...)
```

All properties check cleanly: invariants (`TypeOK`, `AckBeforeComplete`, `NoAckCrashGap`) and the liveness property (`EventuallyDone`) are satisfied under the fairness assumptions in `Spec`.

## PlusCal

The `AgentRun.tla` header (lines 5-14) recommends rewriting the spec in PlusCal, which is better suited for algorithmic control-flow protocols. This has not been done yet. The current raw TLA+ version serves as a baseline.
