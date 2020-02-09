---
ics: 25
title: 处理程序接口
stage: 草案
category: IBC/TAO
kind: 实例化
requires: 2, 3, 4, 23, 24
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-23
modified: 2019-08-25
---

## 概要

本文档描述了标准 IBC 实现（称为 IBC 处理程序）向同一状态机内的模块公开的接口，以及 IBC 处理程序对该接口的实现。

### 动机

IBC 是一种模块间通信协议，旨在促进可靠的，经过身份验证的消息在独立的区块链上的模块之间传递。模块应该能够推理出与之交互的接口以及为了安全的使用接口而必须遵守的要求。

### 定义

相关定义与相应的先前标准（定义了函数）中的定义相同。

### 所需属性

- 客户端，连接和通道的创建应尽可能的不需许可。
- 模块集应该是动态的：链应该能够添加和删除模块，这些模块本身可以使用持久性 IBC 处理程序任意绑定到端口或从端口取消绑定。
- 模块应该能够在 IBC 之上编写自己的更复杂的抽象，以提供附加的语义或保证。

## 技术指标

> 注意：如果主机状态机正在使用对象能力认证（请参阅 [ICS 005](../ics-005-port-allocation) ），则所有使用端口的函数都将带有附加的能力键参数。

### 客户端生命周期管理

默认情况下，客户端是没有所有者的：任何模块都可以创建新客户端，查询任何现有客户端，更新任何现有客户端以及删除任何未使用的现有客户端。

处理程序接口暴露 [ICS 2](../ics-002-client-semantics) 中定义的`createClient` ， `updateClient` ， `queryClientConsensusState` ， `queryClient`和`submitMisbehaviourToClient` 。

### 连接生命周期管理

处理程序接口暴露 [ICS 3](../ics-003-connection-semantics) 中定义的`connOpenInit` ， `connOpenTry` ， `connOpenAck` ， `connOpenConfirm`和`queryConnection` 。

默认的 IBC 路由模块应允许外部调用`connOpenTry` ， `connOpenAck`和`connOpenConfirm` 。

### 通道生命周期管理

默认情况下，通道归创建的端口所有，这意味着只允许绑定到该端口的模块检视，关闭或在通道上发送。模块可以使用同一端口创建任意数量的通道。

处理程序接口暴露了 [ICS 4](../ics-004-channel-and-packet-semantics) 中定义的`chanOpenInit` ， `chanOpenTry` ， `chanOpenAck` ， `chanOpenConfirm` ， `chanCloseInit` ， `chanCloseConfirm`和`queryChannel` 。

默认的 IBC 路由模块应允许外部调用`chanOpenTry` ， `chanOpenAck` ， `chanOpenConfirm`和`chanCloseConfirm` 。

### 数据包中继

数据包是需要通道许可的（只有拥有通道的端口可以在其上发送或接收）。

该处理程序接口暴露`sendPacket` ， `recvPacket` ， `acknowledgePacket` ， `timeoutPacket` ， `timeoutOnClose`和`cleanupPacket`，如 [ICS 4](../ics-004-channel-and-packet-semantics)中定义 。

默认  IBC 路由模块应允许外部调用`sendPacket` ， `recvPacket` ， `acknowledgePacket` ， `timeoutPacket` ， `timeoutOnClose`和`cleanupPacket` 。

### 属性和不变量

此处定义的 IBC 处理程序模块接口继承了其关联规范中定义的功能属性。

## 向后兼容性

不适用。

## 向前兼容性

只要在语义上相同，在新链上实现（或升级到现有链）时，此接口可以更改。

## 示例实现

即将到来。

## 其他实现

即将到来。

## 历史

2019年6月9日-编写草案

2019年8月24日-修订，清理

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
