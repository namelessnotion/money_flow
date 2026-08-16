------------------------------ MODULE eventlog ------------------------------
EXTENDS Naturals, Sequences

CONSTANT Ids, Commands, Events, Handlers

(* no command can have a possible empty set of events, that is all commands
   must result in atleast 1 event *)
ASSUME \A c \in Commands: Handlers[c] \subseteq Events /\ Handlers[c] # {}

VARIABLES log, commandStream, handledCommands

commandCollector(s, c) ==
  LET F[i \in 0..Len(s)] ==
        IF i = 0 THEN << >>
                 ELSE IF s[i] = c THEN Append(F[i-1], s[i])
                                  ELSE F[i-1]
  IN F[Len(s)]

vars == <<log, commandStream, handledCommands>>

TypeOK == /\ \/ log = << >>
             \/ \A i \in 1..Len(log): \E command \in Commands: log[i][2] \in Handlers[command]
             \/ \A i \in 1..Len(log): \E command \in Commands: log[i][1] \in Ids
          /\ \/ commandStream = << >>
             \/ \A i \in 1..Len(commandStream): commandStream[i][2] \in Commands
             \/ \A i \in 1..Len(commandStream): commandStream[i][1] \in Ids
          /\ \/ handledCommands = << >>
             \/ \A i \in 1..Len(handledCommands): handledCommands[i][2] \in Commands
             \/ \A i \in 1..Len(handledCommands): handledCommands[i][1] \in Ids

Init == /\ log = << >>
        /\ handledCommands = << >>
        /\ commandStream = << >>

PushCommand == /\ \E command \in Commands: \E id \in Ids: commandStream' = Append(commandStream, <<id, command>>)
               /\ UNCHANGED <<log, handledCommands>>

HandleCommand == /\ commandStream # << >>
                 /\ commandStream' = Tail(commandStream)
                 /\ handledCommands' = Append(handledCommands, Head(commandStream))
                 /\ IF /\ log # << >>
                       /\ (\E i \in 1..Len(log): /\ log[i][2] \in Handlers[Head(commandStream)[2]]
                                                 /\ log[i][1] = Head(commandStream)[1])
                        THEN log' = log
                        ELSE \E event \in Handlers[Head(commandStream)[2]]: log' = Append(log, <<Head(commandStream)[1], event>>)

Next == \/ PushCommand
        \/ HandleCommand

Spec == Init /\ [][Next]_vars

NoDuplicateEventsInLog == \A i, j \in 1..Len(log): (i # j) => log[i] # log[j]

THEOREM Spec => [](TypeOK /\ NoDuplicateEventsInLog)


=============================================================================
