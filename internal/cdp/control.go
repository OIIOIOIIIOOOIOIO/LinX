package cdp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Room struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Preview string `json:"preview"`
	Unread  string `json:"unread"`
	Active  bool   `json:"active"`
}

type Message struct {
	Timestamp int64  `json:"timestamp"`
	Time      string `json:"time"`
	Sender    string `json:"sender"`
	Content   string `json:"content"`
}

type LoginState struct {
	Required    bool   `json:"required"`
	QRAvailable bool   `json:"qrAvailable"`
	PIN         string `json:"pin"`
	Description string `json:"description"`
}

type AppState struct {
	Target       Target     `json:"target"`
	LoggedIn     bool       `json:"loggedIn"`
	ChatSelected bool       `json:"chatSelected"`
	ChatReady    bool       `json:"chatReady"`
	ActiveRoom   string     `json:"activeRoom"`
	Rooms        []Room     `json:"rooms"`
	Messages     []Message  `json:"messages"`
	Login        LoginState `json:"login"`
	Captured     time.Time  `json:"captured"`
}

const appStateExpression = `(async () => {
	const normalize = value => String(value || "").replace(/\s+/g, " ").trim();
	const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));
	const chatButton = document.querySelector('button[aria-label="Chat"]');
	const chatSelected = Boolean(
		chatButton && chatButton.getAttribute("aria-current") === "true"
	);
	const chatReady = Boolean(document.querySelector('[class*="chatlist-module__chatlist"]'));
	const activeMatch = location.hash.match(/^#\/chats\/([^/?#]+)/);
	const activeID = activeMatch ? decodeURIComponent(activeMatch[1]) : "";
	const seenRooms = new Set();
	const rooms = [];
	const collectVisibleRooms = () => {
		const buttons = Array.from(document.querySelectorAll(
			'[class*="chatlistItem-module__button_chatlist_item"]'
		));
		for (const button of buttons) {
			const row = button.closest("[data-mid]") || button.parentElement;
			if (!row) continue;
			const id = row.dataset.mid || "";
			if (!id || seenRooms.has(id)) continue;
			seenRooms.add(id);
			const title = row.querySelector('[class*="chatlistItem-module__title_box"]');
			const description = row.querySelector('[class*="chatlistItem-module__description"]');
			const unread = row.querySelector('[class*="chatlistItem-module__message_count"]');
			rooms.push({
				id,
				name: normalize(title ? title.innerText : ""),
				preview: normalize(description ? description.innerText : ""),
				unread: normalize(unread ? unread.innerText : ""),
				active: id === activeID
			});
		}
		return buttons[0] || null;
	};
	const firstRoomButton = collectVisibleRooms();
	let roomScroller = firstRoomButton ? firstRoomButton.parentElement : null;
	while (roomScroller && roomScroller.scrollHeight <= roomScroller.clientHeight + 1) {
		roomScroller = roomScroller.parentElement;
	}
	if (roomScroller) {
		const originalTop = roomScroller.scrollTop;
		roomScroller.scrollTop = 0;
		roomScroller.dispatchEvent(new Event("scroll", {bubbles: true}));
		await delay(35);
		seenRooms.clear();
		rooms.length = 0;
		let stableBottom = 0;
		for (let pass = 0; pass < 100; pass++) {
			collectVisibleRooms();
			const maximum = Math.max(0, roomScroller.scrollHeight - roomScroller.clientHeight);
			if (roomScroller.scrollTop >= maximum - 1) {
				await delay(60);
				collectVisibleRooms();
				const nextMaximum = Math.max(
					0, roomScroller.scrollHeight - roomScroller.clientHeight
				);
				if (nextMaximum <= maximum + 1) {
					stableBottom++;
					if (stableBottom >= 2) break;
				} else {
					stableBottom = 0;
				}
			}
			const step = Math.max(200, Math.floor(roomScroller.clientHeight * 0.8));
			roomScroller.scrollTop = Math.min(
				roomScroller.scrollHeight - roomScroller.clientHeight,
				roomScroller.scrollTop + step
			);
			roomScroller.dispatchEvent(new Event("scroll", {bubbles: true}));
			await delay(35);
		}
		roomScroller.scrollTop = originalTop;
		roomScroller.dispatchEvent(new Event("scroll", {bubbles: true}));
		await delay(35);
	}

	const messageList = document.querySelector(".message_list");
	const messageElements = messageList
		? Array.from(messageList.querySelectorAll(
			"[data-timestamp][data-message-content]"
		))
		: [];
	const messages = messageElements.map((element, index) => {
		if (String(element.className).includes("messageDate-module__date")) return null;
		const timestamp = Number(element.dataset.timestamp || 0);
		const content = normalize(element.dataset.messageContent);
		const prefix = normalize(element.dataset.messageContentPrefix);
		const sender = prefix.replace(/^\d{1,2}:\d{2}\s+/, "").trim();
		const date = timestamp ? new Date(timestamp) : null;
		const pad = value => String(value).padStart(2, "0");
		const formatted = date
			? date.getFullYear() + "-" + pad(date.getMonth() + 1) + "-" +
				pad(date.getDate()) + " " + pad(date.getHours()) + ":" +
				pad(date.getMinutes())
			: "";
		return {timestamp, time: formatted, sender, content, index};
	}).filter(message => message && message.content);
	messages.sort((left, right) =>
		(left.timestamp || Number.MAX_SAFE_INTEGER) -
		(right.timestamp || Number.MAX_SAFE_INTEGER) ||
		left.index - right.index
	);

	const qrRoot = document.querySelector('[class*="loginQr-module__thumbnail"]');
	const qrElement = qrRoot ? qrRoot.querySelector("svg, canvas, img") : null;
	const qrRefreshButton = document.querySelector('[class*="loginQr-module__button_refresh"]');
	const pinRoot = document.querySelector('[class*="pinCodeModal-module__modal"]');
	const pinCandidates = pinRoot
		? Array.from(pinRoot.querySelectorAll("strong"))
			.map(element => normalize(element.innerText))
			.filter(value => /^\d{4,8}$/.test(value))
		: [];
	const loginDescription = document.querySelector(
		'[class*="loginQr-module__description"], [class*="loginPage-module__description"]'
	);
	const loggedIn = Boolean(document.querySelector(
		'[class*="chatlist-module__chatlist"], [class*="gnb-module__gnb"]'
	));
	return JSON.stringify({
		loggedIn,
		chatSelected,
		chatReady,
		activeRoom: activeID,
		rooms,
		messages: messages.map(({index, ...message}) => message),
		login: {
			required: !loggedIn,
			qrAvailable: Boolean(qrElement && !qrRefreshButton),
			pin: pinCandidates[0] || "",
			description: normalize(loginDescription ? loginDescription.innerText : "")
		}
	});
})()`

const openChatListExpression = `(() => {
	const button = document.querySelector('button[aria-label="Chat"]');
	if (!button) return JSON.stringify({found:false, clicked:false});
	if (button.getAttribute("aria-current") === "true") {
		return JSON.stringify({found:true, clicked:false});
	}
	button.click();
	return JSON.stringify({found:true, clicked:true});
})()`

type qrBounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

const qrBoundsExpression = `(() => {
	const root = document.querySelector('[class*="loginQr-module__thumbnail"]');
	const element = root ? root.querySelector("svg, canvas, img") : null;
	if (!element) return JSON.stringify(null);
	const rect = element.getBoundingClientRect();
	if (!rect.width || !rect.height) return JSON.stringify(null);
	return JSON.stringify({
		x: Math.max(0, rect.x - 6),
		y: Math.max(0, rect.y - 6),
		width: rect.width + 12,
		height: rect.height + 12
	});
})()`

func (b *Bridge) State(ctx context.Context) (AppState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	target, client, err := b.connectLocked(ctx)
	if err != nil {
		return AppState{}, err
	}

	state, err := readAppState(ctx, client)
	if err != nil {
		b.closeLocked()
		return AppState{}, fmt.Errorf("read LINE state: %w", err)
	}

	if state.LoggedIn && !state.ChatSelected {
		var navigation struct {
			Found   bool `json:"found"`
			Clicked bool `json:"clicked"`
		}
		if err := evaluateJSON(ctx, client, openChatListExpression, &navigation); err != nil {
			b.closeLocked()
			return AppState{}, fmt.Errorf("open LINE Chat tab: %w", err)
		}
		if navigation.Clicked {
			state, err = waitForChatList(ctx, client, state)
			if err != nil {
				b.closeLocked()
				return AppState{}, err
			}
		}
	}
	state.Target = target
	state.Captured = time.Now()
	return state, nil
}

func readAppState(ctx context.Context, client *Client) (AppState, error) {
	var state AppState
	if err := evaluateJSON(ctx, client, appStateExpression, &state); err != nil {
		return AppState{}, err
	}
	return state, nil
}

func waitForChatList(ctx context.Context, client *Client, initial AppState) (AppState, error) {
	state := initial
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return AppState{}, fmt.Errorf("wait for LINE Chat tab: %w", ctx.Err())
		case <-timer.C:
			return state, nil
		case <-ticker.C:
			next, err := readAppState(ctx, client)
			if err != nil {
				return AppState{}, fmt.Errorf("wait for LINE Chat tab: %w", err)
			}
			state = next
			if state.ChatSelected && state.ChatReady {
				return state, nil
			}
		}
	}
}

func (b *Bridge) OpenRoom(ctx context.Context, roomID string) error {
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("room ID is required")
	}
	encodedID, err := json.Marshal(roomID)
	if err != nil {
		return err
	}
	expression := fmt.Sprintf(`(async () => {
		const id = %s;
		const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));
		const findButton = () => {
			const buttons = Array.from(document.querySelectorAll(
				'[class*="chatlistItem-module__button_chatlist_item"]'
			));
			return buttons.find(button => {
				const row = button.closest("[data-mid]") || button.parentElement;
				return row && row.dataset.mid === id;
			}) || null;
		};
		let button = findButton();
		const first = document.querySelector(
			'[class*="chatlistItem-module__button_chatlist_item"]'
		);
		let scroller = first ? first.parentElement : null;
		while (scroller && scroller.scrollHeight <= scroller.clientHeight + 1) {
			scroller = scroller.parentElement;
		}
		if (!button && scroller) {
			const originalTop = scroller.scrollTop;
			scroller.scrollTop = 0;
			for (let pass = 0; pass < 100 && !button; pass++) {
				scroller.dispatchEvent(new Event("scroll", {bubbles: true}));
				await delay(35);
				button = findButton();
				const maximum = Math.max(0, scroller.scrollHeight - scroller.clientHeight);
				if (button || scroller.scrollTop >= maximum - 1) break;
				const step = Math.max(200, Math.floor(scroller.clientHeight * 0.8));
				scroller.scrollTop = Math.min(maximum, scroller.scrollTop + step);
			}
			if (!button) {
				scroller.scrollTop = originalTop;
				scroller.dispatchEvent(new Event("scroll", {bubbles: true}));
			}
		}
		if (!button) return JSON.stringify({ok:false, error:"room was not found in the chat list"});
		button.click();
		const deadline = Date.now() + 1500;
		while (Date.now() < deadline) {
			const match = location.hash.match(/^#\/chats\/([^/?#]+)/);
			const active = match ? decodeURIComponent(match[1]) : "";
			const messages = document.querySelector(
				".message_list [data-timestamp][data-message-content]"
			);
			if (active === id && messages) break;
			await delay(50);
		}
		return JSON.stringify({ok:true});
	})()`, encodedID)
	return b.runAction(ctx, "open room", expression)
}

func (b *Bridge) SendMessage(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("message is empty")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	_, client, err := b.connectLocked(ctx)
	if err != nil {
		return err
	}

	var focused struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	focusExpression := `(() => {
		const editor = document.querySelector(
			'textarea[class*="chatroomEditor-module__textarea"], textarea[placeholder], textarea'
		);
		if (!editor) return JSON.stringify({ok:false, error:"message composer not found"});
		if (editor.value) return JSON.stringify({
			ok:false,
			error:"message composer already contains text; clear it in Chrome before sending from LINX"
		});
		editor.focus();
		return JSON.stringify({ok:true});
	})()`
	if err := evaluateJSON(ctx, client, focusExpression, &focused); err != nil {
		return fmt.Errorf("focus message composer: %w", err)
	}
	if !focused.OK {
		return fmt.Errorf("focus message composer: %s", focused.Error)
	}

	if err := client.Call(ctx, "Input.insertText", map[string]any{"text": text}, nil); err != nil {
		return fmt.Errorf("type message: %w", err)
	}
	dispatchEnter := func(modifiers int) error {
		key := map[string]any{
			"type":                  "keyDown",
			"key":                   "Enter",
			"code":                  "Enter",
			"windowsVirtualKeyCode": 13,
			"nativeVirtualKeyCode":  13,
			"modifiers":             modifiers,
		}
		if err := client.Call(ctx, "Input.dispatchKeyEvent", key, nil); err != nil {
			return err
		}
		key["type"] = "keyUp"
		return client.Call(ctx, "Input.dispatchKeyEvent", key, nil)
	}
	if err := dispatchEnter(0); err != nil {
		return fmt.Errorf("send message with Enter: %w", err)
	}

	composerHasText := func() (bool, error) {
		var result struct {
			HasText bool `json:"hasText"`
		}
		expression := `(() => {
			const editor = document.querySelector(
				'textarea[class*="chatroomEditor-module__textarea"], textarea[placeholder], textarea'
			);
			return JSON.stringify({hasText: Boolean(editor && editor.value)});
		})()`
		if err := evaluateJSON(ctx, client, expression, &result); err != nil {
			return false, err
		}
		return result.HasText, nil
	}
	hasText, err := composerHasText()
	if err != nil {
		return fmt.Errorf("verify message submission: %w", err)
	}
	if !hasText {
		return nil
	}

	// LINE supports either Enter or Alt+Enter as the configured send key.
	// Alt is bit 1 in CDP's Input.dispatchKeyEvent modifiers.
	if err := dispatchEnter(1); err != nil {
		return fmt.Errorf("send message with Alt+Enter: %w", err)
	}
	hasText, err = composerHasText()
	if err != nil {
		return fmt.Errorf("verify Alt+Enter submission: %w", err)
	}
	if hasText {
		return fmt.Errorf("LINE did not submit the message with Enter or Alt+Enter")
	}
	return nil
}

func (b *Bridge) RefreshQR(ctx context.Context) error {
	expression := `(() => {
		const button = document.querySelector('[class*="loginQr-module__button_refresh"]');
		if (!button) return JSON.stringify({ok:false, error:"QR refresh button is not available"});
		button.click();
		return JSON.stringify({ok:true});
	})()`
	return b.runAction(ctx, "refresh QR code", expression)
}

func (b *Bridge) Logout(ctx context.Context) error {
	expression := `(async () => {
		const normalize = value => String(value || "").replace(/\s+/g, " ").trim();
		const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));
		const logoutLabels = new Set([
			"log out", "logout", "ออกจากระบบ", "ログアウト", "로그아웃",
			"登出", "退出登录", "cerrar sesión", "выход", "çıkış"
		]);
		const moreButton = document.querySelector(
			'button[aria-label="Popover more actions"]'
		);
		if (!moreButton) {
			return JSON.stringify({ok:false, error:"LINE more-actions button was not found"});
		}

		moreButton.click();
		const menuDeadline = Date.now() + 1500;
		let logoutButton = null;
		while (Date.now() < menuDeadline) {
			const popovers = Array.from(document.querySelectorAll(
				'#modal-root [class*="actionPopoverLayout-module__popover_wrap"]'
			));
			const popover = popovers[popovers.length - 1] || null;
			const groups = popover
				? Array.from(popover.querySelectorAll(
					'[class*="actionPopoverList-module__action_list"]'
				))
				: [];
			const logoutGroup = groups[groups.length - 1] || null;
			const buttons = logoutGroup
				? Array.from(logoutGroup.querySelectorAll(
					'button[class*="actionPopoverListItem-module__button_action"]'
				))
				: [];
			// LINE renders Log out followed by Quit in the final menu group.
			// Requiring both entries prevents an unrelated popover action from
			// being clicked if the extension changes its menu structure.
			if (buttons.length >= 2) {
				logoutButton = buttons[0];
				break;
			}
			await delay(50);
		}
		if (!logoutButton) {
			return JSON.stringify({ok:false, error:"LINE logout menu item was not found"});
		}
		if (logoutButton.disabled) {
			return JSON.stringify({ok:false, error:"LINE logout menu item is disabled"});
		}
		const logoutLabel = normalize(logoutButton.innerText).toLocaleLowerCase();
		if (!logoutLabels.has(logoutLabel)) {
			return JSON.stringify({
				ok:false,
				error:"refusing unexpected LINE menu item: " + logoutLabel
			});
		}

		logoutButton.click();
		const logoutDeadline = Date.now() + 5000;
		while (Date.now() < logoutDeadline) {
			const loggedIn = Boolean(document.querySelector(
				'[class*="chatlist-module__chatlist"], [class*="gnb-module__gnb"]'
			));
			if (!loggedIn) return JSON.stringify({ok:true});
			await delay(100);
		}
		return JSON.stringify({ok:false, error:"LINE did not return to the login screen"});
	})()`
	return b.runAction(ctx, "log out of LINE", expression)
}

func (b *Bridge) QRCodePNG(ctx context.Context) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, client, err := b.connectLocked(ctx)
	if err != nil {
		return nil, err
	}

	var bounds *qrBounds
	if err := evaluateJSON(ctx, client, qrBoundsExpression, &bounds); err != nil {
		return nil, fmt.Errorf("locate QR code: %w", err)
	}
	if bounds == nil {
		return nil, fmt.Errorf("QR code is not visible")
	}

	var screenshot struct {
		Data string `json:"data"`
	}
	if err := client.Call(ctx, "Page.captureScreenshot", map[string]any{
		"format":      "png",
		"fromSurface": true,
		"clip": map[string]any{
			"x":      bounds.X,
			"y":      bounds.Y,
			"width":  bounds.Width,
			"height": bounds.Height,
			"scale":  1,
		},
	}, &screenshot); err != nil {
		return nil, fmt.Errorf("capture QR code: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(screenshot.Data)
	if err != nil {
		return nil, fmt.Errorf("decode QR screenshot: %w", err)
	}
	return data, nil
}

func (b *Bridge) runAction(ctx context.Context, name, expression string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, client, err := b.connectLocked(ctx)
	if err != nil {
		return err
	}
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := evaluateJSON(ctx, client, expression, &result); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if !result.OK {
		return fmt.Errorf("%s: %s", name, result.Error)
	}
	return nil
}

func evaluateJSON(ctx context.Context, client *Client, expression string, value any) error {
	var evaluated runtimeResult
	if err := client.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	}, &evaluated); err != nil {
		return err
	}
	encoded, ok := evaluated.Result.Value.(string)
	if !ok {
		return fmt.Errorf("JavaScript returned %T instead of JSON string", evaluated.Result.Value)
	}
	if err := json.Unmarshal([]byte(encoded), value); err != nil {
		return fmt.Errorf("decode JavaScript result: %w", err)
	}
	return nil
}
