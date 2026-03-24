package http

import (
	"context"
	"strings"
	"testing"
)

func TestInputTypeFromContext_PanicsWithMessage(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "inputAuthMiddleware not applied") {
			t.Errorf("unexpected panic value: %v", r)
		}
	}()
	inputTypeFromContext(context.Background())
}
