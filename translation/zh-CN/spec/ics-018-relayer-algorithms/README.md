---
ics: 18
title: 中继器算法
stage: 草案
category: IBC/TAO
kind: 接口
requires: 24, 25, 26
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-03-07
modified: 2019-08-25
---

## 概要

中继器算法是 IBC 的“物理”连接层-链下进程通过扫描链的状态，构造适当的数据报，并按照 IBC 规定在对方链上执行，从而在运行 IBC 协议的两条链之间中继数据。

### 动机

在 IBC 协议中，区块链只能记录将特定数据发送到另一条链的*意图* -它不能直接访问网络传输层。物理数据报的中继必须由可访问传输层（例如 TCP/IP）的链下基础设施执行。该标准定义了*中继器*算法，可以查询链状态的链下进程执行这个算法，来实现中继。

### 定义

*中继器*是一种链下进程，能够使用 IBC 协议读取状态并将交易提交到某些账本集。

### 所需属性

- IBC 的仅一次传递或超时安全属性都不应依赖中继器的行为（假设拜占庭行为的中继器）。
- IBC 的中继活性仅应依赖于至少一个正确的，活跃的中继器存在。
- 中继应该是不需许可的，所有必要的验证都应在链上执行。
- 应该最小化 IBC 用户和中继器之间的必要通信。
- 应能在应用层提供中继器激励措施。

## 技术指标

### 基础中继器算法

中继器算法是在一个实现了 IBC 协议的链集`C`上定义的。每个中继器不一定需要访问链间网络中所有链的状态来读取数据报或将数据报写入链间网络中的所有链（尤其是在许可链或私有链的情况下），不同的中继器可以在不同子集之间中继。

`pendingDatagrams`根据两条链的状态计算要从一个链中继到另一个链的所有有效数据报的集合。中继器必须具有为其中继的集合中的区块链实现了哪些 IBC 协议的子集的先验知识（例如，通过阅读源代码）。下面定义了一个示例。

`submitDatagram`是链自己定义的过程（提交某个交易）。数据报可以每个当作单独的交易提交，也可以在链支持的情况下作为一整个交易原子性提交。

`relay`每隔一段时间就会调用一次 - 不高于任一链的出块速度，并且可能根据中继器期望的中继频率而降低一些。

不同的中继器可以在不同的链之间进行中继-只要每对链具有至少一个正确且活跃的中继器，这些链就可以保持活性，网络中链之间流动的所有数据包最终都将被中继。

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

### 待处理的数据报

`pendingDatagrams`整理要从一台机器发送到另一台机器的数据报。此功能的实现将取决于两台机器都支持的 IBC 协议的子集以及源机器的状态布局。特定的中继器可能还会实现其自己的过滤器功能，以便仅中继可被中继的数据报的子集（例如，一个为了能中继而链下付过费的子集）。

下面概述了在两个链之间执行单向中继的示例实现。通过交换`chain`和`counterparty` ，可以更改为执行双向中继。 哪个中继器进程负责哪个数据报是一个灵活的选择-在此示例中，中继器进程中继在`chain`上开始的所有握手（将数据报发送到两个链），中继从`chain`发送的所有数据包到`counterparty` ，并中继所有数据包的确认从`counterparty`发送到`chain` 。

```typescript
function pendingDatagrams(chain: Chain, counterparty: Chain): List<Set<Datagram>> {
  const localDatagrams = []
  const counterpartyDatagrams = []

  // ICS2 : Clients
  // - Determine if light client needs to be updated (local & counterparty)
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
  // - Determine if any connection handshakes are in progress
  connections = chain.getConnectionsUsingClient(counterparty)
  for (const localEnd of connections) {
    remoteEnd = counterparty.getConnection(localEnd.counterpartyIdentifier)
    if (localEnd.state === INIT && remoteEnd === null)
      // Handshake has started locally (1 step done), relay `connOpenTry` to the remote end
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
      // Handshake has started on the other end (2 steps done), relay `connOpenAck` to the local end
      localDatagrams.push(ConnOpenAck{
        identifier: localEnd.identifier,
        version: remoteEnd.version,
        proofTry: remoteEnd.proof(),
        proofConsensus: remoteEnd.client.consensusState.proof(),
        proofHeight: remoteEnd.client.height,
        consensusHeight: remoteEnd.client.height,
      })
    else if (localEnd.state === OPEN && remoteEnd.state === TRYOPEN)
      // Handshake has confirmed locally (3 steps done), relay `connOpenConfirm` to the remote end
      counterpartyDatagrams.push(ConnOpenConfirm{
        identifier: remoteEnd.identifier,
        proofAck: localEnd.proof(),
        proofHeight: height,
      })
  }

  // ICS4 : Channels & Packets
  // - Determine if any channel handshakes are in progress
  // - Determine if any packets, acknowledgements, or timeouts need to be relayed
  channels = chain.getChannelsUsingConnections(connections)
  for (const localEnd of channels) {
    remoteEnd = counterparty.getConnection(localEnd.counterpartyIdentifier)
    // Deal with handshakes in progress
    if (localEnd.state === INIT && remoteEnd === null)
      // Handshake has started locally (1 step done), relay `chanOpenTry` to the remote end
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
      // Handshake has started on the other end (2 steps done), relay `chanOpenAck` to the local end
      localDatagrams.push(ChanOpenAck{
        portIdentifier: localEnd.portIdentifier,
        channelIdentifier: localEnd.channelIdentifier,
        version: remoteEnd.version,
        proofTry: remoteEnd.proof(),
        proofHeight: localEnd.client.height,
      })
    else if (localEnd.state === OPEN && remoteEnd.state === TRYOPEN)
      // Handshake has confirmed locally (3 steps done), relay `chanOpenConfirm` to the remote end
      counterpartyDatagrams.push(ChanOpenConfirm{
        portIdentifier: remoteEnd.portIdentifier,
        channelIdentifier: remoteEnd.channelIdentifier,
        proofAck: localEnd.proof(),
        proofHeight: height
      })

    // Deal with packets
    // First, scan logs for sent packets and relay all of them
    sentPacketLogs = queryByTopic(height, "sendPacket")
    for (const logEntry of sentPacketLogs) {
      // relay packet with this sequence number
      packetData = Packet{logEntry.sequence, logEntry.timeout, localEnd.portIdentifier, localEnd.channelIdentifier,
                          remoteEnd.portIdentifier, remoteEnd.channelIdentifier, logEntry.data}
      counterpartyDatagrams.push(PacketRecv{
        packet: packetData,
        proof: packet.proof(),
        proofHeight: height,
      })
    }
    // Then, scan logs for received packets and relay acknowledgements
    recvPacketLogs = queryByTopic(height, "recvPacket")
    for (const logEntry of recvPacketLogs) {
      // relay packet acknowledgement with this sequence number
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

中继器可以选择过滤这些数据报，以中继特定的客户端，特定的连接，特定的通道，甚至特定种类的数据包，也许是根据费用支付模型（本文档未指定，因为它可能会各不相同）。

### 排序约束

在中继器进程上存在隐式排序约束，以确定必须以什么顺序提交哪些数据报。例如，必须先提交区块头才能最终确定存储在轻客户端中特定高度的共识状态和承诺根，然后才能转发数据包。两条链直接的中继器进程负责频繁查询两条链的状态，以确定何时必须中继什么。

### 捆绑

如果主机状态机支持，则中继器进程可以将许多数据报捆绑到一个交易中，这将导致它们按顺序执行，并平摊所有开销成本（例如，签名检查费用）。

### 竞态条件

在同一对模块和链对之间进行中继的多个中继器可能会尝试同时中继同一数据包（或提交相同的区块头）。如果有两个中继器这样做，则第一个交易将成功，而第二个交易将失败。为缓解这种情况，中继器之间或发送原始数据包的参与者与中继器之间的带外协调是必要的。进一步的讨论超出了本标准的范围。

### 激励措施

中继器进程必须有能够访问两个链的具有足够余额的帐户，以支付交易费用。中继器可以采用应用程序级别的方法来补偿这些费用，例如通过在数据包数据本身中包含少量费用—中继器费用支付的协议将在此 ICS 的未来版本中或在单独的 ICS 中进行描述。

可以安全的并行运行任意数量的中继器进程（实际上，预计单独的中继器会服务于链间的单独子集）。但是，如果他们多次提交相同的证明，则可能会花费不必要的费用，因此一些最小的协调可能是理想的（例如，将特定的中继器分配给特定的数据包或扫描内存池以查找未处理的交易）。

## 向后兼容性

不适用。中继器进程是链下的，可以根据需要进行升级或降级。

## 向前兼容性

不适用。中继器进程是链下的，可以根据需要进行升级或降级。

## 示例实现

即将到来。

## 其他实现

即将到来。

## 历史

2019年3月30日-提交初稿

2019年4月15日-修订格式和清晰度

2019年4月23日-注释修订;草案合并

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
