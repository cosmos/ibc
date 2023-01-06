---
ics: 15
title: Ethereum beacon chain client
stage: draft
category: IBC/TAO
kind: instantiation
author: Seun Lanlege <seun@polytope.technology>
created: 2023-01-05
implements: 2
---

## Synopsis

This technical specification assumes that you’re already aware of [the sync committee protocol](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/beacon-chain.md#introduction) introduced in altair, the first hard fork of the ethereum beacon chain. If not, tl;dr: The original attestation protocol unfortunately did not include succint BLS public key aggregation, which would’ve made it cheap to verify by light clients given that [there are now almost 500k authorities](https://beaconscan.com/) actively validating blocks on the beacon chain.

This hypothetical succint BLS public key aggreation scheme, would’ve allowed us to verify the Casper FFG attestation protocol directly by aggregating all 500k validators’ public keys into a smaller set of BLS public keys. But unfortunately, it doesn’t exist.

Some napkin math reveals:

```markdown
 48 (BLS public key size in bytes) x 500000 (validators) = 24000000 (bytes or 24MB)
```

Light clients would need to load two-thirds of the validator keys (or 16MB) into memory, in order to follow the original Casper FFG protocol and verify the BLS signatures (Signatures, plural, because it would degrade the network to have validators naively pass around a single slot/header to be signed over. So instead the validators are grouped in to sub-sets which are called *attestation* *committees*) of the attested slot. Technically not a huge ask for off-chain light clients, but absolutely impossible for an on-chain light client.

The altair hard fork, introduces the sync committee, a random subset of 512 validators, chosen from the active validator set. Where the super majority of this subset is responsible for producing a BLS signature over beacon headers which are linked to the latest header that is finalized  by the original set. They are essentially attesting to the original validator set’s attestation.

## Motivation

This client will allow IBC enabled chains to interface with the ethereum chain directly & trustlessly using the recently added sync committee protocol.

## Definitions

Functions & terms are as defined in [ICS 2](../../core/ics-002-client-semantics).

## Desired Properties

This specification must satisfy the client interface defined in ICS 2.

## Technical Specification

The following is a technical specification for the mechanism of verifying the attestation of the sync committee: 

## Constants

```rust
/// The block root and state root for every slot are stored in the state for `SLOTS_PER_HISTORICAL_ROOT` slots.
/// When that list is full, both lists are Merkleized into a single Merkle root,
/// which is added to the ever-growing state.historical_roots list.
/// [source](https://eth2book.info/bellatrix/part3/config/preset/#slots_per_historical_root)
const SLOTS_PER_HISTORICAL_ROOT: u64 = 2.pow(13); // 8,192

/// Every `SLOTS_PER_HISTORICAL_ROOT` slots, the list of block roots and the list of state roots in the beacon state
/// are Merkleized and added to state.historical_roots list. Although state.historical_roots is in principle unbounded,
/// all SSZ lists must have maximum sizes specified.
/// 
/// The size `HISTORICAL_ROOTS_LIMIT` will be fine for the next few millennia, after which it will be somebody else's problem.
/// The list grows at less than 10 KB per year. Storing past roots like this allows Merkle proofs to be constructed
/// about anything in the beacon chain's history if required.
/// [source](https://eth2book.info/bellatrix/part3/config/preset/#historical_roots_limit)
const HISTORICAL_ROOTS_LIMIT: u64 = 2.pow(24); // 16,777,216

/// Generalized merkle tree index for the latest finalized header
/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/light-client/sync-protocol.md#constants)
const FINALIZED_ROOT_INDEX: u64 = 105;

/// Generalized merkle tree index for the block_roots in [`BeaconState`]
const BLOCK_ROOTS_INDEX: u64 = 37;

/// Generalized merkle tree index for the historical_roots in [`BeaconState`]
const HISTORICAL_ROOTS_INDEX: u64 = 39;

/// Generalized merkle tree index for the execution_payload in [`BeaconBlockBody`]
const EXECUTION_PAYLOAD_INDEX: u64 = 25; 

/// Generalized merkle tree index for the state_root in [`ExecutionPayload`]
const EXECUTION_PAYLOAD_STATE_ROOT_INDEX: u64 = 18; 

/// Generalized merkle tree index for the state_root in [`ExecutionPayload`]
const EXECUTION_PAYLOAD_BLOCK_NUMBER_INDEX: u64 = 22; 

/// Generalized merkle tree index for the next sync committee
/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/light-client/sync-protocol.md#constants)
const NEXT_SYNC_COMMITTEE_INDEX: u64 = 55;

/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/beacon-chain.md#domain-types)
const DOMAIN_SYNC_COMMITTEE: [u8; 4] = [7, 0, 0, 0];

/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/beacon-chain.md#sync-committee)
const EPOCHS_PER_SYNC_COMMITTEE_PERIOD: u64 = 2.pow(8);

/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/phase0/beacon-chain.md#time-parameters)
const SLOTS_PER_EPOCH: u64 = 2.pow(5);

/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/fork.md#configuration)
const ALTAIR_FORK_EPOCH: u64 = 74240;

/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/fork.md#configuration)
const ALTAIR_FORK_VERSION: [u8; 4] = [1, 0, 0, 0];

/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/phase0/beacon-chain.md#genesis-settings)
const GENESIS_FORK_VERSION: [u8; 4] = [0, 0, 0, 0];

// from eth/v1/beacon/genesis
const GENESIS_VALIDATORS_ROOT: H256 = H256(hex!("4b363db94e286120d76eb905340fdd4e54bfe9f06bf33ff6cf5ad27f511bfe95"));

// from eth/v1/beacon/genesis
const GENESIS_TIMESTAMP: u64 = 1606824023;

// Add the IBC contract address here. Required for state proofs.
const IBC_CONTRACT_ADDRESS: H160 = H160([]);

/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/bellatrix/beacon-chain.md#execution)
const BYTES_PER_LOGS_BLOOM: u64 = 2.pow(8);

/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/bellatrix/beacon-chain.md#execution)
const MAX_EXTRA_DATA_BYTES: u64: 2.pow(5);

/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/bellatrix/beacon-chain.md#execution)
const MAX_TRANSACTIONS_PER_PAYLOAD: u64 = 2.pow(20);
```

## Data Types

```rust
/// The beacon block header
/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/phase0/beacon-chain.md#beaconblockheader)
struct BeaconBlockHeader {
  /// current slot for this block
  slot: u64,
  /// validator index
  proposer_index: ValidatorIndex,
  /// ssz root of parent block
  parent_root: H256,
  /// ssz root of associated [`BeaconState`]
  state_root: H256,
  /// ssz root of associated [`BeaconBlockBody`]
  body_root: H256,
}

/// The beacon block body
/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/beacon-chain.md#beaconblockbody)
struct BeaconBlockBody {
  randao_reveal: BLSSignature,
  eth1_data: Eth1Data, // Eth1 data vote
  graffiti: Bytes32,   // Arbitrary data
  // Operations
  proposer_slashings: [ProposerSlashing; MAX_PROPOSER_SLASHINGS],
  attester_slashings: [AttesterSlashing; MAX_ATTESTER_SLASHINGS],
  attestations: [Attestation; MAX_ATTESTATIONS],
  deposits: [Deposit; MAX_DEPOSITS],
  voluntary_exits: [SignedVoluntaryExit; MAX_VOLUNTARY_EXITS],
  sync_aggregate: SyncAggregate,
  execution_payload: ExecutionPayload,
}

/// The beacon state
/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/beacon-chain.md#synccommittee)
struct BeaconState {
  genesis_time: u64,
  genesis_validators_root: H256,
  slot: Slot,
  fork: Fork,
  latest_block_header: BeaconBlockHeader,
  block_roots: [H256; SLOTS_PER_HISTORICAL_ROOT],
  state_roots: [H256; SLOTS_PER_HISTORICAL_ROOT],
  // historical roots contains a list of the root hash of the block_roots & state_roots of every
  // 256 epochs [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/phase0/beacon-chain.md#historical-roots-updates)
  historical_roots: [H256; HISTORICAL_ROOTS_LIMIT],
  eth1_data: Eth1Data,
  eth1_data_votes: [Eth1Data; EPOCHS_PER_ETH1_VOTING_PERIOD * SLOTS_PER_EPOCH],
  eth1_deposit_index: u64,
  validators: [Validator; VALIDATOR_REGISTRY_LIMIT],
  balances: [Gwei; VALIDATOR_REGISTRY_LIMIT],
  randao_mixes: [H256; EPOCHS_PER_HISTORICAL_VECTOR],
  slashings: [Gwei; EPOCHS_PER_SLASHINGS_VECTOR],
  previous_epoch_participation: [ParticipationFlags; VALIDATOR_REGISTRY_LIMIT],
  current_epoch_participation: [ParticipationFlags; VALIDATOR_REGISTRY_LIMIT],
  justification_bits: Bitvector<JUSTIFICATION_BITS_LENGTH>,
  previous_justified_checkpoint: Checkpoint,
  current_justified_checkpoint: Checkpoint,
  finalized_checkpoint: Checkpoint,
  inactivity_scores: [u64; VALIDATOR_REGISTRY_LIMIT],
  current_sync_committee: SyncCommittee,
  next_sync_committee: SyncCommittee,
}

/// Associated execution block for the beacon chain header
/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/bellatrix/beacon-chain.md#executionpayload)
struct ExecutionPayload {
  // Execution block header fields
  parent_hash: H256,
  // 'beneficiary' in the yellow paper
  fee_recipient: ExecutionAddress,
  state_root: H256,
  receipts_root: H256,
  logs_bloom: [u8; BYTES_PER_LOGS_BLOOM],
  // 'difficulty' in the yellow paper
  prev_randao: H256,
  // 'number' in the yellow paper
  block_number: u64,
  gas_limit: u64,
  gas_used: u64,
  // todo: is this same value we'd arrive at by deriving the timestamp from the beacon chain
  // header slot number?
  timestamp: u64,
  extra_data: [u8; MAX_EXTRA_DATA_BYTES],
  base_fee_per_gas: U256,
  // Extra payload fields
  // Hash of execution block
  block_hash: H256,
  transactions: [Transaction; MAX_TRANSACTIONS_PER_PAYLOAD],
}

/// The sync aggregate;
/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/beacon-chain.md#syncaggregate)
struct SyncAggregate {
  sync_committee_bits: [SYNC_COMMITTEE_SIZE; u8],
  sync_committee_signature: BLSSignature,
}

/// The sync committee responsible for signing blocks
/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/beacon-chain.md#synccommittee)
struct SyncCommittee {
  pubkeys: [BLSPubkey; SYNC_COMMITTEE_SIZE],
  aggregate_pubkey: BLSPubkey,
}

/// The historical roots accumulator object.
/// [source](https://github.com/ethereum/consensus-specs/blob/dev/specs/phase0/beacon-chain.md#historicalbatch)
/// The `historical_roots` field on [`BeaconState`] is updated using this struct as defined by
/// https://github.com/ethereum/consensus-specs/blob/dev/specs/phase0/beacon-chain.md#historical-roots-updates
struct HistoricalBatch {
  block_roots: [H256; SLOTS_PER_HISTORICAL_ROOT],
  state_roots: [H256; SLOTS_PER_HISTORICAL_ROOT],
}

/// This holds the relevant data required to prove the state root in the execution payload.
struct ExecutionPayloadProof {
  /// The state root in the `ExecutionPayload` which represents the commitment to
  /// the ethereum world state in the yellow paper.
  state_root: H256,
  /// the block number of the execution header.
  block_number: u64,
  /// merkle mutli proof for the state_root & block_number in the [`ExecutionPayload`].
  multi_proof: Vec<H256>,
  /// merkle proof for the `ExecutionPayload` in the [`BeaconBlockBody`].
  execution_payload_branch: Vec<H256>,
}

/// Holds the neccessary proofs required to verify a header in the `block_roots` field
/// either in [`BeaconState`] or [`HistoricalBatch`].
struct BlockRootsProof {
  /// Generalized index of the header in the `block_roots` list.
  block_header_index: u64,
  /// The proof for the header, needed to reconstruct `hash_tree_root(state.block_roots)`
  block_header_branch: Vec<H256>,
}

/// The block header ancestry proof, this is an enum because the header may either exist in
/// `state.block_roots` or `state.historical_roots`.
enum AncestryProof {
  /// This variant defines the proof data for a beacon chain header in the `state.block_roots`
  BlockRoots {
    /// Proof for the header in `state.block_roots`
    block_roots_proof: BlockRootsProof,
    /// The proof for the reconstructed `hash_tree_root(state.block_roots)` in [`BeaconState`]
    block_roots_branch: Vec<H256>,
  },
  /// This variant defines the neccessary proofs for a beacon chain header in the
  /// `state.historical_roots`.
  HistoricalRoots {
    /// Proof for the header in `historical_batch.block_roots`
    block_roots_proof: BlockRootsProof,
    /// The proof for the `historical_batch.block_roots`, needed to reconstruct
    /// `hash_tree_root(historical_batch)`
    historical_batch_proof: Vec<H256>,
    /// The proof for the `hash_tree_root(historical_batch)` in `state.historical_roots`
    historical_roots_proof: Vec<H256>,
    /// The generalized index for the historical_batch in `state.historical_roots`.
    historical_roots_index: u64,
    /// The proof for the reconstructed `hash_tree_root(state.historical_roots)` in
    /// [`BeaconState`]
    historical_roots_branch: Vec<H256>,
  },
}

/// This defines the neccesary data needed to prove ancestor blocks, relative to the finalized
/// header.
struct AncestorBlock {
  /// The actual beacon chain header
  header: BeaconBlockHeader,
  /// Associated execution header proofs
  execution_payload: ExecutionPayloadProof,
  /// Ancestry proofs of the beacon chain header.
  ancestry_proof: AncenstryProof,
}

/// Holds the latest sync committee as well as an ssz proof for it's existence
/// in a finalized header.
struct SyncCommitteeUpdate {
  // actual sync committee
  next_sync_committee: SyncCommittee,
  // sync committee, ssz merkle proof.
  next_sync_committee_branch: [H256; NEXT_SYNC_COMMITTEE_INDEX.floor_log2()],
}

/// Minimum state required by the light client to validate new sync committee attestations
struct LightClientState {
  /// The latest recorded finalized header
  finalized_header: BeaconBlockHeader,
  // Sync committees corresponding to the finalized header
  current_sync_committee: SyncCommittee,
  next_sync_committee: SyncCommittee,
}

/// Data required to advance the state of the light client.
struct LightClientUpdate {
  /// the header that the sync committee signed
  attested_header: BeaconBlockHeader,
  /// the sync committee has potentially changed, here's an ssz proof for that.
  sync_committee_update: Option<SyncCommitteeUpdate>,
  /// the actual header which was finalized by the ethereum attestation protocol.
  finalized_header: BeaconBlockHeader,
  /// execution payload of the finalized header
  execution_payload: ExecutionPayloadProof,
  /// the ssz merkle proof for this header in the attested header, finalized headers lag by 2 epochs.
  finality_branch: [H256; FINALIZED_ROOT_INDEX.floor_log2()],
  /// signature & participation bits
  sync_aggregate: SyncAggregate,
  /// slot at which signature was produced
  signature_slot: Slot,
  /// ancestors of the finalized block to be verified, may be empty.
  ancestor_blocks: Vec<AncestorBlock>,
}
```

## Utility Methods

Do note that a lot of these already exist in the [ssz-rs](https://github.com/ralexstokes/ssz-rs) library written by [Alex Stokes](https://twitter.com/ralexstokes).

```rust
fn get_subtree_index(generalized_index: u64) -> u64 {
  generalized_index % 2 ^ (generalized_index.floor_log2())
}

fn compute_sync_committee_period(epoch: u64) -> u64 {
  epoch / EPOCHS_PER_SYNC_COMMITTEE_PERIOD
}

/// Return the epoch number at ``slot``.
fn compute_epoch_at_slot(slot: u64) -> u64 {
  slot / SLOTS_PER_EPOCH
}

/// Return the fork version at the given ``epoch``.
fn compute_fork_version(epoch: u64) -> [u8; 4] {
  if epoch >= ALTAIR_FORK_EPOCH {
    ALTAIR_FORK_VERSION
  } else {
    GENESIS_FORK_VERSION
  }
}

fn compute_sync_committee_period_at_slot(slot: u64) -> u64 {
  compute_sync_committee_period(compute_epoch_at_slot(slot))
}

/// method for hashing objects into a single root by utilizing a hash tree structure, as defined in
/// the SSZ spec.
fn hash_tree_root<T: ssz_rs::SimpleSerializeTrait>(mut object: T) -> Result<H256, Error> {
  let root = object.hash_tree_root()?.try_into()?;
  Ok(root)
}

/// Return the domain for the ``domain_type`` and ``fork_version``.
fn compute_domain(
  domain_type: DomainType,
  fork_version: [u8; 4],
  genesis_validators_root: H256,
) -> H256 {
  let fork_data_root = compute_fork_data_root(fork_version, genesis_validators_root);
  let mut domain = H256::default();
  domain[0..4].copy_from_slice(&(domain_type));
  domain[4..32].copy_from_slice(&(fork_data_root.0[..28]));
  domain
}

/// Return the 32-byte fork data root for the ``current_version`` and ``genesis_validators_root``.
/// This is used primarily in signature domains to avoid collisions across forks/chains.
fn compute_fork_data_root(current_version: [u8; 4], genesis_validators_root: H256) -> H256 {
  hash_tree_root(ForkData { current_version, genesis_validators_root })
}

/// Return the signing root for the corresponding signing data.
fn compute_signing_root<T: ssz_rs::SimpleSerializeTrait>(
  object: T,
  domain: Domain,
) -> Result<H256, Error> {
  let object_root = hash_tree_root(object)?;

  let hash_root = hash_tree_root(SigningData { object_root, domain })?;

  Ok(hash_root)
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
struct GeneralizedIndex(usize);

impl Default for GeneralizedIndex {
  fn default() -> Self {
    Self(1)
  }
}

impl GeneralizedIndex {
  fn get_path_length(&self) -> usize {
    log_2(self.0) as usize
  }

  fn get_bit(&self, position: usize) -> bool {
    self.0 & (1 << position) > 0
  }

  fn sibling(&self) -> Self {
    Self(self.0 ^ 1)
  }

  fn child_left(&self) -> Self {
    Self(self.0 * 2)
  }

  fn child_right(&self) -> Self {
    Self(self.0 * 2 + 1)
  }

  fn parent(&self) -> Self {
    Self(self.0 / 2)
  }
}

fn get_branch_indices(tree_index: &GeneralizedIndex) -> Vec<GeneralizedIndex> {
  let mut focus = tree_index.sibling();
  let mut result = vec![focus.clone()];
  while focus.0 > 1 {
    focus = focus.parent().sibling();
    result.push(focus.clone());
  }
  result.truncate(result.len() - 1);
  result
}

fn get_path_indices(tree_index: &GeneralizedIndex) -> Vec<GeneralizedIndex> {
  let mut focus = *tree_index;
  let mut result = vec![focus.clone()];
  while focus.0 > 1 {
    focus = focus.parent();
    result.push(focus.clone());
  }
  result.truncate(result.len() - 1);
  result
}

fn get_helper_indices(indices: &[GeneralizedIndex]) -> Vec<GeneralizedIndex> {
  let mut all_helper_indices = HashSet::new();
  let mut all_path_indices = HashSet::new();

  for index in indices {
    all_helper_indices.extend(get_branch_indices(index).iter());
    all_path_indices.extend(get_path_indices(index).iter());
  }

  let mut all_branch_indices =
    all_helper_indices.difference(&all_path_indices).cloned().collect::<Vec<_>>();
  all_branch_indices.sort_by(|a: &GeneralizedIndex, b: &GeneralizedIndex| b.cmp(a));
  all_branch_indices
}

fn calculate_merkle_root(leaf: &H256, proof: &[H256], index: u64) -> H256 {
  let mut result = *leaf;

  let mut hasher = Sha256::new();
  for (i, next) in proof.iter().enumerate() {
    if index.get_bit(i) {
      hasher.update(&next.0);
      hasher.update(&result.0);
    } else {
      hasher.update(&result.0);
      hasher.update(&next.0);
    }
    result.0.copy_from_slice(&hasher.finalize_reset());
  }
  result
}

fn calculate_multi_merkle_root(leaves: &[H256], proof: &[H256], indices: &[u64]) -> Node {
  let indices = indices.iter().cloned().map(GeneralizedIndex).collect();
  let helper_indices = get_helper_indices(&indices);

  let mut objects = HashMap::new();
  for (index, node) in indices.iter().zip(leaves.iter()) {
    objects.insert(*index, *node);
  }
  for (index, node) in helper_indices.iter().zip(proof.iter()) {
    objects.insert(*index, *node);
  }

  let mut keys = objects.keys().cloned().collect::<Vec<_>>();
  keys.sort_by(|a, b| b.cmp(a));

  let mut hasher = Sha256::new();
  let mut pos = 0;
  while pos < keys.len() {
    let key = keys.get(pos).unwrap();
    let key_present = objects.contains_key(key);
    let sibling_present = objects.contains_key(&key.sibling());
    let parent_index = key.parent();
    let parent_missing = !objects.contains_key(&parent_index);
    let should_compute = key_present && sibling_present && parent_missing;
    if should_compute {
      let right_index = GeneralizedIndex(key.0 | 1);
      let left_index = right_index.sibling();
      let left_input = objects.get(&left_index).unwrap();
      let right_input = objects.get(&right_index).unwrap();
      hasher.update(&left_input.0);
      hasher.update(&right_input.0);

      let parent = objects.entry(parent_index).or_insert_with(|| Node::default());
      parent.0.copy_from_slice(&hasher.finalize_reset());
      keys.push(parent_index);
    }
    pos += 1;
  }

  objects.get(&GeneralizedIndex(1)).unwrap().clone()
}

/// Check if ``leaf`` at ``index`` verifies against the Merkle ``root`` and ``branch``.
fn is_valid_merkle_branch(
  leaf: H256,
  branch: Vec<H256>,
  depth: u64,
  index: u64,
  root: H256,
) -> Result<(), Error> {
  if branch.len() != depth as usize {
    Err(Error::InvalidProof)?
  }

  let mut value = leaf;
  if leaf.as_bytes().len() < 32 as usize {
    Err(Error::InvalidProof)?
  }

  for i in 0..depth {
    if branch[i as usize].as_bytes().len() < 32 as usize {
      Err(Error::InvalidProof)?
    }

    if (index / (2u32.pow(i as u32) as u64) % 2) == 0 {
      // left node
      let mut data = [0u8; 64];
      data[0..32].copy_from_slice(&(value.0));
      data[32..64].copy_from_slice(&(branch[i as usize].0));
      value = keccak(&data).into();
    } else {
      let mut data = [0u8; 64]; // right node
      data[0..32].copy_from_slice(&(branch[i as usize].0));
      data[32..64].copy_from_slice(&(value.0));
      value = keccak(&data).into();
    }
  }

  if value != root {
    Err(Error::InvalidProof)?
  }

  Ok(())
}
```

## Verification Algorithm

This algorithm is based on [the official altair sync protocol specification](https://github.com/ethereum/consensus-specs/blob/dev/specs/altair/light-client/sync-protocol.md#light-client-state-updates):

- First we verify that the sync committee has super majority participation in this attestation.
- We verify that the forward causality of attestation is satisfied.
- Next we verify the BLS signature of the sync committee.
- Next we verify the SSZ merkle proof for the header & its execution payload finalized by the Casper FFG attestation protocol.
- We optionally verify the SSZ proof for the next sync committee finalized by this new header.
- We optionally verify the SSZ proofs for ancestor blocks.

```rust
/// This function simply verifies a sync committee's attestation & it's finalized counterpart.
fn verify_sync_committee_attestation(
  state: LightClientState,
  update: LightClientUpdate,
) -> Result<(), Error> {
  // Verify sync committee has super majority participants
  let sync_committee_bits = update.sync_aggregate.sync_committee_bits;
  let sync_aggregate_participants = sync_committee_bits.iter().sum();
  if sync_aggregate_participants * 3 >= sync_committee_bits.clone().len() as u64 * 2 {
    Err(Error::SyncCommitteeParticiapntsTooLow)?
  }

  // Verify update does not skip a sync committee period
  let is_valid_update = update.signature_slot > update.attested_header.slot &&
    update.attested_header.slot >= update.finalized_header.slot;
  if !is_valid_update {
    Err(Error::InvalidUpdate)?
  }

  let state_period = compute_sync_committee_period_at_slot(state.finalized_header.slot);
  let update_signature_period = compute_sync_committee_period_at_slot(update.signature_slot);
  if !(state_period..=state_period + 1).contains(update_signature_period) {
    Err(Error::InvalidUpdate)?
  }

  // Verify update is relevant
  let update_attested_period = compute_sync_committee_period_at_slot(update.attested_header.slot);
  let update_has_next_sync_committee =
    update.sync_committee_update.is_some() && update_attested_period == state_period;

  if !(update.attested_header.slot > state.finalized_header.slot || update_has_next_sync_committee)
  {
    Err(Error::InvalidUpdate)?
  }

  // Verify sync committee aggregate signature
  let sync_committee = if update_signature_period == state_period {
    state.current_sync_committee
  } else {
    state.next_sync_committee
  };

  let participant_pubkeys = sync_committee_bits
    .iter()
    .zip(sync_committee_pubkeys.iter())
    .filter_map(|(bit, key)| if bit == 1 { Some(key) } else { None })
    .collect::<Vec<_>>();

  let fork_version = compute_fork_version(compute_epoch_at_slot(update.signature_slot));
  let domain = compute_domain(DOMAIN_SYNC_COMMITTEE, fork_version, GENESIS_VALIDATORS_ROOT);
  let signing_root = compute_signing_root(update.attested_header, domain);

  bls::fast_aggregate_verify(
    participant_pubkeys,
    signing_root,
    sync_aggregate.sync_committee_signature,
  )?;

  // Verify that the `finality_branch` confirms `finalized_header`
  // to match the finalized checkpoint root saved in the state of `attested_header`.
  // Note that the genesis finalized checkpoint root is represented as a zero hash.
  let finalized_root = hash_tree_root(update.finalized_header);
  is_valid_merkle_branch(
    finalized_root,
    update.finality_branch,
    FINALIZED_ROOT_INDEX.floor_log2(),
    get_subtree_index(FINALIZED_ROOT_INDEX),
    update.attested_header.state_root,
  )?;

  // verify the associated execution header of the finalized beacon header.
  let execution_payload = update.execution_payload;
  let execution_payload_root = calculate_multi_merkle_root(
    &[execution_payload.state_root, hash_tree_root(execution_payload.block_number)],
    execution_payload.multi_proof,
    &[EXECUTION_PAYLOAD_STATE_ROOT_INDEX, EXECUTION_PAYLOAD_BLOCK_NUMBER_INDEX],
  );

  is_valid_merkle_branch(
    execution_payload_root,
    execution_payload.execution_payload_branch,
    EXECUTION_PAYLOAD_INDEX.floor_log2(),
    get_subtree_index(EXECUTION_PAYLOAD_INDEX),
    update.finalized_header.body_root,
  )?;

  if let Some(sync_committee_update) = update.sync_committee_update {
    if update_attested_period == state_period {
      if sync_committee_update.next_sync_committee != state.next_sync_committee {
        Err(Error::InvalidUpdate)?
      }
    }
    is_valid_merkle_branch(
      hash_tree_root(sync_committee_update.next_sync_committee),
      sync_committee_update.next_sync_committee_branch,
      NEXT_SYNC_COMMITTEE_INDEX.floor_log2(),
      get_subtree_index(NEXT_SYNC_COMMITTEE_INDEX),
      update.attested_header.state_root,
    )?;
  }

  // verify the ancestry proofs
  for ancestor in update.ancestor_blocks {
    match ancestor.ancestry_proof {
      AncestryProof::BlockRoots { block_roots_proof, block_roots_branch } => {
        let block_roots_root = calculate_merkle_root(
          hash_tree_root(ancestor.header),
          block_roots_proof.block_header_branch,
          block_roots_proof.block_header_index,
        );

        is_valid_merkle_branch(
          block_roots_root,
          block_roots_branch,
          BLOCK_ROOTS_INDEX.floor_log2(),
          get_subtree_index(BLOCK_ROOTS_INDEX),
          update.finalized_header.state_root,
        )?;
      },
      AncestryProof::HistoricalRoots {
        block_roots_proof,
        historical_batch_proof,
        historical_roots_proof,
        historical_roots_index,
        historical_roots_branch,
      } => {
        let block_roots_root = calculate_merkle_root(
          hash_tree_root(ancestor.header),
          block_roots_proof.block_header_branch,
          block_roots_proof.block_header_index,
        );

        let historical_batch_root = calculate_merkle_root(
          block_roots_root,
          historical_batch_proof,
          HISTORICAL_BATCH_BLOCK_ROOTS_INDEX,
        );

        let historical_roots_root = calculate_merkle_root(
          historical_batch_root,
          historical_roots_proof,
          historical_roots_index,
        );

        is_valid_merkle_branch(
          historical_roots_root,
          historical_roots_branch,
          HISTORICAL_ROOTS_INDEX.floor_log2(),
          get_subtree_index(HISTORICAL_ROOTS_INDEX),
          update.finalized_header.state_root,
        );
      },
    };

    // verify the associated execution paylaod header.
    let execution_payload = ancestor.execution_payload;
    let execution_payload_root = calculate_merkle_root(
      hash_tree_root(execution_payload.state_root),
      execution_payload.state_root_branch,
      EXECUTION_PAYLOAD_STATE_ROOT_INDEX,
    );

    is_valid_merkle_branch(
      execution_payload_root,
      execution_payload.execution_payload_branch,
      EXECUTION_PAYLOAD_INDEX.floor_log2(),
      get_subtree_index(EXECUTION_PAYLOAD_INDEX),
      ancestor.header.body_root,
    )?;
  }

  Ok(())
}
```

Next we define the necessary types and methods  for light clients to conform with the [official IBC 02-client specification](https://github.com/cosmos/ibc/blob/main/spec/core/ics-002-client-semantics/README.md):

## IBC Types

```rust
/// The consensus state type for the Beacon chain light client
/// [source](https://github.com/cosmos/ibc/blob/main/spec/core/ics-002-client-semantics/README.md#consensusstate)
struct ConsensusState {
  /// state root of the execution chain
  state_root: H256,
  /// timestamp of the beacon block
  timestamp: u64,
}

impl ConsensusState {
  fn from(header: BeaconHeader, payload: ExecutionPayloadProof) -> Self {
    ConsensusState {
      state_root: payload.state_root,
      timestamp: GENESIS_TIMESTAMP + (header.slot * 12), // 12 seconds per slot.
    }
  }
}

/// The client state type for the Beacon chain light client
/// [source](https://github.com/cosmos/ibc/blob/main/spec/core/ics-002-client-semantics/README.md#clientstate)
struct ClientState {
  state: LightClientState,
  frozen_height: Option<Height>,
}

/// The client message type for the Beacon chain light client
/// [source](https://github.com/cosmos/ibc/blob/main/spec/core/ics-002-client-semantics/README.md#clientmessage)
enum ClientMessage {
  Header(LightClientUpdate),
  Misbehaviour {
    header_1: LightClientUpdate,
    header_2: LightClientUpdate,
  },
}
```

## IBC Methods

```rust
use patricia_merkle_trie::{keccak::KeccakHasher, EIP1186Layout, StorageProof};
use rlp::{Decodable, Rlp};
use rlp_derive::RlpDecodable;
use trie_db::{Trie, TrieDBBuilder};

/// The ethereum account stored in the global state trie.
#[derive(RlpDecodable, Debug)]
struct Account {
  nonce: u64,
  balance: U256,
  storage_root: H256,
  code_hash: H256,
}

/// The state proof consists of the proof of the IBC contract account stored
/// in the global state trie, as well as the storage proof of the IBC paths defined by
/// https://github.com/cosmos/ibc/blob/main/spec/core/ics-024-host-requirements/README.md#path-space
#[derive(RlpDecodable, Debug)]
struct StateProof {
  account_proof: Vec<Vec<u8>>,
  storage_proof: Vec<Vec<u8>>,
}

/// The light client validity predicate
/// [source](https://github.com/cosmos/ibc/blob/main/spec/core/ics-002-client-semantics/README.md#validity-predicate)
fn verify_client_message(client_state: ClientState, message: ClientMessage) -> Result<(), Error> {
  match message {
    ClientMessage::Header(update) => verify_sync_committee_attestation(client_state.state, update)?,
    ClientMessage::Misbehaviour { header_1, header_2 } => {
      if header_1.execution_payload.block_number !=
        header_2.execution_payload.block_number
      {
        Err(Error::InvalidMisbehaviour)?
      }
      verify_sync_committee_attestation(client_state.state, header_1)?;
      verify_sync_committee_attestation(client_state.state, header_2)?;
    },
  };

  Ok(())
}

/// The misbehaviour predicate
/// [source](https://github.com/cosmos/ibc/blob/main/spec/core/ics-002-client-semantics/README.md#misbehaviour-predicate)
fn check_for_misbehaviour(
  ctx: &dyn IbcHost,
  client_id: ClientId,
  message: ClientMessage,
) -> Result<bool, Error> {
  if matches!(client_message, ClientMessage::Misbehaviour(_)) {
    return Ok(true)
  }

  // we also check that this update doesn't include competing consensus states for heights we
  // already processed.
  let header = match client_message {
    ClientMessage::Header(header) => header,
    _ => unreachable!("We've checked for misbehavior; qed"),
  };

  for ancestor in header.ancestors {
    let (height, consensus_state) =
      ConsensusState::from(ancestor.header, ancestor.execution_payload);

    match ctx.maybe_consensus_state(&client_id, height)? {
      Some(cs) => {
        if cs != consensus_state {
          // Houston we have a problem
          return Ok(true)
        }
      },
      None => {},
    };
  }

  Ok(false)
}

/// The update function for a valid light client update
/// [source](https://github.com/cosmos/ibc/blob/main/spec/core/ics-002-client-semantics/README.md#updatestate)
fn update_state(
  mut client_state: ClientState,
  message: ClientMessage,
) -> Result<(ClientState, Vec<(Height, ConsensusState)>), Error> {
  let header = match client_message {
    ClientMessage::Header(header) => header,
    _ => unreachable!("We've checked for misbehavior; qed"),
  };

  let mut consensus_states = vec![];

  // include the verified ancestor blocks as consensus states
  for ancestor in header.ancestors {
    let (height, consensus_state) =
      ConsensusState::from(ancestor.header, ancestor.execution_payload);
    consensus_state.push((height, consensus_state));
  }

  let (height, consensus_state) =
    ConsensusState::from(header.finalized_header, header.execution_payload);
  consensus_state.push((height, consensus_state));

  // rotate the sync committee if an update is present
  if let Some(sync_committee_update) = header.sync_committee_update {
    client_state.state.current_sync_committee = client_state.state.next_sync_committee;
    client_state.state.next_sync_committee = sync_committee_update.next_sync_committee;
  }

  client_state.state.finalized_header = header.finalized_header;

  Ok((client_state, consensus_states))
}

/// The update function for a misbehaviour
/// [source](https://github.com/cosmos/ibc/blob/main/spec/core/ics-002-client-semantics/README.md#updatestateonmisbehaviour)
fn update_state_on_misbehaviour(message: ClientMessage) -> Result<(), Error> {
  client_state.frozen_height = Some(Height::new(0, client_state.latest_para_height as u64));
  Ok(client_state)
}

/// State verification functions
/// [source](https://github.com/cosmos/ibc/blob/main/spec/core/ics-002-client-semantics/README.md#state-verification)
fn verify_membership(
  clientState: ClientState,
  height: Height,
  proof: CommitmentProof,
  path: CommitmentPath,
  value: Vec<u8>,
) -> Result<bool, Error> {
  // the proof should be RLP encoded
  let state_proof = StateProof::decode(&Rlp::new(&proof))?;

  // verify the account proof
  let db = StorageProof::new(state_proof.account_proof).into_memory_db::<KeccakHasher>();
  let trie = TrieDBBuilder::<EIP1186Layout<KeccakHasher>>::new(&db, &root).build();
  let result = trie.get(&keccak(&IBC_CONTRACT_ADDRESS))?.ok_or_else(|| Error::InvalidProof)?;

  // the raw account data stored in the state proof:
  let account = Account::decode(&Rlp::new(&result))?;

  // verify the storage proof
  let db = StorageProof::new(state_proof.storage_proof).into_memory_db::<KeccakHasher>();
  let trie = TrieDBBuilder::<EIP1186Layout<KeccakHasher>>::new(&db, &account.storage_root).build();
  // a custom method which translates ibc paths to their storage keys in the ibc contract storage.
  let result = trie.get(&ibc_path_to_storage_key(path))?.ok_or_else(|| Error::InvalidProof)?;

  if result != value {
    return Ok(false)
  }

  return Ok(true)
}

/// non-membership proof verification
fn verify_non_membership(
  clientState: ClientState,
  height: Height,
  proof: CommitmentProof,
  path: CommitmentPath,
) -> Result<bool, Error> {
  // the proof should be RLP encoded
  let state_proof = StateProof::decode(&Rlp::new(&proof))?;

  // verify the account proof
  let db = StorageProof::new(state_proof.account_proof).into_memory_db::<KeccakHasher>();
  let trie = TrieDBBuilder::<EIP1186Layout<KeccakHasher>>::new(&db, &root).build();
  let result = trie.get(&keccak(&IBC_CONTRACT_ADDRESS))?.ok_or_else(|| Error::InvalidProof)?;

  // the raw account data stored in the state proof:
  let account = Account::decode(&Rlp::new(&result))?;

  // verify the storage proof
  let db = StorageProof::new(state_proof.storage_proof).into_memory_db::<KeccakHasher>();
  let trie = TrieDBBuilder::<EIP1186Layout<KeccakHasher>>::new(&db, &account.storage_root).build();
  // a custom method which translates ibc paths to their storage keys in the ibc contract storage.
  let result = trie.get(&ibc_path_to_storage_key(path))?;

  if result != None {
    return Ok(false)
  }

  return Ok(true)
}
```

## Sources

- [The annotated beacon chain consensus specification](https://eth2book.info/)
- [The official beacon chain consensus specification](https://github.com/ethereum/consensus-specs)
- [The Ethereum yellow paper](https://ethereum.github.io/yellowpaper/paper.pdf)
- [Casper the Finality Friendly Gadget](https://arxiv.org/abs/1710.09437)


## Properties & Invariants

Correctness guarantees as provided by the sync committee protocol.

## Backwards Compatibility

Not applicable.

## Forwards Compatibility

Not applicable. Alterations to the client verification algorithm will require a new client standard.

## Example Implementation

None yet.

## Other Implementations

None at present.

## History

January 05, 2023 - Initial version

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
