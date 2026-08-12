package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"

	"linx/internal/cdp"
)

type Controller interface {
	State(context.Context) (cdp.AppState, error)
	OpenRoom(context.Context, string) error
	SendMessage(context.Context, string) error
	RefreshQR(context.Context) error
	Logout(context.Context) error
	QRCodePNG(context.Context) ([]byte, error)
}

type ControlUI struct {
	Controller Controller
	In         *os.File
	Out        *os.File
	Interval   time.Duration
}

type controlMode int

const (
	modeNormal controlMode = iota
	modeCompose
	modeConfirmLogout
)

type keyKind int

const (
	keyRune keyKind = iota
	keyUp
	keyDown
	keyEnter
	keyBackspace
	keyCancel
	keyQuit
)

type keyEvent struct {
	kind keyKind
	r    rune
}

type controlState struct {
	app        cdp.AppState
	selected   int
	mode       controlMode
	draft      string
	status     string
	err        error
	qr         []string
	updated    time.Time
	refreshing bool
}

func (u ControlUI) Run(ctx context.Context) error {
	if u.Controller == nil {
		return fmt.Errorf("control UI controller is required")
	}
	if u.In == nil {
		u.In = os.Stdin
	}
	if u.Out == nil {
		u.Out = os.Stdout
	}
	if u.Interval <= 0 {
		u.Interval = 2 * time.Second
	}

	fd := int(u.In.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("interactive control requires a terminal")
	}
	previous, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("enable raw terminal mode: %w", err)
	}
	defer term.Restore(fd, previous)

	fmt.Fprint(u.Out, "\x1b[?1049h\x1b[?25l\x1b[?7l")
	defer fmt.Fprint(u.Out, "\x1b[0m\x1b[?7h\x1b[?25h\x1b[?1049l")

	keys := make(chan keyEvent, 16)
	go readKeys(u.In, keys)

	state := controlState{status: "connecting…"}
	u.refresh(ctx, &state, true)
	u.render(state)

	ticker := time.NewTicker(u.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			u.refresh(ctx, &state, false)
			u.render(state)
		case key, ok := <-keys:
			if !ok {
				return nil
			}
			quit := u.handleKey(ctx, &state, key)
			if quit {
				return nil
			}
			u.render(state)
		}
	}
}

func (u ControlUI) refresh(ctx context.Context, state *controlState, forceQR bool) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	selectedID := ""
	if state.selected >= 0 && state.selected < len(state.app.Rooms) {
		selectedID = state.app.Rooms[state.selected].ID
	}
	app, err := u.Controller.State(callCtx)
	if err != nil {
		state.err = err
		state.status = "waiting for LINE"
		return
	}
	state.app = app
	state.err = nil
	switch {
	case app.LoggedIn && !app.ChatSelected:
		state.status = "opening LINE Chat tab…"
	case app.LoggedIn && !app.ChatReady:
		state.status = "waiting for chats…"
	case app.LoggedIn && len(app.Rooms) == 0:
		state.status = "no chats found"
	default:
		state.status = "connected"
	}
	state.updated = time.Now()
	u.syncSelection(state, selectedID)

	if app.Login.Required && app.Login.QRAvailable && (forceQR || len(state.qr) == 0) {
		pngData, err := u.Controller.QRCodePNG(callCtx)
		if err != nil {
			state.err = err
			return
		}
		width, _, _ := term.GetSize(int(u.Out.Fd()))
		if width <= 0 {
			width = 80
		}
		qrWidth := minInt(60, width-4)
		state.qr, err = RenderQRCode(pngData, qrWidth)
		if err != nil {
			state.err = err
		}
	}
	if !app.Login.Required {
		state.qr = nil
	}
}

func (u ControlUI) syncSelection(state *controlState, selectedID string) {
	if len(state.app.Rooms) == 0 {
		state.selected = 0
		return
	}
	if state.selected >= len(state.app.Rooms) {
		state.selected = len(state.app.Rooms) - 1
	}
	for index, room := range state.app.Rooms {
		if room.ID == selectedID {
			state.selected = index
			return
		}
	}
	for index, room := range state.app.Rooms {
		if room.Active {
			state.selected = index
			return
		}
	}
}

func (u ControlUI) handleKey(ctx context.Context, state *controlState, key keyEvent) bool {
	if key.kind == keyQuit {
		return true
	}

	if state.mode == modeCompose {
		switch key.kind {
		case keyCancel:
			state.mode = modeNormal
			state.draft = ""
			state.status = "message cancelled"
		case keyBackspace:
			state.draft = trimLastRune(state.draft)
		case keyEnter:
			if strings.TrimSpace(state.draft) == "" {
				return false
			}
			callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			err := u.Controller.SendMessage(callCtx, state.draft)
			cancel()
			if err != nil {
				state.err = err
				return false
			}
			state.draft = ""
			state.mode = modeNormal
			state.status = "message sent"
			u.refresh(ctx, state, false)
		case keyRune:
			if key.r >= 0x20 && key.r != 0x7f {
				state.draft += string(key.r)
			}
		}
		return false
	}

	if state.mode == modeConfirmLogout {
		switch {
		case key.kind == keyCancel:
			state.mode = modeNormal
			state.status = "logout cancelled"
		case key.kind == keyRune && (key.r == 'n' || key.r == 'N' || key.r == 'q'):
			state.mode = modeNormal
			state.status = "logout cancelled"
		case key.kind == keyRune && (key.r == 'y' || key.r == 'Y'):
			callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			err := u.Controller.Logout(callCtx)
			cancel()
			if err != nil {
				state.err = err
				state.mode = modeNormal
				return false
			}
			state.mode = modeNormal
			state.qr = nil
			state.status = "logged out"
			u.refresh(ctx, state, true)
		}
		return false
	}

	switch key.kind {
	case keyUp:
		if state.selected > 0 {
			state.selected--
		}
	case keyDown:
		if state.selected+1 < len(state.app.Rooms) {
			state.selected++
		}
	case keyEnter:
		if state.app.Login.Required || len(state.app.Rooms) == 0 {
			return false
		}
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := u.Controller.OpenRoom(callCtx, state.app.Rooms[state.selected].ID)
		cancel()
		if err != nil {
			state.err = err
			return false
		}
		state.status = "room opened"
		u.refresh(ctx, state, false)
	case keyRune:
		switch key.r {
		case 'j':
			if state.selected+1 < len(state.app.Rooms) {
				state.selected++
			}
		case 'k':
			if state.selected > 0 {
				state.selected--
			}
		case 'i':
			if !state.app.Login.Required && state.app.ActiveRoom != "" {
				state.mode = modeCompose
				state.draft = ""
				state.err = nil
			}
		case 'r':
			if state.app.Login.Required {
				callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := u.Controller.RefreshQR(callCtx)
				cancel()
				if err != nil && !strings.Contains(err.Error(), "not available") {
					state.err = err
				}
				state.qr = nil
			}
			u.refresh(ctx, state, true)
		case 'l':
			if state.app.LoggedIn {
				state.mode = modeConfirmLogout
				state.err = nil
				state.status = "confirm logout"
			}
		case 'q':
			return true
		}
	}
	return false
}

func (u ControlUI) render(state controlState) {
	width, height, _ := term.GetSize(int(u.Out.Fd()))
	if width < 60 {
		width = 80
	}
	if height < 15 {
		height = 24
	}

	var out strings.Builder
	out.WriteString("\x1b[2J\x1b[H")
	out.WriteString("LINX · LINE terminal control")
	if !state.updated.IsZero() {
		out.WriteString(" · ")
		out.WriteString(state.updated.Format("15:04:05"))
	}
	out.WriteByte('\n')
	out.WriteString(strings.Repeat("─", width))
	out.WriteByte('\n')

	if state.app.Login.Required {
		renderLogin(&out, state, width, height)
		fmt.Fprint(u.Out, rawTerminalText(out.String()))
		return
	}

	roomWidth := width / 3
	if roomWidth < 24 {
		roomWidth = 24
	}
	if roomWidth > 38 {
		roomWidth = 38
	}
	messageWidth := width - roomWidth - 3
	bodyRows := height - 6
	if bodyRows < 5 {
		bodyRows = 5
	}

	roomStart := maxInt(0, state.selected-bodyRows/2)
	if roomStart+bodyRows > len(state.app.Rooms) {
		roomStart = maxInt(0, len(state.app.Rooms)-bodyRows)
	}
	messages := state.app.Messages
	messageStart := maxInt(0, len(messages)-bodyRows)

	writeColumns(&out, "ROOMS", roomWidth, activeRoomName(state.app), messageWidth)
	out.WriteByte('\n')
	out.WriteString(strings.Repeat("─", roomWidth))
	out.WriteString("─┼─")
	out.WriteString(strings.Repeat("─", messageWidth))
	out.WriteByte('\n')

	for row := 0; row < bodyRows; row++ {
		roomText := ""
		roomIndex := roomStart + row
		if roomIndex < len(state.app.Rooms) {
			room := state.app.Rooms[roomIndex]
			prefix := "  "
			if roomIndex == state.selected {
				prefix = "> "
			}
			roomText = prefix + room.Name
			if room.Unread != "" {
				roomText += " (" + room.Unread + ")"
			}
		} else if len(state.app.Rooms) == 0 && row == 0 {
			switch {
			case !state.app.ChatSelected:
				roomText = "  Opening Chat tab…"
			case !state.app.ChatReady:
				roomText = "  Loading chats…"
			default:
				roomText = "  No chats"
			}
		}

		messageText := ""
		messageIndex := messageStart + row
		if messageIndex < len(messages) {
			message := messages[messageIndex]
			timePart := message.Time
			if len(timePart) >= 5 {
				timePart = timePart[len(timePart)-5:]
			}
			messageText = "[" + timePart + "] "
			if message.Sender != "" {
				messageText += message.Sender + ": "
			}
			messageText += message.Content
		}
		writeColumns(&out, roomText, roomWidth, messageText, messageWidth)
		out.WriteByte('\n')
	}

	out.WriteString(strings.Repeat("─", width))
	out.WriteByte('\n')
	if state.mode == modeCompose {
		out.WriteString(fit("Message: "+state.draft+"_", width))
		out.WriteByte('\n')
		out.WriteString(fit("[Enter] send  [Ctrl+G] cancel", width))
	} else if state.mode == modeConfirmLogout {
		out.WriteString(fit("Log out of LINE on this Chrome profile?", width))
		out.WriteByte('\n')
		out.WriteString(fit("[y] confirm  [n/Esc] cancel", width))
	} else {
		status := "[j/k] select  [Enter] open  [i] compose  [r] refresh  [l] logout  [q] quit"
		if state.err != nil {
			status = "error: " + state.err.Error()
		} else if state.status != "" {
			status += " · " + state.status
		}
		out.WriteString(fit(status, width))
	}
	fmt.Fprint(u.Out, rawTerminalText(out.String()))
}

func renderLogin(out *strings.Builder, state controlState, width, height int) {
	out.WriteString("LINE QR login\n")
	if state.app.Login.Description != "" {
		out.WriteString(fit(state.app.Login.Description, width))
		out.WriteByte('\n')
	}
	if state.app.Login.PIN != "" {
		out.WriteString("\nConfirm this PIN on your phone: ")
		out.WriteString(state.app.Login.PIN)
		out.WriteByte('\n')
	}
	if state.err != nil {
		out.WriteString("\nerror: ")
		out.WriteString(fit(state.err.Error(), width-7))
		out.WriteByte('\n')
	}
	if len(state.qr) == 0 {
		out.WriteString("\nWaiting for QR code…\n")
	} else if len(state.qr) > height-5 {
		out.WriteString("\nTerminal is too short for a scan-safe QR code.\n")
		out.WriteString(fmt.Sprintf("Resize it to at least %d rows; current height is %d.\n", len(state.qr)+5, height))
	} else {
		for _, line := range state.qr {
			padding := maxInt(0, (width-visibleWidth(line))/2)
			out.WriteString(strings.Repeat(" ", padding))
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	out.WriteString("\x1b[0m[r] refresh QR  [q] quit")
}

func readKeys(in *os.File, events chan<- keyEvent) {
	defer close(events)
	buffer := make([]byte, 64)
	var pending []byte
	for {
		n, err := in.Read(buffer)
		if err != nil {
			return
		}
		pending = append(pending, buffer[:n]...)
		for len(pending) > 0 {
			if len(pending) >= 3 && pending[0] == 0x1b && pending[1] == '[' {
				switch pending[2] {
				case 'A':
					events <- keyEvent{kind: keyUp}
				case 'B':
					events <- keyEvent{kind: keyDown}
				}
				pending = pending[3:]
				continue
			}
			switch pending[0] {
			case 3:
				events <- keyEvent{kind: keyQuit}
				pending = pending[1:]
				continue
			case 7, 27:
				events <- keyEvent{kind: keyCancel}
				pending = pending[1:]
				continue
			case '\r', '\n':
				events <- keyEvent{kind: keyEnter}
				pending = pending[1:]
				continue
			case 8, 127:
				events <- keyEvent{kind: keyBackspace}
				pending = pending[1:]
				continue
			}
			if !utf8.FullRune(pending) {
				break
			}
			r, size := utf8.DecodeRune(pending)
			events <- keyEvent{kind: keyRune, r: r}
			pending = pending[size:]
		}
	}
}

func RenderQRCode(data []byte, maxWidth int) ([]string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode QR image: %w", err)
	}
	matrix, err := detectQRMatrix(img)
	if err != nil {
		return nil, err
	}
	const quiet = 4
	width := len(matrix) + quiet*2
	if width > maxWidth {
		return nil, fmt.Errorf("terminal is too narrow for a scan-safe QR code: need %d columns", width)
	}

	isDark := func(x, y int) bool {
		x -= quiet
		y -= quiet
		return y >= 0 && y < len(matrix) && x >= 0 && x < len(matrix[y]) && matrix[y][x]
	}
	if width%2 != 0 {
		width++
	}
	lines := make([]string, 0, width/2)
	for y := 0; y < width; y += 2 {
		var line strings.Builder
		for x := 0; x < width; x++ {
			top := isDark(x, y)
			bottom := isDark(x, y+1)
			switch {
			case top && bottom:
				line.WriteString("\x1b[30;40m▀")
			case top && !bottom:
				line.WriteString("\x1b[30;47m▀")
			case !top && bottom:
				line.WriteString("\x1b[37;40m▀")
			default:
				line.WriteString("\x1b[37;47m▀")
			}
		}
		line.WriteString("\x1b[0m")
		lines = append(lines, line.String())
	}
	return lines, nil
}

func detectQRMatrix(img image.Image) ([][]bool, error) {
	bounds := img.Bounds()
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X-1, bounds.Min.Y-1
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if !qrDark(img, x, y) {
				continue
			}
			minX = minInt(minX, x)
			minY = minInt(minY, y)
			maxX = maxInt(maxX, x)
			maxY = maxInt(maxY, y)
		}
	}
	if maxX < minX || maxY < minY {
		return nil, fmt.Errorf("QR image contains no dark pixels")
	}

	pixelWidth := maxX - minX + 1
	pixelHeight := maxY - minY + 1
	finderRun := 0
	for x := minX; x <= maxX && qrDark(img, x, minY); x++ {
		finderRun++
	}

	bestSize := 0
	bestScore := -1
	bestDistance := 1e9
	estimatedSize := 0.0
	if finderRun > 0 {
		estimatedSize = float64(pixelWidth*7) / float64(finderRun)
	}
	for size := 21; size <= 177; size += 4 {
		if float64(pixelWidth)/float64(size) < 1.5 ||
			float64(pixelHeight)/float64(size) < 1.5 {
			break
		}
		score := finderScore(img, minX, minY, pixelWidth, pixelHeight, size)
		distance := estimatedSize - float64(size)
		if distance < 0 {
			distance = -distance
		}
		if score >= 126 && distance < bestDistance ||
			bestSize == 0 && score > bestScore {
			bestScore = score
			bestSize = size
			bestDistance = distance
		}
	}
	if bestSize == 0 || bestScore < 126 {
		return nil, fmt.Errorf("could not determine QR module grid")
	}

	matrix := make([][]bool, bestSize)
	for y := 0; y < bestSize; y++ {
		matrix[y] = make([]bool, bestSize)
		for x := 0; x < bestSize; x++ {
			sourceX := minX + (2*x+1)*pixelWidth/(2*bestSize)
			sourceY := minY + (2*y+1)*pixelHeight/(2*bestSize)
			matrix[y][x] = qrDark(img, sourceX, sourceY)
		}
	}
	return matrix, nil
}

func finderScore(img image.Image, minX, minY, width, height, size int) int {
	score := 0
	origins := [][2]int{{0, 0}, {size - 7, 0}, {0, size - 7}}
	for _, origin := range origins {
		for y := 0; y < 7; y++ {
			for x := 0; x < 7; x++ {
				expected := x == 0 || x == 6 || y == 0 || y == 6 ||
					(x >= 2 && x <= 4 && y >= 2 && y <= 4)
				sourceX := minX + (2*(origin[0]+x)+1)*width/(2*size)
				sourceY := minY + (2*(origin[1]+y)+1)*height/(2*size)
				if qrDark(img, sourceX, sourceY) == expected {
					score++
				}
			}
		}
	}
	return score
}

func qrDark(img image.Image, x, y int) bool {
	r, g, b, a := img.At(x, y).RGBA()
	if a < 0x8000 {
		return false
	}
	luminance := (299*r + 587*g + 114*b) / 1000
	return luminance < 0x9000
}

func activeRoomName(state cdp.AppState) string {
	for _, room := range state.Rooms {
		if room.ID == state.ActiveRoom {
			return room.Name
		}
	}
	return "Messages"
}

func writeColumns(out *strings.Builder, left string, leftWidth int, right string, rightWidth int) {
	out.WriteString(fit(left, leftWidth))
	// Unicode shaping can make the terminal's actual cursor position differ
	// from wcwidth padding (notably for Thai grapheme clusters). CHA places
	// the divider at an absolute cell so every row stays aligned.
	fmt.Fprintf(out, "\x1b[%dG", leftWidth+1)
	out.WriteString(" │ ")
	out.WriteString(fit(right, rightWidth))
	out.WriteString("\x1b[K")
}

func fit(value string, width int) string {
	if width <= 0 {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	if runewidth.StringWidth(value) > width {
		value = runewidth.Truncate(value, width, "…")
	}
	return value + strings.Repeat(" ", maxInt(0, width-runewidth.StringWidth(value)))
}

func visibleWidth(value string) int {
	var visible strings.Builder
	inEscape := false
	for _, r := range value {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case !inEscape:
			visible.WriteRune(r)
		}
	}
	return runewidth.StringWidth(visible.String())
}

func rawTerminalText(value string) string {
	// term.MakeRaw disables the terminal's usual LF -> CRLF output
	// translation. Without an explicit carriage return, every rendered row
	// starts where the previous row ended and the UI drifts diagonally.
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\n", "\r\n")
}

func trimLastRune(value string) string {
	_, size := utf8.DecodeLastRuneInString(value)
	if size == 0 {
		return value
	}
	return value[:len(value)-size]
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
