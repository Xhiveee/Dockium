package agent

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsDisabledError(t *testing.T) {
	disabled := status.Error(codes.FailedPrecondition, "агент отключён")
	if !isDisabledError(fmt.Errorf("панель отклонила подключение: %w", disabled)) {
		t.Fatal("wrapped FailedPrecondition must be detected")
	}
	if isDisabledError(status.Error(codes.FailedPrecondition, "other")) == false {
		t.Fatal("direct FailedPrecondition must be detected")
	}
	if isDisabledError(errors.New("boom")) {
		t.Fatal("plain error must not be detected")
	}
	if isDisabledError(status.Error(codes.Unavailable, "down")) {
		t.Fatal("unavailable must not be detected")
	}
	if isDisabledError(nil) {
		t.Fatal("nil must not be detected")
	}
}
