package util

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRetrier(t *testing.T) {
	r := &Retrier{
		MaxTries:            3,
		ShouldRetry:         nil,
		InitialInterval:     time.Millisecond * 10,
		MaxInterval:         time.Second * 60,
		Multiplier:          2.0,
		MaxElapsedTime:      0,
		RandomizationFactor: 0,
	}
	bg := context.Background()

	i := 0
	err := r.Retry(bg, func() error {
		i++
		return fmt.Errorf("always error")
	})

	if err == nil {
		t.Error("Retry did not report the error.")
	} else if i != 3 {
		t.Error("unexpected number of retries", i)
	}

	next := r.backoff.NextBackOff()
	if next != time.Millisecond*40 {
		t.Error("unexpected next backoff", next)
	}

	err = r.Retry(bg, func() error {
		return nil
	})
	if err != nil {
		t.Error("Retry reported unexpected error", err)
	}

	next = r.backoff.NextBackOff()
	if next != time.Millisecond*10 {
		t.Error("unexpected next backoff", next)
	}
}
