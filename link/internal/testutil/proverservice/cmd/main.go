// SPDX-License-Identifier: Apache-2.0

// Command testprover runs a ProverService a relayer can be pointed at, serving
// the attestation prover from a relayer config so the wire contract can be
// exercised without a second light-client implementation.
//
// It exists for tests and demonstration. It is not part of the ibc CLI and is
// not built by the default build target.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cosmos/ibc/link/internal/testutil/proverservice"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "testprover:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "relayer config to build provers from")
	listen := flag.String("listen", "127.0.0.1:0", "address to serve on")
	flag.Parse()

	if *configPath == "" {
		return errors.New("--config is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server, err := proverservice.NewAttestationServer(ctx, *configPath)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}

	// The address is announced so a caller that asked for port 0 can find it.
	fmt.Println("listening", listener.Addr().String())

	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	select {
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return err
	case <-ctx.Done():
		return server.Close()
	}
}
