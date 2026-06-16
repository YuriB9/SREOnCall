package amqp

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain проверяет, что тесты пакета не оставляют висящих горутин (T3):
// supervisor-петля Consume и reconnect Connection должны завершаться по ctx.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
