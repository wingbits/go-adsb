package beast

import (
	"errors"
	"fmt"
	"testing"
)

func TestBeastErrorWithoutWrapped(t *testing.T) {
	e := newError(nil, "something failed")
	if e.Error() != "something failed" {
		t.Errorf("got %q, want %q", e.Error(), "something failed")
	}
	if e.Unwrap() != nil {
		t.Errorf("got %v, want nil", e.Unwrap())
	}
}

func TestBeastErrorWithWrapped(t *testing.T) {
	inner := fmt.Errorf("inner")
	e := newError(inner, "outer")
	if e.Error() != "outer: inner" {
		t.Errorf("got %q, want %q", e.Error(), "outer: inner")
	}
	if e.Unwrap() != inner {
		t.Errorf("got %v, want %v", e.Unwrap(), inner)
	}
}

func TestBeastErrorUnwrapChain(t *testing.T) {
	inner := fmt.Errorf("root cause")
	e := newError(inner, "wrapper")
	if !errors.Is(e, inner) {
		t.Error("errors.Is failed to find wrapped error")
	}
}

func TestNewErrorf(t *testing.T) {
	e := newErrorf(nil, "got %d items", 42)
	if e.Error() != "got 42 items" {
		t.Errorf("got %q, want %q", e.Error(), "got 42 items")
	}
}

func TestNewErrorfWithWrapped(t *testing.T) {
	inner := fmt.Errorf("root")
	e := newErrorf(inner, "failed at step %d", 3)
	if e.Error() != "failed at step 3: root" {
		t.Errorf("got %q, want %q", e.Error(), "failed at step 3: root")
	}
	if !errors.Is(e, inner) {
		t.Error("errors.Is failed to find wrapped error")
	}
}

func TestErrNoData(t *testing.T) {
	if ErrNoData.Error() != "data not available" {
		t.Errorf("got %q, want %q", ErrNoData.Error(), "data not available")
	}
	if !errors.Is(ErrNoData, errNoData) {
		t.Error("ErrNoData should match errNoData")
	}
}
