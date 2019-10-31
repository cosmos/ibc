---
ics: '26'
title: 라우팅 모듈
stage: 초안
category: IBC/TAO
author: Christopher Goes <cwgoes@tendermint.com>
created: '2019-06-09'
modified: '2019-08-25'
---

## 개요

라우팅 모듈은 외부 데이터그램들을 수신하고 블록체인 간 통신 프로토콜 핸들러를 호출하여 핸드셰이크 및 패킷 중계를 처리하는 보조 모듈의 기본 구현입니다.
라우팅 모듈은 패킷을 수신할 때 모듈을 조회하고 호출하는데 사용할 수 있는 모듈의 룩업 테이블을 유지하므로 외부 relayer가 라우팅 모듈에 패킷을 중계만 하면 됩니다.

### 동기

기본 IBC 핸들러는 포트 바인딩, 핸드셰이크 시작, 핸드셰이크 수락, 패킷 송수신 등을 위해 모듈이 개별적으로 IBC 핸들러를 호출해야 하는 수신자 호출(receiver call) 패턴을 사용합니다. 이 방법은 유연하고 간단하지만([디자인 패턴](../../ibc/5_IBC_DESIGN_PATTERNS.md)참고) 이해하기 약간 까다롭고 모듈의 상태를 추적해야 하는 relayer 프로세스 부분에서 추가 작업이 필요할 수 있습니다. 이 표준은 가장 일반적인 기능들을 자동화하고, 패킷들을 라우팅하며, relayer들의 작업을 단순화하는 IBC "라우팅 모듈"을 설명합니다.

라우팅 모듈은 [ICS 5](../ics-005-port-allocation)에서 설명한대로 모듈 관리자로서의 역할을 수행할 수 있으며 모듈을 포트에 바인딩할 수 있는 시기와 해당 포트의 이름을 지정할 수 있는 로직을 구현할 수 있습니다.

### 정의

IBC 핸들러 인터페이스가 제공하는 모든 함수들은 [ICS 25](../ics-025-handler-interface) 에서와 같이 정의됩니다.

`generate` 및 `authenticate` 함수는 [ICS 5](../ics-005-port-allocation) 와 같이 정의됩니다.

### 요구 속성

- 모듈들은 라우팅 모듈을 통해 포트 및 자체 채널에 바인딩할 수 있어야 합니다.
- 간접 호출(call indirection) 계층 이외의 패킷 송수신에는 오버헤드가 추가되어서는 안됩니다.
- 라우팅 모듈은 패킷을 처리할 때 지정된 핸들러 함수를 모듈에서 호출해야 합니다.

## 기술 사양

> 참고: 만약 호스트 상태머신이 obejct capability 인증([ICS 005](../ics-005-port-allocation) 참고)을 사용중이라면, 포트를 사용하는 모든 함수들은 추가적인 capability 매개변수를 사용합니다.

### 모듈 콜백 인터페이스

다양한 데이터그램들을 수신할 때 호출되는 모듈들은 다음과 같은 함수 시그니처들을 라우팅 모듈에 노출해야 합니다.

```typescript
function onChanOpenInit(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string) {
    // defined by the module
}

function onChanOpenTry(
  order: ChannelOrder,
  connectionHops: [Identifier],
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  counterpartyPortIdentifier: Identifier,
  counterpartyChannelIdentifier: Identifier,
  version: string,
  counterpartyVersion: string) {
    // defined by the module
}

function onChanOpenAck(
  portIdentifier: Identifier,
  channelIdentifier: Identifier,
  version: string) {
    // defined by the module
}

function onChanOpenConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // defined by the module
}

function onChanCloseInit(
  portIdentifier: Identifier,
  channelIdentifier: Identifier) {
    // defined by the module
}

function onChanCloseConfirm(
  portIdentifier: Identifier,
  channelIdentifier: Identifier): void {
    // defined by the module
}

function onRecvPacket(packet: Packet): bytes {
    // defined by the module, returns acknowledgement
}

function onTimeoutPacket(packet: Packet) {
    // defined by the module
}

function onAcknowledgePacket(packet: Packet) {
    // defined by the module
}

function onTimeoutPacketClose(packet: Packet) {
    // defined by the module
}
```

실패를 표시하고 핸드셰이크, 패킷 수신 등을 거부할 때는 반드시 예외가 발생해야 합니다.

위에서 나열한 함수 시그니처들은 `ModuleCallbacks` 인터페이스에서 함께 결합됩니다:

```typescript
interface ModuleCallbacks {
  onChanOpenInit: onChanOpenInit,
  onChanOpenTry: onChanOpenTry,
  onChanOpenAck: onChanOpenAck,
  onChanOpenConfirm: onChanOpenConfirm,
  onChanCloseConfirm: onChanCloseConfirm
  onRecvPacket: onRecvPacket
  onTimeoutPacket: onTimeoutPacket
  onAcknowledgePacket: onAcknowledgePacket,
  onTimeoutPacketClose: onTimeoutPacketClose
}
```

콜백은 모듈이 포트에 바인딩될 때 호출됩니다.

```typescript
function callbackPath(portIdentifier: Identifier): Path {
    return "callbacks/{portIdentifier}"
}
```

콜백을 변경해야할 경우 향후 인증을 위해 호출 모듈 식별자도 저장됩니다.

```typescript
function authenticationPath(portIdentifier: Identifier): Path {
    return "authentication/{portIdentifier}"
}
```

### 모듈 매니저로서의 포트 바인딩

IBC 라우팅 모듈은 핸들러 모듈( [ICS 25](../ics-025-handler-interface) )과 호스트 상태 머신의 개별 모듈들 사이에 위치합니다.

모듈 관리자 역할을 하는 라우팅 모듈은 두 종류의 포트를 구분합니다.

- "기존 이름" 포트: "은행"과 같이 사전적으로 표준화된 의미를 갖고 있는 이름들은 선착순으로 지정할 수 있는 것이 아닙니다.
- "새로운 이름" 포트 : (스마트 컨트랙트와 같이) 이전과는 관계가 없는 새로운 존재, 새로운 랜덤 넘버 포트, 생성 후 포트 이름은 다른 채널을 통해 통신할 수 있습니다

라우팅 모듈이 호스트 상태 머신에 의해 인스턴스화되면 기존의 이름들의 집합이 해당 모듈과 함께 할당됩니다.
라우팅 모듈은 모듈별로 언제든지 새로운 포트를 할당할 수 있지만 특정 표준화된 접두사를 사용해야 합니다.

`bindPort` 함수는 라우팅 모듈을 통해 포트에 바인딩하고 콜백을 설정하기 위해 모듈에 의해 호출될 수 있습니다.

```typescript
function bindPort(
  id: Identifier,
  callbacks: Callbacks) {
    abortTransactionUnless(privateStore.get(callbackPath(id)) === null)
    handler.bindPort(id)
    capability = generate()
    privateStore.set(authenticationPath(id), capability)
    privateStore.set(callbackPath(id), callbacks)
}
```

콜백을 변경하기 위해 모듈에서 `updatePort` 함수를 호출할 수 있습니다.

```typescript
function updatePort(
  id: Identifier,
  newCallbacks: Callbacks) {
    abortTransactionUnless(authenticate(privateStore.get(authenticationPath(id))))
    privateStore.set(callbackPath(id), newCallbacks)
}
```

`releasePort` 함수는 이전에 사용중인 포트를 해제하기 위해 모듈에 의해 호출 될 수 있습니다.

> 경고 : 포트를 해제하면 다른 모듈이 해당 포트에 바인딩되어 들어오는 채널 오프닝 핸드셰이크를 가로챌 수 있습니다. 모듈은 안전한 경우에만 포트를 해제해야 합니다.

```typescript
function releasePort(id: Identifier) {
    abortTransactionUnless(authenticate(privateStore.get(authenticationPath(id))))
    handler.releasePort(id)
    privateStore.delete(callbackPath(id))
    privateStore.delete(authenticationPath(id))
}
```

라우팅 모듈은 특정 포트에 바인딩된 콜백을 조회하기 위해  `lookupModule` 함수를 사용할 수 있습니다.

```typescript
function lookupModule(portId: Identifier) {
    return privateStore.get(callbackPath(portId))
}
```

### 데이터그램 핸들러 (쓰기)

*데이터그램*은 라우팅 모듈에서 트랜잭션으로 허용되는 외부 데이터(대용량 바이너리 객체)입니다. 이 섹션은 각 데이터그램에 대한 *핸들러 함수*를 정의하며, 관련 데이터그램이 트랜잭션으로 라우팅 모듈에 제출될 때 실행됩니다.

모든 데이터그램은 다른 모듈에 의해 라우팅 모듈로 안전하게 제출될 수 있습니다.

명시적으로 표시된 것외의 메시지 서명이나 데이터 유효성 검사는 가정하고 있지 않습니다.

#### 클라이언트 생명주기 관리

`ClientCreate` 는 지정된 식별자 및 합의 상태로 새 라이트 클라이언트를 만듭니다.

```typescript
interface ClientCreate {
  identifier: Identifier
  type: ClientType
  consensusState: ConsensusState
}
```

```typescript
function handleClientCreate(datagram: ClientCreate) {
    handler.createClient(datagram.identifier, datagram.type, datagram.consensusState)
}
```

`ClientUpdate` 는 지정된 식별자 및 새 헤더로 기존 라이트 클라이언트를 업데이트합니다.

```typescript
interface ClientUpdate {
  identifier: Identifier
  header: Header
}
```

```typescript
function handleClientUpdate(datagram: ClientUpdate) {
    handler.updateClient(datagram.identifier, datagram.header)
}
```

`ClientSubmitMisbehaviour` 는 지정된 식별자를 사용하여 기존의 라이트 클라이언트에 허위 증명을 제출합니다.

```typescript
interface ClientMisbehaviour {
  identifier: Identifier
  evidence: bytes
}
```

```typescript
function handleClientMisbehaviour(datagram: ClientUpdate) {
    handler.submitMisbehaviourToClient(datagram.identifier, datagram.evidence)
}
```

#### 커넥션 생명주기 관리

`ConnOpenInit` 데이터그램은 다른 체인의 IBC 모듈과의 커넥션 핸드셰이크 프로세스를 시작합니다.

```typescript
interface ConnOpenInit {
  identifier: Identifier
  desiredCounterpartyIdentifier: Identifier
  clientIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  version: string
}
```

```typescript
function handleConnOpenInit(datagram: ConnOpenInit) {
    handler.connOpenInit(
      datagram.identifier,
      datagram.desiredCounterpartyIdentifier,
      datagram.clientIdentifier,
      datagram.counterpartyClientIdentifier,
      datagram.version
    )
}
```

`ConnOpenTry` 데이터그램은 다른 체인의 IBC 모듈에서 핸드셰이크 요청을 수락합니다.

```typescript
interface ConnOpenTry {
  desiredIdentifier: Identifier
  counterpartyConnectionIdentifier: Identifier
  counterpartyClientIdentifier: Identifier
  clientIdentifier: Identifier
  version: string
  counterpartyVersion: string
  proofInit: CommitmentProof
  proofHeight: uint64
  consensusHeight: uint64
}
```

```typescript
function handleConnOpenTry(datagram: ConnOpenTry) {
    handler.connOpenTry(
      datagram.desiredIdentifier,
      datagram.counterpartyConnectionIdentifier,
      datagram.counterpartyClientIdentifier,
      datagram.clientIdentifier,
      datagram.version,
      datagram.counterpartyVersion,
      datagram.proofInit,
      datagram.proofHeight,
      datagram.consensusHeight
    )
}
```

`ConnOpenAck` 데이터그램은 다른 체인의 IBC 모듈이 핸드셰이크를 수락했음을 확인합니다.

```typescript
interface ConnOpenAck {
  identifier: Identifier
  version: string
  proofTry: CommitmentProof
  proofHeight: uint64
  consensusHeight: uint64
}
```

```typescript
function handleConnOpenAck(datagram: ConnOpenAck) {
    handler.connOpenAck(
      datagram.identifier,
      datagram.version,
      datagram.proofTry,
      datagram.proofHeight,
      datagram.consensusHeight
    )
}
```

`ConnOpenConfirm` 데이터그램은 다른 체인의 IBC 모듈에 의한 핸드셰이크 수락을 인지하고 연결을 마무리합니다.

```typescript
interface ConnOpenConfirm {
  identifier: Identifier
  proofAck: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handleConnOpenConfirm(datagram: ConnOpenConfirm) {
    handler.connOpenConfirm(
      datagram.identifier,
      datagram.proofAck,
      datagram.proofHeight
    )
}
```

#### 채널 생명주기 관리

```typescript
interface ChanOpenInit {
  order: ChannelOrder
  connectionHops: [Identifier]
  portIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyPortIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  version: string
}
```

```typescript
function handleChanOpenInit(datagram: ChanOpenInit) {
    module = lookupModule(datagram.portIdentifier)
    module.onChanOpenInit(
      datagram.order,
      datagram.connectionHops,
      datagram.portIdentifier,
      datagram.channelIdentifier,
      datagram.counterpartyPortIdentifier,
      datagram.counterpartyChannelIdentifier,
      datagram.version
    )
    handler.chanOpenInit(
      datagram.order,
      datagram.connectionHops,
      datagram.portIdentifier,
      datagram.channelIdentifier,
      datagram.counterpartyPortIdentifier,
      datagram.counterpartyChannelIdentifier,
      datagram.version
    )
}
```

```typescript
interface ChanOpenTry {
  order: ChannelOrder
  connectionHops: [Identifier]
  portIdentifier: Identifier
  channelIdentifier: Identifier
  counterpartyPortIdentifier: Identifier
  counterpartyChannelIdentifier: Identifier
  version: string
  counterpartyVersion: string
  proofInit: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handleChanOpenTry(datagram: ChanOpenTry) {
    module = lookupModule(datagram.portIdentifier)
    module.onChanOpenTry(
      datagram.order,
      datagram.connectionHops,
      datagram.portIdentifier,
      datagram.channelIdentifier,
      datagram.counterpartyPortIdentifier,
      datagram.counterpartyChannelIdentifier,
      datagram.version,
      datagram.counterpartyVersion
    )
    handler.chanOpenTry(
      datagram.order,
      datagram.connectionHops,
      datagram.portIdentifier,
      datagram.channelIdentifier,
      datagram.counterpartyPortIdentifier,
      datagram.counterpartyChannelIdentifier,
      datagram.version,
      datagram.counterpartyVersion,
      datagram.proofInit,
      datagram.proofHeight
    )
}
```

```typescript
interface ChanOpenAck {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  version: string
  proofTry: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handleChanOpenAck(datagram: ChanOpenAck) {
    module.onChanOpenAck(
      datagram.portIdentifier,
      datagram.channelIdentifier,
      datagram.version
    )
    handler.chanOpenAck(
      datagram.portIdentifier,
      datagram.channelIdentifier,
      datagram.version,
      datagram.proofTry,
      datagram.proofHeight
    )
}
```

```typescript
interface ChanOpenConfirm {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  proofAck: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handleChanOpenConfirm(datagram: ChanOpenConfirm) {
    module = lookupModule(portIdentifier)
    module.onChanOpenConfirm(
      datagram.portIdentifier,
      datagram.channelIdentifier
    )
    handler.chanOpenConfirm(
      datagram.portIdentifier,
      datagram.channelIdentifier,
      datagram.proofAck,
      datagram.proofHeight
    )
}
```

```typescript
interface ChanCloseInit {
  portIdentifier: Identifier
  channelIdentifier: Identifier
}
```

```typescript
function handleChanCloseInit(datagram: ChanCloseInit) {
    module = lookupModule(portIdentifier)
    module.onChanCloseInit(
      datagram.portIdentifier,
      datagram.channelIdentifier
    )
    handler.chanCloseInit(
      datagram.portIdentifier,
      datagram.channelIdentifier
    )
}
```

```typescript
interface ChanCloseConfirm {
  portIdentifier: Identifier
  channelIdentifier: Identifier
  proofInit: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handleChanCloseConfirm(datagram: ChanCloseConfirm) {
    module = lookupModule(datagram.portIdentifier)
    module.onChanCloseConfirm(
      datagram.portIdentifier,
      datagram.channelIdentifier
    )
    handler.chanCloseConfirm(
      datagram.portIdentifier,
      datagram.channelIdentifier,
      datagram.proofInit,
      datagram.proofHeight
    )
}
```

#### 패킷 중계

패킷은 모듈에서 IBC 핸들러를 호출하는 모듈에 의해 직접 전송됩니다.

```typescript
interface PacketRecv {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handlePacketRecv(datagram: PacketRecv) {
    module = lookupModule(datagram.packet.sourcePort)
    acknowledgement = module.onRecvPacket(datagram.packet)
    handler.recvPacket(
      datagram.packet,
      datagram.proof,
      datagram.proofHeight,
      acknowledgement
    )
}
```

```typescript
interface PacketAcknowledgement {
  packet: Packet
  acknowledgement: string
  proof: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handlePacketAcknowledgement(datagram: PacketAcknowledgement) {
    module = lookupModule(datagram.packet.sourcePort)
    module.onAcknowledgePacket(
      datagram.packet,
      datagram.acknowledgement
    )
    handler.acknowledgePacket(
      datagram.packet,
      datagram.acknowledgement,
      datagram.proof,
      datagram.proofHeight
    )
}
```

#### 패킷 타임아웃

```typescript
interface PacketTimeout {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
  nextSequenceRecv: Maybe<uint64>
}
```

```typescript
function handlePacketTimeout(datagram: PacketTimeout) {
    module = lookupModule(datagram.packet.sourcePort)
    module.onTimeoutPacket(datagram.packet)
    handler.timeoutPacket(
      datagram.packet,
      datagram.proof,
      datagram.proofHeight,
      datagram.nextSequenceRecv
    )
}
```

```typescript
interface PacketTimeoutOnClose {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
}
```

```typescript
function handlePacketTimeoutOnClose(datagram: PacketTimeoutOnClose) {
    module = lookupModule(datagram.packet.sourcePort)
    module.onTimeoutPacket(datagram.packet)
    handler.timeoutOnClose(
      datagram.packet,
      datagram.proof,
      datagram.proofHeight
    )
}
```

#### 타임아웃에 의한 클로저 & 패킷 정리

```typescript
interface PacketCleanup {
  packet: Packet
  proof: CommitmentProof
  proofHeight: uint64
  nextSequenceRecvOrAcknowledgement: Either<uint64, bytes>
}
```

```typescript
function handlePacketCleanup(datagram: PacketCleanup) {
    handler.cleanupPacket(
      datagram.packet,
      datagram.proof,
      datagram.proofHeight,
      datagram.nextSequenceRecvOrAcknowledgement
    )
}
```

### (읽기 전용) 쿼리 함수들

클라이언트, 연결 및 채널에 대한 모든 (읽기 전용) 쿼리 함수들은 IBC 핸들러 모듈에 의해 직접 노출되어야합니다.

### 인터페이스 활용 예제

사용 예시는 [ICS 20](../ics-020-fungible-token-transfer) 을 참조하십시오.

### 속성 및 불변량

- 프록시 포트 바인딩은 선입선출입니다. 일단 모듈이 IBC 라우팅 모듈을 통해 포트에 바인딩되면 포트를 해제하기 전까지 해당 모듈만이 해당 포트를 사용할 수 있습니다.

## 하위 호환성

적용되지 않습니다.

## 상위 호환성

라우팅 모듈은 IBC 핸들러 인터페이스와 밀접한 관련이 있습니다.

## 구현 예제

곧 게시될 예정입니다.

## 다른 구현

곧옵니다.

## 히스토리

2019년 6월 9일 - 초안 제출
2019년 7월 28일 - 주요 개정
2019년 8월 25일 - 주요 개정

## 저작권

이 게시물의 모든 내용은 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 라이센스에 의해 보호받습니다.
