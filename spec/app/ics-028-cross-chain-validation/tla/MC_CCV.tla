--------------------------- MODULE MC_CCV ---------------------------

EXTENDS Integers

Nodes == {"1_OF_N", "2_OF_N", "3_OF_N", "4_OF_N"}
ConsumerChains == {"1_OF_C", "2_OF_C", "3_OF_C", "4_OF_C"}

CONSTANT 
  \* @type: $time;
  UnbondingPeriod,
  \* @type: $time;
  Timeout,
  \* @type: $time;
  MaxDrift,
  \* @type: $time;
  InactivityTimeout

CInit ==
  /\ UnbondingPeriod \in Nat
  /\ Timeout \in Nat
  /\ MaxDrift \in Nat
  /\ MaxDrift < Timeout
  /\ InactivityTimeout \in Nat
  \* Inactivity cutoff has to be no less than than 
  \* the largest reasonable round-trip time
  /\ InactivityTimeout >= 2 * (Timeout + MaxDrift)


\* Provider chain only
VARIABLES
  \* @type: $time -> $votingPowerOnChain;
  votingPowerHist,
  \* @type: $votingPowerOnChain;
  votingPowerRunning,
  \* @type: $chain -> STATUS;
  consumerStatus,
  \* @type: $packet -> Set($chain);
  expectedResponders,
  \* @type: Set($matureVSCPacket);
  maturePackets

\* Consumer chains or both
VARIABLES  
  \* @type: $chain -> $time;
  votingPowerReferences,
  \* @type: $chain -> Seq($packet);
  ccvChannelsPending,
  \* @type: $chain -> Seq($packet);
  ccvChannelsResolved,
  \* @type: $chain -> $time;
  currentTimes,
  \* @type: $chain -> $time -> $time;
  maturityTimes

\* Bookkeeping
VARIABLES 
  \* @type: Str;
  lastAction,
  \* @type: Bool;
  votingPowerHasChanged,
  \* @type: Bool;
  boundedDrift

INSTANCE CCV

=============================================================================