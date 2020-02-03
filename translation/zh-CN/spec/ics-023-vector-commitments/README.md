---
ics: 23
title: 向量承诺
stage: 草案
required-by: 2, 24
category: IBC/TAO
kind: 接口
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-04-16
modified: 2019-08-25
---

## 概要

*向量承诺*是一种构造，它对向量中任何索引和元素的存在/不存在的成员关系的短证明产生恒定大小的绑定承诺。 本规范列举了 IBC 协议中使用的承诺构造所需的函数和特性。特别是，IBC 中使用的承诺必须具有*位置约束力* ：它们必须能够证明在特定位置（索引）的值存在或不存在。

### 动机

为了提供可以在另一条链上验证的一条链上发生的特定状态转换的保证，IBC 需要一种有效的密码构造来证明在状态的特定路径上包含或不包含特定值。

### 定义

向量承诺的*管理者*是具有在承诺中添加或删除项目的能力和责任的参与者。通常，这将是区块链的状态机。

*证明者*是负责生成包含或不包含特定元素的证明的参与者。通常，这将是一个中继器（请参阅 [ICS 18](../ics-018-relayer-algorithms) ）。

*验证者*是检查证明来验证承诺的管理者是否添加了特定元素的参与者。通常，这将是在另一条链上运行的 IBC 处理程序（实现 IBC 的模块）。

使用特定的*路径*和*值*类型实例化承诺，它们的类型假定为任意可序列化的数据。

一个*微不足道的函数*是增长速度比任何正多项式的倒数更慢的函数，如[这里](https://en.wikipedia.org/wiki/Negligible_function)中的定义 。

### 所需属性

本文档仅定义所需的属性，而不是具体的实现-请参见下面的“属性”。

## 技术指标

### 数据类型

承诺构造必须指定以下数据类型，这些数据类型可是不透明的（不需要外部检视），但必须是可序列化的：

#### 承诺状态

`CommitmentState`是承诺的完整状态，将由管理器存储。

```typescript
type CommitmentState = object
```

#### 承诺根

`CommitmentRoot`确保一个特定的承诺状态，并且应为恒定大小。

在状态大小恒定的某些承诺构造中， `CommitmentState`和`CommitmentRoot`可以是同一类型。

```typescript
type CommitmentRoot = object
```

#### 承诺路径

`CommitmentPath`是用于验证承诺证明的路径，该路径可以是任意结构化对象（由承诺类型定义）。它必须由`applyPrefix` （定义如下）计算。

```typescript
type CommitmentPath = object
```

#### 前缀

`CommitmentPrefix`定义承诺证明的存储前缀。它在将路径传递到证明验证功能之前应用于路径。

```typescript
type CommitmentPrefix = object
```

函数`applyPrefix`根据参数构造新的承诺路径。它在前缀参数的上下文中解释路径参数。

对于两个`(prefix, path)`元组， `applyPrefix(prefix, path)`必须仅在元组元素相等时才返回相同的键。

`applyPrefix`必须按`Path`来实现，因为`Path`可以具有不同的具体结构。 `applyPrefix`可以接受多种`CommitmentPrefix`类型。

`applyPrefix`返回的`CommitmentPath`并不需要是可序列化的（例如，它可能是树节点标识符的列表），但它需要能够比较是否相等。

```typescript
type applyPrefix = (prefix: CommitmentPrefix, path: Path) => CommitmentPath
```

#### 证明

一个`CommitmentProof`证明一个元素或一组元素的成员资格或非成员资格，可以与已知的承诺根一起验证。证明应是简洁的。

```typescript
type CommitmentProof = object
```

### 所需函数

承诺构造必须提供以下函数，这些函数在路径上定义为可序列化的对象，在值上定义为字节数组：

```typescript
type Path = string

type Value = []byte
```

#### 初始化

`generate`函数从一个路径到值的映射（可能为空）初始化承诺的状态。

```typescript
type generate = (initial: Map<Path, Value>) => CommitmentState
```

#### 根计算

`calculateRoot`函数计算承诺状态的恒定大小的承诺，可用于验证证明。

```typescript
type calculateRoot = (state: CommitmentState) => CommitmentRoot
```

#### 添加和删除元素

`set`功能为承诺中的值设置路径。

```typescript
type set = (state: CommitmentState, path: Path, value: Value) => CommitmentState
```

`remove`函数从承诺中删除路径和其关联值。

```typescript
type remove = (state: CommitmentState, path: Path) => CommitmentState
```

#### 证明生成

`createMembershipProof`函数生成一个证明，证明特定承诺路径已被设置为承诺中的特定值。

```typescript
type createMembershipProof = (state: CommitmentState, path: CommitmentPath, value: Value) => CommitmentProof
```

`createNonMembershipProof`函数生成一个证明，证明承诺路径尚未设置为任何值。

```typescript
type createNonMembershipProof = (state: CommitmentState, path: CommitmentPath) => CommitmentProof
```

#### 证明验证

`verifyMembership`函数验证在承诺中已将路径设置为特定值的证明。

```typescript
type verifyMembership = (root: CommitmentRoot, proof: CommitmentProof, path: CommitmentPath, value: Value) => boolean
```

`verifyNonMembership`函数验证在承诺中尚未将路径设置为任何值的证明。

```typescript
type verifyNonMembership = (root: CommitmentRoot, proof: CommitmentProof, path: CommitmentPath) => boolean
```

### 可选函数

承诺构造可以提供以下功能：

`batchVerifyMembership`函数验证在承诺中已将许多路径设置为特定值的证明。

```typescript
type batchVerifyMembership = (root: CommitmentRoot, proof: CommitmentProof, items: Map<CommitmentPath, Value>) => boolean
```

`batchVerifyNonMembership`函数可验证证明在承诺中尚未将许多路径设置为任何值的证明。

```typescript
type batchVerifyNonMembership = (root: CommitmentRoot, proof: CommitmentProof, paths: Set<CommitmentPath>) => boolean
```

如果定义这些函数，必须和使用`verifyMembership`和`verifyNonMembership`的联合在一起的结果相同（效率可能有所不同）：

```typescript
batchVerifyMembership(root, proof, items) ===
  all(items.map((item) => verifyMembership(root, proof, item.path, item.value)))
```

```typescript
batchVerifyNonMembership(root, proof, items) ===
  all(items.map((item) => verifyNonMembership(root, proof, item.path)))
```

如果批量验证是可行的并且比单独验证每个元素的证明更有效，则承诺构造应定义批量验证功能。

### 属性和不变量

承诺必须是*完整的* ， *合理的*和*有位置约束的* 。这些属性是相对于安全性参数`k`定义的，此安全性参数必须由管理者，证明者和验证者达成一致（并且对于承诺算法通常是恒定的）。

#### 完整性

承诺证明必须是*完整的* ：已添加到承诺中的路径/值映射始终可以被证明已包含在内，未包含的路径始终可以被证明已被排除，除非是`k`定义的可以忽略的概率。

对于任何前缀`prefix`和任何路径`path`最后一个设置承诺`acc`中的值`value`，

```typescript
root = getRoot(acc)
proof = createMembershipProof(acc, applyPrefix(prefix, path), value)
```

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), value) === false) negligible in k
```

对于任何前缀`prefix`和任何路径`path`都没有在承诺`acc`中设置的`proof`的所有值和`value`的所有值 ，

```typescript
root = getRoot(acc)
proof = createNonMembershipProof(acc, applyPrefix(prefix, path))
```

```
Probability(verifyNonMembership(root, proof, applyPrefix(prefix, path)) === false) negligible in k
```

#### 合理性

承诺证明必须是*合理的* ：除非在可配置安全性参数`k`概率下可以忽略不计，否则不能将未添加到承诺中的路径/值映射证明为已包含，或者将已经添加到承诺中的路径证明为已排除。

对于任何前缀`prefix`和任何路径`path`最后一个设置值`value`在承诺`acc`的所有`proof` 的值，

```
Probability(verifyNonMembership(root, proof, applyPrefix(prefix, path)) === true) negligible in k
```

对于任何前缀`prefix`和任何路径`path`没有在承诺`acc`中设置的 `proof`的所有值和`value`的所有值 ，

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), value) === true) negligible in k
```

#### 位置绑定

承诺证明必须是*有位置约束的* ：给定的承诺路径只能映射到一个值，并且承诺证明不能证明同一路径适用于不同的值，除非在概率k下可以被忽略。

对于承诺`acc`设置的任何前缀`prefix`和任何路径`path` ，都有一个`value` ：

```typescript
root = getRoot(acc)
proof = createMembershipProof(acc, applyPrefix(prefix, path), value)
```

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), value) === false) negligible in k
```

对于所有其他值`otherValue` ，其中`value !== otherValue` ，对于`proof`的所有值，

```
Probability(verifyMembership(root, proof, applyPrefix(prefix, path), otherValue) === true) negligible in k
```

## 向后兼容性

不适用。

## 向前兼容性

承诺算法将是固定的。可以通过对连接和通道进行版本控制来引入新算法。

## 示例实现

即将到来。

## 其他实现

即将到来。

## 历史

安全性定义主要来自这些文章（并进行了一些简化）：

- [向量承诺及其应用](https://eprint.iacr.org/2011/495.pdf)
- [应用程序对保留匿名撤销的承诺](https://eprint.iacr.org/2017/043.pdf)
- [用于 IOP 和无状态区块链的承诺批处理技术](https://eprint.iacr.org/2018/1188.pdf)

感谢 Dev Ojha 对这个规范的广泛评论。

2019年4月25日-提交的草稿

## 版权

本文中的所有内容均根据 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 获得许可。
