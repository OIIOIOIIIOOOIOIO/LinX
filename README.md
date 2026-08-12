# LINX

LINX เป็น proof of concept สำหรับอ่านข้อความที่ **แสดงอยู่บนหน้าต่าง LINE Chrome
extension ของผู้ใช้เอง** แล้วนำมาแสดงใน Linux terminal ผ่าน Chrome DevTools
Protocol (CDP)

โปรเจกต์นี้ไม่แกะ session, cookies หรือรหัสผ่าน และไม่เรียก private LINE API
โดยตรง การอ่านและควบคุมทำผ่าน DOM/CDP ของหน้าต่าง LINE ที่ผู้ใช้เปิดเอง
รองรับ QR login, เลือกห้อง, อ่าน และส่งข้อความจาก TUI โดยข้อความจะแสดงเรียง
จากเก่าไปใหม่ในรูป `[เวลา] ผู้ส่ง: เนื้อหา`

## สิ่งที่ต้องมี

- Go 1.26 ขึ้นไป
- Chromium หรือ Flatpak สำหรับโหมด zero-click (Google Chrome profile เดิมที่
  ติดตั้ง LINE แล้วก็ใช้ต่อได้)
- LINE extension ID `ophjlpahpchlmihnnnihgmmeilfjmjjc`

รองรับ Chrome/Chromium ทั้งแบบ native และ Flatpak โดย launch script จะตรวจ
`com.google.Chrome`, `com.google.ChromeDev` และ `org.chromium.Chromium`
ให้อัตโนมัติ ถ้าเป็น profile ใหม่ ยังไม่มี Chromium และเครื่องมี Flatpak
script จะติดตั้ง `org.chromium.Chromium` จาก Flathub แบบ user-local ให้เอง

## เริ่มใช้งาน

เปิด Chrome ด้วย profile แยกและเปิด CDP เฉพาะ localhost:

```bash
./scripts/launch-chrome.sh
```

ครั้งแรก launch script จะเตรียม Chromium (หากจำเป็น) ดาวน์โหลด LINE extension
รุ่นล่าสุดจาก Chrome Web Store และโหลดให้อัตโนมัติ โดยตรวจ public key ว่าตรงกับ
extension ID ก่อนใช้งาน จึงไม่ต้องเปิด Web Store หรือกด `Add to Chrome`
จากนั้นสแกน QR เพื่อ login ได้เลย การติดตั้ง Chromium ครั้งแรกอาจใช้เวลาตาม
ความเร็วอินเทอร์เน็ต แต่ไม่ถามคำถามระหว่างติดตั้ง

Google Chrome รุ่น 137 ขึ้นไปไม่อนุญาตให้โปรแกรมโหลด extension อัตโนมัติ
script จึงเลือก Chromium สำหรับ profile ใหม่ ส่วน profile Google Chrome เดิม
ที่มี LINE extension อยู่แล้วจะยังใช้งานต่อโดยไม่ย้าย session

สามารถบังคับใช้ Chromium Flatpak ได้:

```bash
LINX_FLATPAK_ID=org.chromium.Chromium ./scripts/launch-chrome.sh
```

การติดตั้งอัตโนมัติบน Chromium ต้องมี `curl`, `unzip` และเครื่องมือมาตรฐาน
ของ Linux หากต้องการเปิดหน้าติดตั้งเองให้ใช้
`./scripts/launch-chrome.sh --setup` หรือปิดระบบอัตโนมัติด้วย
`LINX_AUTO_INSTALL_EXTENSION=0` หากไม่ต้องการให้ script ติดตั้ง Chromium
Flatpak ให้ใช้ `--no-auto-chromium` หรือ `LINX_AUTO_INSTALL_CHROMIUM=0`

หากไม่ต้องการให้มีหน้าต่าง browser สามารถเปิด Chrome และ TUI ด้วยคำสั่งเดียว:

```bash
./scripts/run-headless.sh
```

หรือแยกเปิด Chrome แบบ background แล้วค่อยเปิด LINX:

```bash
./scripts/launch-chrome.sh --headless
./bin/linx
```

บน Chromium สามารถบังคับดาวน์โหลด extension รุ่นล่าสุดใหม่:

```bash
./scripts/launch-chrome.sh --refresh-extension
```

ออกจาก TUI ด้วย `q` จะไม่ทำให้ browser หยุด เพื่อให้ session และการ sync ทำงาน
ต่อได้ หากต้องการปิด headless Chrome ที่ LINX เปิด ให้ใช้:

```bash
./scripts/launch-chrome.sh --stop
```

หากใช้ Flatpak profile แยกจะถูกเก็บใต้
`~/.var/app/<Flatpak ID>/config/linx-chrome` เพื่อให้ข้อมูลคงอยู่ข้ามการเปิด
Chrome และไม่แชร์ session กับ default Chrome profile

ถ้า Chrome debugger ที่พอร์ตเดียวกันเปิดอยู่แล้ว launch script จะใช้ instance
เดิมและเปิดหน้าใหม่ให้ ไม่เปิด Chrome ซ้ำ จึงไม่เกิด `Address already in use`
ส่วน URL `ltsmSandbox.html?sandboxId=...` เป็น sandbox ภายในที่ LINE สร้างเอง
ไม่ต้องเปิด URL นี้โดยตรง

ตรวจว่า Chrome เห็น target ใดบ้าง:

```bash
go run -buildvcs=false ./cmd/linx --list-targets
```

อ่านข้อความหนึ่งครั้ง:

```bash
go run -buildvcs=false ./cmd/linx --once
```

เปิด interactive terminal UI:

```bash
go run -buildvcs=false ./cmd/linx
```

ถ้ายังไม่ได้ล็อกอิน TUI จะแสดง QR ที่ Chrome สร้างให้ สแกนด้วย LINE บนมือถือ
และยืนยันการล็อกอินบนมือถือ หาก LINE แสดง PIN ตัวเลข TUI จะแสดง PIN เดียวกัน
ให้ตรวจสอบด้วย QR แบบ scan-safe ต้องใช้ terminal อย่างน้อย 57 คอลัมน์และประมาณ
34 แถวสำหรับ QR รุ่นปัจจุบัน; หากพื้นที่ไม่พอ TUI จะบอกขนาดที่ต้องขยาย
หลังล็อกอิน LINX จะสลับจากหน้า Friend ไปหน้า Chat และรอรายการห้องให้อัตโนมัติ
ใน headless mode LINX จะ activate หน้า extension และไล่อ่าน virtualized chat list
อัตโนมัติ จึงเลือกห้องที่อยู่นอก viewport และโหลดข้อความของห้องนั้นได้

ปุ่มควบคุม:

- `j`/`k` หรือปุ่มลูกศร: เลือกห้อง
- `Enter`: เปิดห้อง
- `i`: เริ่มพิมพ์ข้อความ
- `Enter`: ส่งข้อความที่กำลังพิมพ์
- `Ctrl+G`: ยกเลิกข้อความ
- `r`: refresh state หรือขอ QR ใหม่
- `l`: logout จาก LINE (ต้องกด `y` ยืนยัน)
- `q` หรือ `Ctrl+C`: ออก

LINX ตรวจ send-key setting ของ LINE อัตโนมัติทั้ง `Enter` และ `Alt+Enter`
และจะไม่ส่งหาก composer ใน Chrome มีข้อความค้างอยู่ก่อน
การ logout จะออกจากบัญชีใน dedicated Chrome profile แต่ไม่ปิด Chrome หรือ LINX
จากนั้น TUI จะกลับมาแสดง QR login ใหม่

ถ้ามีหลาย LINE targets สามารถบังคับเลือกหน้าหลักได้:

```bash
go run -buildvcs=false ./cmd/linx --target index.html
```

ส่งผลลัพธ์ snapshot เป็น JSON:

```bash
go run -buildvcs=false ./cmd/linx --once --json
```

ดู structured application state สำหรับวิเคราะห์ปัญหา:

```bash
go run -buildvcs=false ./cmd/linx --state
```

หรือ build binary ก่อน:

```bash
make build
./bin/linx
```

## ตัวเลือกสำคัญ

```text
--endpoint http://127.0.0.1:9222   local CDP endpoint
--extension-id ID                  extension ID ที่ต้องการอ่าน
--target TEXT                      เลือก target จาก URL/title
--interval 2s                      รอบ refresh
--list-targets                     แสดง target ทั้งหมดแล้วออก
--state                            แสดง rooms/messages/login state เป็น JSON
--once                             อ่านครั้งเดียวแล้วออก
--json                             แสดง JSON
--no-ansi                          ไม่ใช้ alternate terminal screen
```

LINX ปฏิเสธ debugger endpoint และ target WebSocket ที่ไม่ใช่ loopback โดยตั้งใจ
อย่าเปิด remote debugging port ออกสู่ LAN หรืออินเทอร์เน็ต เพราะ CDP มีสิทธิ์อ่าน
และควบคุมข้อมูลใน browser profile นั้นได้ การส่งข้อความจะเกิดขึ้นเมื่อผู้ใช้กด
`Enter` ยืนยันใน compose mode เท่านั้น

## ทดสอบ

```bash
make test
```

tests ใช้ fake Chrome/CDP server ในเครื่อง จึงไม่ต้องล็อกอิน LINE
