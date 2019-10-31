---
ics: '25'
title: 핸들러 인터페이스
stage: 초안
category: IBC/TAO
requires: 2, 3, 4, 23, 24
author: Christopher Goes <cwgoes@tendermint.com>
created: '2019-04-23'
modified: '2019-08-25'
---

## 개요

이 문서는 동일한 상태 머신 내의 모듈들에 대한 표준 IBC 구현 (IBC 핸들러라고 함)에 의해 노출되는 인터페이스와 해당 인터페이스의 구현에 대해 설명합니다.

### 동기

IBC는 모듈 간 통신 프로토콜로, 별도의 블록체인에서 모듈간에 신뢰할 수 있고 인증 된 메시지 전달을 용이하게 하도록 설계되었습니다. 모듈은 인터페이스를 안전하게 사용하기 위해 상호 작용하는 인터페이스와 준수해야하는 요구 사항에 대해 추론 할 수 있어야합니다.

### 정의

관련된 정의들은 (함수들이 정의되어 있는) 이전 표준들에 정의되어 있습니다.

### 요구 속성

- 클라이언트, 커넥션 및 채널들의 생성은 가능한 한 별도의 권한이 없이도 가능해야 합니다.
- 모듈의 집합은 동적이어야 합니다: 체인은 모듈을 추가 및 제거할 수 있어야 합니다. 모듈은 지속적으로 IBC 핸들러를 사용하여 포트에 바인딩하거나 포트에서 바인딩을 해제할 수 있습니다.
- 핸들러 모듈의 신뢰성을 위한 추가적인 약속(semantic) 혹은 보장(guarantee)을 제공하기 위해 IBC 위에 자체적으로 더 복잡한 추상화를 작성할 수 있어야 합니다.

## 기술 사양

> 참고: 만약 호스트 상태머신이 obejct capability 인증([ICS 005](../ics-005-port-allocation) 참고)을 사용중이라면, 포트를 사용하는 모든 함수들은 추가적인 capability 키 매개변수를 사용합니다.

### 클라이언트 생명주기 관리

기본적으로 클라이언트는 소유되지 않습니다. 어떤 모듈이던지 새로운 클라이언트를 생성하고, 기존 클라이언트를 쿼리하고, 기존 클라이언트를 업데이트하고, 사용하지 않는 어떤 기존 클라이언트도 삭제할 수 있습니다.

[ICS 2](../ics-002-client-semantics)에 정의된대로 핸들러 인터페이스는 `createClient`, `updateClient`, `queryClientConsensusState`, `queryClient`와 `submitMisbehaviourToClient` 를 노출합니다.

### 커넥션 생명주기 관리

[ICS 3](../ics-003-connection-semantics)에 정의된대로 핸들러 인터페이스는 `connOpenInit`, `connOpenTry`, `connOpenAck`, `connOpenConfirm`과 `queryConnection`을 노출합니다 .

기본 IBC 라우팅 모듈 SHALL은 `connOpenTry`, `connOpenAck`와 `connOpenConfirm`으로의 외부 호출을 허용합니다.

### 채널 생명주기 관리

기본적으로 채널은 채널을 만든 포트가 소유하게 됩니다. 즉, 해당 포트에 바인딩된 모듈만 채널을 검사, 종료 또는 전송을 할 수 있습니다. 모듈은 동일한 포트를 사용하여 여러 채널을 생성할 수 있습니다.

[ICS 4](../ics-004-channel-and-packet-semantics)에 정의된대로 핸들러 인터페이스는 `chanOpenInit`, `chanOpenTry`, `chanOpenAck`, `chanOpenConfirm`, `chanCloseInit`, `chanCloseConfirm`과 `queryChannel`을 노출합니다.

기본 IBC 라우팅 모듈 SHALL은 `chanOpenTry`, `chanOpenAck`, `chanOpenConfirm`과 `chanCloseConfirm`으로의 외부 호출을 허용합니다.

### 패킷 중계

패킷들은 채널에 의해 허용됩니다 (오직 채널을 가지고 있는 포트만 송수신 가능).

[ICS 4](../ics-004-channel-and-packet-semantics)에 정의된대로 핸들러 인터페이스는 `sendPacket`, `recvPacket`, `acknowledgePacket`, `timeoutPacket`, `timeoutOnClose`와 `cleanupPacket`을 노출합니다.

기본 IBC 라우팅 모듈 SHALL은 `sendPacket`, `recvPacket`, `acknowledgePacket`, `timeoutPacket`, `timeoutOnClose`와 `cleanupPacket`으로의 외부 호출을 허용합니다.

### 속성 및 불변량

여기에 정의된 IBC 핸들러 모듈 인터페이스는 연관된 스펙에 정의된 함수의 속성들을 상속받습니다.

## 하위 호환성

적용되지 않습니다.

## 상위 호환성

이 인터페이스는 시맨틱이 동일하게 유지되는 한 새 체인에서 구현되거나 기존 체인으로 업그레이드될 때 변경될 수 있습니다.

## 구현 예제

곧 게시될 예정입니다.

## 다른 구현

곧 게시될 예정입니다.

## 히스토리

2019년 6월 9일 - 초안 작성
2019년 8월 24일 - 개정, 정리

## 저작권

이 게시물의 모든 내용은 [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) 라이센스에 의해 보호받습니다.
