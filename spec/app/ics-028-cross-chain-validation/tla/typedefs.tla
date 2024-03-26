--------------------------- MODULE typedefs ---------------------------
(* 
  @typeAlias: chain = C; chain type
  @typeAlias: node = N; node type
  @typeAlias: power = Int; voting power
  @typeAlias: time = Int; 
  @typeAlias: votingPowerOnChain = $node -> $power;
  @typeAlias: packet = $time;
  @typeAlias: matureVSCPacket = {chain: $chain, packet: $packet, maturityTime: $time};
*)
AliasesCVV == TRUE
=============================================================================