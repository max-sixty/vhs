package main

import (
	"testing"

	"github.com/go-rod/rod/lib/input"
)

// checkKeyStrokeEvents checks if the key stroke events are as expected.
func checkKeyStrokeEvents(t *testing.T, events *KeyStrokeEvents, expected ...string) {
	for i, event := range events.events {
		if actual := event.Display; expected[i] != actual {
			t.Fatalf("expected event display %q, got %q", expected, actual)
		}
	}
}

func defaultKeyStrokeEvents() *KeyStrokeEvents {
	events := NewKeyStrokeEvents(DefaultMaxDisplaySize)
	events.Enable()

	return events
}

func TestKeyStrokeEventsRemembersKeyStrokes(t *testing.T) {
	events := defaultKeyStrokeEvents()
	events.Push("a")
	events.Push("b")
	events.Push("c")
	// Single-char keystrokes get trailing alignment space
	checkKeyStrokeEvents(t, events, "a ", "a b ", "a b c ")
}

func TestKeyStrokeEventsHonorsMaxDisplaySize(t *testing.T) {
	events := defaultKeyStrokeEvents()
	events.maxDisplaySize = 4 // Increased to account for alignment spaces

	events.Push("a")
	events.Push("b")
	events.Push("c")

	// NOTE: Ring buffer removes one rune at a time when over limit.
	// Single-char keystrokes get trailing alignment space.
	// "a b c " (6 chars) → " b c " (5 chars after trimming 'a')
	checkKeyStrokeEvents(t, events, "a ", "a b ", " b c ")
}

func TestKeyStrokeEventsCompoundAlignment(t *testing.T) {
	events := defaultKeyStrokeEvents()

	// Simulate: 3, ⌃d, ⌃d, q sequence
	events.Push("3")
	events.Push("⌃d") // 2-char compound keystroke
	events.Push("⌃d") // Another compound - should be adjacent, no space
	events.Push("q")

	// Single-char gets trailing space; compounds are adjacent with no space
	// Result: "3 ⌃d⌃d q " - compounds adjacent, single chars have alignment space
	checkKeyStrokeEvents(t, events, "3 ", "3 ⌃d", "3 ⌃d⌃d", "3 ⌃d⌃d q ")
}

func TestKeyStrokeEventsShowsNothingIfDisabled(t *testing.T) {
	events := defaultKeyStrokeEvents()
	events.Disable()

	events.Push("a")
	events.Push("b")
	events.Push("c")

	checkKeyStrokeEvents(t, events)
}

func TestKeyStrokeEventsKeyToDisplay(t *testing.T) {
	cases := []struct {
		name     string
		key      input.Key
		expected string
	}{
		{
			name:     "letter",
			key:      input.KeyA,
			expected: "a",
		},
		{
			name:     "number",
			key:      input.Digit1,
			expected: "1",
		},
		{
			name:     "symbol no override",
			key:      input.Minus,
			expected: "-",
		},
		{
			name:     "shifted key",
			key:      shift(input.Minus),
			expected: "_",
		},
		{
			name:     "symbol override",
			key:      input.Backspace,
			expected: "⌫",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if actual := keyToDisplay(tc.key); tc.expected != actual {
				t.Fatalf("expected display %q, got %q", tc.expected, actual)
			}
		})
	}
}
