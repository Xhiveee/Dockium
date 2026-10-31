package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

type attachWriter struct {
	connection net.Conn
	closeOnce  sync.Once
}

func (w *attachWriter) Write(data []byte) (int, error) {
	return w.connection.Write(data)
}

func (w *attachWriter) Close() error {
	var err error
	w.closeOnce.Do(func() {
		err = w.connection.Close()
	})
	return err
}

func (d *Docker) AttachInput(ctx context.Context, container string) (io.WriteCloser, error) {
	network, address, err := dockerSocket()
	if err != nil {
		return nil, err
	}
	dialCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	connection, err := dockerDial(dialCtx, network, address, dialer)
	if err != nil {
		return nil, fmt.Errorf("не удалось подключиться к Docker: %w", err)
	}
	endpoint := fmt.Sprintf("http://docker/v1.44/containers/%s/attach?stream=1&stdin=1&stdout=1&stderr=1", url.PathEscape(container))
	request, err := http.NewRequestWithContext(dialCtx, http.MethodPost, endpoint, nil)
	if err != nil {
		connection.Close()
		return nil, err
	}
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "tcp")
	request.Header.Set("Host", "docker")
	request.ContentLength = 0
	if err := request.Write(connection); err != nil {
		connection.Close()
		return nil, fmt.Errorf("не удалось отправить запрос Docker: %w", err)
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("Docker не принял подключение: %w", err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		connection.Close()
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return nil, errors.New(message)
	}
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		connection.Close()
	}()
	return &attachWriter{connection: connection}, nil
}

func dockerSocket() (network, address string, err error) {
	host := strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	if host != "" {
		switch {
		case strings.HasPrefix(host, "unix://"):
			return "unix", strings.TrimPrefix(host, "unix://"), nil
		case strings.HasPrefix(host, "npipe://"):
			return "npipe", strings.TrimPrefix(host, "npipe://"), nil
		case strings.HasPrefix(host, "tcp://") || strings.HasPrefix(host, "http://"):
			parsed, parseErr := url.Parse(host)
			if parseErr != nil || parsed.Host == "" {
				return "", "", errors.New("некорректный DOCKER_HOST")
			}
			return "tcp", parsed.Host, nil
		default:
			return "", "", errors.New("DOCKER_HOST: поддерживаются unix://, npipe:// и tcp://")
		}
	}
	if runtime.GOOS == "windows" {
		return "npipe", `\\.\pipe\docker_engine`, nil
	}
	return "unix", "/var/run/docker.sock", nil
}
