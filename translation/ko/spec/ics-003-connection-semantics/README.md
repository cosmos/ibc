---
ics: '3'
title: 커넥션 시멘틱
stage: 초안
category: IBC/TAO
requires: 2, 24
required-by: 4, 25
author: Christopher Goes <cwgoes@tendermint.com>
created: '2019-03-07'
modified: '2019-08-25'
---

## 개요

이 표준 문서는 IBC *연결*의 추상화: 두 개의 분리된 체인에 있는 두 개의 상태 객체(*연결 단말*)에 대해 설명합니다. 각 객체는 다른 체인의 라이트 클라이언트와 관련 있고, 이와 함께 crosschain -sub-state 검증 및 (채널을 통한) 패킷 교환이 가능하게 합니다. 연결을 안전하게 만들기 위한 두 체인 간의 프로토콜에 대한 설명입니다.

### 동기

코어 IBC 프로토콜은 패킷에 대한 *인증*과 *순서* 시멘틱을 제공합니다. 이것은 패킷들이 송신 블록체인에 커밋되는 것과 (그리고 토큰 예치와 같은 해당 상태 전이의 실행), 특정 순서로 딱 한 번 커밋되는 것 및 같은 순서로 정확히 한 번 전달되는 것을 보장합니다. 이 표준과 [ICS 2](../ics-002-client-semantics)에서 명시된 *연결* 추상화는 IBC의 *인증* 시맨틱을 정의합니다. 순서 시멘틱은 [ICS 4](../ics-004-channel-and-packet-semantics)에 설명되어 있습니다.

### 정의

클라이언트 관련 타입과 함수들은 [ICS 2](../ics-002-client-semantics)에 정의되어 있습니다.

Commitment 증명 관련 타입과 함수들은 [ICS 23](../ics-023-vector-commitments)에 정의되어 있습니다.

`식별자(Identifier)`와 다른 호스트 상태 머신 요구사항들은 [ICS 24](../ics-024-host-requirements)에 정의되어 있습니다. 식별자는 사람이 읽을 수 있는 명칭으로 지을 필요는 없습니다 (그러나 사용하지 않는 식별자들을 사용하는건 좋지 않습니다).

개방 핸드쉐이크 프로토콜은 각 체인이 다른 체인에 대한 연결을 참조하기 위한 식별자를 검증하여 각 체인의 모듈들이 서로 다른 체인을 추론할 수 있도록 합니다.

이 명세에서 언급되는 *행위자(actor)*는 (가스 또는 비슷한 메커니즘을 통해) 계산 / 저장 비용을 지불하여 신뢰할 수 없는 데이터그램을 실행할 수 있는 주체입니다. 가능한 행위자는 다음과 같습니다.

- 계정 키를 이용해 서명한 단말 유저
- 자율적으로 실행되거나, 다른 트랜잭션에 반응하는 온-체인 트랜잭션
- 다른 트랜잭션에 대한 반응 혹은 예약된 형식에 따라 행동하는 온-체인 모듈

### 요구 속성

- 블록체인 구현은 신뢰할 수 없는 행위자와 안전하게 연결을 열고 갱신할 수 있어야 합니다.

#### Pre-Establishment

연결 설정 전:

- 교차 체인의 하위 상태를 검증할 수 없기 때문에, 다른 IBC 하위 프로토콜이 작동해서는 안됩니다.
- (연결을 생성한) 시작 actor는 연결하는 체인의 초기 합의 상태와 연결된 체인 체인의 초기 합의 상태를 지정할 수 있어야 합니다. (예: 트랜잭션 전송)

#### During Handshake

핸드쉐이크가 시작되면:

- 올바른 핸드쉐이크 데이터그램만이 순서대로 실행될 수 있습니다.
- 제 3의 체인이 연결 설정 중인 두 체인 중 하나로 둔갑할 수 없습니다.

#### Post-Establishment

핸드쉐이크가 완료됐을때:

- 두 체인에 생성된 연결 객체들은 시작 actor에 의해 특정된 합의 상태를 포함해야 합니다.
- 데이터그램을 다시 실행하여 다른 체인에 악의적으로 다른 연결 객체를 만들 수 없습니다.


## 기술 명세

### 자료 구조

ICS는 `ConnectionState`와 `ConnectionEnd` 타입들을 정의합니다.

```typescript
enum ConnectionState {
  INIT,
  TRYOPEN,
  OPEN,
}
```

```typescript
interface ConnectionEnd {
  state: ConnectionState
  counterpartyConnectionIdentifier: Identifier
  counterpartyPrefix: CommitmentPrefix
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  version: string | []string
}
```

- `state` 필드는 연결 단말의 현재 상태를 나타냅니다.
- `counterpartyConnectionIdentifier` 필드는 이 연결과 관련된 상대 체인의 연결 단말을 식별합니다.
- `clientIdentifier` 필드는 이 연결과 관련된 클라이언트를 식별합니다.
- `counterpartyClientIdentifier` 필드는 이 연결과 관련된 상대 체인의 클라이언트를 식별합니다.
- `version` 필드는 이 연결을 사용하는 채널, 패킷의 인코딩 방법 또는 프로토콜을 나타내는 문자열입니다.

### Store paths

연결 경로는 유일한 식별자로 저장됩니다.

```typescript
function connectionPath(id: Identifier): Path {
    return "connections/{id}"
}
```

클라이언트에서의 (클라이언트를 사용하여 모든 연결을 조회하는데 사용되는) 일련의 연결에 대한 역방향 매핑은 클라이언트마다 고유한 접두사로 저장됩니다.

```typescript
function clientConnectionsPath(clientIdentifier: Identifier): Path {
    return "clients/{clientIdentifier}/connections"
}
```

### Helper functions

`addConnectionToClient`는 클라이언트와 연관된 연결 세트에 연결 식별자를 추가하는데 사용됩니다.

```typescript
function addConnectionToClient(
  clientIdentifier: Identifier,
  connectionIdentifier: Identifier) {
    conns = privateStore.get(clientConnectionsPath(clientIdentifier))
    conns.add(connectionIdentifier)
    privateStore.set(clientConnectionsPath(clientIdentifier), conns)
}
```

`removeConnectionFromClient`는 클라이언트와 연관된 연결 세트에서 연결 식별자를 삭제하는데 사용됩니다.

```typescript
function removeConnectionFromClient(
  clientIdentifier: Identifier,
  connectionIdentifier: Identifier) {
    conns = privateStore.get(clientConnectionsPath(clientIdentifier, connectionIdentifier))
    conns.remove(connectionIdentifier)
    privateStore.set(clientConnectionsPath(clientIdentifier, connectionIdentifier), conns)
}
```

`CommitmentPrefix`의 자동 적용을 위한 두개의 헬퍼 함수가 정의되어 있습니다. 사양의 다른 부분에서, 이 함수들은 클라이언트에서 `verifyMembership`나 `verifyNonMembership` 함수를 직접 호출하는 대신, 반드시 다른 체인의 상태를 검사하는데 사용해야 합니다.

```typescript
function verifyMembership(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  path: Path,
  value: Value): bool {
    client = queryClient(connection.clientIdentifier)
    client.verifyMembership(height, proof, applyPrefix(connection.counterpartyPrefix, path), value)
}
```

```typescript
function verifyNonMembership(
  connection: ConnectionEnd,
  height: uint64,
  proof: CommitmentProof,
  path: Path): bool {
    client = queryClient(connection.clientIdentifier)
    client.verifyNonMembership(height, proof, applyPrefix(connection.counterpartyPrefix, path))
}
```

### Versioning

핸드쉐이크 단계가 진행되는 동안, 연결의 두 단말은 연결과 관련된 버전 바이트 스트링에 동의합니다. 이 때, 버전 바이트 스트링의 내용은 IBC 코어 프로토콜과는 거리가 멉니다. 앞으로는 어떤 종류의 채널이 해당 연결을 사용할 수 있는지, 또는 채널 관련 데이터 그램에서 어떤 인코딩 형식을 사용할지 나타내는 데 사용될 수 있습니다. 지금은, 호스트 상태 머신은 버전 데이터를 이용하여 IBC 위에서의 사용자 정의 로직과 관련된 인코딩, 우선 순위 또는 연결과 관련된 메타데이터를 정할 수 있습니다.

호스트 상태 머신은 버전 데이터를 안전하게 무시하거나, 빈 문자열을 지정할 수 있습니다.

`checkVersion`은 두 버전이 호환되는지 결정하는 호스트 상태 머신에 의해 정의되며, boolean 타입을 반환하는 함수입니다.

```typescript
function checkVersion(
  version: string,
  counterpartyVersion: string): boolean {
    // defined by host state machine
}
```

이 명세의 다음 버전 또한 이 함수를 정의할 것입니다.

### Sub-protocols

이 ICS는 개방 핸드쉐이크 서브 프로토콜을 정의합니다. 연결이 열릴 때, 연결은 닫힐 수 없고 식별자는 재할당 될 수 없습니다 (이것은 패킷의 재발생 또는 인증 혼란을 방지해줍니다).

헤더 추적과 오동작 감지는 [ICS 2](../ics-002-client-semantics)에 정의되어 있습니다.

![State Machine Diagram](../../../spec/ics-003-connection-semantics/state.png)

#### 식별자 검증

연결은 유일한 `Identifier` 접두어에 의해 저장됩니다. 검증 함수 `validateConnectionIdentifier`은 제공될 것입니다.

```typescript
type validateConnectionIdentifier = (id: Identifier) => boolean
```

만약 제공되지 않는다면, 기본적인 `validateConnectionIdentifier` 함수는 항상 `true`를 반환합니다.

#### Versioning

구현은 반드시 지원하는 버전 리스트를 내림차순으로 반환하는 `getCompatibleVersions` 함수를 정의해야 합니다.

```typescript
type getCompatibleVersions = () => []string
```

구현은 반드시 상대방이 제안한 버전 리스트에서 버전을 선택하는 `pickVersion` 함수를 정의해야 합니다.

```typescript
type pickVersion = ([]string) => string
```

#### Opening Handshake

핸드쉐이크 개방 서브 프로토콜은 두 체인의 합의 상태를 초기화합니다.

개방 핸드쉐이크는 네 개의 데이터그램을 정의합니다: *ConnOpenInit*, *ConnOpenTry*, *ConnOpenAck*, 그리고 *ConnOpenConfirm*.

올바른 프로토콜 실행은 다음과 같은 순서로 실행됩니다 (모든 호출은 ICS 25에 정의된 모듈들에 의해 실행됩니다):

Initiator | Datagram | Chain acted upon | 이전 상태 (A, B) | 이후 상태 (A, B)
--- | --- | --- | --- | ---
Actor | `ConnOpenInit` | A | (none, none) | (INIT, none)
Relayer | `ConnOpenTry` | B | (INIT, none) | (INIT, TRYOPEN)
Relayer | `ConnOpenAck` | A | (INIT, TRYOPEN) | (OPEN, TRYOPEN)
Relayer | `ConnOpenConfirm` | B | (OPEN, TRYOPEN) | (OPEN, OPEN)

서브 프로토콜을 구현하는 두 체인의 개방 핸드쉐이크의 종료 시점에서, 다음과 같은 속성들이 유지됩니다:

- 각 체인은 초기 행위자(actor)가 지정한 대로 서로의 올바른 합의 상태를 갖고 있습니다.
- 각 체인은 다른 체인의 식별자를 알고 있으며, 이에 합의합니다.

이 서브 프로토콜은 모듈로 안티 스팸 대책으로 허가 받을 필요는 없습니다.

*ConnOpenInit*는 체인 A에서의 연결 시도를 초기화합니다.

```typescript
function connOpenInit(
  identifier: Identifier,
  desiredCounterpartyConnectionIdentifier: Identifier,
  counterpartyPrefix: CommitmentPrefix,
  clientIdentifier: Identifier,
  counterpartyClientIdentifier: Identifier) {
    abortTransactionUnless(validateConnectionIdentifier(identifier))
    abortTransactionUnless(provableStore.get(connectionPath(identifier)) == null)
    state = INIT
    connection = ConnectionEnd{state, desiredCounterpartyConnectionIdentifier, counterpartyPrefix,
      clientIdentifier, counterpartyClientIdentifier, getCompatibleVersions()}
    provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
```

*ConnOpenTry*는 체인 A에서 체인 B로의 연결 시도 알림을 전달합니다 (이 코드는 체인 B에서 실행됩니다).

```typescript
function connOpenTry(
  desiredIdentifier: Identifier,
  counterpartyConnectionIdentifier: Identifier,
  counterpartyPrefix: CommitmentPrefix,
  counterpartyClientIdentifier: Identifier,
  clientIdentifier: Identifier,
  counterpartyVersions: string[],
  proofInit: CommitmentProof,
  proofHeight: uint64,
  consensusHeight: uint64) {
    abortTransactionUnless(validateConnectionIdentifier(desiredIdentifier))
    abortTransactionUnless(consensusHeight <= getCurrentHeight())
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{INIT, desiredIdentifier, getCommitmentPrefix(), counterpartyClientIdentifier,
                             clientIdentifier, counterpartyVersions}
    version = pickVersion(counterpartyVersions)
    connection = ConnectionEnd{state, counterpartyConnectionIdentifier, counterpartyPrefix,
                               clientIdentifier, counterpartyClientIdentifier, version}
    abortTransactionUnless(
      connection.verifyMembership(proofHeight, proofInit,
                                  connectionPath(counterpartyConnectionIdentifier),
                                  expected))
    abortTransactionUnless(
      connection.verifyMembership(proofHeight, proofInit,
                                  consensusStatePath(counterpartyClientIdentifier),
                                  expectedConsensusState))
    abortTransactionUnless(provableStore.get(connectionPath(desiredIdentifier)) === null)
    abortTransactionUnless(checkVersion(version, counterpartyVersion))
    identifier = desiredIdentifier
    state = TRYOPEN
       provableStore.set(connectionPath(identifier), connection)
    addConnectionToClient(clientIdentifier, identifier)
}
```

*ConnOpenAck*는 체인 B에서 체인 A로의 연결 개방 시도에 대한 수락 메세지를 전달합니다 (이 코드는 체인 A에서 실행됩니다).

```typescript
function connOpenAck(
  identifier: Identifier,
  version: string,
  proofTry: CommitmentProof,
  proofHeight: uint64,
  consensusHeight: uint64) {
    abortTransactionUnless(consensusHeight <= getCurrentHeight())
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === INIT)
    abortTransactionUnless(checkVersion(connection.version, version))
    expectedConsensusState = getConsensusState(consensusHeight)
    expected = ConnectionEnd{TRYOPEN, identifier, getCommitmentPrefix(),
                             connection.counterpartyClientIdentifier, connection.clientIdentifier,
                             version}
    abortTransactionUnless(
      connection.verifyMembership(proofHeight, proofTry,
                                  connectionPath(connection.counterpartyConnectionIdentifier),
                                  expected))
    abortTransactionUnless(
      connection.verifyMembership(proofHeight, proofTry,
                                  consensusStatePath(connection.counterpartyClientIdentifier),
                                  expectedConsensusState))
    connection.state = OPEN
    abortTransactionUnless(getCompatibleVersions().indexOf(version) !== -1)
    connection.version = version
    provableStore.set(connectionPath(identifier), connection)
}
```

*ConnOpenConfirm*는 두 체인 모두에서 연결이 개방된 이후에 체인 A에서 체인 B로의 연결을 확인합니다 (이 코드는 체인 B에서 실행됩니다).

```typescript
function connOpenConfirm(
  identifier: Identifier,
  proofAck: CommitmentProof,
  proofHeight: uint64) {
    connection = provableStore.get(connectionPath(identifier))
    abortTransactionUnless(connection.state === TRYOPEN)
    expected = ConnectionEnd{OPEN, identifier, getCommitmentPrefix(), connection.counterpartyClientIdentifier,
                             connection.clientIdentifier, connection.version}
    abortTransactionUnless(
      connection.verifyMembership(proofHeight, proofAck,
                                  connectionPath(connection.counterpartyConnectionIdentifier),
                                  expected))
    connection.state = OPEN
    provableStore.set(connectionPath(identifier), connection)
}
```

#### Querying

`queryConnection`를 사용하여 식별자로 연결은 조회될 수 있습니다.

```typescript
function queryConnection(id: Identifier): ConnectionEnd | void {
    return provableStore.get(connectionPath(id))
}
```

특정 클라이언트와 관련된 연결은 `queryClientConnections`를 사용하여 클라이언트 식별자로 조회될 수 있습니다.

```typescript
function queryClientConnections(id: Identifier): Set<Identifier> {
    return privateStore.get(clientConnectionsPath(id))
}
```

### Properties & Invariants

- 연결 식별자들은 선착순입니다: 일단 연결이 성사되면, 두 체인 사이의 유일한 식별자 쌍이 존재하게 됩니다.
- 다른 블록 체인의 IBC 핸들러가 연결 핸드쉐이크를 중간에 간섭할 수 없습니다.

## 하위 호환성

적용되지 않습니다.

## 상위 호환성

이 ICS의 앞으로의 버전은 개방 핸드쉐이크의 버전 협의를 포함합니다. 연결이 성립되고 버전이 협의되면 ICS 6에 따라 향후 버전을 협의할 수 있습니다.

합의 상태는 연결이 성립될 때 선택된 합의 프로토콜에 정의된 `updateConsensusState` 함수에 따라서만 변경될 수 있습니다.

## 예제 구현

곧 구현 될 예정입니다.

## 다른 구현

곧 구현 될 예정입니다.

## History

이 문서의 몇 부분은 [previous IBC specification](https://github.com/cosmos/cosmos-sdk/tree/master/docs/spec/ibc)를 참조했습니다.

2019년 3월 29일 - 초안 제출
2019년 5월 17일 - 초안 확정
2019년 7월 29일 - 클라이언트와 관련된 연결 세트 추적을 위한 개정

## Copyright

모든 컨텐츠는 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 라이센스에 의해 보호 받습니다.
