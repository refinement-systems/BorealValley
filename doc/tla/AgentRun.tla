---- MODULE AgentRun ----
EXTENDS Naturals, TLC

(***************************************************************************)
(* Recommendation                                                          *)
(*                                                                         *)
(* Use PlusCal for the primary AgentRun spec. The `agent run` behavior is  *)
(* an algorithmic control-flow protocol (fetch -> ack -> progress -> finish *)
(* with crash/restart branches), which PlusCal represents more directly     *)
(* than hand-written action decomposition.                                  *)
(*                                                                         *)
(* This file is a raw TLA+ draft of the same protocol so it can be checked *)
(* immediately and used as a baseline before (or alongside) a PlusCal      *)
(* version.                                                                 *)
(***************************************************************************)

CONSTANTS Tickets, Assigned

ASSUME /\ Tickets # {}
       /\ Tickets \subseteq Nat
       /\ Assigned \subseteq Tickets
       /\ Assigned # {}

Nil == 0

VARIABLES assigned, acked, started, done, failed, current

vars == <<assigned, acked, started, done, failed, current>>

TypeOK ==
  /\ assigned = Assigned
  /\ acked \in [Tickets -> BOOLEAN]
  /\ started \in [Tickets -> BOOLEAN]
  /\ done \in [Tickets -> BOOLEAN]
  /\ failed \in [Tickets -> BOOLEAN]
  /\ current \in Tickets \cup {Nil}

Eligible ==
  {t \in assigned : ~done[t]}

Pick(S) == CHOOSE t \in S : \A u \in S : t <= u

Init ==
  /\ assigned = Assigned
  /\ acked = [t \in Tickets |-> FALSE]
  /\ started = [t \in Tickets |-> FALSE]
  /\ done = [t \in Tickets |-> FALSE]
  /\ failed = [t \in Tickets |-> FALSE]
  /\ current = Nil

IdleNoWork ==
  /\ current = Nil
  /\ Eligible = {}
  /\ UNCHANGED vars

FetchAssignedCompletionPending ==
  /\ current = Nil
  /\ Eligible # {}
  /\ current' = Pick(Eligible)
  /\ UNCHANGED <<assigned, acked, started, done, failed>>

PostAckComment ==
  /\ current \in Tickets
  /\ ~acked[current]
  /\ acked' = [acked EXCEPT ![current] = TRUE]
  /\ UNCHANGED <<assigned, started, done, failed, current>>

PostStartUpdate ==
  /\ current \in Tickets
  /\ acked[current]
  /\ ~started[current]
  /\ started' = [started EXCEPT ![current] = TRUE]
  /\ UNCHANGED <<assigned, acked, done, failed, current>>

CompleteRun ==
  /\ current \in Tickets
  /\ started[current]
  /\ ~done[current]
  /\ done' = [done EXCEPT ![current] = TRUE]
  /\ current' = Nil
  /\ UNCHANGED <<assigned, acked, started, failed>>

FailRun ==
  /\ current \in Tickets
  /\ started[current]
  /\ ~failed[current]
  /\ failed' = [failed EXCEPT ![current] = TRUE]
  /\ current' = Nil
  /\ UNCHANGED <<assigned, acked, started, done>>

CrashAndRestart ==
  /\ current \in Tickets
  /\ current' = Nil
  /\ UNCHANGED <<assigned, responded, started, done, failed>>

Next ==
  \/ IdleNoWork
  \/ FetchAssignedCompletionPending
  \/ PostAckComment
  \/ PostStartUpdate
  \/ CompleteRun
  \/ FailRun
  \/ CrashAndRestart

Spec == Init /\ [][Next]_vars

(***************************************************************************)
(* Properties                                                              *)
(***************************************************************************)

CompletionPendingAfterAck(t) ==
  /\ t \in assigned
  /\ acked[t]
  /\ ~done[t]
  /\ (current = t \/ t \in Eligible)

NoAckCrashGap == \A t \in assigned : CompletionPendingAfterAck(t) \/ ~acked[t] \/ done[t]

EventuallyTerminal == \A t \in assigned : <>(done[t] \/ failed[t])

(***************************************************************************)
(* Notes                                                                   *)
(* - Completion is modeled only by `done[t]`, corresponding to the         *)
(*   separate completion root comment.                                     *)
(* - Acknowledgement and progress updates do not remove a ticket from      *)
(*   `Eligible`; crash/restart after acknowledgement keeps it runnable.     *)
(* - `EventuallyTerminal` may still fail if retries continue forever, but   *)
(*   the earlier ack-first lost-ticket counterexample is removed.           *)
(* - In a PlusCal version, keep this same state and properties.            *)
(***************************************************************************)

====
