//go:build !windows

package agent

import (
	"context"
	"errors"
	"net"
)

func dockerDial(ctx context.Context, network, address string, dialer *net.Dialer) (net.Conn, error) {
	switch network {
	case "unix", "tcp":
		return dialer.DialContext(ctx, network, address)
	default:
		return nil, errors.New("платформа не поддерживает этот тип подключения к Docker")
	}
}
