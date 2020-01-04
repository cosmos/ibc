---
ics: '18'
title: Relayer 알고리즘
stage: 초안
category: IBC/TAO
kind: interface
requires: 24, 25, 26
author: Christopher Goes <cwgoes@tendermint.com>
created: '2019-03-07'
modified: '2019-08-25'
---

## 개요

Relayer 알고리즘은 IBC의 "물리적인" 연결 계층입니다. 이 알고리즘은 IBC 프로토콜을 실행하는 두 체인 사이에 정보를 전달(중계)하는 off-chain 프로세스로, 프로토콜에서 허용하는 대로 각 체인의 상태를 살펴보고, 적절한 데이터그램을 만들며, 이를 상대 체인에서 실행합니다.

### 동기

IBC 프로토콜에서, 블록체인은 다른 체인으로 특정 정보를 보낸다는 *의도*만을 기록합니다 —  즉 블록체인은 네트워크 전송 계층에 직접 접근하지 않습니다. 물리적인 데이터그램의 전달은 TCP/IP와 같은 전송 계층에 접근하는 off-chain 인프라가 수행해야 합니다. 이 표준은 *relayer* 알고리즘의 개념을 정의하며, 알고리즘은 체인간 중계를 수행하기 위하여 체인의 상태를 질의할 수 있는 off-chain 프로세스에 의해 실행됩니다.

### 정의

*relayer*란 IBC 프로토콜을 이용하는 원장의 상태를 읽고 원장에 트랜잭션을 보내는 off-chain 프로세스입니다.

### 요구 속성

- Relayer가 비잔틴이라고 가정하면, IBC의 안전 속성인 '정확히-한번(exactly-once)'이나 '전달-또는-시간초과(deliver-or-timeout)'는 relayer 행동에 의존하면 안 됩니다.
- IBC의 패킷 전달의 liveness는 최소 하나의 올바른 live 상태의 relayer의 존재에만 의존해야 합니다.
- 전달은 권한에 관계 없이 일어나야 하며, 필요한 모든 검증은 on-chain에서 수행되어야 합니다.
- IBC 사용자와 relayer 사이 소통의 필요성은 최소화해야 합니다.
- 어플리케이션 계층에서 relayer에게 보상을 제공할 수 있어야 합니다.

## 기술 사양

### 기본 relayer 알고리즘

Relayer 알고리즘은 IBC 프로토콜로 구현된 체인 집합 `C`에서 정의됩니다. 각 relayer는 꼭 인터체인 네트워크의 모든 체인의 상태를 읽거나 데이터그램을 쓸 필요는 없습니다 (특히 허가형 또는 프라이빗 체인의 경우) — 다른 relayer는 다른 부분집합 사이를 중계할 것입니다.

`pendingDatagrams`은 한 체인에서 다른 체인으로 전달되는 모든 유효한 데이터그램 집합을 두 체인의 상태에 기반하여 계산합니다. Relayer는 중계하려는 블록체인이 IBC 프로토콜의 어떤 부분을 구현했는지 사전에 알고 있어야 합니다 (예. 소스 코드를 읽음). 이 예시는 아래에 정의됩니다.

`submitDatagram`은 체인마다 정의된 (일종의 트랜잭션을 제출하는) 절차입니다. 여러 데이터그램은 각각 하나의 트랜잭션으로, 또는 체인이 지원하는 경우 원자성을 가지고 하나의 트랜잭션으로 제출될 수 있습니다.

Relayer는 `relay`를 자주 호출합니다. 어떤 체인에서든 블록당 한번 이상 발생하지는 않으며, relayer가 얼마나 자주 중계하는지에 따라 아마 이보다는 덜 자주 발생합니다.

다른 relayer는 다른 체인들 사이를 중계합니다. 체인의 쌍이 각각 최소 하나의 올바르고 live한 relayer를 가지고 체인은 계속 live 상태이기만 하면, 네트워크에서 체인 간에 흐르는 모든 패킷은 결국 중계됩니다.

```typescript
function relay(C: Set<Chain>) {
  for (const chain of C)
    for (const counterparty of C)
      if (counterparty !== chain) {
        const datagrams = chain.pendingDatagrams(counterparty)
        for (const localDatagram of datagrams[0])
          chain.submitDatagram(localDatagram)
        for (const counterpartyDatagram of datagrams[1])
          counterparty.submitDatagram(counterpartyDatagram)
      }
}
```

### 보류 중인 데이터그램 (pending datagrams)

`pendingDatagrams`은 한 machine에서 다른 machine으로 전송되는 데이터그램을 수집합니다. 이 기능의 구현은 두 machine이 지원하는 IBC 프로토콜의 범위와 보내는 machine의 상태 형식(state layout)에 따라 다릅니다. 특정한 relayer는 중계할 수 있는 일부 데이터그램만 중계하기 위해 자체 필터 기능을 구현하고자 할 수 있습니다. (예, off-chain 방식으로 중계하도록 지불된 데이터그램만 중계)

두 체인간 단방향 중계를 수행하는 구현의 예시는 다음과 같습니다. 이 구현은 `chain`와 `counterparty`를 수정하여 양방향 중계를 수행하도록 변경될 수 있습니다.
어떤 relayer 프로세스가 어떤 데이터그램을 맡을지는 유연하게 선택합니다. 이 예시에서, relayer 프로세스는 `chain`에서 시작된 모든 (양 체인에 데이터그램을 보내는) handshake를 중계하고, `chain`에서 `counterparty`로 보내는 모든 패킷을 중계하며, `counterparty`에서 `chain`로 보내는 모든 패킷의 승인을 중계합니다.

```typescript
function pendingDatagrams(chain: Chain, counterparty: Chain): List<Set<Datagram>> {
  const localDatagrams = []
  const counterpartyDatagrams = []

  // ICS2 : Clients
  // - light client가 업데이트 될 필요가 있는지 결정 (local & counterparty)
  height = chain.latestHeight()
  client = counterparty.queryClientConsensusState(chain)
  if client.height < height {
    header = chain.latestHeader()
    counterpartyDatagrams.push(ClientUpdate{chain, header})
  }
  counterpartyHeight = counterparty.latestHeight()
  client = chain.queryClientConsensusState(counterparty)
  if client.height < counterpartyHeight {
    header = counterparty.latestHeader()
    localDatagrams.push(ClientUpdate{counterparty, header})
  }

  // ICS3 : Connections
  // - connection handshake가 진행 중인지 확인
  connections = chain.getConnectionsUsingClient(counterparty)
  for (const localEnd of connections) {
    remoteEnd = counterparty.getConnection(localEnd.counterpartyIdentifier)
    if (localEnd.state === INIT && remoteEnd === null)
      // Handshake가 local에서 시작되고(1단계 완료), `connOpenTry`를 remote end으로 전달
      counterpartyDatagrams.push(ConnOpenTry{
        desiredIdentifier: localEnd.counterpartyConnectionIdentifier,
        counterpartyConnectionIdentifier: localEnd.identifier,
        counterpartyClientIdentifier: localEnd.clientIdentifier,
        clientIdentifier: localEnd.counterpartyClientIdentifier,
        version: localEnd.version,
        counterpartyVersion: localEnd.version,
        proofInit: localEnd.proof(),
        proofConsensus: localEnd.client.consensusState.proof(),
        proofHeight: height,
        consensusHeight: localEnd.client.height,
      })
    else if (localEnd.state === INIT && remoteEnd.state === TRYOPEN)
      // Handshake가 다른쪽 말단에서 시작되고(2단계 완료), `connOpenAck`를 local end로 전달
      localDatagrams.push(ConnOpenAck{
        identifier: localEnd.identifier,
        version: remoteEnd.version,
        proofTry: remoteEnd.proof(),
        proofConsensus: remoteEnd.client.consensusState.proof(),
        proofHeight: remoteEnd.client.height,
        consensusHeight: remoteEnd.client.height,
      })
    else if (localEnd.state === OPEN && remoteEnd.state === TRYOPEN)
      // Handshake는 local에서 확정되고(3단계 완료), `connOpenConfirm`를 remote end에 전달
      counterpartyDatagrams.push(ConnOpenConfirm{
        identifier: remoteEnd.identifier,
        proofAck: localEnd.proof(),
        proofHeight: height,
      })
  }

  // ICS4 : Channels & Packets
  // - channel handshake가 진행 중인지 확인
  // - 패킷, 인증, 또는 타임아웃을 전달할 필요가 있는지 결정
  channels = chain.getChannelsUsingConnections(connections)
  for (const localEnd of channels) {
    remoteEnd = counterparty.getConnection(localEnd.counterpartyIdentifier)
    // 진행 중인 handshakes 처리
    if (localEnd.state === INIT && remoteEnd === null)
      // Handshake가 local에서 시작되고(1단계 완료), `chanOpenTry`를 remote end로 전달
      counterpartyDatagrams.push(ChanOpenTry{
        order: localEnd.order,
        connectionHops: localEnd.connectionHops.reverse(),
        portIdentifier: localEnd.counterpartyPortIdentifier,
        channelIdentifier: localEnd.counterpartyChannelIdentifier,
        counterpartyPortIdentifier: localEnd.portIdentifier,
        counterpartyChannelIdentifier: localEnd.channelIdentifier,
        version: localEnd.version,
        counterpartyVersion: localEnd.version,
        proofInit: localEnd.proof(),
        proofHeight: height,
      })
    else if (localEnd.state === INIT && remoteEnd.state === TRYOPEN)
      // Handshake가 다른 말단에서 시작되고(2단계 완료), `chanOpenAck`를 local end로 전달
      localDatagrams.push(ChanOpenAck{
        portIdentifier: localEnd.portIdentifier,
        channelIdentifier: localEnd.channelIdentifier,
        version: remoteEnd.version,
        proofTry: remoteEnd.proof(),
        proofHeight: localEnd.client.height,
      })
    else if (localEnd.state === OPEN && remoteEnd.state === TRYOPEN)
      // Handshake가 local에서 확정되고(3단계 완료), `chanOpenConfirm`을 remote end로 전달
      counterpartyDatagrams.push(ChanOpenConfirm{
        portIdentifier: remoteEnd.portIdentifier,
        channelIdentifier: remoteEnd.channelIdentifier,
        proofAck: localEnd.proof(),
        proofHeight: height
      })

    // 패킷 처리
    // 먼저, 보낸 패킷의 로그를 살펴보고 모두 전달
    sentPacketLogs = queryByTopic(height, "sendPacket")
    for (const logEntry of sentPacketLogs) {
      // 패킷을 시퀀스 번호와 함께 전달
      packetData = Packet{logEntry.sequence, logEntry.timeout, localEnd.portIdentifier, localEnd.channelIdentifier,
                          remoteEnd.portIdentifier, remoteEnd.channelIdentifier, logEntry.data}
      counterpartyDatagrams.push(PacketRecv{
        packet: packetData,
        proof: packet.proof(),
        proofHeight: height,
      })
    }
    // 그 다음, 받은 패킷의 로그를 살펴보고 인증을 전달
    recvPacketLogs = queryByTopic(height, "recvPacket")
    for (const logEntry of recvPacketLogs) {
      // 패킷 인증을 시퀀스 번호와 함께 전달
      packetData = Packet{logEntry.sequence, logEntry.timeout, localEnd.portIdentifier, localEnd.channelIdentifier,
                          remoteEnd.portIdentifier, remoteEnd.channelIdentifier, logEntry.data}
      counterpartyDatagrams.push(PacketAcknowledgement{
        packet: packetData,
        acknowledgement: logEntry.acknowledgement,
        proof: packet.proof(),
        proofHeight: height,
      })
    }
  }

  return [localDatagrams, counterpartyDatagrams]
}
```

Relayer는 수수료 지불 모델(이는 달라질 수 있으므로, 이 문서에서 특정하지 않음)에 따라 특정한 클라이언트, 특정한 연결, 특정한 채널, 또는 심지어 특정한 종류의 패킷을 중계하기 위해 이 데이터그램을 필터하여 선택할 수도 있습니다.

### 순서 결정의 제약 사항

Relayer 프로세스에는 데이터그램을 어떤 순서로 제출할지 결정할 때 고려하는 암시적인 순서 결정의 제약 사항이 있습니다. 예를 들어, 패킷이 전달되기 전에 light client의 특정 블록 높이에 저장된 합의 상태와 commitment root를 완결시키기 위하여 헤더가 제출되어야 합니다. Relay 프로세스는 무엇을 언제 전달할지를 결정하기 위하여 해당 체인의 상태를 자주 질의할 의무가 있습니다.

### Bundling

만약 대상 상태머신이 기능을 지원한다면, relayer 프로세스는 여러 데이터그램을 하나의 트랜잭션에 넣을 수 있습니다. 이는 데이터그램이 순서대로 실행되도록 하고, 수수료 지불을 위한 서명 확인 같은 오버헤드 비용을 나누어 부담하도록 합니다.

### 경쟁 상태 (Race conditions)

같은 모듈과 체인 사이를 중계하는 여러 relayer는 동시에 동일한 패킷을 전달하려고 (또는 동일한 헤더를 제출하려고) 시도할 수도 있습니다. 만약 두 relayer가 그렇게 한다면, 첫번째 트랜잭션은 성공할 것이고, 두번째 트랜잭션은 실패할 것입니다. 이를 완화하기 위해서는, relayer 간의 또는 원본 패킷을 보내는 actor와 relayer 간의 대역 외 조정이 필요합니다. 이 이상의 논의는 이 표준 문서의 범위 밖입니다.

### 장려금 및 보상 (Incentivisation)

Relay 프로세스는 트랜잭션 수수료를 지불해야 하기 때문에 중계하는 양쪽 체인의 충분한 잔액이 있는 계정에 접근할 수 있어야만 합니다. Relayer는 이 수수료를 회수하기 위해서 어플리케이션 레벨의 메소드을 사용할 수도 있습니다. 예로, 패킷 데이터0에 relayer 스스로를 위한 작은 양의 지불을 포함할 수 있습니다. Relayer의 수수료 지불에 대한 프로토콜은 해당 ICS의 미래 버전이나 별개의 ICS에서 설명할 것입니다.

몇 relayer 프로세스는 안전하게 병렬로 실행될 수도 있습니다. (병렬로 실행되는 프로세스는 각각 다른 인터체인 부분집합을 처리할 것으로 기대됩니다.) 그러나 병렬로 실행되는 프로세스가 동일한 증명을 여러번 제출한다면 불필요한 수수료를 지출하게 되고, 따라서 몇가지 최소한의 조직화를 하는 것이 이상적일 수 있습니다. 예를 들어 특정한 패킷에 특정한 relayer를 할당하거나 보류 중(pending)인 트랜잭션의 mempool을 살펴봅니다.

## 하위 호환성

적용되지 않습니다. Relayer 프로세스는 off-chain이며, 필요에 의해 업그레이드되거나 다운그레이드 될 수 있습니다.

## 상위 호환성

적용되지 않습니다. Relayer 프로세스는 off-chain이며, 필요에 의해 업그레이드되거나 다운그레이드 될 수 있습니다.

## 구현 예제

곧 추가될 예정입니다.

## 다른 구현

곧 추가될 예정입니다.

## 역사

2019년 5월 30일 - 초안 제출

2019년 4월 15일 - 형식과 명확성을 위한 수정

2019년 4월 23일 - 의견에 따른 수정; 초안 병합

## 저작권

이 게시물의 모든 내용은 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 라이센스에 의해 보호받습니다.
