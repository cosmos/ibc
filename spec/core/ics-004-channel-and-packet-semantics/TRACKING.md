# Overview

This is draft packet correlation algorithm.

## Algorithm 

```go
package main

import "fmt"

type Height interface {
}

// is used to track packet
type PacketTrackingRecord struct {
}

type TransferPacketTrackingRecord struct {
}

func on_send_packet_event(data: bytes, timeoutHeight : Height, timeout) {
}

func main() {
	fmt.Println("Hello, 世界")
}
```
