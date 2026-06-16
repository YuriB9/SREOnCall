package consumer_test

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain ловит утечки горутин консьюмера эскалации (T3).
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
