# TLA+ Model Checking

## Overview

The `doc/tla/` directory contains TLA+ formal specifications that model agent behavior. These specifications can be checked with the TLC model checker to verify safety and liveness properties of the agent ticket-processing protocol.

Currently one specification exists:

- **`AgentRun.tla`** — models the `agent run` lifecycle: fetch an assigned ticket, post acknowledgement, process it, and post a completion comment. Properties verify that acknowledged tickets cannot silently disappear (NoAckCrashGap) and that all tickets eventually reach a terminal state (EventuallyTerminal).

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

This requires `doc/tla/AgentRun.cfg` to exist alongside the `.tla` file. The `.cfg` file defines model constants and which properties to check.

Example `AgentRun.cfg` (not yet created — see known issues):

```
CONSTANTS
  Tickets = {1, 2, 3}
  Assigned = {1, 2}

INVARIANT TypeOK
INVARIANT NoAckCrashGap

PROPERTY EventuallyTerminal
```

### macOS

Define a shell alias for convenience (or add to your shell profile):

```bash
alias tlc='"/Applications/TLA+ Toolbox.app/Contents/Eclipse/plugins/org.lamport.openjdk.macosx.x86_64_14.0.1.7/Contents/Home/bin/java" -cp "/Applications/TLA+ Toolbox.app/Contents/Eclipse/tla2tools.jar" tlc2.TLC'
```

Then from the project root:

```bash
tlc doc/tla/AgentRun
```

Or without the alias:

```bash
"/Applications/TLA+ Toolbox.app/Contents/Eclipse/plugins/org.lamport.openjdk.macosx.x86_64_14.0.1.7/Contents/Home/bin/java" \
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
   - `Tickets` = `{1, 2, 3}` (model value set)
   - `Assigned` = `{1, 2}` (model value set)
5. Under "What to check?":
   - Invariants: `TypeOK`, `NoAckCrashGap`
   - Properties: `EventuallyTerminal`
6. Run the model checker.

## Known Issues

The TLA+ specification has several open issues that must be resolved before model checking succeeds cleanly. These are tracked in chainlink:

1. **#34 (critical)**: Line 94 references undefined variable `responded` — should be `acked`. TLC will refuse to parse the spec until this is fixed.

2. **#35 (high)**: No `.cfg` file exists. TLC requires `doc/tla/AgentRun.cfg` to define constants and properties. Blocked by #34.

3. **#36 (high, epic)**: The TLA+ spec diverges from the Go implementation in four ways:
   - `failed` state is terminal in TLA+ but non-terminal in Go (tickets retry after failure)
   - Acknowledgement is one-time per ticket in TLA+ but repeated per run in Go
   - `started` variable models a log line, not a real state transition
   - Spec models multi-ticket processing; Go agent processes exactly one ticket per invocation

4. **#41 (medium)**: No fairness assumptions in the `Spec` definition, so the `EventuallyTerminal` liveness property cannot be verified. Blocked by #36.

## PlusCal

The `AgentRun.tla` header (lines 5-14) recommends rewriting the spec in PlusCal, which is better suited for algorithmic control-flow protocols. This has not been done yet. The current raw TLA+ version serves as a baseline.
