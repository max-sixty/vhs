package main

import (
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
)

// KeyStrokeEvent represents a key press event for the purposes of keystroke
// overlay.
type KeyStrokeEvent struct {
	// Display generally includes the current key stroke sequence.
	Display string
	// WhenMS is the time in milliseconds when the key was pressed starting
	// from the beginning of the recording.
	WhenMS int64
}

// KeyStrokeEvents is a collection of key press events that you can push to.
type KeyStrokeEvents struct {
	enabled        bool
	display        string
	events         []KeyStrokeEvent
	startTimeSet   bool
	startTime      time.Time
	duration       time.Duration
	maxDisplaySize int
}

const (
	// DefaultMaxDisplaySize is the default maximum display size for the
	// keystroke overlay.
	DefaultMaxDisplaySize = 20
)

// NewKeyStrokeEvents creates a new KeyStrokeEvents struct.
func NewKeyStrokeEvents(maxDisplaySize int) *KeyStrokeEvents {
	return &KeyStrokeEvents{
		display: "",
		events:  make([]KeyStrokeEvent, 0),
		// NOTE: This is actually setting the startTime too early. It
		// takes a while (in computer time) to get to the point where
		// we start recording. Therefore, we actually set this another
		// time on the first push. Without this, the final overlay
		// would be slightly desynced by a 20-40 ms, which is
		// noticeable to the human eye.
		startTime:      time.Now(),
		maxDisplaySize: maxDisplaySize,
	}
}

// keystrokeSymbolOverrides maps certain input keys to their corresponding
// keystroke string or symbol. These override the default rune for the
// corresponding input key to improve the visuals or readability of the
// keystroke overlay. A good example of this improvement can be seen in things
// like Enter (newline). The description string and symbol are embedded into an
// inner map, which can be indexed into based on whether special symbols are
// requested or not.
var keystrokeSymbolOverrides = map[input.Key]string{
	input.Backspace:    "⌫",
	input.Delete:       "⌦",
	input.ControlLeft:  "⌃",
	input.ControlRight: "⌃",
	input.AltLeft:      "⌥",
	input.AltRight:     "⌥",
	input.ShiftLeft:    "⇧",
	input.ShiftRight:   "⇧",
	input.ArrowDown:    "↓",
	input.PageDown:     "⇟",
	input.ArrowUp:      "↑",
	input.PageUp:       "⇞",
	input.ArrowLeft:    "←",
	input.ArrowRight:   "→",
	input.Space:        "·",
	input.Enter:        "⏎",
	input.Escape:       "⎋",
	input.Tab:          "⇥",
}

func keyToDisplay(key input.Key) string {
	if symbol, ok := keystrokeSymbolOverrides[key]; ok {
		return symbol
	}
	return string(inverseKeymap[key])
}

func isModifierKey(key input.Key) bool {
	return key == input.ControlLeft || key == input.ControlRight ||
		key == input.AltLeft || key == input.AltRight ||
		key == input.ShiftLeft || key == input.ShiftRight
}

// Enable enables key press event recording.
// Also resets the timing so keystrokes sync with video when used after Hide/Show.
func (k *KeyStrokeEvents) Enable() {
	k.enabled = true
	k.startTimeSet = false  // Reset so next Push sets the start time
	k.display = ""          // Clear any accumulated display
}

// Disable disables key press event recording.
func (k *KeyStrokeEvents) Disable() {
	k.enabled = false
}

// End signals to the KeyStrokeEvents that the recording has finished.
// This _seems_ small, but it is crucial to ensure that a final key stroke event
// is not lost due to the recording finishing 1 frame too early.
func (k *KeyStrokeEvents) End() {
	k.duration = time.Now().Sub(k.startTime)
}

// Push adds a new key press event to the collection.
func (k *KeyStrokeEvents) Push(display string) {
	// If we're not enabled, we don't want to do anything.
	if !k.enabled {
		return
	}

	// Set start time on first enabled push (reset when Enable() is called)
	if !k.startTimeSet {
		k.startTime = time.Now()
		k.startTimeSet = true
	}

	// Add space for visual alignment:
	// - Single-char keystrokes get a trailing space (to match 2-char compounds like ⌃d)
	// - 2-char compound keystrokes get NO space (before or after) - they appear adjacent
	// Result: "3 ⌃d⌃d⌃d s " - compounds are adjacent, single chars have alignment space
	if k.display != "" && len([]rune(display)) == 1 {
		displayRunes := []rune(k.display)
		// Only add separator before single-char if previous was 2+ chars (didn't end with space)
		if displayRunes[len(displayRunes)-1] != ' ' {
			k.display += " "
		}
	}
	k.display += display
	// Add trailing alignment space for single-char keystrokes only
	if len([]rune(display)) == 1 {
		k.display += " "
	}
	// Keep k.display @ 20 max.
	// Anymore than that is probably overkill, and we don't want to run into
	// issues where the overlay text is longer than the video width itself.
	if displayRunes := []rune(k.display); len(displayRunes) > k.maxDisplaySize {
		// We need to be cognizant of unicode -- we can't just slice off a byte,
		// we have to slice off a _rune_. The conversion back-and-forth may be a
		// bit inefficient, but k.display will always be tiny thanks to
		// k.maxDisplaySize.
		k.display = string(displayRunes[1:])
	}
	event := KeyStrokeEvent{Display: k.display, WhenMS: time.Now().Sub(k.startTime).Milliseconds()}
	k.events = append(k.events, event)
}

// Page is a wrapper around the rod.Page object.
// It's primary purpose is to decorate the rod.Page struct such that we can
// record keystroke events during the recording for keystroke overlays. We
// prefer decorating so that we that minimize the possibility of future bugs
// around forgetting to log key presses, since all input is done through
// rod.Page (and technically rod.Page.MustElement() + rod.Page.Keyboard).
type Page struct {
	*rod.Page
	Keyboard        Keyboard
	KeyStrokeEvents *KeyStrokeEvents
}

// NewPage creates a new wrapper Page object.
func NewPage(page *rod.Page) *Page {
	keyStrokeEvents := NewKeyStrokeEvents(DefaultMaxDisplaySize)
	return &Page{Page: page, KeyStrokeEvents: keyStrokeEvents, Keyboard: Keyboard{page.Keyboard, page.MustElement("textarea"), keyStrokeEvents}}
}

// MustSetViewport is a wrapper around the rod.Page#MustSetViewport method.
func (p *Page) MustSetViewport(width, height int, deviceScaleFactor float64, mobile bool) *Page {
	p.Page.MustSetViewport(width, height, deviceScaleFactor, mobile)
	return p
}

// MustWait is a wrapper around the rod.Page#MustWait method.
func (p *Page) MustWait(js string) *Page {
	p.Page.MustWait(js)
	return p
}

// KeyActions is a wrapper around the rod.Page#KeyActions method.
func (p *Page) KeyActions() *KeyActions {
	return &KeyActions{
		KeyActions:      p.Page.KeyActions(),
		displays:        []string{},
		KeyStrokeEvents: p.KeyStrokeEvents,
	}
}

// Keyboard is a wrapper around the rod.KeyActions method.
type KeyActions struct {
	*rod.KeyActions
	displays        []string
	KeyStrokeEvents *KeyStrokeEvents
	modifierActive  bool
	modifierSymbol  string
}

// Press is a wrapper around the rod.KeyActions#Press method.
func (k *KeyActions) Press(key input.Key) *KeyActions {
	display := keyToDisplay(key)
	modifierSymbol := ""

	// Track if this is a modifier key - don't add to displays yet, buffer it
	if isModifierKey(key) {
		modifierSymbol = display
	} else {
		k.displays = append(k.displays, display)
	}

	return &KeyActions{
		KeyActions:      k.KeyActions.Press(key),
		displays:        k.displays,
		KeyStrokeEvents: k.KeyStrokeEvents,
		modifierActive:  isModifierKey(key),
		modifierSymbol:  modifierSymbol,
	}
}

// Type is a wrapper around the rod.KeyActions#Type method.
func (k *KeyActions) Type(key input.Key) *KeyActions {
	display := keyToDisplay(key)

	// If a modifier is active, combine modifier symbol with the key (lowercase)
	if k.modifierActive && !isModifierKey(key) {
		display = k.modifierSymbol + strings.ToLower(display)
	}

	k.displays = append(k.displays, display)
	return &KeyActions{
		KeyActions:      k.KeyActions.Type(key),
		displays:        k.displays,
		KeyStrokeEvents: k.KeyStrokeEvents,
		modifierActive:  false,
		modifierSymbol:  "",
	}
}

// MustDo is a wrapper around the rod.KeyActions#MustDo method.
func (k *KeyActions) MustDo() {
	for _, display := range k.displays {
		k.KeyStrokeEvents.Push(display)
	}
	k.KeyActions.MustDo()
}

// Keyboard is a wrapper around the rod.Keyboard object.
type Keyboard struct {
	*rod.Keyboard
	textAreaElem    *rod.Element
	KeyStrokeEvents *KeyStrokeEvents
}

// Press is a wrapper around the rod.Keyboard#Press method.
func (k *Keyboard) Press(key input.Key) {
	k.KeyStrokeEvents.Push(keyToDisplay(key))
	k.Keyboard.Press(key)
}

// Type is a wrapper around the rod.Keyboard#Type method.
func (k *Keyboard) Type(key input.Key) {
	k.KeyStrokeEvents.Push(keyToDisplay(key))
	k.Keyboard.Type(key)
}

// Input is a wrapper around the rod.Page#MustElement("textarea")#Input method.
func (k *Keyboard) Input(text string) {
	k.KeyStrokeEvents.Push(text)
	k.textAreaElem.Input(text)
}
