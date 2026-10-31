//go:build windows

package agent

import (
	"context"
	"errors"
	"net"

	"github.com/Microsoft/go-winio"
)

func dockerDial(ctx context.Context, network, address string, dialer *net.Dialer) (net.Conn, error) {
	switch network {
	case "npipe":
		if address != "" && address[0] != '\\' {
			address = `\\` + address
		}
		return winio.DialPipeContext(ctx, address)
	case "tcp":
		return dialer.DialContext(ctx, "tcp", address)
	default:
		return nil, errors.New("платформа не поддерживает этот тип подключения к Docker")
	}
}
