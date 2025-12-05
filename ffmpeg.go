package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FilterComplexBuilder generates -filter_complex option of ffmepg.
type FilterComplexBuilder struct {
	filterComplex *strings.Builder
	style         *StyleOptions
	termWidth     int
	termHeight    int
	prevStageName string
}

// NewVideoFilterBuilder returns instance of FilterComplexBuilder with video config.
func NewVideoFilterBuilder(videoOpts *VideoOptions) *FilterComplexBuilder {
	filterCode := strings.Builder{}
	termWidth, termHeight := calcTermDimensions(*videoOpts.Style)

	filterCode.WriteString(
		fmt.Sprintf(`
		[0][1]overlay[merged];
		[merged]scale=%d:%d:force_original_aspect_ratio=1[scaled];
		[scaled]fps=%d,setpts=PTS/%f[speed];
		[speed]pad=%d:%d:(ow-iw)/2:(oh-ih)/2:%s[padded];
		[padded]fillborders=left=%d:right=%d:top=%d:bottom=%d:mode=fixed:color=%s[padded]
		`,
			termWidth-double(videoOpts.Style.Padding),
			termHeight-double(videoOpts.Style.Padding),

			videoOpts.Framerate,
			videoOpts.PlaybackSpeed,

			termWidth,
			termHeight,
			videoOpts.Style.BackgroundColor,

			videoOpts.Style.Padding,
			videoOpts.Style.Padding,
			videoOpts.Style.Padding,
			videoOpts.Style.Padding,
			videoOpts.Style.BackgroundColor,
		),
	)

	return &FilterComplexBuilder{
		filterComplex: &filterCode,
		termHeight:    termHeight,
		termWidth:     termWidth,
		style:         videoOpts.Style,
		prevStageName: "padded",
	}
}

// NewScreenshotFilterComplexBuilder returns instance of FilterComplexBuilder with screenshot config.
func NewScreenshotFilterComplexBuilder(style *StyleOptions) *FilterComplexBuilder {
	filterCode := strings.Builder{}
	termWidth, termHeight := calcTermDimensions(*style)

	filterCode.WriteString(
		fmt.Sprintf(`
		[0][1]overlay[merged];
		[merged]scale=%d:%d:force_original_aspect_ratio=1[scaled];
		[scaled]pad=%d:%d:(ow-iw)/2:(oh-ih)/2:%s[padded];
		[padded]fillborders=left=%d:right=%d:top=%d:bottom=%d:mode=fixed:color=%s[padded]
		`,
			termWidth-double(style.Padding),
			termHeight-double(style.Padding),

			termWidth,
			termHeight,
			style.BackgroundColor,

			style.Padding,
			style.Padding,
			style.Padding,
			style.Padding,
			style.BackgroundColor,
		),
	)

	return &FilterComplexBuilder{
		filterComplex: &filterCode,
		termHeight:    termHeight,
		termWidth:     termWidth,
		style:         style,
		prevStageName: "padded",
	}
}

// calcTermDimensions computes terminal dimensions.
// It returns width and height values.
func calcTermDimensions(style StyleOptions) (int, int) {
	width := style.Width
	height := style.Height
	if style.MarginFill != "" {
		width = width - double(style.Margin)
		height = height - double(style.Margin)
	}
	if style.WindowBar != "" {
		height = height - style.WindowBarSize
	}

	return width, height
}

// WithWindowBar adds window bar options to ffmepg filter_complex.
func (fb *FilterComplexBuilder) WithWindowBar(barStream int) *FilterComplexBuilder {
	if fb.style.WindowBar != "" {
		fb.filterComplex.WriteString(";")
		fb.filterComplex.WriteString(
			fmt.Sprintf(`
			[%d]loop=-1[loopbar];
			[loopbar][%s]overlay=0:%d[withbar]
			`,
				barStream,
				fb.prevStageName,
				fb.style.WindowBarSize,
			),
		)

		fb.prevStageName = "withbar"
	}

	return fb
}

// WithBorderRadius adds border radius options to ffmepg filter_complex.
func (fb *FilterComplexBuilder) WithBorderRadius(cornerMarkStream int) *FilterComplexBuilder {
	if fb.style.BorderRadius != 0 {
		fb.filterComplex.WriteString(";")
		fb.filterComplex.WriteString(
			fmt.Sprintf(`
				[%d]loop=-1[loopmask];
				[%s][loopmask]alphamerge[rounded]
				`,
				cornerMarkStream,
				fb.prevStageName,
			),
		)
		fb.prevStageName = "rounded"
	}

	return fb
}

// WithMarginFill adds margin options to ffmepg filter_complex.
func (fb *FilterComplexBuilder) WithMarginFill(marginStream int) *FilterComplexBuilder {
	// Overlay terminal on margin
	if fb.style.MarginFill != "" {
		// ffmpeg will complain if the final filter ends with a semicolon,
		// so we add one BEFORE we start adding filters.
		fb.filterComplex.WriteString(";")
		fb.filterComplex.WriteString(
			fmt.Sprintf(`
			[%d]scale=%d:%d[bg];
			[bg][%s]overlay=(W-w)/2:(H-h)/2:shortest=1[withbg]
			`,
				marginStream,
				fb.style.Width,
				fb.style.Height,
				fb.prevStageName,
			),
		)
		fb.prevStageName = "withbg"
	}

	return fb
}

// Keystroke overlay constants
const (
	keystrokeFontFamily   = "Menlo"
	keystrokeFontSize     = 160
	keystrokeRingBuffer   = 6
	keystrokeDelayMS      = 500.0
	keystrokeBoxPadding   = 25
	keystrokeCharWidthPct = 0.6 // Approximate width/height ratio for Menlo monospace

	// ASS color format: &HAABBGGRR& (AA=alpha, 00=opaque)
	assColorBlack = "&H00000000&"
	assColorAmber = "&H00B0FF&" // Amber/gold for newest keystroke (inline override format: &HBBGGRR&)
)

// getKeystrokesForDisplay extracts the last N keystrokes from a space-separated display string.
func getKeystrokesForDisplay(display string, bufferSize int) []string {
	keystrokes := strings.Split(display, " ")
	if len(keystrokes) > bufferSize {
		return keystrokes[len(keystrokes)-bufferSize:]
	}
	return keystrokes
}

// smartJoinKeystrokes joins keystrokes with spaces only before single-char keystrokes.
// This gives right-aligned appearance: "3^d^d^d s" instead of "3 ^d ^d ^d s"
func smartJoinKeystrokes(keystrokes []string) string {
	var result strings.Builder
	for i, ks := range keystrokes {
		if i > 0 && len([]rune(ks)) == 1 {
			result.WriteString(" ")
		}
		result.WriteString(ks)
	}
	return result.String()
}

// formatASSTime formats a time in seconds to ASS timestamp format (H:MM:SS.cc)
func formatASSTime(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	centiseconds := int((seconds - float64(int(seconds))) * 100)
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, secs, centiseconds)
}

// generateASSContent creates ASS subtitle content for keystroke overlay with multi-colored text.
// History keystrokes are black, the newest keystroke is amber/gold.
// Note: This generates text-only subtitles. The background box is drawn separately via drawbox filter.
func generateASSContent(opts VideoOptions, termWidth, termHeight int) string {
	events := opts.KeyStrokeOverlay.Events
	var ass strings.Builder

	// ASS header
	ass.WriteString("[Script Info]\n")
	ass.WriteString("ScriptType: v4.00+\n")
	ass.WriteString(fmt.Sprintf("PlayResX: %d\n", termWidth))
	ass.WriteString(fmt.Sprintf("PlayResY: %d\n", termHeight))
	ass.WriteString("\n")

	// Style definition
	// Alignment 5 = center (middle center)
	// BorderStyle 1 = outline + drop shadow (we set both to 0 for clean text)
	// No background - we'll use drawbox filter for that
	ass.WriteString("[V4+ Styles]\n")
	ass.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	ass.WriteString(fmt.Sprintf("Style: Keystrokes,%s,%d,%s,%s,&H00000000&,&H00000000&,0,0,0,0,100,100,0,0,1,0,0,5,0,0,0,0\n",
		keystrokeFontFamily, keystrokeFontSize, assColorBlack, assColorBlack))
	ass.WriteString("\n")

	// Events
	ass.WriteString("[Events]\n")
	ass.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")

	for i := range events {
		event := events[i]

		// Apply delay to sync with terminal rendering
		startTimeS := (float64(event.WhenMS) + keystrokeDelayMS) / 1000

		// Calculate end time for this event (must match WithKeyStrokes box timing)
		var endTimeS float64
		if i < len(events)-1 {
			endTimeS = (float64(events[i+1].WhenMS) + keystrokeDelayMS) / 1000
		} else {
			// Last event: show for 10 seconds (arbitrary, recording usually ends before this)
			endTimeS = startTimeS + 10.0
		}

		keystrokes := getKeystrokesForDisplay(event.Display, keystrokeRingBuffer)

		// Build the ASS text with color override for the newest keystroke
		var assText string
		if len(keystrokes) == 1 {
			// Only one keystroke - show it in amber
			assText = fmt.Sprintf("{\\c%s}%s", assColorAmber, keystrokes[0])
		} else {
			// History in black (default), newest in amber
			historyKeystrokes := keystrokes[:len(keystrokes)-1]
			newPart := keystrokes[len(keystrokes)-1]
			historyPart := smartJoinKeystrokes(historyKeystrokes)

			// Add trailing space only if newPart is single-char
			if len([]rune(newPart)) == 1 {
				historyPart += " "
			}

			// History uses default color (black from style), newest gets amber override
			assText = fmt.Sprintf("%s{\\c%s}%s", historyPart, assColorAmber, newPart)
		}

		ass.WriteString(fmt.Sprintf("Dialogue: 0,%s,%s,Keystrokes,,0,0,0,,%s\n",
			formatASSTime(startTimeS),
			formatASSTime(endTimeS),
			assText))
	}

	return ass.String()
}

// WithKeyStrokes adds keystroke overlay using ASS subtitles for multi-colored text.
// History keystrokes appear in black, the newest keystroke appears in amber/gold.
// Uses drawbox for background + ASS for colored text.
func (fb *FilterComplexBuilder) WithKeyStrokes(opts VideoOptions) *FilterComplexBuilder {
	events := opts.KeyStrokeOverlay.Events
	charWidth := int(float64(keystrokeFontSize) * keystrokeCharWidthPct)

	// When we are dealing with the last event, things can actually get very
	// subtly tricky.
	// If the last keystroke event is _very_ close to the end of the recording
	// (e.g. the last line of the .tape is Type), then there is a chance that
	// this keystroke draw event gets effectively dropped. That is because the
	// gte() clause here may have a timestamp that is exactly equal to or
	// slightly greater than the actual length of the recording itself. In order
	// to fix this, we actually want to "pad" the recording a little extra in
	// this case.  While we could simply unconditionally record a few more
	// seconds at the end of every vhs recording, this is actually not a
	// generalizable solution for a dynamic typing speed, where we would
	// proportionally require _more_ extra frames to make sure to not drop any
	// keystrokes.
	// Now, the condition in which we need to do this corrective action is if
	// the last event is recorded after the duration of the recording with a
	// tolerance of 100 ms:
	if overflow := events[len(events)-1].WhenMS - opts.KeyStrokeOverlay.Duration.Milliseconds(); overflow > -100 && overflow < 100 {
		// If so, extend the recording by a window twice the length of the
		// typing speed. This should give ample time for the last keystroke
		// event to be properly rendered.
		fb.filterComplex.WriteString(fmt.Sprintf(";\n[%s]tpad=stop_mode=clone:stop_duration=%f[endpad]\n", fb.prevStageName, opts.KeyStrokeOverlay.TypingSpeed.Seconds()*2))
		fb.prevStageName = "endpad"
	}

	// First pass: draw background boxes for each keystroke event
	prevStageName := fb.prevStageName
	for i := range events {
		event := events[i]

		// Apply delay to sync with terminal rendering
		startTimeS := (float64(event.WhenMS) + keystrokeDelayMS) / 1000

		// Calculate end time for this event
		var endTimeS float64 = -1
		if i < len(events)-1 {
			endTimeS = (float64(events[i+1].WhenMS) + keystrokeDelayMS) / 1000
		}

		keystrokes := getKeystrokesForDisplay(event.Display, keystrokeRingBuffer)

		// Enable condition for ffmpeg filter
		enableCondition := fmt.Sprintf("gte(t,%f)", startTimeS)
		if endTimeS > 0 {
			enableCondition = fmt.Sprintf("between(t,%f,%f)", startTimeS, endTimeS)
		}

		// Calculate box dimensions
		fullText := smartJoinKeystrokes(keystrokes)
		fullTextWidth := len([]rune(fullText)) * charWidth
		startX := (fb.termWidth - fullTextWidth) / 2
		textY := (fb.termHeight - keystrokeFontSize) / 2

		boxX := startX - keystrokeBoxPadding
		boxY := textY - keystrokeBoxPadding
		boxW := fullTextWidth + 2*keystrokeBoxPadding
		boxH := keystrokeFontSize + 2*keystrokeBoxPadding

		// Draw semi-transparent white background box (70% opacity)
		fb.filterComplex.WriteString(";")
		boxStageName := fmt.Sprintf("keystrokeBox%d", i)
		fb.filterComplex.WriteString(
			fmt.Sprintf(`[%s]drawbox=x=%d:y=%d:w=%d:h=%d:color=white@0.7:t=fill:enable='%s'[%s]`,
				prevStageName,
				boxX,
				boxY,
				boxW,
				boxH,
				enableCondition,
				boxStageName,
			),
		)
		prevStageName = boxStageName
	}

	// Generate ASS subtitle file for the colored text
	assContent := generateASSContent(opts, fb.termWidth, fb.termHeight)
	assPath := filepath.Join(opts.Input, "keystrokes.ass")
	if err := os.WriteFile(assPath, []byte(assContent), 0644); err != nil {
		fmt.Println(ErrorStyle.Render("Unable to write ASS file, skipping subtitles: "), err)
		fb.prevStageName = prevStageName
		return fb
	}

	// Apply ASS subtitles on top of the boxes
	fb.filterComplex.WriteString(";")
	fb.filterComplex.WriteString(
		fmt.Sprintf(`[%s]ass=%s[withkeystrokes]`,
			prevStageName,
			assPath,
		),
	)
	fb.prevStageName = "withkeystrokes"

	return fb
}

// WithGIF adds gif options to ffmepg filter_complex.
func (fb *FilterComplexBuilder) WithGIF() *FilterComplexBuilder {
	fb.filterComplex.WriteString(";")
	fb.filterComplex.WriteString(
		fmt.Sprintf(`
			[%s]split[plt_a][plt_b];
			[plt_a]palettegen=max_colors=256[plt];
			[plt_b][plt]paletteuse[palette]`,
			fb.prevStageName,
		),
	)
	fb.prevStageName = "palette"

	return fb
}

// Build returns filter_complex used in ffmepg.
func (fb *FilterComplexBuilder) Build() []string {
	return []string{
		"-filter_complex", fb.filterComplex.String(),
		"-map", "[" + fb.prevStageName + "]",
	}
}

// StreamBuilder generates streams used by ffmepg.
type StreamBuilder struct {
	args         []string
	counter      int
	style        *StyleOptions
	termWidth    int
	termHeight   int
	input        string
	barStream    int
	cornerStream int
	marginStream int
}

// NewStreamBuilder returns instance of StreamBuilder.
func NewStreamBuilder(streamCounter int, input string, style *StyleOptions) *StreamBuilder {
	termWidth, termHeight := calcTermDimensions(*style)

	return &StreamBuilder{
		counter:    streamCounter,
		args:       []string{},
		style:      style,
		termWidth:  termWidth,
		termHeight: termHeight,
		input:      input,
	}
}

// WithMargin adds margin stream.
func (sb *StreamBuilder) WithMargin() *StreamBuilder {
	if sb.style.MarginFill != "" {
		if marginFillIsColor(sb.style.MarginFill) {
			// Create plain color stream
			sb.args = append(sb.args,
				"-f", "lavfi",
				"-i",
				fmt.Sprintf(
					"color=%s:s=%dx%d",
					sb.style.MarginFill,
					sb.style.Width,
					sb.style.Height,
				),
			)
		} else {
			// Check for existence first.
			_, err := os.Stat(sb.style.MarginFill)
			if err != nil {
				fmt.Println(ErrorStyle.Render("Unable to read margin file: "), sb.style.MarginFill)
			}

			// Add image stream
			sb.args = append(sb.args,
				"-loop", "1",
				"-i", sb.style.MarginFill,
			)
		}

		sb.marginStream = sb.counter
		sb.counter++
	}

	return sb
}

// WithBar adds bar stream.
func (sb *StreamBuilder) WithBar() *StreamBuilder {
	barPath := filepath.Join(sb.input, "bar.png")

	if sb.style.WindowBar != "" {
		MakeWindowBar(sb.termWidth, sb.termHeight, *sb.style, barPath)

		sb.args = append(sb.args,
			"-i", barPath,
		)

		sb.barStream = sb.counter
		sb.counter++
	}

	return sb
}

// WithCorner adds corner stream.
func (sb *StreamBuilder) WithCorner() *StreamBuilder {
	maskPath := filepath.Join(sb.input, "mask.png")

	if sb.style.BorderRadius != 0 {
		if sb.style.WindowBar != "" {
			MakeBorderRadiusMask(sb.termWidth, sb.termHeight+sb.style.WindowBarSize, sb.style.BorderRadius, maskPath)
		} else {
			MakeBorderRadiusMask(sb.termWidth, sb.termHeight, sb.style.BorderRadius, maskPath)
		}

		sb.args = append(sb.args,
			"-i", maskPath,
		)

		sb.cornerStream = sb.counter
		sb.counter++
	}

	return sb
}

// WithMP4 adds mp4 stream with required config.
func (sb *StreamBuilder) WithMP4() *StreamBuilder {
	sb.args = append(sb.args,
		"-vcodec", "libx264",
		"-pix_fmt", "yuv420p",
		"-an",
		"-crf", "20",
	)

	return sb
}

// WithWebm adds webm stream with required config.
func (sb *StreamBuilder) WithWebm() *StreamBuilder {
	sb.args = append(sb.args,
		"-pix_fmt", "yuv420p",
		"-an",
		"-crf", "30",
		"-b:v", "0",
	)
	return sb
}

// Build returns streams for using with ffmepg.
func (sb *StreamBuilder) Build() []string {
	return sb.args
}
