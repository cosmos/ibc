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

이 문서는 표준 IBC 구현체('IBC 핸들러'라고 함)가 동일한 상태 머신(state machine)에 존재하는 모듈들을 노출하는 인터페이스와 해당 인터페이스의 구현에 대해 설명합니다.

### 의도

IBC는 모듈 간 통신 프로토콜로 다른 블록체인의 모듈과 안정적이고 검증된 형태로 메시지를 상호 전달할 수 있게 설계되었습니다. 모듈은 인터페이스를 안전하게 사용하기 위해 상호 작용하는 인터페이스와 준수해야하는 요구 사항에 대해 추론 할 수 있어야합니다.

### 정의

관련된 정의들은 (함수들이 정의되어 있는) 이전 표준들에 정의되어 있습니다.

### 지향 속성

- 클라이언트, 커넥션 및 채널들의 생성은 최대한 무허가적으로 가능해야 합니다.
- 모듈 세트는 동적이어야 합니다: 체인은 모듈을 추가하거나 및 제거할 수 있어야 하며, 모듈은 지속되는 IBC 핸들러를 사용해 포트에 바인딩하거나 포트에서 바인딩을 해제할 수 있어야 합니다.
- 모듈은 추가적인 시맨틱(semantic) 혹은 보장(guarantee)을 제공하기 위해 IBC 상위 계층에서 자체적으로 더 복잡한 추상화를 개발할 수 있어야 합니다.

## 기술 사양

> 참고: 호스트 상태머신이 obejct capability 인증([ICS 005](../ics-005-port-allocation) 참고)을 사용하는 경우, 포트를 사용하는 모든 함수는 추가 capability 키 매개변수를 사용합니다.

### 클라이언트 생명주기 관리

기본적으로 클라이언트는 소유되지 않습니다. 어떤 모듈이던지 새로운 클라이언트를 생성하고, 기존 클라이언트를 쿼리하고, 기존 클라이언트를 업데이트하고, 사용하지 않는 어떤 기존 클라이언트도 삭제할 수 있습니다.

핸들러 인터페이스는 [ICS 2](../ics-002-client-semantics)에 정의된 것과 같이 `createClient`, `updateClient`, `queryClientConsensusState`, `queryClient`와 `submitMisbehaviourToClient` 를 노출합니다.

### 커넥션 생명주기 관리

핸들러 인터페이스는 [ICS 3](../ics-003-connection-semantics)에 정의된 것과 같이 `connOpenInit`, `connOpenTry`, `connOpenAck`, `connOpenConfirm`과 `queryConnection`을 노출합니다 .

기본 IBC 라우팅 모듈은 `connOpenTry`, `connOpenAck`와 `connOpenConfirm`으로의 외부 호출을 허용해야 합니다(['Shall' allow](https://tools.ietf.org/html/rfc2119)).

### 채널 생명주기 관리

기본적으로 채널은 채널을 생성한 포트가 소유하게 됩니다. 즉, 해당 포트에 바인딩된 모듈만이 채널을 검사, 종료 또는 전송을 할 수 있습니다. 모듈은 동일한 포트를 사용하여 여러 채널을 생성할 수 있습니다.

 핸들러 인터페이스는 [ICS 4](../ics-004-channel-and-packet-semantics)에 정의된 것과 같이 `chanOpenInit`, `chanOpenTry`, `chanOpenAck`, `chanOpenConfirm`, `chanCloseInit`, `chanCloseConfirm`과 `queryChannel`을 노출합니다.

기본 IBC 라우팅 모듈은 `chanOpenTry`, `chanOpenAck`, `chanOpenConfirm`과 `chanCloseConfirm`으로의 외부 호출을 허용해야 합니다(['Shall' allow](https://tools.ietf.org/html/rfc2119)).

### 패킷 중계

패킷은 채널에 의해 허용됩니다 (오직 채널을 소유한 포트만이 송수신 가능).

 핸들러 인터페이스는 [ICS 4](../ics-004-channel-and-packet-semantics)에 정의된 것과 같이 `sendPacket`, `recvPacket`, `acknowledgePacket`, `timeoutPacket`, `timeoutOnClose`와 `cleanupPacket`을 노출합니다.

기본 IBC 라우팅 모듈은 `sendPacket`, `recvPacket`, `acknowledgePacket`, `timeoutPacket`, `timeoutOnClose`와 `cleanupPacket`으로의 외부 호출을 허용해야 합니다(['Shall' allow](https://tools.ietf.org/html/rfc2119)).

### 속성 및 불변량

이 문서에 정의된 IBC 핸들러 모듈 인터페이스는 연관된 스펙에 정의된 함수의 속성들을 상속받습니다.

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
