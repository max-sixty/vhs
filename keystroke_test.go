package main

import (
	"reflect"
	"testing"

	"github.com/go-rod/rod"
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
	events := NewKeyStrokeEvents(DefaultMaxDisplaySize, func() int64 { return 0 })
	events.Enable()

	return events
}

func TestKeyStrokeEventsRemembersKeyStrokes(t *testing.T) {
	events := defaultKeyStrokeEvents()
	events.Push("a")
	events.Push("b")
	events.Push("c")
	// Space BEFORE single-char keystrokes (no trailing space)
	checkKeyStrokeEvents(t, events, "a", "a b", "a b c")
}

func TestKeyStrokeEventsHonorsMaxDisplaySize(t *testing.T) {
	events := defaultKeyStrokeEvents()
	events.maxDisplaySize = 4

	events.Push("a")
	events.Push("b")
	events.Push("c")

	// NOTE: Ring buffer removes one rune at a time when over limit.
	// "a b c" (5 chars) > 4, trim to " b c" (4 chars)
	checkKeyStrokeEvents(t, events, "a", "a b", " b c")
}

func TestKeyStrokeEventsCompoundAlignment(t *testing.T) {
	events := defaultKeyStrokeEvents()

	// Simulate: 3, ⌃d, ⌃d, q sequence
	events.Push("3")
	events.Push("⌃d") // 2-char compound keystroke
	events.Push("⌃d") // Another compound
	events.Push("q")

	// All keystrokes space-separated for parsing; visual joining happens at render
	checkKeyStrokeEvents(t, events, "3", "3 ⌃d", "3 ⌃d ⌃d", "3 ⌃d ⌃d q")
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

// TestKeyActionsRecordsAModifiedKeyAsOneKeystroke pins the display a key
// combination contributes to the overlay. A modifier recorded on its own would
// be greyed into the history while the key it modifies was highlighted as the
// newest keystroke, so ⌥8 would read as an unrelated ⌥ followed by an 8.
func TestKeyActionsRecordsAModifiedKeyAsOneKeystroke(t *testing.T) {
	cases := []struct {
		name     string
		press    func(*KeyActions)
		expected []string
	}{
		{
			name:     "alt digit",
			press:    func(k *KeyActions) { k.Press(input.AltLeft).Type(input.Digit8) },
			expected: []string{"⌥8"},
		},
		{
			name:     "ctrl letter",
			press:    func(k *KeyActions) { k.Press(input.ControlLeft).Type(input.KeyD) },
			expected: []string{"⌃d"},
		},
		{
			name: "two modifiers",
			press: func(k *KeyActions) {
				k.Press(input.ControlLeft).Press(input.ShiftLeft).Type(input.KeyD)
			},
			expected: []string{"⌃⇧d"},
		},
		{
			name:     "no modifier",
			press:    func(k *KeyActions) { k.Type(input.KeyA) },
			expected: []string{"a"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// A zero Page is enough: rod's KeyActions only records the actions
			// until they are executed, which needs a browser and a tape.
			actions := &KeyActions{
				KeyActions:      (&rod.Page{}).KeyActions(),
				KeyStrokeEvents: defaultKeyStrokeEvents(),
			}
			tc.press(actions)
			if !reflect.DeepEqual(actions.displays, tc.expected) {
				t.Fatalf("expected displays %q, got %q", tc.expected, actions.displays)
			}
		})
	}
}
