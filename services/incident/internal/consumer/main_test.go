package consumer_test

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain ловит утечки горутин консьюмера (T3): обработка сообщений и
// graceful-drain не должны оставлять висящих горутин после завершения теста.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
