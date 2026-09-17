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
	// WhenMS is where the key press sits in the recorded video, in
	// milliseconds from its first frame.
	WhenMS int64
}

// KeyStrokeEvents is a collection of key press events that you can push to.
type KeyStrokeEvents struct {
	enabled bool
	display string
	events  []KeyStrokeEvent
	// videoMS reports how many milliseconds of video have been recorded so
	// far. Keystroke times come from it rather than the wall clock, so each
	// event is timed by where it lands in the finished video: a stretch the
	// tape hid records no frames and so advances it by nothing, and a tape
	// that types before its Show needs no correction afterwards.
	videoMS        func() int64
	duration       time.Duration
	maxDisplaySize int
}

const (
	// DefaultMaxDisplaySize is the default maximum display size for the
	// keystroke overlay.
	DefaultMaxDisplaySize = 20
)

// NewKeyStrokeEvents creates a new KeyStrokeEvents struct.
func NewKeyStrokeEvents(maxDisplaySize int, videoMS func() int64) *KeyStrokeEvents {
	return &KeyStrokeEvents{
		display:        "",
		events:         make([]KeyStrokeEvent, 0),
		videoMS:        videoMS,
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
func (k *KeyStrokeEvents) Enable() {
	k.enabled = true
	k.display = "" // Clear any accumulated display
}

// Disable disables key press event recording.
func (k *KeyStrokeEvents) Disable() {
	k.enabled = false
}

// End signals to the KeyStrokeEvents that the recording has finished.
// This _seems_ small, but it is crucial to ensure that a final key stroke event
// is not lost due to the recording finishing 1 frame too early.
func (k *KeyStrokeEvents) End() {
	k.duration = time.Duration(k.videoMS()) * time.Millisecond
}

// Push adds a new key press event to the collection.
func (k *KeyStrokeEvents) Push(display string) {
	// If we're not enabled, we don't want to do anything.
	if !k.enabled {
		return
	}

	// Always space-separate keystrokes for proper parsing in ffmpeg.go
	// The visual joining (compounds without leading space) happens at render time
	if k.display != "" {
		k.display += " "
	}
	k.display += display
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
	event := KeyStrokeEvent{Display: k.display, WhenMS: k.videoMS()}
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
func NewPage(page *rod.Page, videoMS func() int64) *Page {
	keyStrokeEvents := NewKeyStrokeEvents(DefaultMaxDisplaySize, videoMS)
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

// KeyActions is a wrapper around the rod.KeyActions method.
type KeyActions struct {
	*rod.KeyActions
	displays        []string
	KeyStrokeEvents *KeyStrokeEvents
	// heldModifiers is the symbols of the modifiers pressed so far in this
	// chain, which rod holds down until the chain is executed. They prefix the
	// keys they modify rather than being recorded as keystrokes of their own,
	// so that the overlay highlights one compound keystroke (⌥8) instead of
	// greying the modifier into the history and lighting up the key alone.
	heldModifiers string
}

// Press is a wrapper around the rod.KeyActions#Press method.
func (k *KeyActions) Press(key input.Key) *KeyActions {
	display := keyToDisplay(key)
	if isModifierKey(key) {
		k.heldModifiers += display
	} else {
		k.displays = append(k.displays, k.compound(display))
	}

	k.KeyActions.Press(key)
	return k
}

// Type is a wrapper around the rod.KeyActions#Type method.
func (k *KeyActions) Type(key input.Key) *KeyActions {
	k.displays = append(k.displays, k.compound(keyToDisplay(key)))
	k.KeyActions.Type(key)
	return k
}

// compound prefixes a key's display with the modifiers held over it, and
// lowercases the key so that the modifier is what stands out: ⌃d, not ⌃D.
func (k *KeyActions) compound(display string) string {
	if k.heldModifiers == "" {
		return display
	}
	return k.heldModifiers + strings.ToLower(display)
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
