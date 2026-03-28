---- MODULE AgentRun ----
EXTENDS Naturals, TLC

(***************************************************************************)
(* Models the 'agent run' lifecycle for a single assigned ticket.          *)
(*                                                                         *)
(* One invocation processes at most one ticket. If the agent crashes or    *)
(* fails at any point, the ticket remains eligible for a future invocation.*)
(* Multiple invocations are modeled by allowing CrashAndRestart to reset   *)
(* per-run state so FetchTicket may fire again.                            *)
(*                                                                         *)
(* Go source of truth: src/cmd/agent/run.go (runAgentOnce)                *)
(* Design doc:         doc/spec/agent.md section 4                        *)
(*                                                                         *)
(* Recommendation: rewrite in PlusCal for clearer control-flow expression. *)
(***************************************************************************)

CONSTANT Ticket   \* The single assigned ticket; a natural number

Nil == 0

ASSUME Ticket \in Nat /\ Ticket # Nil

VARIABLES current, acked, done

vars == <<current, acked, done>>

TypeOK ==
  /\ current \in {Ticket, Nil}
  /\ acked \in BOOLEAN
  /\ done \in BOOLEAN

Init ==
  /\ current = Nil
  /\ acked = FALSE
  /\ done = FALSE

\* Pick up the assigned ticket for processing.
FetchTicket ==
  /\ current = Nil
  /\ ~done
  /\ current' = Ticket
  /\ UNCHANGED <<acked, done>>

\* Post an acknowledgement comment; happens once at the start of each run.
PostAckComment ==
  /\ current = Ticket
  /\ ~acked
  /\ acked' = TRUE
  /\ UNCHANGED <<current, done>>

\* Run completes successfully; posts a completion comment and exits.
CompleteRun ==
  /\ current = Ticket
  /\ acked
  /\ done' = TRUE
  /\ current' = Nil
  /\ UNCHANGED acked

\* Agent crashes or fails at any point — ticket stays eligible for retry.
\* acked is reset because the next invocation posts a fresh ack comment.
CrashAndRestart ==
  /\ current = Ticket
  /\ current' = Nil
  /\ acked' = FALSE
  /\ UNCHANGED done

Next ==
  \/ FetchTicket
  \/ PostAckComment
  \/ CompleteRun
  \/ CrashAndRestart

Spec == Init /\ [][Next]_vars

(***************************************************************************)
(* Properties                                                              *)
(***************************************************************************)

\* A run cannot complete without first posting an ack comment.
AckBeforeComplete == done => acked

\* Once acked in a run, the ticket is either still being processed or done.
\* (CrashAndRestart resets acked, keeping the ticket eligible via FetchTicket.)
NoAckCrashGap == acked => (current = Ticket \/ done)

\* Eventually the ticket is done. Requires fairness — see issue #41.
EventuallyDone == <>(done)

(***************************************************************************)
(* Notes                                                                   *)
(* - 'failed' and FailRun are removed: Go failure is non-terminal. The    *)
(*   ticket is not marked failed on the server; it stays eligible.        *)
(* - 'started' and PostStartUpdate are removed: the start marker in the   *)
(*   Go code is an informational progress update, not a server-side state.*)
(* - acked resets on CrashAndRestart because each invocation posts a new  *)
(*   ack comment unconditionally (doc/spec/agent.md section 8).           *)
(* - EventuallyDone requires WF on the non-crash actions but cannot be    *)
(*   verified without bounding crashes. Tracked in issue #41.             *)
(***************************************************************************)

====
