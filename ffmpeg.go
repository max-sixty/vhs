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

// WithKeyStrokes adds key stroke drawtext options to the ffmpeg filter_complex.
func (fb *FilterComplexBuilder) WithKeyStrokes(opts VideoOptions) *FilterComplexBuilder {
	var (
		defaultFontFamily = "Menlo"
		fontSize          = 160     // Very large font for maximum visibility
		ringBufferSize    = 6       // Show last 6 keystrokes (space-separated)
		keystrokeDelayMS  = 500.0   // Delay to sync with terminal rendering
		amberColor        = "#FFBF00"
	)
	events := opts.KeyStrokeOverlay.Events

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

		// Ring buffer logic: get the last N keystrokes (space-separated)
		fullDisplay := event.Display
		keystrokes := strings.Split(fullDisplay, " ")
		if len(keystrokes) > ringBufferSize {
			keystrokes = keystrokes[len(keystrokes)-ringBufferSize:]
		}

		// Split into history (all but last) and new (last keystroke)
		historyPart := ""
		newPart := ""
		if len(keystrokes) > 0 {
			newPart = keystrokes[len(keystrokes)-1]
			if len(keystrokes) > 1 {
				historyPart = strings.Join(keystrokes[:len(keystrokes)-1], " ") + " "
			}
		}

		// Enable condition
		enableCondition := fmt.Sprintf("gte(t,%f)", startTimeS)
		if endTimeS > 0 {
			enableCondition = fmt.Sprintf("between(t,%f,%f)", startTimeS, endTimeS)
		}

		// Draw each part only once to avoid color bleed-through
		fullText := strings.Join(keystrokes, " ")
		charWidth := fontSize * 6 / 10 // Approximate monospace char width
		boxPadding := 25

		// Calculate positions - center the full text
		fullTextWidth := len([]rune(fullText)) * charWidth
		startX := (fb.termWidth - fullTextWidth) / 2
		historyWidth := len([]rune(historyPart)) * charWidth

		// Calculate box dimensions (text height is roughly fontSize)
		boxX := startX - boxPadding
		boxY := (fb.termHeight - fontSize) / 2 - boxPadding
		boxW := fullTextWidth + 2*boxPadding
		boxH := fontSize + 2*boxPadding

		// Draw white background box
		fb.filterComplex.WriteString(";")
		boxStageName := fmt.Sprintf("keystrokeBox%d", i)
		fb.filterComplex.WriteString(
			fmt.Sprintf(`
			[%s]drawbox=x=%d:y=%d:w=%d:h=%d:color=white:t=fill:enable='%s'[%s]
			`,
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

		// Draw history part in black (if there is history)
		if historyPart != "" {
			fb.filterComplex.WriteString(";")
			historyStageName := fmt.Sprintf("keystrokeHist%d", i)
			fb.filterComplex.WriteString(
				fmt.Sprintf(`
				[%s]drawtext=font=%s:text='%s':fontcolor=black:fontsize=%d:x=%d:y='(h-text_h)/2':enable='%s'[%s]
				`,
					prevStageName,
					defaultFontFamily,
					historyPart,
					fontSize,
					startX,
					enableCondition,
					historyStageName,
				),
			)
			prevStageName = historyStageName
		}

		// Draw new keystroke in amber
		fb.filterComplex.WriteString(";")
		newStageName := fmt.Sprintf("keystrokeNew%d", i)
		newX := startX + historyWidth
		fb.filterComplex.WriteString(
			fmt.Sprintf(`
			[%s]drawtext=font=%s:text='%s':fontcolor=%s:fontsize=%d:x=%d:y='(h-text_h)/2':enable='%s'[%s]
			`,
				prevStageName,
				defaultFontFamily,
				newPart,
				amberColor,
				fontSize,
				newX,
				enableCondition,
				newStageName,
			),
		)
		prevStageName = newStageName
	}

	// At the end of the loop, the previous stage name is now transfered to the filter complex builder's
	// state for use in subsequent filters.
	fb.prevStageName = prevStageName
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
