Implementation of the IBC protocol can naturally be split up, between companies, teams, or other organisational units, into several discrete components which have well-defined interface points.

1. Core protocol

    The core protocol, consisting of ports, clients, connections, channels, and associated host requirements, is tightly coupled and should be implemented by a single entity. Given a reasonable state machine framework, this should require no more than a few thousand lines of code, excluding test-cases.

2. Light clients

    Light clients for different consensus algorithms have a well-defined interface point to the core protocol (defined in ICS 2). Separate light clients for different consensus algorithms can easily be implemented by different entities, then hooked into core protocol implementations once complete.

3. Application-layer protocols

    Application-layer protocols such as ICS 20 (fungible token transfer) have a well-defined interface to the core protocol (defined in ICS 25/26), which they can use to send packets, receive packets, handle timeouts, conduct handshakes, etc. Thus these protocols can easily be implemented by different entities, although it is probably easiest to do so once a core protocol implementation already exists that they can utilise & test with.
  
4. Relayer process

    Relayer processes have a well-defined interface point to the IBC protocol (outlined in ICS 18) and do not even need to be in the same language as the core protocol or light client implementations with which they communicate (though they need to use mutually intelligible data structure encoding formats). Relayer processes can easily be implemented by different entities, although again, it is probably easiest to do so once a core protocol implementation (or several) exists which they can test with.
